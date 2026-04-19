package extension

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// InstallEventEmitter is the narrow interface the worker needs to
// signal the frontend. The app layer passes a closure bound to wails ctx.
type InstallEventEmitter func()

// InstallWorker runs extension installs in background goroutines and emits
// an `extensions:changed` Wails event each time a row's status transitions.
type InstallWorker struct {
	paths Paths
	store *SQLiteStore
	emit  InstallEventEmitter
	// OnInstallSucceeded is an optional post-success hook. Called after the row
	// flips to 'succeeded' but before emit, so consumers (e.g. CDP live-sync)
	// observe the final DB state. Must be set before the first Kick();
	// the worker does not lock around reads.
	OnInstallSucceeded func(extID string)
	mu                 sync.Mutex
	running            map[string]context.CancelFunc // extensionID → cancel (reserved for future)
}

func NewInstallWorker(store *SQLiteStore, paths Paths, emit InstallEventEmitter) *InstallWorker {
	return &InstallWorker{
		store:   store,
		paths:   paths,
		emit:    emit,
		running: map[string]context.CancelFunc{},
	}
}

// Kick launches a background install for the given extension + metadata.
// The row MUST already exist in the DB with install_status = 'installing'.
func (w *InstallWorker) Kick(extID string, meta Metadata) {
	ctx, cancel := context.WithCancel(context.Background())
	w.mu.Lock()
	w.running[extID] = cancel
	w.mu.Unlock()
	go func() {
		defer func() {
			w.mu.Lock()
			delete(w.running, extID)
			w.mu.Unlock()
		}()
		err := w.runInstall(ctx, extID, meta)
		if err != nil {
			w.markFailed(extID, err.Error())
		} else {
			w.markSucceeded(extID)
			if w.OnInstallSucceeded != nil {
				w.OnInstallSucceeded(extID)
			}
		}
		if w.emit != nil {
			w.emit()
		}
	}()
}

// runInstall does the real work: acquire the raw bytes (download or read local),
// verify chromeID matches what identify claimed, unpack into the final extension
// directory, write the icon, update the DB row with manifest-derived fields.
func (w *InstallWorker) runInstall(ctx context.Context, extID string, meta Metadata) error {
	var raw []byte
	switch meta.SourceType {
	case SourceTypeStore:
		vendor, id, err := ParseStoreURL(meta.SourceURL)
		if err != nil {
			return fmt.Errorf("store url: %w", err)
		}
		dl := CRXDownloadURL(vendor, id)
		if dl == "" {
			return fmt.Errorf("unsupported vendor")
		}
		b, err := DownloadBytes(dl, 30000)
		if err != nil {
			return fmt.Errorf("download crx: %w", err)
		}
		raw = b
	case SourceTypeLocalCRX, SourceTypeLocalZIP:
		b, err := os.ReadFile(meta.SourceURL)
		if err != nil {
			return fmt.Errorf("read local file: %w", err)
		}
		raw = b
	default:
		return fmt.Errorf("unknown source type %q", meta.SourceType)
	}

	zipBytes := raw
	derivedChromeID := ""
	if DetectKind(raw) == "crx" {
		z, cid, err := StripCRXHeader(raw)
		if err != nil {
			return fmt.Errorf("invalid crx: %w", err)
		}
		zipBytes, derivedChromeID = z, cid
	}

	if meta.SourceType != SourceTypeStore && derivedChromeID != "" && meta.ChromeID != "" && derivedChromeID != meta.ChromeID {
		return fmt.Errorf("chromeId mismatch (identify=%s, crx=%s)", meta.ChromeID, derivedChromeID)
	}

	target := w.paths.ExtensionDir(extID)
	unpackedDir := filepath.Join(target, "unpacked")

	_ = os.RemoveAll(target)
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}

	if err := extractZip(zipBytes, unpackedDir); err != nil {
		_ = os.RemoveAll(target)
		return fmt.Errorf("unpack: %w", err)
	}

	if ctx.Err() != nil {
		_ = os.RemoveAll(target)
		return ctx.Err()
	}

	manifestBytes, err := os.ReadFile(filepath.Join(unpackedDir, "manifest.json"))
	if err != nil {
		_ = os.RemoveAll(target)
		return fmt.Errorf("manifest missing after unpack: %w", err)
	}
	manifest, err := ParseManifest(manifestBytes)
	if err != nil {
		_ = os.RemoveAll(target)
		return fmt.Errorf("manifest parse: %w", err)
	}
	resolved := ResolveManifestStrings(manifest, unpackedDir)

	iconPath := ""
	if resolved.IconRelPath != "" {
		_, hasIcon := exportIcon(unpackedDir, resolved.IconRelPath, filepath.Join(target, "icon.png"))
		if hasIcon {
			iconPath = "icon.png"
		}
	}

	ext, err := w.store.GetByID(extID)
	if err != nil {
		return err
	}
	ext.ChromeID = firstNonEmpty(meta.ChromeID, derivedChromeID, ext.ChromeID)
	ext.Version = resolved.Version
	ext.Description = firstNonEmpty(ext.Description, resolved.Description)
	ext.Provider = firstNonEmpty(ext.Provider, resolved.Provider)
	ext.IconPath = iconPath
	ext.UnpackedPath = filepath.Join("extensions", extID, "unpacked")
	ext.InstallStatus = InstallStatusSucceeded
	ext.InstallError = ""
	return w.store.Update(ext)
}

func (w *InstallWorker) markFailed(id, msg string) {
	_ = w.store.UpdateInstallStatus(id, InstallStatusFailed, msg)
	_ = os.RemoveAll(w.paths.ExtensionDir(id))
}

func (w *InstallWorker) markSucceeded(id string) {
	_ = w.store.UpdateInstallStatus(id, InstallStatusSucceeded, "")
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}

// DefaultEmitter produces an emitter bound to a Wails app context.
func DefaultEmitter(ctx context.Context) InstallEventEmitter {
	return func() {
		if ctx == nil {
			return
		}
		wailsruntime.EventsEmit(ctx, "extensions:changed")
	}
}
