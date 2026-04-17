package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/jacobjnilsson/sediment/internal/model"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := sql.Open("sqlite3", filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	_, err = db.Exec(`
	CREATE TABLE memories (
		id TEXT PRIMARY KEY, content TEXT, confidence REAL,
		state TEXT, access_count INTEGER, created_at TEXT,
		updated_at TEXT, last_accessed_at TEXT, tags TEXT, source TEXT
	)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestScanMemoryUnmarshalTagsError(t *testing.T) {
	t.Parallel()
	db := testDB(t)

	now := time.Now().Format(time.RFC3339Nano)
	_, err := db.Exec(
		`INSERT INTO memories VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"id-1", "content", 0.9, "active", 0, now, now, now, "NOT-JSON", "",
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	row := db.QueryRow(`SELECT * FROM memories WHERE id = 'id-1'`)
	_, err = scanMemory(row)
	if err == nil {
		t.Fatal("expected unmarshal error")
	}
}

func TestScanMemoryScanError(t *testing.T) {
	t.Parallel()
	db := testDB(t)

	// Query a row that doesn't have the right number of columns.
	row := db.QueryRow(`SELECT id FROM memories WHERE id = 'nonexistent'`)
	_, err := scanMemory(row)
	if err == nil {
		t.Fatal("expected scan error or not found")
	}
}

func TestCollectMemoriesScanError(t *testing.T) {
	t.Parallel()
	db := testDB(t)

	now := time.Now().Format(time.RFC3339Nano)
	_, err := db.Exec(
		`INSERT INTO memories VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"id-2", "content", 0.9, "active", 0, now, now, now, "BAD-JSON", "",
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	rows, err := db.Query(`SELECT * FROM memories`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	_, err = collectMemories(rows)
	if err == nil {
		t.Fatal("expected error from collectMemories with bad tags")
	}
}

func TestScanMemoryErrNoRows(t *testing.T) {
	t.Parallel()
	db := testDB(t)

	row := db.QueryRow(`SELECT * FROM memories WHERE id = 'nonexistent'`)
	_, err := scanMemory(row)
	if err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestInsertNilTagsViaDB(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()
	if err := d.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now()
	m := &model.Memory{
		ID: "nil-tags", Content: "test", Confidence: 0.5,
		State: model.StateActive, CreatedAt: now, UpdatedAt: now,
		LastAccessedAt: now, Tags: nil, Source: "",
	}
	if err := d.Insert(m); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := d.Get("nil-tags")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.Tags) != 0 {
		t.Errorf("tags = %v, want empty", got.Tags)
	}
}

// TestOpenSqlOpenError must not be parallel: it replaces the global sqlOpen.
func TestOpenSqlOpenError(t *testing.T) {
	old := sqlOpen
	sqlOpen = func(driver, dsn string) (*sql.DB, error) {
		return nil, fmt.Errorf("driver error")
	}
	t.Cleanup(func() { sqlOpen = old })

	_, err := Open("/tmp/test.db")
	if err == nil {
		t.Fatal("expected error from sql.Open")
	}
}
