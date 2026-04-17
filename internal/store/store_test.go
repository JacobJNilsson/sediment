package store_test

import (
	"os"
	"path/filepath"
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

func TestOpenInvalidPath(t *testing.T) {
	t.Parallel()
	_, err := store.Open("/nonexistent/dir/db.sqlite")
	if err == nil {
		t.Fatal("expected error for invalid path")
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
	if got.Content != m.Content {
		t.Errorf("content = %q, want %q", got.Content, m.Content)
	}
	if got.Confidence != m.Confidence {
		t.Errorf("confidence = %v, want %v", got.Confidence, m.Confidence)
	}
	if got.State != m.State {
		t.Errorf("state = %v, want %v", got.State, m.State)
	}
	if got.Source != m.Source {
		t.Errorf("source = %q, want %q", got.Source, m.Source)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "test" {
		t.Errorf("tags = %v, want [test]", got.Tags)
	}
}

func TestGetNotFound(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	_, err := db.Get("nonexistent")
	if err != store.ErrNotFound {
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
	m.UpdatedAt = time.Now()
	if err := db.Update(m); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := db.Get("mem-2")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Content != "updated content" {
		t.Errorf("content = %q, want %q", got.Content, "updated content")
	}
	if got.Confidence != 0.5 {
		t.Errorf("confidence = %v, want 0.5", got.Confidence)
	}
	if got.State != model.StateDormant {
		t.Errorf("state = %v, want dormant", got.State)
	}
}

func TestUpdateNotFound(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	m := testMemory("nonexistent", "content")
	err := db.Update(m)
	if err != store.ErrNotFound {
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
	if err != store.ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDeleteNotFound(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	err := db.Delete("nonexistent")
	if err != store.ErrNotFound {
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
	// Should be ordered by confidence DESC.
	if actives[0].ID != "mem-c" {
		t.Errorf("first active = %s, want mem-c", actives[0].ID)
	}

	dormants, err := db.ListByState(model.StateDormant)
	if err != nil {
		t.Fatalf("list dormant: %v", err)
	}
	if len(dormants) != 1 {
		t.Fatalf("got %d dormant memories, want 1", len(dormants))
	}
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
	if all[0].ID != "mem-y" {
		t.Errorf("first = %s, want mem-y (highest confidence)", all[0].ID)
	}
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
	if len(got.Tags) != 0 {
		t.Errorf("tags = %v, want empty", got.Tags)
	}
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
	if len(got.Tags) != 0 {
		t.Errorf("tags = %v, want empty", got.Tags)
	}
}

func TestOpenPermissionDenied(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Make directory read-only so SQLite can't create files.
	if err := os.Chmod(dir, 0o444); err != nil {
		t.Skipf("cannot change permissions: %v", err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	_, err := store.Open(filepath.Join(dir, "nope.db"))
	if err == nil {
		t.Fatal("expected error for permission-denied path")
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
