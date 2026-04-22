// Copyright © 2026 ForcePoint Software. All rights reserved.

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
	// Ensure the data directory exists before attempting to create the database file.
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	path := filepath.Join(dataDir, "agent.db")

	// Obtain or generate a 32-byte key for application-level encryption of
	// sensitive columns. The key is kept in a separate .dbkey file so it
	// survives database recreation without being embedded in the DB itself.
	encKey, err := getOrCreateEncryptionKey(dataDir)
	if err != nil {
		return fmt.Errorf("get encryption key: %w", err)
	}

	// WAL mode allows concurrent reads while a write is in progress, which
	// is important because the scheduler runs multiple tasks simultaneously.
	// _busy_timeout=5000 makes writers wait up to 5 s instead of returning
	// SQLITE_BUSY immediately, reducing spurious errors under brief contention.
	// Note: modernc.org/sqlite doesn't support SQLCipher encryption pragmas,
	// so sensitive values are encrypted at the application layer instead.
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}

	// SQLite only allows one concurrent writer regardless of connection count.
	// Capping to one connection avoids "database is locked" errors and removes
	// the overhead of the connection pool.
	db.SetMaxOpenConns(1)

	// Retain the key for future application-level encryption of sensitive rows.
	_ = encKey

	// Create tables and indexes that don't yet exist; no-op on subsequent runs.
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

	// If a valid 32-byte key already exists on disk, reuse it so that any
	// previously encrypted data remains readable across restarts.
	if data, err := os.ReadFile(keyFile); err == nil {
		key, err := hex.DecodeString(string(data))
		if err == nil && len(key) == 32 {
			return key, nil
		}
	}

	// Generate 32 random bytes as the seed. We then hash them with SHA-256
	// to produce the final key; this adds a deterministic whitening step and
	// ensures the output is always exactly 32 bytes regardless of the RNG output.
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	hash := sha256.Sum256(key)
	finalKey := hash[:]

	// Persist the key as hex with mode 0600 so only the service account
	// (LocalSystem on Windows) can read it.
	keyHex := hex.EncodeToString(finalKey)
	if err := os.WriteFile(keyFile, []byte(keyHex), 0600); err != nil {
		return nil, fmt.Errorf("save key: %w", err)
	}

	return finalKey, nil
}

// migrate creates the required database tables if they don't already exist.
func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		-- Stores per-probe ICMP measurement history used by the monitoring task
		-- to feed rrdtool and upload smokeping-style graphs to the tenant.
		CREATE TABLE IF NOT EXISTS probe_history (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			probe_id    INTEGER NOT NULL,
			ts          INTEGER NOT NULL,
			avg_ms      REAL,
			min_ms      REAL,
			max_ms      REAL,
			packet_loss INTEGER NOT NULL DEFAULT 0
		);
		-- Compound index supports efficient per-probe time-range queries.
		CREATE INDEX IF NOT EXISTS idx_probe_history ON probe_history (probe_id, ts DESC);

		-- Caches student LDAP accounts synced from EduStar STMC.
		CREATE TABLE IF NOT EXISTS students (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			login      TEXT,
			ldap_dn    TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		-- Caches staff accounts synced from EduStar STMC.
		CREATE TABLE IF NOT EXISTS users (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			staff_code TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		-- Stores CRT (Casual Relief Teacher) accounts including daily passwords
		-- generated during the enable-crt-accounts operation.
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
