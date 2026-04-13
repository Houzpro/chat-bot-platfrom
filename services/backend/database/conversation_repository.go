package database

import (
	"fmt"

	"gorm.io/gorm"
)

type ConversationRepository struct {
	db *DB
}

func NewConversationRepository(db *DB) *ConversationRepository {
	return &ConversationRepository{db: db}
}

func (r *ConversationRepository) CreateConversation(conv *Conversation) (*Conversation, error) {
	if err := r.db.Conn.Create(conv).Error; err != nil {
		return nil, fmt.Errorf("failed to create conversation: %w", err)
	}
	return conv, nil
}

func (r *ConversationRepository) GetConversationByID(id string) (*Conversation, error) {
	var conv Conversation
	err := r.db.Conn.Where("id = ?", id).First(&conv).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("conversation not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation: %w", err)
	}
	return &conv, nil
}

func (r *ConversationRepository) GetConversationsByBotID(botID string) ([]Conversation, error) {
	var convs []Conversation
	err := r.db.Conn.Where("bot_id = ?", botID).
		Order("updated_at DESC").
		Find(&convs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get conversations: %w", err)
	}
	return convs, nil
}

func (r *ConversationRepository) GetConversationsByBotIDAndUserID(botID string, userID string) ([]Conversation, error) {
	var convs []Conversation
	err := r.db.Conn.Where("bot_id = ? AND user_id = ?", botID, userID).
		Order("updated_at DESC").
		Find(&convs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get conversations: %w", err)
	}
	return convs, nil
}

func (r *ConversationRepository) DeleteConversation(id string) error {
	// Delete feedbacks for messages in this conversation
	if err := r.db.Conn.Where("message_id IN (?)",
		r.db.Conn.Model(&Message{}).Select("id").Where("conversation_id = ?", id),
	).Delete(&MessageFeedback{}).Error; err != nil {
		return fmt.Errorf("failed to delete feedbacks: %w", err)
	}

	// Delete messages (GORM AutoMigrate does not create ON DELETE CASCADE)
	if err := r.db.Conn.Where("conversation_id = ?", id).Delete(&Message{}).Error; err != nil {
		return fmt.Errorf("failed to delete messages: %w", err)
	}

	result := r.db.Conn.Where("id = ?", id).Delete(&Conversation{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete conversation: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("conversation not found")
	}
	return nil
}

func (r *ConversationRepository) DeleteConversationsByBotID(botID string) error {
	// Delete all feedbacks for messages in conversations belonging to this bot
	msgSubquery := r.db.Conn.Model(&Message{}).Select("id").Where("conversation_id IN (?)",
		r.db.Conn.Model(&Conversation{}).Select("id").Where("bot_id = ?", botID),
	)
	if err := r.db.Conn.Where("message_id IN (?)", msgSubquery).Delete(&MessageFeedback{}).Error; err != nil {
		return fmt.Errorf("failed to delete feedbacks for bot: %w", err)
	}

	// Delete all messages for conversations belonging to this bot
	if err := r.db.Conn.Where("conversation_id IN (?)",
		r.db.Conn.Model(&Conversation{}).Select("id").Where("bot_id = ?", botID),
	).Delete(&Message{}).Error; err != nil {
		return fmt.Errorf("failed to delete messages for bot: %w", err)
	}
	// Delete all conversations
	if err := r.db.Conn.Where("bot_id = ?", botID).Delete(&Conversation{}).Error; err != nil {
		return fmt.Errorf("failed to delete conversations for bot: %w", err)
	}
	return nil
}

func (r *ConversationRepository) AddMessage(msg *Message) (*Message, error) {
	if err := r.db.Conn.Create(msg).Error; err != nil {
		return nil, fmt.Errorf("failed to add message: %w", err)
	}
	if msg.ConversationID != nil && *msg.ConversationID != "" {
		r.db.Conn.Model(&Conversation{}).Where("id = ?", *msg.ConversationID).
			Update("updated_at", msg.CreatedAt)
	}
	return msg, nil
}

func (r *ConversationRepository) GetMessages(conversationID string) ([]Message, error) {
	var msgs []Message
	err := r.db.Conn.Where("conversation_id = ?", conversationID).
		Order("created_at ASC").
		Find(&msgs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}
	return msgs, nil
}

func (r *ConversationRepository) GetRecentMessages(conversationID string, limit int) ([]Message, error) {
	var msgs []Message
	err := r.db.Conn.Where("conversation_id = ?", conversationID).
		Order("created_at DESC").
		Limit(limit).
		Find(&msgs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get recent messages: %w", err)
	}
	// Reverse to chronological order
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

// AddFeedback creates a feedback rating for a message.
// Returns an error if feedback already exists (immutable).
func (r *ConversationRepository) AddFeedback(fb *MessageFeedback) (*MessageFeedback, error) {
	var existing MessageFeedback
	query := r.db.Conn.Where("message_id = ?", fb.MessageID)
	if fb.UserID != nil {
		query = query.Where("user_id = ?", *fb.UserID)
	} else {
		query = query.Where("user_id IS NULL")
	}

	err := query.First(&existing).Error
	if err == nil {
		return &existing, fmt.Errorf("feedback already exists")
	}
	if err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("failed to check feedback: %w", err)
	}

	if err := r.db.Conn.Create(fb).Error; err != nil {
		return nil, fmt.Errorf("failed to add feedback: %w", err)
	}
	return fb, nil
}

// FeedbackStats holds aggregated feedback statistics for a bot.
type FeedbackStats struct {
	TotalMessages   int64 `json:"total_messages"`
	TotalFeedbacks  int64 `json:"total_feedbacks"`
	PositiveCount   int64 `json:"positive_count"`
	NegativeCount   int64 `json:"negative_count"`
}

// GetFeedbackStats returns aggregated feedback stats for all messages belonging to a bot.
func (r *ConversationRepository) GetFeedbackStats(botID string) (*FeedbackStats, error) {
	var stats FeedbackStats

	// Total assistant messages for this bot
	r.db.Conn.Model(&Message{}).
		Where("bot_id = ? AND role = ?", botID, "assistant").
		Count(&stats.TotalMessages)

	// Feedback counts
	r.db.Conn.Model(&MessageFeedback{}).
		Joins("JOIN messages ON messages.id = message_feedbacks.message_id").
		Where("messages.bot_id = ?", botID).
		Count(&stats.TotalFeedbacks)

	r.db.Conn.Model(&MessageFeedback{}).
		Joins("JOIN messages ON messages.id = message_feedbacks.message_id").
		Where("messages.bot_id = ? AND message_feedbacks.rating = 1", botID).
		Count(&stats.PositiveCount)

	r.db.Conn.Model(&MessageFeedback{}).
		Joins("JOIN messages ON messages.id = message_feedbacks.message_id").
		Where("messages.bot_id = ? AND message_feedbacks.rating = -1", botID).
		Count(&stats.NegativeCount)

	return &stats, nil
}

// GetMessageByID returns a single message by its ID.
func (r *ConversationRepository) GetMessageByID(id uint) (*Message, error) {
	var msg Message
	err := r.db.Conn.Where("id = ?", id).First(&msg).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("message not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get message: %w", err)
	}
	return &msg, nil
}

func (r *ConversationRepository) UpdateConversationTitle(id string, title string) error {
	result := r.db.Conn.Model(&Conversation{}).Where("id = ?", id).Update("title", title)
	if result.Error != nil {
		return fmt.Errorf("failed to update title: %w", result.Error)
	}
	return nil
}
