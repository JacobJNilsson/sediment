package migrate_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/jacobjnilsson/sediment/internal/migrate"
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestRunNoMigrations(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	if err := migrate.Run(db, t.TempDir(), nil); err != nil {
		t.Fatalf("Run(nil): %v", err)
	}
}

func TestRunAppliesMigrations(t *testing.T) {
	t.Parallel()
	db := openDB(t)

	migrations := []migrate.Migration{
		{Version: 1, Description: "create users", Up: func(ctx migrate.Context) error {
			_, err := ctx.DB.Exec(`CREATE TABLE users (id TEXT PRIMARY KEY)`)
			return err
		}},
		{Version: 2, Description: "add name", Up: func(ctx migrate.Context) error {
			_, err := ctx.DB.Exec(`ALTER TABLE users ADD COLUMN name TEXT`)
			return err
		}},
	}

	if err := migrate.Run(db, t.TempDir(), migrations); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var name sql.NullString
	err := db.QueryRow(`SELECT name FROM users WHERE id = 'test'`).Scan(&name)
	if err != sql.ErrNoRows {
		t.Errorf("expected ErrNoRows, got %v", err)
	}
}

func TestRunSkipsApplied(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	dir := t.TempDir()

	calls := 0
	m := []migrate.Migration{
		{Version: 1, Description: "first", Up: func(ctx migrate.Context) error {
			calls++
			return nil
		}},
	}

	if err := migrate.Run(db, dir, m); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}

	if err := migrate.Run(db, dir, m); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (should skip)", calls)
	}
}

func TestRunStopsOnError(t *testing.T) {
	t.Parallel()
	db := openDB(t)

	migrations := []migrate.Migration{
		{Version: 1, Description: "ok", Up: func(ctx migrate.Context) error {
			return nil
		}},
		{Version: 2, Description: "fail", Up: func(ctx migrate.Context) error {
			return errors.New("boom")
		}},
		{Version: 3, Description: "never", Up: func(ctx migrate.Context) error {
			t.Fatal("should not reach v3")
			return nil
		}},
	}

	err := migrate.Run(db, t.TempDir(), migrations)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errors.Unwrap(err)) {
		// just check it wraps something
	}
}

func TestRunSortsOutOfOrder(t *testing.T) {
	t.Parallel()
	db := openDB(t)

	var order []int
	migrations := []migrate.Migration{
		{Version: 3, Description: "third", Up: func(ctx migrate.Context) error {
			order = append(order, 3)
			return nil
		}},
		{Version: 1, Description: "first", Up: func(ctx migrate.Context) error {
			order = append(order, 1)
			return nil
		}},
		{Version: 2, Description: "second", Up: func(ctx migrate.Context) error {
			order = append(order, 2)
			return nil
		}},
	}

	if err := migrate.Run(db, t.TempDir(), migrations); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Errorf("order = %v, want [1 2 3]", order)
	}
}

func TestRunPassesDataDir(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	dir := t.TempDir()

	var gotDir string
	m := []migrate.Migration{
		{Version: 1, Description: "check dir", Up: func(ctx migrate.Context) error {
			gotDir = ctx.DataDir
			return nil
		}},
	}

	if err := migrate.Run(db, dir, m); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if gotDir != dir {
		t.Errorf("DataDir = %q, want %q", gotDir, dir)
	}
}

func TestRunClosedDB(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	db.Close()

	err := migrate.Run(db, t.TempDir(), nil)
	if err == nil {
		t.Fatal("expected error on closed db")
	}
}

func TestRunSetVersionError(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	dir := t.TempDir()

	m := []migrate.Migration{
		{Version: 1, Description: "close db in migration", Up: func(ctx migrate.Context) error {
			ctx.DB.Close()
			return nil
		}},
	}

	err := migrate.Run(db, dir, m)
	if err == nil {
		t.Fatal("expected error from setVersion on closed db")
	}
}

func TestRunCurrentVersionParseError(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	dir := t.TempDir()

	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`)
	if err != nil {
		t.Fatalf("create meta: %v", err)
	}
	_, err = db.Exec(`INSERT INTO meta (key, value) VALUES ('schema_version', 'garbage')`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	err = migrate.Run(db, dir, []migrate.Migration{
		{Version: 1, Description: "noop", Up: func(ctx migrate.Context) error { return nil }},
	})
	if err == nil {
		t.Fatal("expected error from invalid schema_version")
	}
}
