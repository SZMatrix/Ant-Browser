package extension

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var (
	ErrDuplicateTargetNotFound   = errors.New("duplicate target extension not found")
	ErrDuplicateChromeIDMismatch = errors.New("duplicate chrome_id mismatch")
	ErrInvalidPackage            = errors.New("invalid extension package")
)

// Installer bundles the stage-and-commit machinery. It does NOT hold the
// SQLite store; callers pass the store into Commit* explicitly to make
// the data flow obvious in tests.
type Installer struct {
	paths Paths
}

func NewInstaller(p Paths) *Installer { return &Installer{paths: p} }

// Preview is the in-memory equivalent of ExtensionPreview used before Wails serialization.
type Preview = ExtensionPreview

// StageLocal accepts a raw file blob (crx or zip) and stores it in the staging area.
func (in *Installer) StageLocal(raw []byte, filenameHint string) (*Preview, error) {
	return in.stage(raw, detectLocalSource(raw, filenameHint), "", "")
}

// StageStore accepts a raw CRX blob downloaded from a vendor store.
// knownChromeID is the id parsed from the store URL; it overrides whatever
// StripCRXHeader derives, because store-served CRXs can carry multiple
// sha256_with_rsa proofs (developer + Google publisher) and the parser would
// otherwise pick whichever happens to be first — sometimes the Google
// publisher key, which is shared across every store extension and would make
// every subsequent install look like a duplicate of the first.
func (in *Installer) StageStore(raw []byte, vendor StoreVendor, sourceURL, knownChromeID, storeName, storeProvider, storeDesc string) (*Preview, error) {
	p, err := in.stage(raw, SourceTypeStore, sourceURL, string(vendor))
	if err != nil {
		return nil, err
	}
	if knownChromeID != "" {
		p.ChromeID = knownChromeID
	}
	if storeName != "" {
		p.Name = storeName
	}
	if storeProvider != "" {
		p.Provider = storeProvider
	}
	if storeDesc != "" && p.Description == "" {
		p.Description = storeDesc
	}
	p.SourceURL = sourceURL
	p.StoreVendor = vendor
	stagingDir := in.paths.StagingDir(p.StagingToken)
	if err := writeStagedMeta(stagingDir, previewToStaged(p)); err != nil {
		return nil, err
	}
	return p, nil
}

func (in *Installer) stage(raw []byte, sourceType SourceType, sourceURL, vendor string) (*Preview, error) {
	if len(raw) == 0 {
		return nil, ErrInvalidPackage
	}
	kind := DetectKind(raw)
	zipBytes := raw
	chromeID := ""
	if kind == "crx" {
		var err error
		zipBytes, chromeID, err = StripCRXHeader(raw)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidPackage, err)
		}
	}

	if err := os.MkdirAll(in.paths.StagingRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create staging root: %w", err)
	}

	var token, stagingDir string
	for attempt := 0; attempt < 2; attempt++ {
		token = newNonce()
		stagingDir = in.paths.StagingDir(token)
		if err := os.MkdirAll(stagingDir, 0o755); err != nil {
			return nil, fmt.Errorf("create staging dir: %w", err)
		}
		if dirIsEmpty(stagingDir) {
			break
		}
		_ = os.RemoveAll(stagingDir)
		if attempt == 1 {
			return nil, errors.New("staging nonce collision")
		}
	}

	unpackedDir := filepath.Join(stagingDir, "unpacked")
	if err := extractZip(zipBytes, unpackedDir); err != nil {
		_ = os.RemoveAll(stagingDir)
		return nil, fmt.Errorf("%w: %v", ErrInvalidPackage, err)
	}

	manifestBytes, err := os.ReadFile(filepath.Join(unpackedDir, "manifest.json"))
	if err != nil {
		_ = os.RemoveAll(stagingDir)
		return nil, fmt.Errorf("%w: manifest.json missing", ErrInvalidPackage)
	}
	manifest, err := ParseManifest(manifestBytes)
	if err != nil {
		_ = os.RemoveAll(stagingDir)
		return nil, fmt.Errorf("%w: %v", ErrInvalidPackage, err)
	}
	resolved := ResolveManifestStrings(manifest, unpackedDir)

	rawName := "source.zip"
	if kind == "crx" {
		rawName = "source.crx"
	}
	if err := os.WriteFile(filepath.Join(stagingDir, rawName), raw, 0o644); err != nil {
		_ = os.RemoveAll(stagingDir)
		return nil, err
	}

	iconDataURL, hasIcon := exportIcon(unpackedDir, resolved.IconRelPath, filepath.Join(stagingDir, "icon.png"))

	name := strings.TrimSpace(resolved.Name)
	if name == "" {
		name = "未命名扩展"
	}
	if resolved.Provider == "" {
		resolved.Provider = "未知"
	}

	preview := &Preview{
		StagingToken: token,
		ChromeID:     chromeID,
		Name:         name,
		Provider:     resolved.Provider,
		Description:  resolved.Description,
		Version:      resolved.Version,
		SourceType:   sourceType,
		StoreVendor:  StoreVendor(vendor),
		SourceURL:    sourceURL,
		IconDataURL:  iconDataURL,
	}

	if err := writeStagedMeta(stagingDir, &StagedMeta{
		ChromeID:    chromeID,
		Name:        preview.Name,
		Provider:    preview.Provider,
		Description: preview.Description,
		Version:     preview.Version,
		SourceType:  sourceType,
		StoreVendor: StoreVendor(vendor),
		SourceURL:   sourceURL,
		HasIcon:     hasIcon,
	}); err != nil {
		_ = os.RemoveAll(stagingDir)
		return nil, err
	}
	return preview, nil
}

func (in *Installer) CancelPreview(token string) error {
	if token == "" {
		return nil
	}
	return os.RemoveAll(in.paths.StagingDir(token))
}

// RecoverOnBoot reconciles half-committed overwrites and purges staged blobs.
// Callers should invoke this before any other Installer method on startup.
func (in *Installer) RecoverOnBoot() error {
	if err := os.MkdirAll(in.paths.ExtRoot, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(in.paths.ExtRoot)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasSuffix(e.Name(), ".old") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".old")
		target := in.paths.ExtensionDir(id)
		backup := target + ".old"
		if _, err := os.Stat(target); err == nil {
			_ = os.RemoveAll(backup)
		} else {
			_ = os.Rename(backup, target)
		}
	}
	_ = os.RemoveAll(in.paths.StagingRoot)
	return os.MkdirAll(in.paths.StagingRoot, 0o755)
}

// helpers ---------------------------------------------------------------------

func detectLocalSource(raw []byte, filenameHint string) SourceType {
	if DetectKind(raw) == "crx" {
		return SourceTypeLocalCRX
	}
	_ = filenameHint
	return SourceTypeLocalZIP
}

func newNonce() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
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

func writeStagedMeta(stagingDir string, meta *StagedMeta) error {
	b, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stagingDir, "preview.json"), b, 0o644)
}

func readStagedMeta(stagingDir string) (*StagedMeta, error) {
	b, err := os.ReadFile(filepath.Join(stagingDir, "preview.json"))
	if err != nil {
		return nil, err
	}
	var m StagedMeta
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func previewToStaged(p *Preview) *StagedMeta {
	return &StagedMeta{
		ChromeID:    p.ChromeID,
		Name:        p.Name,
		Provider:    p.Provider,
		Description: p.Description,
		Version:     p.Version,
		SourceType:  p.SourceType,
		StoreVendor: p.StoreVendor,
		SourceURL:   p.SourceURL,
		HasIcon:     p.IconDataURL != "",
	}
}

// CommitInsert promotes a staged package into the extensions directory and inserts a DB row.
// Atomicity: a successful rename is durable on the same filesystem; if the DB
// insert fails afterwards we rename the directory back into staging and
// surface the error so the user can retry.
func (in *Installer) CommitInsert(store *SQLiteStore, token string, scope Scope, overrideName string) (*Extension, error) {
	meta, err := readStagedMeta(in.paths.StagingDir(token))
	if err != nil {
		return nil, fmt.Errorf("read staged meta: %w", err)
	}
	extID := newExtensionID()
	stagingDir := in.paths.StagingDir(token)
	target := in.paths.ExtensionDir(extID)
	if err := os.MkdirAll(in.paths.ExtRoot, 0o755); err != nil {
		return nil, err
	}
	if err := os.Rename(stagingDir, target); err != nil {
		return nil, fmt.Errorf("rename staging to extension: %w", err)
	}
	name := strings.TrimSpace(overrideName)
	if name == "" {
		name = meta.Name
	}
	ext := &Extension{
		ExtensionID:  extID,
		ChromeID:     meta.ChromeID,
		Name:         name,
		Provider:     meta.Provider,
		Description:  meta.Description,
		Version:      meta.Version,
		IconPath:     iconPathIfAny(meta),
		UnpackedPath: filepath.Join("extensions", extID, "unpacked"),
		SourceType:   meta.SourceType,
		StoreVendor:  meta.StoreVendor,
		SourceURL:    meta.SourceURL,
		Enabled:      true,
		Scope:        scope,
	}
	if err := store.Insert(ext); err != nil {
		// DB failed: move the directory back into staging so the user can retry.
		_ = os.Rename(target, stagingDir)
		return nil, fmt.Errorf("insert extension: %w", err)
	}
	return ext, nil
}

func newExtensionID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:]) // 12-char hex
}

func iconPathIfAny(meta *StagedMeta) string {
	if meta.HasIcon {
		return "icon.png"
	}
	return ""
}

// CommitOverwrite replaces an existing extension with the staged content via
// the 4-step backup+swap protocol (spec § 4.3). The server-side chrome_id
// check rejects bogus duplicateOf values from the frontend.
func (in *Installer) CommitOverwrite(store *SQLiteStore, token, targetID string, scope Scope, overrideName string) (*Extension, error) {
	stagingDir := in.paths.StagingDir(token)
	meta, err := readStagedMeta(stagingDir)
	if err != nil {
		return nil, fmt.Errorf("read staged meta: %w", err)
	}

	existing, err := store.GetByID(targetID)
	if err != nil || existing == nil {
		return nil, ErrDuplicateTargetNotFound
	}
	if meta.ChromeID == "" || existing.ChromeID == "" || meta.ChromeID != existing.ChromeID {
		return nil, ErrDuplicateChromeIDMismatch
	}

	target := in.paths.ExtensionDir(targetID)
	backup := target + ".old"

	// Step 1: old → backup
	if err := os.Rename(target, backup); err != nil {
		return nil, fmt.Errorf("rename old→backup: %w", err)
	}
	// Step 2: staging → target
	if err := os.Rename(stagingDir, target); err != nil {
		_ = os.Rename(backup, target)
		return nil, fmt.Errorf("rename staging→target: %w", err)
	}
	// Step 3: DB update
	name := strings.TrimSpace(overrideName)
	if name == "" {
		name = meta.Name
	}
	updated := &Extension{
		ExtensionID:  targetID,
		ChromeID:     meta.ChromeID,
		Name:         name,
		Provider:     meta.Provider,
		Description:  meta.Description,
		Version:      meta.Version,
		IconPath:     iconPathIfAny(meta),
		UnpackedPath: filepath.Join("extensions", targetID, "unpacked"),
		SourceType:   meta.SourceType,
		StoreVendor:  meta.StoreVendor,
		SourceURL:    meta.SourceURL,
		Enabled:      existing.Enabled,
		Scope:        scope,
		CreatedAt:    existing.CreatedAt,
	}
	if err := store.Update(updated); err != nil {
		_ = os.Rename(target, stagingDir)
		_ = os.Rename(backup, target)
		return nil, fmt.Errorf("db update: %w", err)
	}
	// Step 4: remove backup (non-fatal; RecoverOnBoot scavenges *.old if this fails).
	_ = os.RemoveAll(backup)
	return updated, nil
}
