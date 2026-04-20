// Package store provides SQLite-backed persistence for sediment memories.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver

	"github.com/jacobjnilsson/sediment/internal/model"
)

// ErrNotFound is returned when a memory does not exist.
var ErrNotFound = errors.New("memory not found")

// executor is the common interface between *sql.DB and *sql.Tx.
type executor interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
	Query(query string, args ...any) (*sql.Rows, error)
}

// DB wraps a SQLite database for memory storage.
type DB struct {
	conn *sql.DB
	exec executor // points to conn normally, swapped to tx inside RunInTx
}

// sqlOpen is the function used to open a sql.DB. Overridable for tests.
var sqlOpen = sql.Open

// Open creates or opens a SQLite database at the given path.
// It verifies connectivity immediately with a ping.
func Open(path string) (*DB, error) {
	sqlDB, err := sqlOpen("sqlite3", path+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	return &DB{conn: sqlDB, exec: sqlDB}, nil
}

// Migrate runs schema migrations to ensure tables exist.
func (d *DB) Migrate() error {
	const schema = `
	CREATE TABLE IF NOT EXISTS memories (
		id              TEXT PRIMARY KEY,
		content         TEXT NOT NULL,
		confidence      REAL NOT NULL DEFAULT 1.0,
		state           TEXT NOT NULL DEFAULT 'active',
		access_count    INTEGER NOT NULL DEFAULT 0,
		created_at      TEXT NOT NULL,
		updated_at      TEXT NOT NULL,
		last_accessed_at TEXT NOT NULL,
		tags            TEXT NOT NULL DEFAULT '[]',
		source          TEXT NOT NULL DEFAULT '',
		hardness        INTEGER NOT NULL DEFAULT 5
	);
	CREATE INDEX IF NOT EXISTS idx_memories_state ON memories(state);
	CREATE INDEX IF NOT EXISTS idx_memories_confidence ON memories(confidence);
	CREATE TABLE IF NOT EXISTS meta (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	`
	_, err := d.exec.Exec(schema)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	d.exec.Exec(`ALTER TABLE memories ADD COLUMN hardness INTEGER NOT NULL DEFAULT 5`)

	return nil
}

func (d *DB) GetMeta(key string) (string, error) {
	var value string
	err := d.exec.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get meta %q: %w", key, err)
	}
	return value, nil
}

func (d *DB) SetMeta(key, value string) error {
	_, err := d.exec.Exec(
		`INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	if err != nil {
		return fmt.Errorf("set meta %q: %w", key, err)
	}
	return nil
}

// MarshalTags converts a string slice to its JSON representation.
func MarshalTags(tags []string) string {
	if tags == nil {
		return "[]"
	}
	b, _ := json.Marshal(tags) // []string never fails to marshal
	return string(b)
}

// Insert stores a new memory.
func (d *DB) Insert(m *model.Memory) error {
	_, err := d.exec.Exec(
		`INSERT INTO memories (id, content, confidence, state, access_count, created_at, updated_at, last_accessed_at, tags, source, hardness)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.Content, m.Confidence, string(m.State), m.AccessCount,
		m.CreatedAt.Format(time.RFC3339Nano),
		m.UpdatedAt.Format(time.RFC3339Nano),
		m.LastAccessedAt.Format(time.RFC3339Nano),
		MarshalTags(m.Tags), m.Source, int(m.Hardness),
	)
	if err != nil {
		return fmt.Errorf("insert memory: %w", err)
	}
	return nil
}

// Get retrieves a single memory by ID.
func (d *DB) Get(id string) (*model.Memory, error) {
	row := d.exec.QueryRow(
		`SELECT id, content, confidence, state, access_count, created_at, updated_at, last_accessed_at, tags, source, hardness
		 FROM memories WHERE id = ?`, id,
	)
	return scanMemory(row)
}

// Update replaces a memory's mutable fields.
func (d *DB) Update(m *model.Memory) error {
	res, err := d.exec.Exec(
		`UPDATE memories
		 SET content = ?, confidence = ?, state = ?, access_count = ?,
		     updated_at = ?, last_accessed_at = ?, tags = ?, source = ?, hardness = ?
		 WHERE id = ?`,
		m.Content, m.Confidence, string(m.State), m.AccessCount,
		m.UpdatedAt.Format(time.RFC3339Nano),
		m.LastAccessedAt.Format(time.RFC3339Nano),
		MarshalTags(m.Tags), m.Source, int(m.Hardness), m.ID,
	)
	if err != nil {
		return fmt.Errorf("update memory: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a memory by ID.
func (d *DB) Delete(id string) error {
	res, err := d.exec.Exec(`DELETE FROM memories WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete memory: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListByState returns all memories in a given state, ordered by confidence descending.
func (d *DB) ListByState(state model.State) ([]*model.Memory, error) {
	rows, err := d.exec.Query(
		`SELECT id, content, confidence, state, access_count, created_at, updated_at, last_accessed_at, tags, source, hardness
		 FROM memories WHERE state = ? ORDER BY confidence DESC`, string(state),
	)
	if err != nil {
		return nil, fmt.Errorf("list by state: %w", err)
	}
	defer rows.Close()
	return collectMemories(rows)
}

// ListAll returns all memories ordered by confidence descending.
func (d *DB) ListAll() ([]*model.Memory, error) {
	rows, err := d.exec.Query(
		`SELECT id, content, confidence, state, access_count, created_at, updated_at, last_accessed_at, tags, source, hardness
		 FROM memories ORDER BY confidence DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list all: %w", err)
	}
	defer rows.Close()
	return collectMemories(rows)
}

// beginTx starts a new transaction. Overridable for tests.
var beginTx = func(conn *sql.DB) (*sql.Tx, error) {
	return conn.Begin()
}

// RunInTx executes fn inside a database transaction. All store operations
// called on this DB within fn will participate in the transaction. If fn
// returns an error the transaction is rolled back. Otherwise it is committed.
func (d *DB) RunInTx(fn func() error) error {
	tx, err := beginTx(d.conn)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	origExec := d.exec
	d.exec = tx
	defer func() { d.exec = origExec }()

	if err := fn(); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// Close closes the database connection.
func (d *DB) Close() error {
	return d.conn.Close()
}

// scanner is implemented by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanMemory(s scanner) (*model.Memory, error) {
	var (
		m                                    model.Memory
		state                                string
		createdAt, updatedAt, lastAccessedAt string
		tagsJSON                             string
	)
	var hardness int
	err := s.Scan(
		&m.ID, &m.Content, &m.Confidence, &state, &m.AccessCount,
		&createdAt, &updatedAt, &lastAccessedAt, &tagsJSON, &m.Source, &hardness,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan memory: %w", err)
	}
	m.State = model.State(state)
	m.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	m.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	m.LastAccessedAt, _ = time.Parse(time.RFC3339Nano, lastAccessedAt)
	if err := json.Unmarshal([]byte(tagsJSON), &m.Tags); err != nil {
		return nil, fmt.Errorf("unmarshal tags: %w", err)
	}
	m.Hardness = model.Hardness(hardness)
	return &m, nil
}

// rowsIterator abstracts sql.Rows for testing. Overridable for tests.
var rowsErr = func(rows *sql.Rows) error {
	return rows.Err()
}

func collectMemories(rows *sql.Rows) ([]*model.Memory, error) {
	var memories []*model.Memory
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	if err := rowsErr(rows); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}
	return memories, nil
}
