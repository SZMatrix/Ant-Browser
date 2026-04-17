package backend

import (
	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/config"
	"ant-chrome/backend/internal/database"
	"ant-chrome/backend/internal/extension"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ensureDir creates p (and parents) under t.TempDir scope.
func ensureDir(t *testing.T, p string) string {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// makeInMemoryStoreForTest spins up a temp sqlite DB with migrations applied
// and returns an extension SQLiteStore wired to it. The DB file lives under
// t.TempDir and is auto-cleaned.
func makeInMemoryStoreForTest(t *testing.T) *extension.SQLiteStore {
	t.Helper()
	dbFile := filepath.Join(t.TempDir(), "ext-int.db")
	db, err := database.NewDB(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return extension.NewSQLiteStore(db.GetConn())
}

// TestResolveExtensionsForProfileAppendsLoadExtension verifies that when an
// enabled extension's scope matches a profile, its unpacked path is surfaced
// via resolveExtensionsForProfile and that buildLoadExtensionArg produces the
// correct comma-joined flag. This covers the Phase 1 integration contract
// without launching a real browser.
func TestResolveExtensionsForProfileAppendsLoadExtension(t *testing.T) {
	dataDir := t.TempDir()
	paths := extension.NewPaths(dataDir)
	ensureDir(t, paths.ExtRoot)
	ensureDir(t, filepath.Join(paths.ExtensionDir("ext-1"), "unpacked"))

	cfg := &config.Config{}
	app := &App{}
	app.browserMgr = browser.NewManager(cfg, dataDir)
	store := makeInMemoryStoreForTest(t)
	if err := store.Insert(&extension.Extension{
		ExtensionID: "ext-1",
		Name:        "x",
		SourceType:  extension.SourceTypeLocalZIP,
		Enabled:     true,
		Scope:       extension.Scope{Kind: extension.ScopeKindInstances, IDs: []string{"p-ok"}},
	}); err != nil {
		t.Fatal(err)
	}
	app.extMgr = extension.NewManager(store, extension.NewInstaller(paths), extension.NewPendingRestartTracker(), paths)

	profileOK := &browser.Profile{ProfileId: "p-ok"}
	profileNope := &browser.Profile{ProfileId: "p-other"}

	gotOK := app.resolveExtensionsForProfile(profileOK)
	if len(gotOK) != 1 || !strings.HasSuffix(gotOK[0], filepath.Join("ext-1", "unpacked")) {
		t.Fatalf("expected matching profile to get unpacked path, got %v", gotOK)
	}
	gotNope := app.resolveExtensionsForProfile(profileNope)
	if len(gotNope) != 0 {
		t.Fatalf("non-matching profile should get no paths, got %v", gotNope)
	}

	arg := buildLoadExtensionArg(gotOK)
	if !strings.HasPrefix(arg, "--load-extension=") || !strings.HasSuffix(arg, filepath.Join("ext-1", "unpacked")) {
		t.Fatalf("unexpected arg: %q", arg)
	}
}

func TestResolveExtensionsForProfileSkipsMissingDir(t *testing.T) {
	dataDir := t.TempDir()
	paths := extension.NewPaths(dataDir)
	// Do NOT create the unpacked dir — resolve should skip with a warn.
	ensureDir(t, paths.ExtRoot)

	cfg := &config.Config{}
	app := &App{}
	app.browserMgr = browser.NewManager(cfg, dataDir)
	store := makeInMemoryStoreForTest(t)
	_ = store.Insert(&extension.Extension{
		ExtensionID: "ext-missing",
		Name:        "x",
		SourceType:  extension.SourceTypeLocalZIP,
		Enabled:     true,
		Scope:       extension.Scope{Kind: extension.ScopeKindInstances, IDs: []string{"p"}},
	})
	app.extMgr = extension.NewManager(store, extension.NewInstaller(paths), extension.NewPendingRestartTracker(), paths)

	got := app.resolveExtensionsForProfile(&browser.Profile{ProfileId: "p"})
	if len(got) != 0 {
		t.Fatalf("expected empty slice for missing unpacked dir, got %v", got)
	}
}

// newTestAppWithExtensions spins up a minimal App wired with a real SQLite-backed
// browser manager + extension manager, suitable for delete-cascade tests.
// Caller is responsible for invoking the returned cleanup.
func newTestAppWithExtensions(t *testing.T) (*App, func()) {
	t.Helper()
	dataDir := t.TempDir()
	dbFile := filepath.Join(dataDir, "app.db")
	db, err := database.NewDB(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	mgr := browser.NewManager(cfg, dataDir)
	mgr.ProfileDAO = browser.NewSQLiteProfileDAO(db.GetConn())
	mgr.GroupDAO = browser.NewSQLiteGroupDAO(db.GetConn())
	mgr.InitData()

	paths := extension.NewPaths(dataDir)
	if err := os.MkdirAll(paths.ExtRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	app := &App{browserMgr: mgr}
	app.extMgr = extension.NewManager(
		extension.NewSQLiteStore(db.GetConn()),
		extension.NewInstaller(paths),
		extension.NewPendingRestartTracker(),
		paths,
	)
	return app, func() { _ = db.Close() }
}
