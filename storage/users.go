package storage

import "fmt"

// list all users
func (s *Storage) GetAllUsers() ([][]string, error) {
	query := `
		SELECT 
			username, 
			CAST(is_active AS TEXT), 
			CAST(is_approved AS TEXT),
			COALESCE(role, 'user')
		FROM users
		ORDER BY username ASC
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users [][]string
	for rows.Next() {
		var name, active, approved, role string
		if err := rows.Scan(&name, &active, &approved, &role); err != nil {
			continue
		}

		isOnline := "No"
		if active == "1" || active == "true" {
			isOnline = "Yes"
		}

		isApproved := "Revoked"
		if approved == "1" || approved == "true" {
			isApproved = "Approved"
		}

		users = append(users, []string{name, isOnline, isApproved, role})
	}
	return users, nil
}

// make user an admin
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

// make admin a user
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

// check if user is admin
func (s *Storage) IsAdmin(username string) bool {
	var role string
	if err := s.db.QueryRow(`SELECT role FROM users WHERE username = ?`, username).Scan(&role); err != nil {
		return false
	}
	return role == "admin"
}

// set user online or offline
func (s *Storage) SetOnlineStatus(username string, isOnline bool) error {
	status := 0
	if isOnline {
		status = 1
	}
	_, err := s.db.Exec(`UPDATE users SET is_active = ? WHERE username = ?`, status, username)
	return err
}
