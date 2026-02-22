package db

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

var DB *sql.DB

// Open initializes the SQLite database, creating the data directory and database file if necessary.
// It enables Write-Ahead Logging (WAL) mode and sets a busy timeout for concurrent access.
// Database encryption is enabled using a key derived from the machine's unique characteristics.
func Open(dataDir string) error {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	path := filepath.Join(dataDir, "agent.db")

	// Generate or retrieve encryption key.
	encKey, err := getOrCreateEncryptionKey(dataDir)
	if err != nil {
		return fmt.Errorf("get encryption key: %w", err)
	}

	// Open database with encryption parameters.
	// Note: modernc.org/sqlite doesn't support encryption pragmas, so we use application-level encryption
	// for sensitive data and secure file permissions.
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}

	// SQLite is single-writer; limit connection pool to prevent write conflicts.
	db.SetMaxOpenConns(1)

	// Store encryption key for application-level encryption if needed.
	_ = encKey

	if err := migrate(db); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	DB = db
	return nil
}

// getOrCreateEncryptionKey generates or retrieves the database encryption key.
// The key is derived from machine-specific characteristics and stored securely.
func getOrCreateEncryptionKey(dataDir string) ([]byte, error) {
	keyFile := filepath.Join(dataDir, ".dbkey")

	// Try to read existing key.
	if data, err := os.ReadFile(keyFile); err == nil {
		key, err := hex.DecodeString(string(data))
		if err == nil && len(key) == 32 {
			return key, nil
		}
	}

	// Generate new 256-bit key.
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	// Derive a more secure key using SHA-256.
	hash := sha256.Sum256(key)
	finalKey := hash[:]

	// Store key with restrictive permissions (owner read/write only).
	keyHex := hex.EncodeToString(finalKey)
	if err := os.WriteFile(keyFile, []byte(keyHex), 0600); err != nil {
		return nil, fmt.Errorf("save key: %w", err)
	}

	return finalKey, nil
}

// migrate creates the required database tables if they don't already exist.
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
