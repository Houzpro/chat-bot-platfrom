package database

import (
	"fmt"

	"gorm.io/gorm"
)

// BotRepository handles bot database operations using GORM
type BotRepository struct {
	db *DB
}

// NewBotRepository creates a new BotRepository
func NewBotRepository(db *DB) *BotRepository {
	return &BotRepository{db: db}
}

// Create creates a new bot (UUID generated automatically by BeforeCreate hook)
func (r *BotRepository) Create(bot *Bot) (*Bot, error) {
	if err := r.db.Conn.Create(bot).Error; err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}
	return bot, nil
}

// GetByID retrieves a bot by ID
func (r *BotRepository) GetByID(id string) (*Bot, error) {
	var bot Bot
	err := r.db.Conn.Where("id = ? AND is_active = ?", id, true).First(&bot).Error

	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("bot not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get bot: %w", err)
	}

	return &bot, nil
}

// GetByIDForOwner retrieves a bot by ID without is_active filter (for owner operations)
func (r *BotRepository) GetByIDForOwner(id string, ownerID string) (*Bot, error) {
	var bot Bot
	err := r.db.Conn.Where("id = ? AND owner_id = ?", id, ownerID).First(&bot).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("bot not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get bot: %w", err)
	}
	return &bot, nil
}

// GetByOwnerID retrieves all bots for a specific owner (including inactive)
func (r *BotRepository) GetByOwnerID(ownerID string) ([]*Bot, error) {
	var bots []*Bot
	err := r.db.Conn.Where("owner_id = ?", ownerID).
		Order("is_active DESC, created_at DESC").
		Find(&bots).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get bots: %w", err)
	}

	return bots, nil
}

// Update updates an existing bot
func (r *BotRepository) Update(bot *Bot) error {
	result := r.db.Conn.Model(bot).
		Where("id = ? AND owner_id = ?", bot.ID, bot.OwnerID).
		Select("*").
		Omit("id", "owner_id", "created_at").
		Updates(bot)

	if result.Error != nil {
		return fmt.Errorf("failed to update bot: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("bot not found")
	}

	return nil
}

// Delete soft deletes a bot by setting is_active to false
func (r *BotRepository) Delete(id string, ownerID string) error {
	result := r.db.Conn.
		Where("id = ? AND owner_id = ? AND is_active = ?", id, ownerID, true).
		Delete(&Bot{})

	if result.Error != nil {
		return fmt.Errorf("failed to delete bot: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("bot not found or not owned by user")
	}

	return nil
}

// AddDocument adds a document metadata entry for a bot
func (r *BotRepository) AddDocument(doc *BotDocument) error {
	if err := r.db.Conn.Create(doc).Error; err != nil {
		return fmt.Errorf("failed to add document: %w", err)
	}
	return nil
}

// GetDocuments retrieves all documents for a bot
func (r *BotRepository) GetDocuments(botID string) ([]BotDocument, error) {
	var docs []BotDocument
	err := r.db.Conn.Where("bot_id = ?", botID).
		Order("uploaded_at DESC").
		Find(&docs).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get documents: %w", err)
	}

	return docs, nil
}

// GetDocumentByID retrieves a single document by its ID
func (r *BotRepository) GetDocumentByID(docID uint) (*BotDocument, error) {
	var doc BotDocument
	err := r.db.Conn.Where("id = ?", docID).First(&doc).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("document not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}
	return &doc, nil
}

// DeleteDocument removes a document metadata entry
func (r *BotRepository) DeleteDocument(docID uint, botID string) error {
	result := r.db.Conn.Where("id = ? AND bot_id = ?", docID, botID).Delete(&BotDocument{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete document: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("document not found")
	}
	return nil
}

// DeleteDocumentsByBotID removes all document metadata entries for a bot
func (r *BotRepository) DeleteDocumentsByBotID(botID string) error {
	if err := r.db.Conn.Where("bot_id = ?", botID).Delete(&BotDocument{}).Error; err != nil {
		return fmt.Errorf("failed to delete documents: %w", err)
	}
	return nil
}

// CheckOwnership verifies if a user owns a specific bot
func (r *BotRepository) CheckOwnership(botID string, ownerID string) (bool, error) {
	var count int64
	err := r.db.Conn.Model(&Bot{}).
		Where("id = ? AND owner_id = ? AND is_active = ?", botID, ownerID, true).
		Count(&count).Error

	if err != nil {
		return false, fmt.Errorf("failed to check ownership: %w", err)
	}

	return count > 0, nil
}
