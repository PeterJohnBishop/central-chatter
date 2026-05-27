package storage

import (
	"crypto/rand"
	"database/sql"
	"fmt"

	"charm.land/log/v2"
	"github.com/charmbracelet/ssh"
)

// builds uuids
func GenerateUUIDv4() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

// in-app public key validation
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

// fetch all access requests
func (s *Storage) GetAccessRequests() ([][]string, error) {
	query := `SELECT id, username, public_key, COALESCE(message, ''), created_at FROM access_requests ORDER BY created_at DESC`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var requests [][]string
	for rows.Next() {
		var id, username, pubKey, message, createdAt string
		if err := rows.Scan(&id, &username, &pubKey, &message, &createdAt); err != nil {
			log.Error("Failed to scan access request row", "err", err)
			continue
		}
		requests = append(requests, []string{id, username, pubKey, message, createdAt})
	}
	return requests, nil
}

// toggle user approval status
func (s *Storage) ToggleApproval(username string) error {
	_, err := s.db.Exec(`UPDATE users SET is_approved = NOT is_approved WHERE username = ?`, username)
	return err
}

// creates the approval request
func (s *Storage) SubmitRequest(username, pubKey, message string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err = tx.Exec(`DELETE FROM access_requests WHERE username = ?`, username); err != nil {
		return fmt.Errorf("failed to clear old requests: %w", err)
	}

	reqID, _ := GenerateUUIDv4()
	if _, err = tx.Exec(`INSERT INTO access_requests (id, username, public_key, message) VALUES (?, ?, ?, ?)`, reqID, username, pubKey, message); err != nil {
		return fmt.Errorf("failed to save request message: %w", err)
	}

	return tx.Commit()
}

// approve request, commit user, and clear access request
func (s *Storage) ApproveRequest(username string) error {
	var pubKey string
	if err := s.db.QueryRow(`SELECT public_key FROM access_requests WHERE username = ?`, username).Scan(&pubKey); err != nil {
		return fmt.Errorf("request not found: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var userID string
	err = tx.QueryRow(`SELECT id FROM users WHERE username = ?`, username).Scan(&userID)

	if err == sql.ErrNoRows {
		userID, _ = GenerateUUIDv4()
		if _, err := tx.Exec(`INSERT INTO users (id, username, is_active, is_approved) VALUES (?, ?, 1, 1)`, userID, username); err != nil {
			return fmt.Errorf("failed to insert user: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("database error checking user: %w", err)
	}

	keyID, _ := GenerateUUIDv4()
	if _, err := tx.Exec(`INSERT INTO public_keys (id, user_id, authorized_key) VALUES (?, ?, ?)`, keyID, userID, pubKey); err != nil {
		return fmt.Errorf("failed to insert public key: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM access_requests WHERE username = ?`, username); err != nil {
		return fmt.Errorf("failed to delete access request: %w", err)
	}

	return tx.Commit()
}
