package main

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/appcd-dev/order-service/internal/db"
)

func main() {
	database, err := db.Open()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	if err := initSchema(database); err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize schema: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Database initialized at %s\n", db.DBPath())
}

func initSchema(database *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL UNIQUE,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS orders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			customer_email TEXT NOT NULL,
			total_amount REAL NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		// Migration: add missing columns to existing databases that were
		// created without customer_email / total_amount. SQLite ignores
		// "duplicate column" errors, so we swallow them explicitly.
		`ALTER TABLE orders ADD COLUMN customer_email TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE orders ADD COLUMN total_amount REAL NOT NULL DEFAULT 0`,
	}

	for _, stmt := range stmts {
		if _, err := database.Exec(stmt); err != nil {
			// Ignore "duplicate column" errors from ALTER TABLE on already-migrated DBs
			if isDuplicateColumnErr(err) {
				continue
			}
			return err
		}
	}

	return nil
}

// isDuplicateColumnErr returns true when SQLite reports that a column
// being added via ALTER TABLE already exists (error code 1, message
// contains "duplicate column name").
func isDuplicateColumnErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "duplicate column name") ||
		contains(msg, "already exists")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
