package storage

import (
	"database/sql"
	"fmt"
)

// inserts a new chat message into the content table
func (s *Storage) SaveMessage(username, message string) error {
	var userID string
	err := s.db.QueryRow(`SELECT id FROM users WHERE username = ?`, username).Scan(&userID)

	if err == sql.ErrNoRows {
		userID, _ = GenerateUUIDv4()
		_, err = s.db.Exec(`INSERT INTO users (id, username, is_active, is_approved, role) VALUES (?, ?, 1, 1, 'admin')`, userID, username)
		if err != nil {
			return fmt.Errorf("failed to auto-create system user: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("database error looking up user: %w", err)
	}

	msgID, _ := GenerateUUIDv4()

	_, err = s.db.Exec(`INSERT INTO content (id, user_id, content) VALUES (?, ?, ?)`, msgID, userID, []byte(message))
	return err
}

// fetch messages
func (s *Storage) GetRecentMessages(limit int) ([]string, error) {
	query := `
		SELECT username, content FROM (
			SELECT u.username, c.content, c.rowid
			FROM content c
			JOIN users u ON c.user_id = u.id
			ORDER BY c.rowid DESC
			LIMIT ?
		) ORDER BY rowid ASC
	`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []string
	for rows.Next() {
		var username string
		var content []byte
		if err := rows.Scan(&username, &content); err != nil {
			continue
		}
		messages = append(messages, fmt.Sprintf("%s: %s", username, string(content)))
	}
	return messages, nil
}
