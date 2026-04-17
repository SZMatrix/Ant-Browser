package extension

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCommitOverwriteRejectsUnknownTarget(t *testing.T) {
	dir := t.TempDir()
	p := NewPaths(dir)
	in := NewInstaller(p)
	store := newTestStore(t)

	manifest, _ := json.Marshal(map[string]any{"manifest_version": 3, "name": "N", "version": "1"})
	pkg := buildTestZip(t, map[string]string{"manifest.json": string(manifest)})
	preview, _ := in.StageLocal(pkg, "x.zip")

	_, err := in.CommitOverwrite(store, preview.StagingToken, "missing-id", Scope{Kind: ScopeKindInstances}, "")
	if !errors.Is(err, ErrDuplicateTargetNotFound) {
		t.Fatalf("want ErrDuplicateTargetNotFound, got %v", err)
	}
}

func TestCommitOverwriteRejectsChromeIDMismatch(t *testing.T) {
	dir := t.TempDir()
	p := NewPaths(dir)
	in := NewInstaller(p)
	store := newTestStore(t)
	_ = os.MkdirAll(p.ExtRoot, 0o755)

	existingDir := p.ExtensionDir("target-ext")
	_ = os.MkdirAll(filepath.Join(existingDir, "unpacked"), 0o755)
	_ = store.Insert(&Extension{ExtensionID: "target-ext", ChromeID: "aaaa", Name: "old", SourceType: SourceTypeLocalZIP, Enabled: true})

	manifest, _ := json.Marshal(map[string]any{"manifest_version": 3, "name": "N", "version": "2"})
	pkg := buildTestZip(t, map[string]string{"manifest.json": string(manifest)})
	preview, _ := in.StageLocal(pkg, "x.zip")

	_, err := in.CommitOverwrite(store, preview.StagingToken, "target-ext", Scope{Kind: ScopeKindInstances}, "")
	if !errors.Is(err, ErrDuplicateChromeIDMismatch) {
		t.Fatalf("want ErrDuplicateChromeIDMismatch, got %v", err)
	}
	if _, err := os.Stat(existingDir); err != nil {
		t.Fatalf("existing dir removed: %v", err)
	}
}

func TestCommitOverwriteSwapsDirectories(t *testing.T) {
	dir := t.TempDir()
	p := NewPaths(dir)
	in := NewInstaller(p)
	store := newTestStore(t)
	_ = os.MkdirAll(p.ExtRoot, 0o755)

	existingDir := p.ExtensionDir("ext-abc")
	_ = os.MkdirAll(filepath.Join(existingDir, "unpacked"), 0o755)
	_ = os.WriteFile(filepath.Join(existingDir, "source.zip"), []byte("old"), 0o644)
	_ = store.Insert(&Extension{ExtensionID: "ext-abc", ChromeID: "same", Name: "old", Version: "1", SourceType: SourceTypeLocalZIP, Enabled: true})

	manifest, _ := json.Marshal(map[string]any{"manifest_version": 3, "name": "new-name", "version": "2"})
	pkg := buildTestZip(t, map[string]string{"manifest.json": string(manifest)})
	preview, _ := in.StageLocal(pkg, "x.zip")
	// Inject the matching chrome_id into the staged meta so overwrite is allowed.
	meta, _ := readStagedMeta(p.StagingDir(preview.StagingToken))
	meta.ChromeID = "same"
	_ = writeStagedMeta(p.StagingDir(preview.StagingToken), meta)

	ext, err := in.CommitOverwrite(store, preview.StagingToken, "ext-abc", Scope{Kind: ScopeKindInstances}, "")
	if err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	if ext.Name != "new-name" || ext.Version != "2" {
		t.Fatalf("unexpected: %+v", ext)
	}
	if _, err := os.Stat(existingDir + ".old"); !os.IsNotExist(err) {
		t.Fatalf(".old should be gone: %v", err)
	}
	if _, err := os.Stat(p.StagingDir(preview.StagingToken)); !os.IsNotExist(err) {
		t.Fatalf("staging dir should be consumed: %v", err)
	}
}
