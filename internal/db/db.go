package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"fmt"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps the single SQLite connection pool. SQLite allows one writer at a time,
// so we keep MaxOpenConns at 1 for writes to avoid SQLITE_BUSY storms; reads are cheap.
type DB struct {
	*sql.DB
}

func Open(dir string) (*DB, error) {
	path := filepath.Join(dir, "splitfriends.db")
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)", path)
	sqldb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// A single connection serialises all access, which is what SQLite wants and
	// what an app of this size needs. It also makes "BEGIN IMMEDIATE" unnecessary.
	sqldb.SetMaxOpenConns(1)
	sqldb.SetConnMaxLifetime(0)
	d := &DB{sqldb}
	if err := d.migrate(context.Background()); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return d, nil
}

// Tx runs fn inside a transaction, rolling back on error or panic.
func (d *DB) Tx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewID returns a 26-char random, URL-safe identifier.
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return b32.EncodeToString(b[:])
}

// NewToken returns a longer random token for sessions and invites.
func NewToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return b32.EncodeToString(b[:])
}

func Now() string { return time.Now().UTC().Format(time.RFC3339) }
