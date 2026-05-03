package database

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// UserRepository handles user database operations using GORM
type UserRepository struct {
	db *DB
}

// NewUserRepository creates a new UserRepository
func NewUserRepository(db *DB) *UserRepository {
	return &UserRepository{db: db}
}

// Create creates a new user with hashed password
func (r *UserRepository) Create(email, password, name string) (*User, error) {
	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	user := &User{
		Email:        email,
		PasswordHash: string(hashedPassword),
		Name:         name,
	}

	// GORM automatically handles validation and returns duplicate key errors
	if err := r.db.Conn.Create(user).Error; err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

// GetByEmail retrieves a user by email
func (r *UserRepository) GetByEmail(email string) (*User, error) {
	var user User
	err := r.db.Conn.Where("email = ?", email).First(&user).Error

	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

// GetByID retrieves a user by ID
func (r *UserRepository) GetByID(id string) (*User, error) {
	var user User
	err := r.db.Conn.Where("id = ?", id).First(&user).Error

	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

// VerifyPassword checks if the provided password matches the user's hashed password
func (r *UserRepository) VerifyPassword(user *User, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
}

// GetRole returns the user's role ("user" or "admin"). Used by AdminMiddleware
// without loading the rest of the user record on every admin request.
func (r *UserRepository) GetRole(userID string) (string, error) {
	var role string
	err := r.db.Conn.Model(&User{}).Where("id = ?", userID).Pluck("role", &role).Error
	if err != nil {
		return "", err
	}
	if role == "" {
		role = "user"
	}
	return role, nil
}

// ListUsersPaginated returns a page of users (admin-only).
func (r *UserRepository) ListUsersPaginated(search string, offset, limit int) ([]User, int64, error) {
	q := r.db.Conn.Model(&User{})
	if search != "" {
		pattern := "%" + search + "%"
		q = q.Where("email ILIKE ? OR name ILIKE ?", pattern, pattern)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var users []User
	if err := q.Order("created_at DESC, id DESC").Offset(offset).Limit(limit).Find(&users).Error; err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

// SetRole updates a user's role. Admins use this to promote/demote.
func (r *UserRepository) SetRole(userID, role string) error {
	if role != "user" && role != "admin" {
		return fmt.Errorf("invalid role: %s", role)
	}
	result := r.db.Conn.Model(&User{}).Where("id = ?", userID).Update("role", role)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// DeleteUser permanently removes a user. Cascades through FK ON DELETE rules
// (bots, conversations, etc.) — admins should be aware destructive actions
// take ALL the user's data with them.
func (r *UserRepository) DeleteUser(userID string) error {
	result := r.db.Conn.Where("id = ?", userID).Delete(&User{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// CountUsers returns the total number of registered users (for admin stats).
func (r *UserRepository) CountUsers() (int64, error) {
	var n int64
	err := r.db.Conn.Model(&User{}).Count(&n).Error
	return n, err
}
