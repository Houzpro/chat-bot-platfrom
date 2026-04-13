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

func (r *ConversationRepository) GetConversationsByBotIDAndUserID(botID string, userID uint) ([]Conversation, error) {
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
	result := r.db.Conn.Where("id = ?", id).Delete(&Conversation{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete conversation: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("conversation not found")
	}
	return nil
}

func (r *ConversationRepository) AddMessage(msg *Message) (*Message, error) {
	if err := r.db.Conn.Create(msg).Error; err != nil {
		return nil, fmt.Errorf("failed to add message: %w", err)
	}
	r.db.Conn.Model(&Conversation{}).Where("id = ?", msg.ConversationID).
		Update("updated_at", msg.CreatedAt)
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

func (r *ConversationRepository) UpdateConversationTitle(id string, title string) error {
	result := r.db.Conn.Model(&Conversation{}).Where("id = ?", id).Update("title", title)
	if result.Error != nil {
		return fmt.Errorf("failed to update title: %w", result.Error)
	}
	return nil
}
