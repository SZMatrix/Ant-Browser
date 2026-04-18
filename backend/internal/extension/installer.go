package extension

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var ErrInvalidPackage = errors.New("invalid extension package")

// Installer bundles the stage-and-commit machinery. It does NOT hold the
// SQLite store; callers pass the store into Commit* explicitly to make
// the data flow obvious in tests.
type Installer struct {
	paths Paths
}

func NewInstaller(p Paths) *Installer { return &Installer{paths: p} }

// CreatePlaceholder writes a DB row for a not-yet-installed extension. It is
// the synchronous half of the add-flow: returns a fully-formed Extension that
// the API layer wraps into a View and hands back to the frontend. The install
// worker is expected to be kicked off AFTER this returns.
func (in *Installer) CreatePlaceholder(store *SQLiteStore, meta Metadata, scope Scope, overrideName string) (*Extension, error) {
	name := strings.TrimSpace(overrideName)
	if name == "" {
		name = meta.Name
	}
	if name == "" {
		name = "未命名扩展"
	}
	extID := newExtensionID()
	ext := &Extension{
		ExtensionID:   extID,
		ChromeID:      meta.ChromeID,
		Name:          name,
		Provider:      meta.Provider,
		Description:   meta.Description,
		Version:       meta.Version,
		IconPath:      "", // filled in by worker if applicable
		UnpackedPath:  "",
		SourceType:    meta.SourceType,
		StoreVendor:   meta.StoreVendor,
		SourceURL:     meta.SourceURL,
		Enabled:       true,
		Scope:         scope,
		InstallStatus: InstallStatusInstalling,
		InstallError:  "",
	}
	if err := store.Insert(ext); err != nil {
		return nil, fmt.Errorf("insert placeholder: %w", err)
	}
	return ext, nil
}

// RecoverOnBoot cleans up leftover *.old backup directories from the pre-async
// overwrite flow (safe to keep: existing installs may still have them on disk)
// and purges the _staging root which the new worker treats as scratch-only.
func (in *Installer) RecoverOnBoot() error {
	if err := os.MkdirAll(in.paths.ExtRoot, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(in.paths.ExtRoot)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), ".old") {
			_ = os.RemoveAll(filepath.Join(in.paths.ExtRoot, e.Name()))
		}
	}
	_ = os.RemoveAll(in.paths.StagingRoot)
	return os.MkdirAll(in.paths.StagingRoot, 0o755)
}

// helpers ---------------------------------------------------------------------

func newNonce() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func newExtensionID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:]) // 12-char hex
}

func dirIsEmpty(dir string) bool {
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) == 0
}

func extractZip(data []byte, target string) error {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	for _, zf := range r.File {
		cleanName := path.Clean(zf.Name)
		if strings.HasPrefix(cleanName, "..") || strings.Contains(cleanName, "..\\") || strings.HasPrefix(cleanName, "/") {
			return fmt.Errorf("zip entry escapes target: %q", zf.Name)
		}
		dst := filepath.Join(target, filepath.FromSlash(cleanName))
		if zf.FileInfo().IsDir() {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return err
		}
		out.Close()
		rc.Close()
	}
	return nil
}

func exportIcon(unpackedDir, iconRel, dst string) (dataURL string, hasIcon bool) {
	if iconRel == "" {
		return "", false
	}
	// Guard against malicious manifests pointing icons outside the extension
	// directory (e.g. "../../etc/passwd"). path.Clean normalizes slash-based
	// traversals; the explicit checks catch absolute and backslash-based ones.
	cleaned := path.Clean(iconRel)
	if strings.HasPrefix(cleaned, "..") || strings.HasPrefix(cleaned, "/") || strings.Contains(cleaned, "..\\") {
		return "", false
	}
	src := filepath.Join(unpackedDir, filepath.FromSlash(cleaned))
	b, err := os.ReadFile(src)
	if err != nil {
		return "", false
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		return "", false
	}
	mime := "image/png"
	lower := strings.ToLower(iconRel)
	if strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg") {
		mime = "image/jpeg"
	} else if strings.HasSuffix(lower, ".svg") {
		mime = "image/svg+xml"
	}
	return "data:" + mime + ";base64," + stdEncodeBase64(b), true
}
