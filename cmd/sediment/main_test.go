package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jacobjnilsson/sediment/internal/model"
	"github.com/jacobjnilsson/sediment/internal/store"
)

// mockStore is a test double for storeI.
type mockStore struct {
	migrateErr    error
	listAllFn     func() ([]*model.Memory, error)
	listByStateFn func(model.State) ([]*model.Memory, error)
	getFn         func(string) (*model.Memory, error)
	insertFn      func(*model.Memory) error
	updateFn      func(*model.Memory) error
	deleteFn      func(string) error
	runInTxFn     func(func() error) error
	closed        bool
}

func (m *mockStore) Migrate() error { return m.migrateErr }
func (m *mockStore) Close() error   { m.closed = true; return nil }
func (m *mockStore) ListAll() ([]*model.Memory, error) {
	if m.listAllFn != nil {
		return m.listAllFn()
	}
	return nil, nil
}
func (m *mockStore) ListByState(s model.State) ([]*model.Memory, error) {
	if m.listByStateFn != nil {
		return m.listByStateFn(s)
	}
	return nil, nil
}
func (m *mockStore) Get(id string) (*model.Memory, error) {
	if m.getFn != nil {
		return m.getFn(id)
	}
	return nil, store.ErrNotFound
}
func (m *mockStore) Insert(mem *model.Memory) error {
	if m.insertFn != nil {
		return m.insertFn(mem)
	}
	return nil
}
func (m *mockStore) Update(mem *model.Memory) error {
	if m.updateFn != nil {
		return m.updateFn(mem)
	}
	return nil
}
func (m *mockStore) Delete(id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(id)
	}
	return nil
}
func (m *mockStore) RunInTx(fn func() error) error {
	if m.runInTxFn != nil {
		return m.runInTxFn(fn)
	}
	// Default: just run fn directly (no actual transaction in mock).
	return fn()
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

func setUUID(id string) func() {
	old := newUUID
	newUUID = func() string { return id }
	return func() { newUUID = old }
}

func setTimeNow(t time.Time) func() {
	old := timeNow
	timeNow = func() time.Time { return t }
	return func() { timeNow = old }
}

// --- run / routing tests ---

func TestRunNoArgs(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := run(nil, &buf)
	if err == nil {
		t.Fatal("expected error with no args")
	}
	if !strings.Contains(err.Error(), "usage:") {
		t.Errorf("error = %q, want usage message", err)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := run([]string{"bogus"}, &buf)
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
	if !strings.Contains(err.Error(), "unknown command: bogus") {
		t.Errorf("error = %q, want 'unknown command: bogus'", err)
	}
}

// --- init tests ---

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
	want := map[string]string{"status": "ok", "path": dbFile}
	wantJSON, _ := json.Marshal(want)
	gotJSON, _ := json.Marshal(result)
	if string(gotJSON) != string(wantJSON) {
		t.Errorf("init output\n got: %s\nwant: %s", gotJSON, wantJSON)
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
	if !strings.Contains(err.Error(), "open database") {
		t.Errorf("error = %q, want 'open database'", err)
	}
}

func TestCmdInitDefaultPath(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(oldWd) })

	var buf bytes.Buffer
	if err := run([]string{"init"}, &buf); err != nil {
		t.Fatalf("init with default path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, defaultDBFile)); os.IsNotExist(err) {
		t.Fatal("default database file was not created")
	}
}

// --- status tests ---

func TestCmdStatus(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbFile := filepath.Join(dir, "status.db")

	var initBuf bytes.Buffer
	if err := run([]string{"init", "--db", dbFile}, &initBuf); err != nil {
		t.Fatalf("init: %v", err)
	}

	var buf bytes.Buffer
	if err := run([]string{"status", "--db", dbFile}, &buf); err != nil {
		t.Fatalf("status: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	want := map[string]any{
		"total": float64(0), "active": float64(0),
		"dormant": float64(0), "archived": float64(0),
		"path": dbFile,
	}
	wantJSON, _ := json.Marshal(want)
	gotJSON, _ := json.Marshal(result)
	if string(gotJSON) != string(wantJSON) {
		t.Errorf("status output\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
}

func TestCmdStatusInvalidDB(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := run([]string{"status", "--db", "/nonexistent/path/test.db"}, &buf)
	if err == nil {
		t.Fatal("expected error for invalid db path")
	}
	if !strings.Contains(err.Error(), "open database") {
		t.Errorf("error = %q, want 'open database'", err)
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
	if !strings.Contains(err.Error(), "list") {
		t.Errorf("error = %q, want 'list'", err)
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
	if err := cmdStatus([]string{}, &buf); err != nil {
		t.Fatalf("status: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result["total"].(float64) != 4 {
		t.Errorf("total = %v, want 4", result["total"])
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

// --- deposit tests ---

func TestCmdDeposit(t *testing.T) {
	// Not parallel: mutates global newUUID and timeNow.
	dir := t.TempDir()
	dbFile := filepath.Join(dir, "deposit.db")

	var initBuf bytes.Buffer
	if err := run([]string{"init", "--db", dbFile}, &initBuf); err != nil {
		t.Fatalf("init: %v", err)
	}

	fixedTime := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	restoreUUID := setUUID("fixed-uuid-123")
	defer restoreUUID()
	restoreTime := setTimeNow(fixedTime)
	defer restoreTime()

	var buf bytes.Buffer
	err := run([]string{
		"deposit",
		"--db", dbFile,
		"--content", "User prefers dark mode",
		"--tags", "preference,ui",
		"--source", "conversation-42",
	}, &buf)
	if err != nil {
		t.Fatalf("deposit: %v", err)
	}

	var got model.Memory
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}

	want := model.Memory{
		ID:             "fixed-uuid-123",
		Content:        "User prefers dark mode",
		Confidence:     1.0,
		State:          model.StateActive,
		AccessCount:    0,
		CreatedAt:      fixedTime,
		UpdatedAt:      fixedTime,
		LastAccessedAt: fixedTime,
		Tags:           []string{"preference", "ui"},
		Source:         "conversation-42",
	}
	wantJSON, _ := json.Marshal(want)
	gotJSON, _ := json.Marshal(got)
	if string(gotJSON) != string(wantJSON) {
		t.Errorf("deposit output\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
}

func TestCmdDepositMissingContent(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := run([]string{"deposit"}, &buf)
	if err == nil {
		t.Fatal("expected error for missing content")
	}
	if !strings.Contains(err.Error(), "--content is required") {
		t.Errorf("error = %q, want '--content is required'", err)
	}
}

func TestCmdDepositInsertError(t *testing.T) {
	restore := setOpenFunc(func(_ string) (storeI, error) {
		return &mockStore{
			insertFn: func(_ *model.Memory) error {
				return fmt.Errorf("disk full")
			},
		}, nil
	})
	defer restore()

	var buf bytes.Buffer
	err := cmdDeposit([]string{"--content", "test"}, &buf)
	if err == nil {
		t.Fatal("expected insert error")
	}
	if !strings.Contains(err.Error(), "deposit") {
		t.Errorf("error = %q, want 'deposit'", err)
	}
}

func TestCmdDepositNoTags(t *testing.T) {
	// Not parallel: mutates global newUUID.
	dir := t.TempDir()
	dbFile := filepath.Join(dir, "notags.db")

	var initBuf bytes.Buffer
	if err := run([]string{"init", "--db", dbFile}, &initBuf); err != nil {
		t.Fatalf("init: %v", err)
	}

	restoreUUID := setUUID("no-tags-id")
	defer restoreUUID()

	var buf bytes.Buffer
	err := run([]string{"deposit", "--db", dbFile, "--content", "simple memory"}, &buf)
	if err != nil {
		t.Fatalf("deposit: %v", err)
	}

	var got model.Memory
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Tags != nil {
		t.Errorf("tags = %v, want nil", got.Tags)
	}
}

func TestCmdDepositInvalidDB(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := run([]string{"deposit", "--db", "/nonexistent/path/test.db", "--content", "test"}, &buf)
	if err == nil {
		t.Fatal("expected error for invalid db path")
	}
	if !strings.Contains(err.Error(), "open database") {
		t.Errorf("error = %q, want 'open database'", err)
	}
}

// --- strata tests ---

func TestCmdStrata(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbFile := filepath.Join(dir, "strata.db")

	var initBuf bytes.Buffer
	if err := run([]string{"init", "--db", dbFile}, &initBuf); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Empty strata.
	var buf bytes.Buffer
	err := run([]string{"strata", "--db", dbFile}, &buf)
	if err != nil {
		t.Fatalf("strata: %v", err)
	}
	if strings.TrimSpace(buf.String()) != "null" {
		t.Errorf("strata output = %q, want null for empty list", buf.String())
	}
}

func TestCmdStrataWithMemories(t *testing.T) {
	fixedTime := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	restoreTime := setTimeNow(fixedTime)
	defer restoreTime()

	memories := []*model.Memory{
		{ID: "a", State: model.StateActive, Content: "first", Confidence: 0.9,
			CreatedAt: fixedTime, UpdatedAt: fixedTime, LastAccessedAt: fixedTime,
			Tags: []string{"x"}},
		{ID: "b", State: model.StateDormant, Content: "second", Confidence: 0.3,
			CreatedAt: fixedTime, UpdatedAt: fixedTime, LastAccessedAt: fixedTime,
			Tags: []string{"y"}},
	}

	restore := setOpenFunc(func(_ string) (storeI, error) {
		return &mockStore{
			listAllFn: func() ([]*model.Memory, error) {
				return memories, nil
			},
		}, nil
	})
	defer restore()

	var buf bytes.Buffer
	if err := cmdStrata([]string{}, &buf); err != nil {
		t.Fatalf("strata: %v", err)
	}

	var got []*model.Memory
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	wantJSON, _ := json.Marshal(memories)
	gotJSON, _ := json.Marshal(got)
	if string(gotJSON) != string(wantJSON) {
		t.Errorf("strata output\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
}

func TestCmdStrataFilterByState(t *testing.T) {
	activeMemories := []*model.Memory{
		{ID: "active-1", State: model.StateActive, Content: "active only", Confidence: 0.8},
	}

	restore := setOpenFunc(func(_ string) (storeI, error) {
		return &mockStore{
			listByStateFn: func(s model.State) ([]*model.Memory, error) {
				if s != model.StateActive {
					t.Errorf("state = %q, want active", s)
				}
				return activeMemories, nil
			},
		}, nil
	})
	defer restore()

	var buf bytes.Buffer
	if err := cmdStrata([]string{"--state", "active"}, &buf); err != nil {
		t.Fatalf("strata: %v", err)
	}

	var got []*model.Memory
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 1 || got[0].ID != "active-1" {
		t.Errorf("strata filtered output = %v, want [active-1]", got)
	}
}

func TestCmdStrataListError(t *testing.T) {
	restore := setOpenFunc(func(_ string) (storeI, error) {
		return &mockStore{
			listAllFn: func() ([]*model.Memory, error) {
				return nil, fmt.Errorf("query failed")
			},
		}, nil
	})
	defer restore()

	var buf bytes.Buffer
	err := cmdStrata([]string{}, &buf)
	if err == nil {
		t.Fatal("expected error from list")
	}
	if !strings.Contains(err.Error(), "strata") {
		t.Errorf("error = %q, want 'strata'", err)
	}
}

func TestCmdStrataInvalidDB(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := run([]string{"strata", "--db", "/nonexistent/path/test.db"}, &buf)
	if err == nil {
		t.Fatal("expected error for invalid db path")
	}
	if !strings.Contains(err.Error(), "open database") {
		t.Errorf("error = %q, want 'open database'", err)
	}
}

func TestCmdStrataInvalidState(t *testing.T) {
	restore := setOpenFunc(func(_ string) (storeI, error) {
		return &mockStore{}, nil
	})
	defer restore()

	var buf bytes.Buffer
	err := cmdStrata([]string{"--state", "banana"}, &buf)
	if err == nil {
		t.Fatal("expected error for invalid state")
	}
	if !strings.Contains(err.Error(), "unknown state: banana") {
		t.Errorf("error = %q, want 'unknown state: banana'", err)
	}
}

func TestCmdStrataListByStateError(t *testing.T) {
	restore := setOpenFunc(func(_ string) (storeI, error) {
		return &mockStore{
			listByStateFn: func(_ model.State) ([]*model.Memory, error) {
				return nil, fmt.Errorf("state query failed")
			},
		}, nil
	})
	defer restore()

	var buf bytes.Buffer
	err := cmdStrata([]string{"--state", "active"}, &buf)
	if err == nil {
		t.Fatal("expected error from list by state")
	}
	if !strings.Contains(err.Error(), "strata") {
		t.Errorf("error = %q, want 'strata'", err)
	}
}

// --- openAndMigrate tests ---

func TestOpenAndMigrateMigrateError(t *testing.T) {
	restore := setOpenFunc(func(_ string) (storeI, error) {
		return &mockStore{migrateErr: fmt.Errorf("bad schema")}, nil
	})
	defer restore()

	_, _, err := openAndMigrate([]string{})
	if err == nil {
		t.Fatal("expected migrate error")
	}
	if !strings.Contains(err.Error(), "migrate") {
		t.Errorf("error = %q, want 'migrate'", err)
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
	if !strings.Contains(err.Error(), "open database") {
		t.Errorf("error = %q, want 'open database'", err)
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
	if !strings.Contains(err.Error(), "resolve path") {
		t.Errorf("error = %q, want 'resolve path'", err)
	}
}

// --- helper tests ---

func TestFlagValueDefault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		flag     string
		fallback string
		want     string
	}{
		{"found", []string{"--db", "/tmp/custom.db"}, "--db", "default.db", "/tmp/custom.db"},
		{"not found uses fallback", []string{}, "--db", "default.db", "default.db"},
		{"flag at end without value uses fallback", []string{"--db"}, "--db", "default.db", "default.db"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := flagValueDefault(tt.args, tt.flag, tt.fallback)
			if got != tt.want {
				t.Errorf("flagValueDefault(%v, %q, %q) = %q, want %q", tt.args, tt.flag, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestFlagValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		flag string
		want string
	}{
		{"found", []string{"--content", "hello"}, "--content", "hello"},
		{"not found", []string{"--other", "x"}, "--content", ""},
		{"no value", []string{"--content"}, "--content", ""},
		{"empty args", []string{}, "--content", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := flagValue(tt.args, tt.flag)
			if got != tt.want {
				t.Errorf("flagValue(%v, %q) = %q, want %q", tt.args, tt.flag, got, tt.want)
			}
		})
	}
}

func TestWriteJSON(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := writeJSON(&buf, map[string]string{"key": "value"}); err != nil {
		t.Fatalf("writeJSON returned unexpected error: %v", err)
	}
	output := buf.String()
	if output != `{"key":"value"}`+"\n" {
		t.Errorf("output = %q, want JSON line", output)
	}
}

func TestWriteJSON_MarshalError(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	// channels cannot be marshaled to JSON
	err := writeJSON(&buf, make(chan int))
	if err == nil {
		t.Fatal("expected error for unmarshalable value")
	}
	if !strings.Contains(err.Error(), "json marshal") {
		t.Errorf("error = %q, want 'json marshal'", err)
	}
}

