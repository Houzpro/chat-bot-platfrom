package database

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// User represents a registered user
type User struct {
	ID           string    `gorm:"type:uuid;primaryKey" json:"id"`
	Email        string    `gorm:"unique;not null;size:255" json:"email"`
	PasswordHash string    `gorm:"not null;size:255" json:"-"` // Never expose in JSON
	Name         string    `gorm:"size:255" json:"name"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// Relationships
	Bots []Bot `gorm:"foreignKey:OwnerID" json:"bots,omitempty"`
}

// BeforeCreate hook to generate UUID for User
func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == "" {
		u.ID = uuid.New().String()
	}
	return nil
}

// Bot represents a configured chatbot
type Bot struct {
	ID          string `gorm:"type:uuid;primaryKey" json:"id"`
	OwnerID     string `gorm:"type:uuid;not null;index" json:"owner_id"`
	Name        string `gorm:"not null;size:255" json:"name"`
	Description string `gorm:"type:text" json:"description"`
	Config      string `gorm:"type:jsonb;default:'{}'" json:"config"`

	// Generation parameters
	Temperature  float64 `gorm:"default:0.75" json:"temperature"`
	TopP         float64 `gorm:"default:0.92" json:"top_p"`
	TopK         int     `gorm:"default:40" json:"top_k"`
	MaxNewTokens int     `gorm:"default:512" json:"max_new_tokens"`
	DoSample     bool    `gorm:"default:true" json:"do_sample"`
	SystemPrompt string  `gorm:"type:text" json:"system_prompt"`

	// RAG settings
	ChunkSize     int `gorm:"default:800" json:"chunk_size"`
	ChunkOverlap  int `gorm:"default:200" json:"chunk_overlap"`
	ContextWindow int `gorm:"default:0" json:"context_window"`

	// Status
	IsActive  bool      `gorm:"default:true;index" json:"is_active"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// Relationships
	Owner     User          `gorm:"foreignKey:OwnerID" json:"owner,omitempty"`
	Documents []BotDocument `gorm:"foreignKey:BotID" json:"documents,omitempty"`
}

// BeforeCreate hook to generate UUID
func (b *Bot) BeforeCreate(tx *gorm.DB) error {
	if b.ID == "" {
		b.ID = uuid.New().String()
	}
	return nil
}

// BotDocument represents metadata about documents uploaded for a bot
type BotDocument struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	BotID       string    `gorm:"type:uuid;not null;index" json:"bot_id"`
	Filename    string    `gorm:"not null;size:255" json:"filename"`
	FileType    string    `gorm:"size:50" json:"file_type"`
	FileSize    int64     `json:"file_size"`
	ChunksCount int       `gorm:"default:0" json:"chunks_count"`
	UploadedAt  time.Time `gorm:"autoCreateTime;column:uploaded_at" json:"uploaded_at"`

	// Relationships
	Bot Bot `gorm:"foreignKey:BotID" json:"bot,omitempty"`
}

// Conversation represents a chat conversation (dialog) for a bot
type Conversation struct {
	ID        string    `gorm:"type:uuid;primaryKey" json:"id"`
	BotID     string    `gorm:"type:uuid;not null;index" json:"bot_id"`
	UserID    *string   `gorm:"type:uuid;index" json:"user_id,omitempty"`
	Title     string    `gorm:"size:255" json:"title"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// Relationships
	Bot      Bot       `gorm:"foreignKey:BotID" json:"-"`
	Messages []Message `gorm:"foreignKey:ConversationID" json:"messages,omitempty"`
}

// BeforeCreate hook to generate UUID for Conversation
func (conv *Conversation) BeforeCreate(tx *gorm.DB) error {
	if conv.ID == "" {
		conv.ID = uuid.New().String()
	}
	return nil
}

// Message represents a single message in a conversation
type Message struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	ConversationID *string   `gorm:"type:uuid;index" json:"conversation_id,omitempty"`
	BotID          *string   `gorm:"type:uuid;index" json:"bot_id,omitempty"`
	Role           string    `gorm:"size:20;not null" json:"role"`
	Content        string    `gorm:"type:text;not null" json:"content"`
	Metadata       string    `gorm:"type:jsonb;default:'{}'" json:"metadata,omitempty"`
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`

	// Relationships
	Conversation *Conversation `gorm:"foreignKey:ConversationID" json:"-"`
}

// MessageFeedback represents a thumbs-up/down rating on an assistant message
type MessageFeedback struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	MessageID uint      `gorm:"not null;uniqueIndex:idx_feedback_message_user" json:"message_id"`
	UserID    *string   `gorm:"type:uuid;uniqueIndex:idx_feedback_message_user" json:"user_id,omitempty"`
	Rating    int16     `gorm:"not null" json:"rating"` // 1 = thumbs up, -1 = thumbs down
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`

	// Relationships
	Message Message `gorm:"foreignKey:MessageID" json:"-"`
}

// PublicBot represents a bot with only public information (no config details)
type PublicBot struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

// ToPublic converts a Bot to PublicBot (safe for external access)
func (b *Bot) ToPublic() PublicBot {
	return PublicBot{
		ID:          b.ID,
		Name:        b.Name,
		Description: b.Description,
		CreatedAt:   b.CreatedAt,
	}
}
