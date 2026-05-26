package storage

import (
	"crypto/rand"
	"database/sql"
	"fmt"

	"charm.land/log/v2"
	"github.com/charmbracelet/ssh"
	_ "github.com/mattn/go-sqlite3"
)

func GenerateUUIDv4() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

type Storage struct {
	db *sql.DB
}

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
        is_approved BOOLEAN DEFAULT 0,
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

	return &Storage{db: db}, nil
}

func (s *Storage) ValidatePublicKey(username string, incomingKey ssh.PublicKey) bool {
	query := `
        SELECT pk.authorized_key, CAST(u.is_active AS TEXT), CAST(u.is_approved AS TEXT)
        FROM public_keys pk
        JOIN users u ON u.id = pk.user_id
        WHERE u.username = ?;
    `
	rows, err := s.db.Query(query, username)
	if err != nil {
		log.Error("Database query failed", "err", err)
		return false
	}
	defer rows.Close()

	for rows.Next() {
		var dbKeyStr, activeStr, approvedStr string
		if err := rows.Scan(&dbKeyStr, &activeStr, &approvedStr); err != nil {
			continue
		}

		if (activeStr != "1" && activeStr != "true") || (approvedStr != "1" && approvedStr != "true") {
			continue
		}

		parsedKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(dbKeyStr))
		if err != nil {
			continue
		}

		if ssh.KeysEqual(parsedKey, incomingKey) {
			return true
		}
	}
	return false
}

func (s *Storage) AddUser(username, authorizedKey string) error {
	_, _, _, _, err := ssh.ParseAuthorizedKey([]byte(authorizedKey))
	if err != nil {
		return fmt.Errorf("invalid SSH public key: %w", err)
	}

	userID, _ := GenerateUUIDv4()
	keyID, _ := GenerateUUIDv4()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`INSERT INTO users (id, username, is_active) VALUES (?, ?, 1)`, userID, username); err != nil {
		return fmt.Errorf("failed to insert user: %w", err)
	}

	if _, err := tx.Exec(`INSERT INTO public_keys (id, user_id, authorized_key) VALUES (?, ?, ?)`, keyID, userID, authorizedKey); err != nil {
		return fmt.Errorf("failed to insert public key: %w", err)
	}

	return tx.Commit()
}

func (s *Storage) GetAllUsers() ([][]string, error) {
	query := `
        SELECT 
            u.username, 
            CAST(u.is_active AS TEXT), 
            CAST(u.is_approved AS TEXT), 
            COALESCE(u.role, 'user'),
            CASE WHEN a.id IS NOT NULL THEN 'Yes' ELSE 'No' END as pending_device
        FROM users u
        LEFT JOIN access_requests a ON u.username = a.username
    `
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users [][]string
	for rows.Next() {
		var name, active, approved, role, pending string

		if err := rows.Scan(&name, &active, &approved, &role, &pending); err != nil {
			log.Error("Failed to scan user row", "err", err)
			continue
		}

		isActive := "false"
		if active == "1" || active == "true" {
			isActive = "true"
		}

		isApproved := "false"
		if approved == "1" || approved == "true" {
			isApproved = "true"
		}

		users = append(users, []string{name, isActive, isApproved, role, pending})
	}
	return users, nil
}

func (s *Storage) ToggleApproval(username string) error {
	_, err := s.db.Exec(`UPDATE users SET is_approved = NOT is_approved WHERE username = ?`, username)
	return err
}

func (s *Storage) SubmitRequest(username, pubKey, message string) error {
	var userID string

	// The fixed unique variable to prevent shadowing errors
	lookupErr := s.db.QueryRow(`SELECT id FROM users WHERE username = ?`, username).Scan(&userID)

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if lookupErr == sql.ErrNoRows {
		userID, _ = GenerateUUIDv4()
		if _, err = tx.Exec(`INSERT INTO users (id, username, is_active) VALUES (?, ?, 1)`, userID, username); err != nil {
			return fmt.Errorf("failed to insert new user: %w", err)
		}
	} else if lookupErr != nil {
		return fmt.Errorf("database error checking user: %w", lookupErr)
	}

	if _, err = tx.Exec(`DELETE FROM access_requests WHERE username = ?`, username); err != nil {
		return fmt.Errorf("failed to clear old requests: %w", err)
	}

	reqID, _ := GenerateUUIDv4()
	if _, err = tx.Exec(`INSERT INTO access_requests (id, username, public_key, message) VALUES (?, ?, ?, ?)`, reqID, username, pubKey, message); err != nil {
		return fmt.Errorf("failed to save request message: %w", err)
	}

	return tx.Commit()
}

func (s *Storage) ApproveNewDevice(username string) error {
	var userID string
	if err := s.db.QueryRow(`SELECT id FROM users WHERE username = ?`, username).Scan(&userID); err != nil {
		return fmt.Errorf("user not found")
	}

	var pubKey string
	if err := s.db.QueryRow(`SELECT public_key FROM access_requests WHERE username = ?`, username).Scan(&pubKey); err != nil {
		return fmt.Errorf("no pending device requests for this user")
	}

	tx, _ := s.db.Begin()
	defer tx.Rollback()

	keyID, _ := GenerateUUIDv4()
	tx.Exec(`INSERT INTO public_keys (id, user_id, authorized_key) VALUES (?, ?, ?)`, keyID, userID, pubKey)
	tx.Exec(`DELETE FROM access_requests WHERE username = ?`, username)
	tx.Exec(`UPDATE users SET is_approved = 1 WHERE username = ?`, username)

	return tx.Commit()
}

func (s *Storage) PromoteAdmin(username string) error {
	res, err := s.db.Exec(`UPDATE users SET role = 'admin' WHERE username = ?`, username)
	if err != nil {
		return fmt.Errorf("failed to promote user: %w", err)
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("user '%s' not found", username)
	}
	return nil
}

func (s *Storage) DemoteAdmin(username string) error {
	res, err := s.db.Exec(`UPDATE users SET role = 'user' WHERE username = ?`, username)
	if err != nil {
		return fmt.Errorf("failed to demote user: %w", err)
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("user '%s' not found", username)
	}
	return nil
}

func (s *Storage) IsAdmin(username string) bool {
	var role string
	if err := s.db.QueryRow(`SELECT role FROM users WHERE username = ?`, username).Scan(&role); err != nil {
		return false
	}
	return role == "admin"
}
