package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jacobjnilsson/sediment/internal/model"
)

// mockStore is a test double for storeI.
type mockStore struct {
	migrateErr error
	listAllFn  func() ([]*model.Memory, error)
	closed     bool
}

func (m *mockStore) Migrate() error               { return m.migrateErr }
func (m *mockStore) Insert(_ *model.Memory) error { return nil }
func (m *mockStore) Close() error                 { m.closed = true; return nil }
func (m *mockStore) ListAll() ([]*model.Memory, error) {
	if m.listAllFn != nil {
		return m.listAllFn()
	}
	return nil, nil
}

func setOpenFunc(fn func(string) (storeI, error)) func() {
	old := openFunc
	openFunc = fn
	return func() { openFunc = old }
}

func setAbsFunc(fn func(string) (string, error)) func() {
	old := absFunc
	absFunc = fn
	return func() { absFunc = old }
}

func TestRunNoArgs(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := run(nil, &buf)
	if err == nil {
		t.Fatal("expected error with no args")
	}
}

func TestRunUnknownCommand(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := run([]string{"bogus"}, &buf)
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
}

func TestCmdInit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbFile := filepath.Join(dir, "test.db")

	var buf bytes.Buffer
	err := run([]string{"init", "--db", dbFile}, &buf)
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	var result map[string]string
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("status = %q, want ok", result["status"])
	}
	if _, err := os.Stat(dbFile); os.IsNotExist(err) {
		t.Fatal("database file was not created")
	}
}

func TestCmdInitInvalidDB(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := run([]string{"init", "--db", "/nonexistent/path/test.db"}, &buf)
	if err == nil {
		t.Fatal("expected error for invalid db path")
	}
}

func TestCmdStatus(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbFile := filepath.Join(dir, "status.db")

	var initBuf bytes.Buffer
	if err := run([]string{"init", "--db", dbFile}, &initBuf); err != nil {
		t.Fatalf("init: %v", err)
	}

	var buf bytes.Buffer
	err := run([]string{"status", "--db", dbFile}, &buf)
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	if result["total"].(float64) != 0 {
		t.Errorf("total = %v, want 0", result["total"])
	}
}

func TestCmdStatusInvalidDB(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := run([]string{"status", "--db", "/nonexistent/path/test.db"}, &buf)
	if err == nil {
		t.Fatal("expected error for invalid db path")
	}
}

func TestCmdStatusListError(t *testing.T) {
	restore := setOpenFunc(func(_ string) (storeI, error) {
		return &mockStore{
			listAllFn: func() ([]*model.Memory, error) {
				return nil, fmt.Errorf("db on fire")
			},
		}, nil
	})
	defer restore()

	var buf bytes.Buffer
	err := cmdStatus([]string{}, &buf)
	if err == nil {
		t.Fatal("expected error from list")
	}
}

func TestCmdStatusWithMemories(t *testing.T) {
	restore := setOpenFunc(func(_ string) (storeI, error) {
		return &mockStore{
			listAllFn: func() ([]*model.Memory, error) {
				return []*model.Memory{
					{State: model.StateActive},
					{State: model.StateActive},
					{State: model.StateDormant},
					{State: model.StateArchived},
				}, nil
			},
		}, nil
	})
	defer restore()

	var buf bytes.Buffer
	err := cmdStatus([]string{}, &buf)
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result["active"].(float64) != 2 {
		t.Errorf("active = %v, want 2", result["active"])
	}
	if result["dormant"].(float64) != 1 {
		t.Errorf("dormant = %v, want 1", result["dormant"])
	}
	if result["archived"].(float64) != 1 {
		t.Errorf("archived = %v, want 1", result["archived"])
	}
}

func TestOpenAndMigrateMigrateError(t *testing.T) {
	restore := setOpenFunc(func(_ string) (storeI, error) {
		return &mockStore{migrateErr: fmt.Errorf("bad schema")}, nil
	})
	defer restore()

	_, _, err := openAndMigrate([]string{})
	if err == nil {
		t.Fatal("expected migrate error")
	}
}

func TestOpenAndMigrateOpenError(t *testing.T) {
	restore := setOpenFunc(func(_ string) (storeI, error) {
		return nil, fmt.Errorf("cannot open")
	})
	defer restore()

	_, _, err := openAndMigrate([]string{})
	if err == nil {
		t.Fatal("expected open error")
	}
}

func TestOpenAndMigrateAbsError(t *testing.T) {
	restore := setAbsFunc(func(_ string) (string, error) {
		return "", fmt.Errorf("abs failed")
	})
	defer restore()

	_, _, err := openAndMigrate([]string{})
	if err == nil {
		t.Fatal("expected abs error")
	}
}

func TestCmdInitDefaultPath(t *testing.T) {
	// Not parallel: changes working directory.
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(oldWd) })

	var buf bytes.Buffer
	err := run([]string{"init"}, &buf)
	if err != nil {
		t.Fatalf("init with default path: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, defaultDBFile)); os.IsNotExist(err) {
		t.Fatal("default database file was not created")
	}
}

func TestDbPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"with flag", []string{"--db", "/tmp/custom.db"}, "/tmp/custom.db"},
		{"without flag", []string{}, defaultDBFile},
		{"flag at end without value", []string{"--db"}, defaultDBFile},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := dbPath(tt.args)
			if got != tt.want {
				t.Errorf("dbPath(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestWriteJSON(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	writeJSON(&buf, map[string]string{"key": "value"})
	output := buf.String()
	if output != `{"key":"value"}`+"\n" {
		t.Errorf("output = %q, want JSON line", output)
	}
}
