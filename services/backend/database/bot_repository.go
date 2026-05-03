package database

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// escapeLike makes a user-supplied string safe for LIKE/ILIKE: the
// wildcards `%` and `_` (and the escape char `\`) must be neutralized
// so input like "my_bot" doesn't silently match "mytest" too.
// Call sites pair this with `ESCAPE '\\'` on the SQL side.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

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

// GetByOwnerID retrieves all bots for a specific owner (including inactive).
// Kept for internal callers that don't paginate; public listings should use
// GetByOwnerIDPaginated so clients can't request unbounded result sets.
func (r *BotRepository) GetByOwnerID(ownerID string) ([]*Bot, error) {
	var bots []*Bot
	err := r.db.Conn.Where("owner_id = ?", ownerID).
		Order("is_active DESC, created_at DESC, id DESC").
		Find(&bots).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get bots: %w", err)
	}

	return bots, nil
}

// GetByOwnerIDPaginated returns a page of bots plus the total count. Ordering
// ends with `id DESC` as a tie-breaker: without it, rows sharing a created_at
// can swap positions between page fetches and users see duplicates / gaps.
//
// `search` is matched case-insensitively against name and description. Empty
// string disables filtering. LIKE metacharacters in the input are escaped so
// "my_bot" does not match "mytest".
func (r *BotRepository) GetByOwnerIDPaginated(ownerID, search string, offset, limit int) ([]*Bot, int64, error) {
	query := r.db.Conn.Model(&Bot{}).Where("owner_id = ?", ownerID)
	if s := strings.TrimSpace(search); s != "" {
		pattern := "%" + escapeLike(s) + "%"
		query = query.Where(
			`(name ILIKE ? ESCAPE '\' OR description ILIKE ? ESCAPE '\')`,
			pattern, pattern,
		)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count bots: %w", err)
	}

	var bots []*Bot
	err := query.
		Order("is_active DESC, created_at DESC, id DESC").
		Offset(offset).
		Limit(limit).
		Find(&bots).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get bots: %w", err)
	}

	return bots, total, nil
}

// GetAccessibleBotsPaginated returns owned + shared bots for a user as a single
// page. Owned rows include `role = "owner"`; shared rows carry the collaborator
// role ("editor"/"viewer"). Sort matches GetByOwnerIDPaginated so the existing
// dashboard layout keeps working.
type AccessibleBot struct {
	Bot
	Role string `json:"role"` // "owner" | "editor" | "viewer"
}

func (r *BotRepository) GetAccessibleBotsPaginated(userID, search string, offset, limit int) ([]AccessibleBot, int64, error) {
	// Build the same search predicate twice (owner side + shared side). Using
	// UNION ALL keeps each branch's own filter and still sorts by the merged
	// set's created_at + id tie-breaker so pagination is stable across pages.
	args := []any{userID}
	ownerWhere := "owner_id = ?"
	sharedWhere := "1=1"
	if s := strings.TrimSpace(search); s != "" {
		pattern := "%" + escapeLike(s) + "%"
		ownerWhere = `owner_id = ? AND (name ILIKE ? ESCAPE '\' OR description ILIKE ? ESCAPE '\')`
		args = []any{userID, pattern, pattern}
		sharedWhere = `(name ILIKE ? ESCAPE '\' OR description ILIKE ? ESCAPE '\')`
	}

	// Count = owned matches + collaborator matches.
	var total int64
	countSQL := `
		SELECT (
			SELECT COUNT(*) FROM bots WHERE ` + ownerWhere + `
		) + (
			SELECT COUNT(*) FROM bots b
			JOIN bot_collaborators bc ON bc.bot_id = b.id
			WHERE bc.user_id = ?` + func() string {
		if strings.TrimSpace(search) != "" {
			return ` AND ` + sharedWhere
		}
		return ""
	}() + `
		) AS total
	`
	countArgs := append([]any{}, args...)
	countArgs = append(countArgs, userID)
	if strings.TrimSpace(search) != "" {
		pattern := "%" + escapeLike(strings.TrimSpace(search)) + "%"
		countArgs = append(countArgs, pattern, pattern)
	}
	if err := r.db.Conn.Raw(countSQL, countArgs...).Scan(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count accessible bots: %w", err)
	}

	listSQL := `
		SELECT * FROM (
			SELECT b.*, 'owner' AS role FROM bots b WHERE b.` + ownerWhere + `
			UNION ALL
			SELECT b.*, bc.role AS role
			FROM bots b
			JOIN bot_collaborators bc ON bc.bot_id = b.id
			WHERE bc.user_id = ?` + func() string {
		if strings.TrimSpace(search) != "" {
			return ` AND ` + sharedWhere
		}
		return ""
	}() + `
		) AS combined
		ORDER BY is_active DESC, created_at DESC, id DESC
		LIMIT ? OFFSET ?
	`
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, userID)
	if strings.TrimSpace(search) != "" {
		pattern := "%" + escapeLike(strings.TrimSpace(search)) + "%"
		listArgs = append(listArgs, pattern, pattern)
	}
	listArgs = append(listArgs, limit, offset)

	var bots []AccessibleBot
	if err := r.db.Conn.Raw(listSQL, listArgs...).Scan(&bots).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to query accessible bots: %w", err)
	}
	return bots, total, nil
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
		Order("uploaded_at DESC, id DESC").
		Find(&docs).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get documents: %w", err)
	}

	return docs, nil
}

// GetDocumentsPaginated returns a page of documents for a bot plus total count.
func (r *BotRepository) GetDocumentsPaginated(botID string, offset, limit int) ([]BotDocument, int64, error) {
	var total int64
	if err := r.db.Conn.Model(&BotDocument{}).Where("bot_id = ?", botID).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count documents: %w", err)
	}

	var docs []BotDocument
	err := r.db.Conn.Where("bot_id = ?", botID).
		Order("uploaded_at DESC, id DESC").
		Offset(offset).
		Limit(limit).
		Find(&docs).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get documents: %w", err)
	}

	return docs, total, nil
}

// GetDocumentByID retrieves a single document by its ID
func (r *BotRepository) GetDocumentByID(docID string) (*BotDocument, error) {
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
func (r *BotRepository) DeleteDocument(docID string, botID string) error {
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

// AdminBotRow is a bot row enriched with the owner's email/name. Used by the
// admin panel so the operator can see who owns each bot at a glance.
type AdminBotRow struct {
	ID         string `json:"id"`
	OwnerID    string `json:"owner_id"`
	Name       string `json:"name"`
	IsActive   bool   `json:"is_active"`
	CreatedAt  string `json:"created_at"`
	OwnerEmail string `json:"owner_email"`
	OwnerName  string `json:"owner_name"`
}

// ListAllBotsPaginated returns a page of all bots in the system (admin-only).
func (r *BotRepository) ListAllBotsPaginated(search string, offset, limit int) ([]AdminBotRow, int64, error) {
	args := []any{}
	whereSQL := ""
	if s := strings.TrimSpace(search); s != "" {
		pattern := "%" + escapeLike(s) + "%"
		whereSQL = `WHERE b.name ILIKE ? ESCAPE '\' OR b.description ILIKE ? ESCAPE '\' OR u.email ILIKE ? ESCAPE '\'`
		args = []any{pattern, pattern, pattern}
	}

	var total int64
	countSQL := "SELECT COUNT(*) FROM bots b JOIN users u ON u.id = b.owner_id " + whereSQL
	if err := r.db.Conn.Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count bots: %w", err)
	}

	listSQL := `
		SELECT b.id, b.owner_id, b.name, b.is_active,
		       to_char(b.created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS created_at,
		       u.email AS owner_email, u.name AS owner_name
		FROM bots b
		JOIN users u ON u.id = b.owner_id
		` + whereSQL + `
		ORDER BY b.created_at DESC, b.id DESC
		LIMIT ? OFFSET ?
	`
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, limit, offset)
	var rows []AdminBotRow
	if err := r.db.Conn.Raw(listSQL, listArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list bots: %w", err)
	}
	return rows, total, nil
}

// CountAllBots returns the total number of bots (admin stats).
func (r *BotRepository) CountAllBots() (int64, error) {
	var n int64
	err := r.db.Conn.Model(&Bot{}).Count(&n).Error
	return n, err
}

// AdminDeleteBot deletes a bot regardless of owner. Used by admins.
func (r *BotRepository) AdminDeleteBot(botID string) error {
	result := r.db.Conn.Where("id = ?", botID).Delete(&Bot{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete bot: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("bot not found")
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
