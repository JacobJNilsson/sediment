package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
		updated_at TEXT, last_accessed_at TEXT, tags TEXT, source TEXT,
		hardness INTEGER NOT NULL DEFAULT 5
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
		`INSERT INTO memories VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"id-1", "content", 0.9, "active", 0, now, now, now, "NOT-JSON", "", 5,
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	row := db.QueryRow(`SELECT * FROM memories WHERE id = 'id-1'`)
	_, err = scanMemory(row)
	if err == nil {
		t.Fatal("expected unmarshal error")
	}
	if !strings.Contains(err.Error(), "unmarshal tags") {
		t.Errorf("error = %q, want it to mention 'unmarshal tags'", err)
	}
}

func TestScanMemoryScanError(t *testing.T) {
	t.Parallel()
	db := testDB(t)

	// Wrong column count triggers a scan error on an existing row.
	now := time.Now().Format(time.RFC3339Nano)
	_, err := db.Exec(
		`INSERT INTO memories VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"id-scan", "content", 0.9, "active", 0, now, now, now, "[]", "", 5,
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	row := db.QueryRow(`SELECT id FROM memories WHERE id = 'id-scan'`)
	_, err = scanMemory(row)
	if err == nil {
		t.Fatal("expected scan error")
	}
	if !strings.Contains(err.Error(), "scan memory") {
		t.Errorf("error = %q, want it to mention 'scan memory'", err)
	}
}

func TestCollectMemoriesScanError(t *testing.T) {
	t.Parallel()
	db := testDB(t)

	now := time.Now().Format(time.RFC3339Nano)
	_, err := db.Exec(
		`INSERT INTO memories VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"id-2", "content", 0.9, "active", 0, now, now, now, "BAD-JSON", "", 5,
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
	if !strings.Contains(err.Error(), "unmarshal tags") {
		t.Errorf("error = %q, want it to mention 'unmarshal tags'", err)
	}
}

func TestCollectMemoriesRowsErr(t *testing.T) {
	db := testDB(t)

	now := time.Now().Format(time.RFC3339Nano)
	_, err := db.Exec(
		`INSERT INTO memories VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"id-ok", "content", 0.9, "active", 0, now, now, now, `["tag"]`, "", 5,
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Override rowsErr to simulate an iteration error.
	old := rowsErr
	rowsErr = func(_ *sql.Rows) error {
		return fmt.Errorf("simulated rows error")
	}
	t.Cleanup(func() { rowsErr = old })

	rows, err := db.Query(`SELECT * FROM memories`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	_, err = collectMemories(rows)
	if err == nil {
		t.Fatal("expected error from collectMemories")
	}
	if !strings.Contains(err.Error(), "iterate rows") {
		t.Errorf("error = %q, want 'iterate rows'", err)
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
	if err := d.Migrate(""); err != nil {
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
	if !strings.Contains(err.Error(), "open database") {
		t.Errorf("error = %q, want it to mention 'open database'", err)
	}
}

func TestRunInTxCommit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "tx.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()
	if err := d.Migrate(""); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now()
	m := &model.Memory{
		ID: "tx-1", Content: "transactional", Confidence: 1.0,
		State: model.StateActive, CreatedAt: now, UpdatedAt: now,
		LastAccessedAt: now, Tags: []string{}, Source: "",
	}

	err = d.RunInTx(func() error {
		return d.Insert(m)
	})
	if err != nil {
		t.Fatalf("RunInTx: %v", err)
	}

	got, err := d.Get("tx-1")
	if err != nil {
		t.Fatalf("get after commit: %v", err)
	}
	if got.Content != "transactional" {
		t.Errorf("content = %q, want transactional", got.Content)
	}
}

func TestRunInTxRollback(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "tx.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()
	if err := d.Migrate(""); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now()
	m := &model.Memory{
		ID: "tx-rollback", Content: "should not persist", Confidence: 1.0,
		State: model.StateActive, CreatedAt: now, UpdatedAt: now,
		LastAccessedAt: now, Tags: []string{}, Source: "",
	}

	err = d.RunInTx(func() error {
		if err := d.Insert(m); err != nil {
			return err
		}
		return fmt.Errorf("forced rollback")
	})
	if err == nil {
		t.Fatal("expected error from RunInTx")
	}
	if !strings.Contains(err.Error(), "forced rollback") {
		t.Errorf("error = %q, want 'forced rollback'", err)
	}

	// Memory should not exist after rollback.
	_, err = d.Get("tx-rollback")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after rollback, got %v", err)
	}
}

func TestRunInTxOnClosedDB(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "tx-closed.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	d.Close()

	err = d.RunInTx(func() error { return nil })
	if err == nil {
		t.Fatal("expected error on closed db")
	}
	if !strings.Contains(err.Error(), "begin transaction") {
		t.Errorf("error = %q, want 'begin transaction'", err)
	}
}

// TestRunInTxCommitError must not be parallel: it replaces the global beginTx.
func TestRunInTxCommitError(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "tx-commit-err.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()
	if err := d.Migrate(""); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Rollback the tx before RunInTx tries to commit, making Commit fail.
	old := beginTx
	var sneakyTx *sql.Tx
	beginTx = func(conn *sql.DB) (*sql.Tx, error) {
		tx, err := conn.Begin()
		sneakyTx = tx
		return tx, err
	}
	t.Cleanup(func() { beginTx = old })

	err = d.RunInTx(func() error {
		// Rollback behind RunInTx's back so Commit will fail.
		sneakyTx.Rollback()
		return nil
	})
	if err == nil {
		t.Fatal("expected commit error")
	}
	if !strings.Contains(err.Error(), "commit transaction") {
		t.Errorf("error = %q, want 'commit transaction'", err)
	}
}

func TestRunInTxMultipleOps(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "tx-multi.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()
	if err := d.Migrate(""); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now()
	m1 := &model.Memory{
		ID: "tx-m1", Content: "first", Confidence: 1.0,
		State: model.StateActive, CreatedAt: now, UpdatedAt: now,
		LastAccessedAt: now, Tags: []string{}, Source: "",
	}
	// Pre-insert m1 so we can delete it inside the transaction.
	if err := d.Insert(m1); err != nil {
		t.Fatalf("pre-insert: %v", err)
	}

	m2 := &model.Memory{
		ID: "tx-m2", Content: "second", Confidence: 0.8,
		State: model.StateActive, CreatedAt: now, UpdatedAt: now,
		LastAccessedAt: now, Tags: []string{}, Source: "",
	}

	// Transaction: insert m2, then delete m1.
	err = d.RunInTx(func() error {
		if err := d.Insert(m2); err != nil {
			return err
		}
		return d.Delete("tx-m1")
	})
	if err != nil {
		t.Fatalf("RunInTx: %v", err)
	}

	// m2 should exist.
	got, err := d.Get("tx-m2")
	if err != nil {
		t.Fatalf("get m2: %v", err)
	}
	if got.Content != "second" {
		t.Errorf("m2 content = %q, want second", got.Content)
	}
	// m1 should be gone.
	_, err = d.Get("tx-m1")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for m1, got %v", err)
	}
}

func TestMigrateMetaSchemaError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "metaerr.db")

	rawDB, err := os.Create(dbPath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	rawDB.WriteString("not a database")
	rawDB.Close()

	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()

	d := &DB{exec: conn, conn: conn}
	err = d.Migrate("")
	if err == nil {
		t.Fatal("expected error on corrupt db")
	}
	if !strings.Contains(err.Error(), "migrate") {
		t.Errorf("error = %q, want mention of migrate", err)
	}
}
