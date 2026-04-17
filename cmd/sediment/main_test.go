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

// --- excavate tests ---

func TestCmdExcavate(t *testing.T) {
	fixedTime := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	restoreTime := setTimeNow(fixedTime)
	defer restoreTime()

	mem := &model.Memory{
		ID:             "mem-1",
		Content:        "dark mode preference",
		Confidence:     0.8,
		State:          model.StateActive,
		AccessCount:    2,
		CreatedAt:      fixedTime.Add(-48 * time.Hour),
		UpdatedAt:      fixedTime.Add(-24 * time.Hour),
		LastAccessedAt: fixedTime.Add(-24 * time.Hour),
		Tags:           []string{"pref"},
		Source:         "chat",
	}

	var updated *model.Memory
	restore := setOpenFunc(func(_ string) (storeI, error) {
		return &mockStore{
			getFn: func(id string) (*model.Memory, error) {
				if id != "mem-1" {
					return nil, store.ErrNotFound
				}
				// Return a copy so the original isn't mutated.
				cp := *mem
				cp.Tags = make([]string, len(mem.Tags))
				copy(cp.Tags, mem.Tags)
				return &cp, nil
			},
			updateFn: func(m *model.Memory) error {
				updated = m
				return nil
			},
		}, nil
	})
	defer restore()

	var buf bytes.Buffer
	err := cmdExcavate([]string{"--id", "mem-1"}, &buf)
	if err != nil {
		t.Fatalf("excavate: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Should have the expected keys.
	for _, key := range []string{"memory", "decayed_confidence", "boosted_confidence", "state"} {
		if _, ok := result[key]; !ok {
			t.Errorf("missing key %q in output", key)
		}
	}

	// AccessCount should have incremented.
	if updated == nil {
		t.Fatal("expected update to be called")
	}
	if updated.AccessCount != 3 {
		t.Errorf("AccessCount = %d, want 3", updated.AccessCount)
	}
	// Confidence should be boosted above decayed value.
	if result["boosted_confidence"].(float64) <= result["decayed_confidence"].(float64) {
		t.Errorf("boosted (%v) should be > decayed (%v)",
			result["boosted_confidence"], result["decayed_confidence"])
	}
}

func TestCmdExcavateMissingID(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := run([]string{"excavate"}, &buf)
	if err == nil {
		t.Fatal("expected error for missing --id")
	}
	if !strings.Contains(err.Error(), "--id is required") {
		t.Errorf("error = %q, want '--id is required'", err)
	}
}

func TestCmdExcavateNotFound(t *testing.T) {
	restore := setOpenFunc(func(_ string) (storeI, error) {
		return &mockStore{
			getFn: func(_ string) (*model.Memory, error) {
				return nil, store.ErrNotFound
			},
		}, nil
	})
	defer restore()

	var buf bytes.Buffer
	err := cmdExcavate([]string{"--id", "missing"}, &buf)
	if err == nil {
		t.Fatal("expected error for missing memory")
	}
	if !strings.Contains(err.Error(), "excavate") {
		t.Errorf("error = %q, want 'excavate'", err)
	}
}

func TestCmdExcavateUpdateError(t *testing.T) {
	fixedTime := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	restoreTime := setTimeNow(fixedTime)
	defer restoreTime()

	restore := setOpenFunc(func(_ string) (storeI, error) {
		return &mockStore{
			getFn: func(_ string) (*model.Memory, error) {
				return &model.Memory{
					ID: "x", Confidence: 0.5, State: model.StateActive,
					LastAccessedAt: fixedTime, Tags: []string{},
				}, nil
			},
			updateFn: func(_ *model.Memory) error {
				return fmt.Errorf("write failed")
			},
		}, nil
	})
	defer restore()

	var buf bytes.Buffer
	err := cmdExcavate([]string{"--id", "x"}, &buf)
	if err == nil {
		t.Fatal("expected update error")
	}
	if !strings.Contains(err.Error(), "excavate update") {
		t.Errorf("error = %q, want 'excavate update'", err)
	}
}

func TestCmdExcavateInvalidDB(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := run([]string{"excavate", "--id", "x", "--db", "/nonexistent/path/test.db"}, &buf)
	if err == nil {
		t.Fatal("expected error for invalid db path")
	}
	if !strings.Contains(err.Error(), "open database") {
		t.Errorf("error = %q, want 'open database'", err)
	}
}

// --- erode tests ---

func TestCmdErodeNoMemories(t *testing.T) {
	restore := setOpenFunc(func(_ string) (storeI, error) {
		return &mockStore{
			listAllFn: func() ([]*model.Memory, error) {
				return nil, nil
			},
		}, nil
	})
	defer restore()

	var buf bytes.Buffer
	if err := cmdErode([]string{}, &buf); err != nil {
		t.Fatalf("erode: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result["processed"].(float64) != 0 {
		t.Errorf("processed = %v, want 0", result["processed"])
	}
	if result["updated"].(float64) != 0 {
		t.Errorf("updated = %v, want 0", result["updated"])
	}
}

func TestCmdErodeWithTransitions(t *testing.T) {
	fixedTime := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	restoreTime := setTimeNow(fixedTime)
	defer restoreTime()

	// Memory that was active 200 hours ago with confidence 0.5:
	// 0.5 * e^(-0.01*200) = 0.5 * e^(-2) ≈ 0.0677 -> below archive threshold (0.1)
	oldMem := &model.Memory{
		ID:             "old-mem",
		Content:        "stale fact",
		Confidence:     0.5,
		State:          model.StateActive,
		AccessCount:    1,
		CreatedAt:      fixedTime.Add(-200 * time.Hour),
		UpdatedAt:      fixedTime.Add(-200 * time.Hour),
		LastAccessedAt: fixedTime.Add(-200 * time.Hour),
		Tags:           []string{"fact"},
	}

	// Fresh memory: accessed now, no decay.
	freshMem := &model.Memory{
		ID:             "fresh-mem",
		Content:        "new fact",
		Confidence:     0.9,
		State:          model.StateActive,
		LastAccessedAt: fixedTime,
		UpdatedAt:      fixedTime,
		Tags:           []string{},
	}

	var updatedIDs []string
	restore := setOpenFunc(func(_ string) (storeI, error) {
		return &mockStore{
			listAllFn: func() ([]*model.Memory, error) {
				// Return copies.
				o := *oldMem
				o.Tags = append([]string{}, oldMem.Tags...)
				f := *freshMem
				f.Tags = append([]string{}, freshMem.Tags...)
				return []*model.Memory{&o, &f}, nil
			},
			updateFn: func(m *model.Memory) error {
				updatedIDs = append(updatedIDs, m.ID)
				return nil
			},
		}, nil
	})
	defer restore()

	var buf bytes.Buffer
	if err := cmdErode([]string{}, &buf); err != nil {
		t.Fatalf("erode: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result["processed"].(float64) != 2 {
		t.Errorf("processed = %v, want 2", result["processed"])
	}
	// old-mem should transition; fresh-mem should not change.
	if result["updated"].(float64) < 1 {
		t.Errorf("updated = %v, want >= 1", result["updated"])
	}

	transitions := result["transitions"]
	if transitions == nil {
		t.Fatal("expected transitions array")
	}
	ts := transitions.([]any)
	if len(ts) == 0 {
		t.Fatal("expected at least one transition")
	}
	first := ts[0].(map[string]any)
	if first["id"] != "old-mem" {
		t.Errorf("transition id = %v, want old-mem", first["id"])
	}
	if first["new_state"] != "archived" {
		t.Errorf("new_state = %v, want archived", first["new_state"])
	}
}

func TestCmdErodeListError(t *testing.T) {
	restore := setOpenFunc(func(_ string) (storeI, error) {
		return &mockStore{
			listAllFn: func() ([]*model.Memory, error) {
				return nil, fmt.Errorf("db gone")
			},
		}, nil
	})
	defer restore()

	var buf bytes.Buffer
	err := cmdErode([]string{}, &buf)
	if err == nil {
		t.Fatal("expected list error")
	}
	if !strings.Contains(err.Error(), "erode list") {
		t.Errorf("error = %q, want 'erode list'", err)
	}
}

func TestCmdErodeUpdateError(t *testing.T) {
	fixedTime := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	restoreTime := setTimeNow(fixedTime)
	defer restoreTime()

	restore := setOpenFunc(func(_ string) (storeI, error) {
		return &mockStore{
			listAllFn: func() ([]*model.Memory, error) {
				return []*model.Memory{
					{
						ID: "fail", Confidence: 0.5, State: model.StateActive,
						LastAccessedAt: fixedTime.Add(-100 * time.Hour), Tags: []string{},
					},
				}, nil
			},
			updateFn: func(_ *model.Memory) error {
				return fmt.Errorf("write failed")
			},
		}, nil
	})
	defer restore()

	var buf bytes.Buffer
	err := cmdErode([]string{}, &buf)
	if err == nil {
		t.Fatal("expected update error")
	}
	if !strings.Contains(err.Error(), "erode update") {
		t.Errorf("error = %q, want 'erode update'", err)
	}
}

func TestCmdErodeInvalidDB(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := run([]string{"erode", "--db", "/nonexistent/path/test.db"}, &buf)
	if err == nil {
		t.Fatal("expected error for invalid db path")
	}
	if !strings.Contains(err.Error(), "open database") {
		t.Errorf("error = %q, want 'open database'", err)
	}
}

func TestCmdErodeDormantTransition(t *testing.T) {
	fixedTime := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	restoreTime := setTimeNow(fixedTime)
	defer restoreTime()

	// Memory with confidence 0.5 accessed 50 hours ago:
	// 0.5 * e^(-0.01*50) = 0.5 * e^(-0.5) ≈ 0.303 -> dormant (between 0.1 and 0.4)
	mem := &model.Memory{
		ID:             "mid-mem",
		Content:        "aging fact",
		Confidence:     0.5,
		State:          model.StateActive,
		LastAccessedAt: fixedTime.Add(-50 * time.Hour),
		UpdatedAt:      fixedTime.Add(-50 * time.Hour),
		Tags:           []string{},
	}

	restore := setOpenFunc(func(_ string) (storeI, error) {
		return &mockStore{
			listAllFn: func() ([]*model.Memory, error) {
				cp := *mem
				return []*model.Memory{&cp}, nil
			},
			updateFn: func(_ *model.Memory) error { return nil },
		}, nil
	})
	defer restore()

	var buf bytes.Buffer
	if err := cmdErode([]string{}, &buf); err != nil {
		t.Fatalf("erode: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	ts := result["transitions"].([]any)
	if len(ts) != 1 {
		t.Fatalf("transitions = %d, want 1", len(ts))
	}
	first := ts[0].(map[string]any)
	if first["new_state"] != "dormant" {
		t.Errorf("new_state = %v, want dormant", first["new_state"])
	}
}

// --- compact tests ---

func TestCmdCompactCandidates(t *testing.T) {
	dormant := []*model.Memory{
		{ID: "d1", State: model.StateDormant, Content: "dormant fact", Tags: []string{}},
	}
	archived := []*model.Memory{
		{ID: "a1", State: model.StateArchived, Content: "archived fact", Tags: []string{}},
	}

	restore := setOpenFunc(func(_ string) (storeI, error) {
		return &mockStore{
			listByStateFn: func(s model.State) ([]*model.Memory, error) {
				switch s {
				case model.StateDormant:
					return dormant, nil
				case model.StateArchived:
					return archived, nil
				}
				return nil, nil
			},
		}, nil
	})
	defer restore()

	var buf bytes.Buffer
	if err := cmdCompact([]string{}, &buf); err != nil {
		t.Fatalf("compact: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result["count"].(float64) != 2 {
		t.Errorf("count = %v, want 2", result["count"])
	}
	candidates := result["candidates"].([]any)
	if len(candidates) != 2 {
		t.Errorf("candidates len = %d, want 2", len(candidates))
	}
}

func TestCmdCompactCandidatesDormantError(t *testing.T) {
	restore := setOpenFunc(func(_ string) (storeI, error) {
		return &mockStore{
			listByStateFn: func(s model.State) ([]*model.Memory, error) {
				if s == model.StateDormant {
					return nil, fmt.Errorf("dormant query failed")
				}
				return nil, nil
			},
		}, nil
	})
	defer restore()

	var buf bytes.Buffer
	err := cmdCompact([]string{}, &buf)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "compact list dormant") {
		t.Errorf("error = %q, want 'compact list dormant'", err)
	}
}

func TestCmdCompactCandidatesArchivedError(t *testing.T) {
	restore := setOpenFunc(func(_ string) (storeI, error) {
		return &mockStore{
			listByStateFn: func(s model.State) ([]*model.Memory, error) {
				if s == model.StateArchived {
					return nil, fmt.Errorf("archived query failed")
				}
				return nil, nil
			},
		}, nil
	})
	defer restore()

	var buf bytes.Buffer
	err := cmdCompact([]string{}, &buf)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "compact list archived") {
		t.Errorf("error = %q, want 'compact list archived'", err)
	}
}

func TestCmdCompactApply(t *testing.T) {
	fixedTime := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	restoreTime := setTimeNow(fixedTime)
	defer restoreTime()
	restoreUUID := setUUID("compacted-uuid")
	defer restoreUUID()

	memStore := map[string]*model.Memory{
		"s1": {ID: "s1", Content: "fact A", Tags: []string{"alpha", "shared"}, State: model.StateDormant},
		"s2": {ID: "s2", Content: "fact B", Tags: []string{"beta", "shared"}, State: model.StateArchived},
	}

	var insertedMem *model.Memory
	restore := setOpenFunc(func(_ string) (storeI, error) {
		return &mockStore{
			getFn: func(id string) (*model.Memory, error) {
				m, ok := memStore[id]
				if !ok {
					return nil, store.ErrNotFound
				}
				cp := *m
				cp.Tags = append([]string{}, m.Tags...)
				return &cp, nil
			},
			deleteFn: func(id string) error {
				delete(memStore, id)
				return nil
			},
			insertFn: func(m *model.Memory) error {
				insertedMem = m
				return nil
			},
		}, nil
	})
	defer restore()

	var buf bytes.Buffer
	err := cmdCompact([]string{
		"--apply", "Combined fact about A and B",
		"--sources", "s1,s2",
	}, &buf)
	if err != nil {
		t.Fatalf("compact apply: %v", err)
	}

	if insertedMem == nil {
		t.Fatal("expected a memory to be inserted")
	}
	if insertedMem.ID != "compacted-uuid" {
		t.Errorf("ID = %q, want compacted-uuid", insertedMem.ID)
	}
	if insertedMem.Content != "Combined fact about A and B" {
		t.Errorf("Content = %q", insertedMem.Content)
	}
	if insertedMem.Confidence != 0.8 {
		t.Errorf("Confidence = %v, want 0.8", insertedMem.Confidence)
	}
	if !strings.Contains(insertedMem.Source, "compact:") {
		t.Errorf("Source = %q, want compact: prefix", insertedMem.Source)
	}
	// Tags should be sorted and deduplicated from both sources.
	wantTags := []string{"alpha", "beta", "shared"}
	if len(insertedMem.Tags) != len(wantTags) {
		t.Errorf("Tags = %v, want %v", insertedMem.Tags, wantTags)
	} else {
		for i, tag := range wantTags {
			if insertedMem.Tags[i] != tag {
				t.Errorf("Tags[%d] = %q, want %q", i, insertedMem.Tags[i], tag)
			}
		}
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result["source_count"].(float64) != 2 {
		t.Errorf("source_count = %v, want 2", result["source_count"])
	}
}

func TestCmdCompactApplyMissingSources(t *testing.T) {
	restore := setOpenFunc(func(_ string) (storeI, error) {
		return &mockStore{}, nil
	})
	defer restore()

	var buf bytes.Buffer
	err := cmdCompact([]string{"--apply", "merged content"}, &buf)
	if err == nil {
		t.Fatal("expected error for missing --sources")
	}
	if !strings.Contains(err.Error(), "--sources is required") {
		t.Errorf("error = %q, want '--sources is required'", err)
	}
}

func TestCmdCompactApplyEmptySources(t *testing.T) {
	restore := setOpenFunc(func(_ string) (storeI, error) {
		return &mockStore{}, nil
	})
	defer restore()

	var buf bytes.Buffer
	err := cmdCompact([]string{"--apply", "merged", "--sources", " , , "}, &buf)
	if err == nil {
		t.Fatal("expected error for empty sources")
	}
	if !strings.Contains(err.Error(), "--sources must contain at least one ID") {
		t.Errorf("error = %q", err)
	}
}

func TestCmdCompactApplySourceNotFound(t *testing.T) {
	restore := setOpenFunc(func(_ string) (storeI, error) {
		return &mockStore{
			getFn: func(_ string) (*model.Memory, error) {
				return nil, store.ErrNotFound
			},
		}, nil
	})
	defer restore()

	var buf bytes.Buffer
	err := cmdCompact([]string{"--apply", "merged", "--sources", "missing-id"}, &buf)
	if err == nil {
		t.Fatal("expected error for missing source")
	}
	if !strings.Contains(err.Error(), "compact source") {
		t.Errorf("error = %q, want 'compact source'", err)
	}
}

func TestCmdCompactApplyDeleteError(t *testing.T) {
	restore := setOpenFunc(func(_ string) (storeI, error) {
		return &mockStore{
			getFn: func(_ string) (*model.Memory, error) {
				return &model.Memory{ID: "x", Tags: []string{}}, nil
			},
			deleteFn: func(_ string) error {
				return fmt.Errorf("delete failed")
			},
		}, nil
	})
	defer restore()

	var buf bytes.Buffer
	err := cmdCompact([]string{"--apply", "merged", "--sources", "x"}, &buf)
	if err == nil {
		t.Fatal("expected delete error")
	}
	if !strings.Contains(err.Error(), "compact delete") {
		t.Errorf("error = %q, want 'compact delete'", err)
	}
}

func TestCmdCompactApplyInsertError(t *testing.T) {
	restore := setOpenFunc(func(_ string) (storeI, error) {
		return &mockStore{
			getFn: func(_ string) (*model.Memory, error) {
				return &model.Memory{ID: "x", Tags: []string{}}, nil
			},
			insertFn: func(_ *model.Memory) error {
				return fmt.Errorf("insert failed")
			},
		}, nil
	})
	defer restore()

	var buf bytes.Buffer
	err := cmdCompact([]string{"--apply", "merged", "--sources", "x"}, &buf)
	if err == nil {
		t.Fatal("expected insert error")
	}
	if !strings.Contains(err.Error(), "compact insert") {
		t.Errorf("error = %q, want 'compact insert'", err)
	}
}

func TestCmdCompactInvalidDB(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := run([]string{"compact", "--db", "/nonexistent/path/test.db"}, &buf)
	if err == nil {
		t.Fatal("expected error for invalid db path")
	}
	if !strings.Contains(err.Error(), "open database") {
		t.Errorf("error = %q, want 'open database'", err)
	}
}

