package extension

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildViewIncludesIconDataURL(t *testing.T) {
	dir := t.TempDir()
	p := NewPaths(dir)
	_ = os.MkdirAll(p.ExtensionDir("ext-1"), 0o755)
	// Full PNG signature so http.DetectContentType recognizes it as image/png.
	iconBytes := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	if err := os.WriteFile(filepath.Join(p.ExtensionDir("ext-1"), "icon.png"), iconBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	m := NewManager(nil, nil, NewPendingRestartTracker(), p)
	ext := &Extension{ExtensionID: "ext-1", Name: "N", IconPath: "icon.png", Enabled: true}
	view := m.BuildView(ext, nil, nil)
	want := "data:image/png;base64," + base64.StdEncoding.EncodeToString(iconBytes)
	if view.IconDataURL != want {
		t.Fatalf("iconDataURL mismatch:\n got=%q\nwant=%q", view.IconDataURL, want)
	}
}

func TestBuildViewEmptyIconWhenMissing(t *testing.T) {
	dir := t.TempDir()
	p := NewPaths(dir)
	m := NewManager(nil, nil, NewPendingRestartTracker(), p)
	ext := &Extension{ExtensionID: "ext-2", Name: "N", IconPath: "icon.png", Enabled: true}
	view := m.BuildView(ext, nil, nil)
	if view.IconDataURL != "" {
		t.Fatalf("expected empty dataURL for missing icon, got %q", view.IconDataURL)
	}
}

func TestStaleIDsDetectsMissingGroupsAndProfiles(t *testing.T) {
	validGroups := map[string]struct{}{"gA": {}, "gB": {}}
	validProfiles := map[string]struct{}{"p1": {}}

	got := staleIDs(Scope{Kind: ScopeKindGroups, IDs: []string{"gA", "gONE"}}, validGroups, validProfiles)
	if len(got) != 1 || got[0] != "gONE" {
		t.Fatalf("groups: %+v", got)
	}

	got = staleIDs(Scope{Kind: ScopeKindInstances, IDs: []string{"p1", "pDEAD"}}, validGroups, validProfiles)
	if len(got) != 1 || got[0] != "pDEAD" {
		t.Fatalf("instances: %+v", got)
	}

	if staleIDs(Scope{Kind: "unknown"}, validGroups, validProfiles) != nil {
		t.Fatal("unknown kind must return nil")
	}
}

func TestBuildViewPopulatesPendingRestarts(t *testing.T) {
	p := NewPaths(t.TempDir())
	tracker := NewPendingRestartTracker()
	tracker.Mark("ext-1", "p1")
	tracker.Mark("ext-1", "p2")
	m := NewManager(nil, nil, tracker, p)
	view := m.BuildView(&Extension{ExtensionID: "ext-1"}, nil, nil)
	if len(view.PendingRestartProfileIDs) != 2 {
		t.Fatalf("pending: %+v", view.PendingRestartProfileIDs)
	}
}
