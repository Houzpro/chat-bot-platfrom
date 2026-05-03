package database

import (
	"fmt"

	"gorm.io/gorm"
)

// CollaboratorRepository handles bot_collaborators CRUD plus the access checks
// the rest of the codebase uses to decide whether a non-owner is allowed to
// chat/edit a bot. The repository intentionally returns a *role string* from
// access lookups so callers can branch on viewer vs editor without re-querying.
type CollaboratorRepository struct {
	db *DB
}

func NewCollaboratorRepository(db *DB) *CollaboratorRepository {
	return &CollaboratorRepository{db: db}
}

// CollaboratorWithUser is the JSON shape returned by ListByBot — it pairs the
// collaborator row with the user fields the UI needs (email/name) so the
// frontend can render the list without a second round-trip per user.
type CollaboratorWithUser struct {
	ID        string `json:"id"`
	BotID     string `json:"bot_id"`
	UserID    string `json:"user_id"`
	Role      string `json:"role"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// ListByBot returns all collaborators for a bot joined with user info.
// Returns an empty slice (never nil) so JSON encoding produces `[]` instead
// of `null` — frontend code uses .map/.length on the result.
func (r *CollaboratorRepository) ListByBot(botID string) ([]CollaboratorWithUser, error) {
	rows := []CollaboratorWithUser{}
	err := r.db.Conn.Raw(`
		SELECT bc.id, bc.bot_id, bc.user_id, bc.role, u.email, u.name,
		       to_char(bc.created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS created_at
		FROM bot_collaborators bc
		JOIN users u ON u.id = bc.user_id
		WHERE bc.bot_id = ?
		ORDER BY bc.created_at ASC
	`, botID).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list collaborators: %w", err)
	}
	return rows, nil
}

// Add inserts a new collaborator. Returns ErrAlreadyCollaborator when the
// (bot,user) pair already exists so the handler can return 409.
func (r *CollaboratorRepository) Add(botID, userID, role string) (*BotCollaborator, error) {
	if role != "editor" && role != "viewer" {
		return nil, fmt.Errorf("invalid role: %s", role)
	}
	collab := &BotCollaborator{
		BotID:  botID,
		UserID: userID,
		Role:   role,
	}
	if err := r.db.Conn.Create(collab).Error; err != nil {
		// GORM doesn't expose a typed unique-violation error portably across
		// drivers; we check the existence ourselves on a duplicate to give a
		// stable error string.
		var existing BotCollaborator
		check := r.db.Conn.Where("bot_id = ? AND user_id = ?", botID, userID).First(&existing).Error
		if check == nil {
			return nil, fmt.Errorf("already a collaborator")
		}
		return nil, fmt.Errorf("failed to add collaborator: %w", err)
	}
	return collab, nil
}

// Remove deletes a collaborator by (bot,user). Idempotent enough — a missing
// row returns "not found" so the handler can surface 404.
func (r *CollaboratorRepository) Remove(botID, userID string) error {
	result := r.db.Conn.Where("bot_id = ? AND user_id = ?", botID, userID).Delete(&BotCollaborator{})
	if result.Error != nil {
		return fmt.Errorf("failed to remove collaborator: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("collaborator not found")
	}
	return nil
}

// UpdateRole changes a collaborator's role. Used by the UI when toggling
// between editor and viewer without forcing a remove+add.
func (r *CollaboratorRepository) UpdateRole(botID, userID, role string) error {
	if role != "editor" && role != "viewer" {
		return fmt.Errorf("invalid role: %s", role)
	}
	result := r.db.Conn.Model(&BotCollaborator{}).
		Where("bot_id = ? AND user_id = ?", botID, userID).
		Update("role", role)
	if result.Error != nil {
		return fmt.Errorf("failed to update role: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("collaborator not found")
	}
	return nil
}

// GetRole returns the user's role for a bot ("editor" / "viewer") or "" if
// the user is not a collaborator. Owners are NOT collaborators — handlers
// must check ownership separately before calling this.
func (r *CollaboratorRepository) GetRole(botID, userID string) (string, error) {
	var collab BotCollaborator
	err := r.db.Conn.Where("bot_id = ? AND user_id = ?", botID, userID).First(&collab).Error
	if err == gorm.ErrRecordNotFound {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get collaborator role: %w", err)
	}
	return collab.Role, nil
}

// SharedBotIDs returns IDs of bots the user has been added to as a collaborator.
// Used by GetMyBots to merge owned + shared bots into a single listing.
func (r *CollaboratorRepository) SharedBotIDs(userID string) ([]string, error) {
	var ids []string
	err := r.db.Conn.Model(&BotCollaborator{}).
		Where("user_id = ?", userID).
		Pluck("bot_id", &ids).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list shared bots: %w", err)
	}
	return ids, nil
}
