package database

import (
	"fmt"
	"log"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type DB struct {
	Conn *gorm.DB
}

// NewDB creates a new GORM database connection
func NewDB(databaseURL string) (*DB, error) {
	// Configure GORM with connection pool settings
	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
		PrepareStmt: true, // Prepared statements for performance
	}

	db, err := gorm.Open(postgres.Open(databaseURL), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Get underlying SQL DB to configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	// Connection pool settings
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(30 * time.Minute)

	// Test connection
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("unable to ping database: %w", err)
	}

	log.Println("✅ Database connected successfully")
	return &DB{Conn: db}, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	if db.Conn != nil {
		sqlDB, err := db.Conn.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}

// AutoMigrate runs database migrations for all models
func (db *DB) AutoMigrate() error {
	// Handle constraint migration issues by checking if old constraint exists
	if db.Conn.Migrator().HasTable(&User{}) {
		// Check if old constraint exists before GORM tries to drop it
		var count int64
		db.Conn.Raw(`
			SELECT COUNT(*) FROM information_schema.table_constraints 
			WHERE constraint_name = 'uni_users_email' 
			AND table_name = 'users'
		`).Scan(&count)

		// If old constraint exists, drop it manually
		if count > 0 {
			db.Conn.Exec(`ALTER TABLE users DROP CONSTRAINT IF EXISTS uni_users_email`)
		}
	}

	if err := db.Conn.AutoMigrate(
		&User{},
		&Model{},
		&Bot{},
		&BotDocument{},
		&Conversation{},
		&Message{},
		&MessageFeedback{},
		&BotCollaborator{},
	); err != nil {
		return err
	}

	// Enforce model.type CHECK at the DB level; GORM tags don't emit it.
	db.Conn.Exec(`
		DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'models_type_check') THEN
				ALTER TABLE models ADD CONSTRAINT models_type_check CHECK (type IN ('base', 'finetuned'));
			END IF;
		END $$;
	`)
	db.Conn.Exec(`
		DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'models_status_check') THEN
				ALTER TABLE models ADD CONSTRAINT models_status_check
					CHECK (status IN ('ready', 'training', 'converting', 'deploying', 'running', 'stopped', 'error'));
			END IF;
		END $$;
	`)

	// Backfill role for existing users that pre-date the role column.
	// AutoMigrate adds the column with default 'user', but rows inserted before
	// the column existed get NULL on some Postgres versions — normalize to 'user'.
	db.Conn.Exec(`UPDATE users SET role = 'user' WHERE role IS NULL OR role = ''`)

	// Enforce role check constraints at the DB level. GORM tags don't emit
	// CHECK constraints reliably across drivers, so we add them explicitly.
	db.Conn.Exec(`
		DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'users_role_check') THEN
				ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('user', 'admin'));
			END IF;
		END $$;
	`)
	db.Conn.Exec(`
		DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'bot_collaborators_role_check') THEN
				ALTER TABLE bot_collaborators ADD CONSTRAINT bot_collaborators_role_check CHECK (role IN ('editor', 'viewer'));
			END IF;
		END $$;
	`)

	return nil
}
