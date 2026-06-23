package db

import (
	"database/sql"
	"embed"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type DB struct {
	*sql.DB
}

func Open(databaseURL string) (*DB, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("database URL is empty")
	}
	conn, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(15)
	conn.SetMaxIdleConns(5)
	conn.SetConnMaxLifetime(30 * time.Minute)
	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	db := &DB{DB: conn}
	if err := db.migrate(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return db, nil
}

func (db *DB) migrate() error {
	files := []string{
		"migrations/001_init.sql",
		"migrations/002_user_stats.sql",
		"migrations/003_pause_collecting.sql",
	}
	for _, name := range files {
		raw, err := migrationsFS.ReadFile(name)
		if err != nil {
			return err
		}
		if _, err := db.Exec(string(raw)); err != nil {
			return err
		}
	}
	return nil
}
