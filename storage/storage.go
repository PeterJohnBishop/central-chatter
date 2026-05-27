package storage

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

type Storage struct {
	db *sql.DB
}

// builds the database instance
func NewStorage(dbPath string) (*Storage, error) {
	dsn := fmt.Sprintf("file:%s?_fk=1&_journal_mode=WAL", dbPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS content (
		id TEXT PRIMARY KEY, 
		user_id TEXT NOT NULL, 
		content BLOB NOT NULL,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		username TEXT UNIQUE NOT NULL,
		is_active BOOLEAN DEFAULT 1,   
		is_approved BOOLEAN DEFAULT 1, 
		role TEXT DEFAULT 'user',      
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS public_keys (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		authorized_key TEXT NOT NULL, 
		fingerprint TEXT,              
		added_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);
	CREATE TABLE IF NOT EXISTS access_requests (
		id TEXT PRIMARY KEY,
		username TEXT UNIQUE NOT NULL,
		public_key TEXT NOT NULL,
		message TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
	CREATE INDEX IF NOT EXISTS idx_public_keys_user_id ON public_keys(user_id);
	`
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	db.Exec("UPDATE users SET is_active = 0")
	return &Storage{db: db}, nil
}
