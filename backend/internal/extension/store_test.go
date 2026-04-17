package extension

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"ant-chrome/backend/internal/database"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.NewDB(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := NewSQLiteStore(db.GetConn())
	t.Cleanup(func() {
		db.Close()
	})
	return store
}

func TestStoreInsertAndGet(t *testing.T) {
	s := newTestStore(t)
	ext := &Extension{
		ExtensionID: "ext-1",
		ChromeID:    "abc",
		Name:        "My Ext",
		SourceType:  SourceTypeLocalZIP,
		Enabled:     true,
		Scope:       Scope{Kind: ScopeKindInstances, IDs: []string{"p1"}},
	}
	if err := s.Insert(ext); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := s.GetByID("ext-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "My Ext" || got.ChromeID != "abc" {
		t.Fatalf("unexpected %+v", got)
	}
	if got.Scope.Kind != ScopeKindInstances || len(got.Scope.IDs) != 1 || got.Scope.IDs[0] != "p1" {
		t.Fatalf("scope mismatch %+v", got.Scope)
	}
}

func TestStoreFindByChromeID(t *testing.T) {
	s := newTestStore(t)
	_ = s.Insert(&Extension{ExtensionID: "e1", ChromeID: "ccc", Name: "a", SourceType: SourceTypeStore, Enabled: true})
	id, err := s.FindByChromeID("ccc")
	if err != nil {
		t.Fatal(err)
	}
	if id != "e1" {
		t.Fatalf("got %q", id)
	}
	id, err = s.FindByChromeID("missing")
	if err != nil || id != "" {
		t.Fatalf("expected empty, got %q err=%v", id, err)
	}
}

func TestStoreSetEnabledUpdateScopeDelete(t *testing.T) {
	s := newTestStore(t)
	_ = s.Insert(&Extension{ExtensionID: "e1", Name: "a", SourceType: SourceTypeLocalZIP, Enabled: true})

	if err := s.SetEnabled("e1", false); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetByID("e1")
	if got.Enabled {
		t.Fatal("should be disabled")
	}

	if err := s.UpdateScope("e1", Scope{Kind: ScopeKindGroups, IDs: []string{"g1"}}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetByID("e1")
	if got.Scope.Kind != ScopeKindGroups || got.Scope.IDs[0] != "g1" {
		t.Fatalf("%+v", got.Scope)
	}

	if err := s.Rename("e1", "b"); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetByID("e1")
	if got.Name != "b" {
		t.Fatalf("rename failed: %q", got.Name)
	}

	if err := s.Delete("e1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetByID("e1"); err != sql.ErrNoRows {
		t.Fatalf("expected ErrNoRows, got %v", err)
	}
}

func TestStoreList(t *testing.T) {
	s := newTestStore(t)
	_ = s.Insert(&Extension{ExtensionID: "e1", Name: "a", SourceType: SourceTypeLocalZIP, Enabled: true})
	_ = s.Insert(&Extension{ExtensionID: "e2", Name: "b", SourceType: SourceTypeLocalZIP, Enabled: false})
	xs, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(xs) != 2 {
		t.Fatalf("expected 2, got %d", len(xs))
	}
}
