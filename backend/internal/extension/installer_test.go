package extension

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func buildTestZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestStageLocalZipSucceeds(t *testing.T) {
	dir := t.TempDir()
	p := NewPaths(dir)
	manifest, _ := json.Marshal(map[string]any{
		"manifest_version": 3,
		"name":             "Hello",
		"version":          "0.1.0",
		"description":      "Hi",
		"author":           "Nobody",
	})
	pkg := buildTestZip(t, map[string]string{"manifest.json": string(manifest)})

	in := NewInstaller(p)
	preview, err := in.StageLocal(pkg, "pkg.zip")
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	if preview.Name != "Hello" || preview.Version != "0.1.0" {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	stagingDir := p.StagingDir(preview.StagingToken)
	if _, err := os.Stat(filepath.Join(stagingDir, "unpacked", "manifest.json")); err != nil {
		t.Fatalf("unpacked manifest missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stagingDir, "source.zip")); err != nil {
		t.Fatalf("source.zip missing: %v", err)
	}
}

func TestStageLocalRejectsInvalid(t *testing.T) {
	dir := t.TempDir()
	in := NewInstaller(NewPaths(dir))
	if _, err := in.StageLocal([]byte(""), ""); err == nil {
		t.Fatal("expected error on empty raw")
	}
	if _, err := in.StageLocal([]byte("not a zip"), "x.zip"); err == nil {
		t.Fatal("expected error on corrupt zip")
	}
}

func TestCancelPreviewRemovesStaging(t *testing.T) {
	dir := t.TempDir()
	p := NewPaths(dir)
	in := NewInstaller(p)

	manifest, _ := json.Marshal(map[string]any{"manifest_version": 3, "name": "X", "version": "1"})
	pkg := buildTestZip(t, map[string]string{"manifest.json": string(manifest)})
	preview, err := in.StageLocal(pkg, "x.zip")
	if err != nil {
		t.Fatal(err)
	}
	if err := in.CancelPreview(preview.StagingToken); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p.StagingDir(preview.StagingToken)); !os.IsNotExist(err) {
		t.Fatalf("staging should be removed, got err=%v", err)
	}
}

func TestRecoverOnBootCleansStagingAndOldDirs(t *testing.T) {
	dir := t.TempDir()
	p := NewPaths(dir)
	in := NewInstaller(p)

	// Pre-state:
	//   ext-keep/ (normal)            → keep
	//   ext-recovered.old (no primary) → rename to ext-recovered
	//   ext-trim/ + ext-trim.old       → drop .old
	_ = os.MkdirAll(p.ExtensionDir("ext-keep"), 0o755)
	_ = os.MkdirAll(p.ExtensionDir("ext-recovered")+".old", 0o755)
	_ = os.MkdirAll(p.ExtensionDir("ext-trim"), 0o755)
	_ = os.MkdirAll(p.ExtensionDir("ext-trim")+".old", 0o755)
	_ = os.MkdirAll(filepath.Join(p.StagingRoot, "abc"), 0o755)

	if err := in.RecoverOnBoot(); err != nil {
		t.Fatalf("recover: %v", err)
	}

	if _, err := os.Stat(p.ExtensionDir("ext-keep")); err != nil {
		t.Fatalf("ext-keep missing: %v", err)
	}
	if _, err := os.Stat(p.ExtensionDir("ext-recovered")); err != nil {
		t.Fatalf("ext-recovered should have been restored from .old: %v", err)
	}
	if _, err := os.Stat(p.ExtensionDir("ext-recovered") + ".old"); !os.IsNotExist(err) {
		t.Fatalf("ext-recovered.old should be gone: err=%v", err)
	}
	if _, err := os.Stat(p.ExtensionDir("ext-trim") + ".old"); !os.IsNotExist(err) {
		t.Fatalf("ext-trim.old should have been removed: err=%v", err)
	}
	if entries, _ := os.ReadDir(p.StagingRoot); len(entries) != 0 {
		t.Fatalf("staging should be empty after RecoverOnBoot, got %d entries", len(entries))
	}
}

func TestCommitInsertHappyPath(t *testing.T) {
	dir := t.TempDir()
	p := NewPaths(dir)
	in := NewInstaller(p)
	store := newTestStore(t)

	manifest, _ := json.Marshal(map[string]any{"manifest_version": 3, "name": "N", "version": "1"})
	pkg := buildTestZip(t, map[string]string{"manifest.json": string(manifest)})
	preview, err := in.StageLocal(pkg, "x.zip")
	if err != nil {
		t.Fatal(err)
	}

	ext, err := in.CommitInsert(store, preview.StagingToken, Scope{Kind: ScopeKindInstances, IDs: []string{"p1"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if ext.ExtensionID == "" || ext.Name != "N" {
		t.Fatalf("unexpected: %+v", ext)
	}
	if _, err := os.Stat(p.ExtensionDir(ext.ExtensionID)); err != nil {
		t.Fatalf("extension dir missing: %v", err)
	}
	got, _ := store.GetByID(ext.ExtensionID)
	if got == nil || got.Name != "N" {
		t.Fatalf("db row missing: %+v", got)
	}
}

func TestCommitInsertOverrideName(t *testing.T) {
	dir := t.TempDir()
	p := NewPaths(dir)
	in := NewInstaller(p)
	store := newTestStore(t)

	manifest, _ := json.Marshal(map[string]any{"manifest_version": 3, "name": "original", "version": "1"})
	pkg := buildTestZip(t, map[string]string{"manifest.json": string(manifest)})
	preview, _ := in.StageLocal(pkg, "x.zip")

	ext, err := in.CommitInsert(store, preview.StagingToken, Scope{Kind: ScopeKindInstances}, "  custom  ")
	if err != nil {
		t.Fatal(err)
	}
	if ext.Name != "custom" {
		t.Fatalf("override should trim and win: %q", ext.Name)
	}
}
