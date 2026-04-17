package store_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jacobjnilsson/sediment/internal/model"
	"github.com/jacobjnilsson/sediment/internal/store"
)

func newTestDB(t *testing.T) *store.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func testMemory(id, content string) *model.Memory {
	now := time.Now().Truncate(time.Millisecond)
	return &model.Memory{
		ID:             id,
		Content:        content,
		Confidence:     0.9,
		State:          model.StateActive,
		AccessCount:    0,
		CreatedAt:      now,
		UpdatedAt:      now,
		LastAccessedAt: now,
		Tags:           []string{"test"},
		Source:         "unit-test",
	}
}

// memoryEqual compares two memories by serialising to JSON.
// Time fields are compared at second precision to tolerate DB round-trips.
func memoryEqual(t *testing.T, got, want *model.Memory) {
	t.Helper()
	g, _ := json.Marshal(got)
	w, _ := json.Marshal(want)
	if string(g) != string(w) {
		t.Errorf("memory mismatch\n got: %s\nwant: %s", g, w)
	}
}

func TestOpenInvalidPath(t *testing.T) {
	t.Parallel()
	_, err := store.Open("/nonexistent/dir/db.sqlite")
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
	if !strings.Contains(err.Error(), "connect to database") {
		t.Errorf("error = %q, want it to mention 'connect to database'", err)
	}
}

func TestMigrate(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	// Running migrate twice should be idempotent.
	if err := db.Migrate(); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestInsertAndGet(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	m := testMemory("mem-1", "Go is great for CLI tools")
	if err := db.Insert(m); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := db.Get("mem-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	memoryEqual(t, got, m)
}

func TestGetNotFound(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	_, err := db.Get("nonexistent")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestUpdate(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	m := testMemory("mem-2", "original content")
	if err := db.Insert(m); err != nil {
		t.Fatalf("insert: %v", err)
	}

	m.Content = "updated content"
	m.Confidence = 0.5
	m.State = model.StateDormant
	m.AccessCount = 3
	m.UpdatedAt = time.Now().Truncate(time.Millisecond)
	if err := db.Update(m); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := db.Get("mem-2")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	memoryEqual(t, got, m)
}

func TestUpdateNotFound(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	m := testMemory("nonexistent", "content")
	err := db.Update(m)
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestDelete(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	m := testMemory("mem-3", "to be deleted")
	if err := db.Insert(m); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := db.Delete("mem-3"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err := db.Get("mem-3")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDeleteNotFound(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	err := db.Delete("nonexistent")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestListByState(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	m1 := testMemory("mem-a", "active memory")
	m1.Confidence = 0.8
	m2 := testMemory("mem-b", "dormant memory")
	m2.State = model.StateDormant
	m2.Confidence = 0.3
	m3 := testMemory("mem-c", "another active")
	m3.Confidence = 0.95

	for _, m := range []*model.Memory{m1, m2, m3} {
		if err := db.Insert(m); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	actives, err := db.ListByState(model.StateActive)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(actives) != 2 {
		t.Fatalf("got %d active memories, want 2", len(actives))
	}
	// Ordered by confidence DESC: mem-c (0.95) then mem-a (0.8).
	memoryEqual(t, actives[0], m3)
	memoryEqual(t, actives[1], m1)

	dormants, err := db.ListByState(model.StateDormant)
	if err != nil {
		t.Fatalf("list dormant: %v", err)
	}
	if len(dormants) != 1 {
		t.Fatalf("got %d dormant memories, want 1", len(dormants))
	}
	memoryEqual(t, dormants[0], m2)
}

func TestListAll(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	m1 := testMemory("mem-x", "first")
	m1.Confidence = 0.5
	m2 := testMemory("mem-y", "second")
	m2.Confidence = 0.9
	for _, m := range []*model.Memory{m1, m2} {
		if err := db.Insert(m); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	all, err := db.ListAll()
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("got %d, want 2", len(all))
	}
	// Ordered by confidence DESC: mem-y (0.9) then mem-x (0.5).
	memoryEqual(t, all[0], m2)
	memoryEqual(t, all[1], m1)
}

func TestListByStateEmpty(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	list, err := db.ListByState(model.StateArchived)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if list != nil {
		t.Errorf("expected nil for empty list, got %v", list)
	}
}

func TestListAllEmpty(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	list, err := db.ListAll()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if list != nil {
		t.Errorf("expected nil for empty list, got %v", list)
	}
}

func TestClose(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "close.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestInsertEmptyTags(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	m := testMemory("mem-empty-tags", "no tags")
	m.Tags = []string{}
	if err := db.Insert(m); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := db.Get("mem-empty-tags")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	memoryEqual(t, got, m)
}

func TestInsertNilTags(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	m := testMemory("mem-nil-tags", "nil tags")
	m.Tags = nil
	if err := db.Insert(m); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := db.Get("mem-nil-tags")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	// nil tags are stored as "[]" and round-trip back as empty slice.
	expected := *m
	expected.Tags = []string{}
	memoryEqual(t, got, &expected)
}

func TestOpenPermissionDenied(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o444); err != nil {
		t.Skipf("cannot change permissions: %v", err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	_, err := store.Open(filepath.Join(dir, "nope.db"))
	if err == nil {
		t.Fatal("expected error for permission-denied path")
	}
	if !strings.Contains(err.Error(), "connect to database") {
		t.Errorf("error = %q, want it to mention 'connect to database'", err)
	}
}

func TestListByStateOnClosedDB(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "closed.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db.Close()

	_, err = db.ListByState(model.StateActive)
	if err == nil {
		t.Fatal("expected error on closed db")
	}
	if !strings.Contains(err.Error(), "list by state") {
		t.Errorf("error = %q, want it to mention 'list by state'", err)
	}
}

func TestListAllOnClosedDB(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "closed.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db.Close()

	_, err = db.ListAll()
	if err == nil {
		t.Fatal("expected error on closed db")
	}
	if !strings.Contains(err.Error(), "list all") {
		t.Errorf("error = %q, want it to mention 'list all'", err)
	}
}

func TestInsertOnClosedDB(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "closed.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db.Close()

	m := testMemory("fail", "content")
	err = db.Insert(m)
	if err == nil {
		t.Fatal("expected error on closed db")
	}
	if !strings.Contains(err.Error(), "insert memory") {
		t.Errorf("error = %q, want it to mention 'insert memory'", err)
	}
}

func TestUpdateOnClosedDB(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "closed.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db.Close()

	m := testMemory("fail", "content")
	err = db.Update(m)
	if err == nil {
		t.Fatal("expected error on closed db")
	}
	if !strings.Contains(err.Error(), "update memory") {
		t.Errorf("error = %q, want it to mention 'update memory'", err)
	}
}

func TestDeleteOnClosedDB(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "closed.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db.Close()

	err = db.Delete("fail")
	if err == nil {
		t.Fatal("expected error on closed db")
	}
	if !strings.Contains(err.Error(), "delete memory") {
		t.Errorf("error = %q, want it to mention 'delete memory'", err)
	}
}

func TestGetOnClosedDB(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "closed.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db.Close()

	_, err = db.Get("fail")
	if err == nil {
		t.Fatal("expected error on closed db")
	}
	if !strings.Contains(err.Error(), "scan memory") {
		t.Errorf("error = %q, want it to mention 'scan memory'", err)
	}
}

func TestMigrateOnClosedDB(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "closed.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.Close()

	err = db.Migrate()
	if err == nil {
		t.Fatal("expected error on closed db")
	}
	if !strings.Contains(err.Error(), "migrate") {
		t.Errorf("error = %q, want it to mention 'migrate'", err)
	}
}

func TestMarshalTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tags []string
		want string
	}{
		{"nil tags", nil, "[]"},
		{"empty tags", []string{}, "[]"},
		{"one tag", []string{"foo"}, `["foo"]`},
		{"multiple tags", []string{"a", "b"}, `["a","b"]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := store.MarshalTags(tt.tags)
			if got != tt.want {
				t.Errorf("MarshalTags(%v) = %q, want %q", tt.tags, got, tt.want)
			}
		})
	}
}
