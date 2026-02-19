package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

var DB *sql.DB

// Open initialises the SQLite database, creating it if necessary.
func Open(dataDir string) error {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	path := filepath.Join(dataDir, "agent.db")
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}

	db.SetMaxOpenConns(1) // SQLite is single-writer

	if err := migrate(db); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	DB = db
	return nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS students (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			login      TEXT,
			ldap_dn    TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS users (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			staff_code TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS edupass_accounts (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			login      TEXT,
			ldap_dn    TEXT,
			password   TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`)
	return err
}

// Student represents a row in the students table.
type Student struct {
	Login string
}

// Staff represents a row in the users table.
type Staff struct {
	StaffCode string
}

// GetStudents returns all students that have a login set.
func GetStudents() ([]Student, error) {
	rows, err := DB.Query(`SELECT login FROM students WHERE login IS NOT NULL ORDER BY login ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Student
	for rows.Next() {
		var s Student
		if err := rows.Scan(&s.Login); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetStaff returns all staff members that have a staff_code set.
func GetStaff() ([]Staff, error) {
	rows, err := DB.Query(`SELECT staff_code FROM users WHERE staff_code IS NOT NULL ORDER BY staff_code ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Staff
	for rows.Next() {
		var s Staff
		if err := rows.Scan(&s.StaffCode); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
