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

	fmt.Printf("Database initialized at %s\n\n", db.DBPath())
	fmt.Println("Schema mismatch (intentional):")
	fmt.Println("  App expects orders columns: customer_email, total_amount, status")
	fmt.Println("  DB has orders columns:      amount, status")
	fmt.Println()
	fmt.Println("POST /api/orders will return HTTP 500 due to this mismatch.")
}

func initSchema(database *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL UNIQUE,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		// INTENTIONAL BUG: schema does not match OrdersCreateHandler INSERT.
		// Agent fix: add customer_email and total_amount columns here.
		`CREATE TABLE IF NOT EXISTS orders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			amount REAL NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
	}

	for _, stmt := range stmts {
		if _, err := database.Exec(stmt); err != nil {
			return err
		}
	}

	return nil
}
