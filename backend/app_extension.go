package backend

import (
	"ant-chrome/backend/internal/extension"
	"ant-chrome/backend/internal/logger"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type ExtensionView = extension.ExtensionView
type ExtensionMetadata = extension.Metadata
type ExtensionScope = extension.Scope

func (a *App) ExtensionList() ([]*ExtensionView, error) {
	if a.extMgr == nil {
		return nil, fmt.Errorf("extension manager not ready")
	}
	exts, err := a.extMgr.Store.List()
	if err != nil {
		return nil, err
	}
	validGroups, validProfiles := a.extensionValidIDSets()
	out := make([]*ExtensionView, 0, len(exts))
	for _, ext := range exts {
		out = append(out, a.extMgr.BuildView(ext, validGroups, validProfiles))
	}
	return out, nil
}

func (a *App) ExtensionGet(id string) (*ExtensionView, error) {
	if a.extMgr == nil {
		return nil, fmt.Errorf("extension manager not ready")
	}
	ext, err := a.extMgr.Store.GetByID(id)
	if err != nil {
		return nil, err
	}
	validGroups, validProfiles := a.extensionValidIDSets()
	return a.extMgr.BuildView(ext, validGroups, validProfiles), nil
}

// ExtensionIdentifyFromLocal opens a file picker when path is empty, otherwise
// parses the provided file. Returns nil when the user cancels the picker.
func (a *App) ExtensionIdentifyFromLocal(path string) (*ExtensionMetadata, error) {
	if a.extMgr == nil {
		return nil, fmt.Errorf("extension manager not ready")
	}
	p := strings.TrimSpace(path)
	if p == "" {
		if a.ctx == nil {
			return nil, fmt.Errorf("app context not ready")
		}
		picked, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
			Title: "选择扩展文件",
			Filters: []wailsruntime.FileFilter{
				{DisplayName: "扩展文件 (*.crx;*.zip)", Pattern: "*.crx;*.zip"},
			},
		})
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(picked) == "" {
			return nil, nil
		}
		p = picked
	}
	meta, err := extension.IdentifyFromLocal(p)
	if err != nil {
		return nil, err
	}
	if meta.ChromeID != "" {
		if dup, derr := a.extMgr.Store.FindByChromeID(meta.ChromeID); derr == nil {
			meta.DuplicateOf = dup
		}
	}
	return meta, nil
}

// ExtensionIdentifyFromStore fetches metadata only; no CRX is downloaded.
func (a *App) ExtensionIdentifyFromStore(storeURL string) (*ExtensionMetadata, error) {
	if a.extMgr == nil {
		return nil, fmt.Errorf("extension manager not ready")
	}
	meta, err := extension.IdentifyFromStore(strings.TrimSpace(storeURL))
	if err != nil {
		return nil, err
	}
	if meta.ChromeID != "" {
		if dup, derr := a.extMgr.Store.FindByChromeID(meta.ChromeID); derr == nil {
			meta.DuplicateOf = dup
		}
	}
	return meta, nil
}

// ExtensionCreateInstalling inserts the row synchronously and kicks a
// background install. The returned view has installStatus='installing'.
//
// If an extension with the same chromeID already exists, the existing row is
// promoted back to 'installing' and reinstalled in place. This collapses the
// duplicate add-flow into an overwrite-update instead of piling up separate
// rows for the same underlying extension.
//
// Live-sync to running instances is deferred to the worker's OnInstallSucceeded
// hook (see tryLiveSyncForInstalledExtension), since install is async.
func (a *App) ExtensionCreateInstalling(meta extension.Metadata, scope ExtensionScope, overrideName string) (*ExtensionView, error) {
	if a.extMgr == nil {
		return nil, fmt.Errorf("extension manager not ready")
	}
	if a.extMgr.Worker == nil {
		return nil, fmt.Errorf("install worker not ready")
	}
	var (
		ext *extension.Extension
		err error
	)
	if meta.ChromeID != "" {
		if dupID, ferr := a.extMgr.Store.FindByChromeID(meta.ChromeID); ferr == nil && dupID != "" {
			ext, err = a.extMgr.Installer.PromoteToReinstall(a.extMgr.Store, dupID, meta, scope, overrideName)
			if err != nil {
				return nil, err
			}
		}
	}
	if ext == nil {
		ext, err = a.extMgr.Installer.CreatePlaceholder(a.extMgr.Store, meta, scope, overrideName)
		if err != nil {
			return nil, err
		}
	}
	a.extMgr.Worker.Kick(ext.ExtensionID, meta)
	validGroups, validProfiles := a.extensionValidIDSets()
	return a.extMgr.BuildView(ext, validGroups, validProfiles), nil
}

// ExtensionRetryInstall flips a failed row back to installing and re-kicks
// the worker. Only failed rows can be retried.
func (a *App) ExtensionRetryInstall(id string) error {
	if a.extMgr == nil {
		return fmt.Errorf("extension manager not ready")
	}
	if a.extMgr.Worker == nil {
		return fmt.Errorf("install worker not ready")
	}
	ext, err := a.extMgr.Store.GetByID(id)
	if err != nil {
		return err
	}
	if ext.InstallStatus != extension.InstallStatusFailed {
		return fmt.Errorf("not a failed extension")
	}
	if err := a.extMgr.Store.UpdateInstallStatus(id, extension.InstallStatusInstalling, ""); err != nil {
		return err
	}
	meta := extension.Metadata{
		ChromeID:    ext.ChromeID,
		Name:        ext.Name,
		Provider:    ext.Provider,
		Description: ext.Description,
		Version:     ext.Version,
		SourceType:  ext.SourceType,
		StoreVendor: ext.StoreVendor,
		SourceURL:   ext.SourceURL,
	}
	a.extMgr.Worker.Kick(id, meta)
	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, "extensions:changed")
	}
	return nil
}

func (a *App) ExtensionSetEnabled(id string, enabled bool) (*ExtensionView, error) {
	if a.extMgr == nil {
		return nil, fmt.Errorf("extension manager not ready")
	}
	preExt, err := a.extMgr.Store.GetByID(id)
	if err != nil {
		return nil, err
	}
	if preExt.InstallStatus != extension.InstallStatusSucceeded {
		return nil, fmt.Errorf("extension not installed yet")
	}
	if err := a.extMgr.Store.SetEnabled(id, enabled); err != nil {
		return nil, err
	}
	ext, err := a.extMgr.Store.GetByID(id)
	if err != nil {
		return nil, err
	}
	// Best-effort live sync; failures are silently ignored by design.
	// Resolve() short-circuits on !Enabled, which is a launch-time guard that
	// isn't applicable here — for the disable direction we still need the
	// scope match to find which running profiles to Unload from.
	probe := *ext
	probe.Enabled = true
	unpacked := filepath.Join(a.extMgr.Paths.ExtensionDir(ext.ExtensionID), "unpacked")
	for _, t := range a.affectedRunningProfilesForExt(&probe) {
		var ok bool
		var reason string
		if enabled {
			ok, reason = t.CDP.LoadUnpacked(unpacked)
			logCDPSync("SetEnabled:load", t, "Extensions.loadUnpacked", ok, reason)
		} else {
			ok, reason = t.CDP.Unload(ext.ChromeID)
			logCDPSync("SetEnabled:unload", t, "Extensions.unload", ok, reason)
		}
	}
	validGroups, validProfiles := a.extensionValidIDSets()
	return a.extMgr.BuildView(ext, validGroups, validProfiles), nil
}

func (a *App) ExtensionUpdateScope(id string, scope ExtensionScope) (*ExtensionView, error) {
	if a.extMgr == nil {
		return nil, fmt.Errorf("extension manager not ready")
	}
	old, err := a.extMgr.Store.GetByID(id)
	if err != nil {
		return nil, err
	}
	if old.InstallStatus != extension.InstallStatusSucceeded {
		return nil, fmt.Errorf("extension not installed yet")
	}

	validGroups, validProfiles := a.extensionValidIDSets()
	if scope.Kind == extension.ScopeKindAll {
		// 'all' ignores IDs entirely; store a canonical empty slice.
		scope.IDs = []string{}
	} else {
		ref := validProfiles
		if scope.Kind == extension.ScopeKindGroups {
			ref = validGroups
		}
		cleaned := make([]string, 0, len(scope.IDs))
		for _, sid := range scope.IDs {
			if _, ok := ref[sid]; ok {
				cleaned = append(cleaned, sid)
			}
		}
		scope.IDs = cleaned
	}
	if err := a.extMgr.Store.UpdateScope(id, scope); err != nil {
		return nil, err
	}
	updated, err := a.extMgr.Store.GetByID(id)
	if err != nil {
		return nil, err
	}

	// Diff old/new affected profiles: newly-included → LoadUnpacked;
	// newly-excluded → Unload. All CDP calls are best-effort.
	oldTargets := a.affectedRunningProfilesForExt(old)
	newTargets := a.affectedRunningProfilesForExt(updated)
	oldSet := map[string]struct{}{}
	for _, t := range oldTargets {
		oldSet[t.ProfileID] = struct{}{}
	}
	newSet := map[string]struct{}{}
	for _, t := range newTargets {
		newSet[t.ProfileID] = struct{}{}
	}

	unpacked := filepath.Join(a.extMgr.Paths.ExtensionDir(id), "unpacked")
	for _, t := range newTargets {
		if _, was := oldSet[t.ProfileID]; was {
			continue
		}
		ok, reason := t.CDP.LoadUnpacked(unpacked)
		logCDPSync("UpdateScope:load", t, "Extensions.loadUnpacked", ok, reason)
	}
	for _, t := range oldTargets {
		if _, still := newSet[t.ProfileID]; still {
			continue
		}
		ok, reason := t.CDP.Unload(updated.ChromeID)
		logCDPSync("UpdateScope:unload", t, "Extensions.unload", ok, reason)
	}

	return a.extMgr.BuildView(updated, validGroups, validProfiles), nil
}

func (a *App) ExtensionRename(id, name string) error {
	if a.extMgr == nil {
		return fmt.Errorf("extension manager not ready")
	}
	ext, err := a.extMgr.Store.GetByID(id)
	if err != nil {
		return err
	}
	if ext.InstallStatus == extension.InstallStatusInstalling {
		return fmt.Errorf("extension not installed yet")
	}
	return a.extMgr.Store.Rename(id, strings.TrimSpace(name))
}

func (a *App) ExtensionDelete(id string) (*ExtensionView, error) {
	log := logger.New("Extension")
	if a.extMgr == nil {
		return nil, fmt.Errorf("extension manager not ready")
	}
	ext, err := a.extMgr.Store.GetByID(id)
	if err != nil {
		return nil, err
	}
	if ext.InstallStatus == extension.InstallStatusInstalling {
		return nil, fmt.Errorf("cannot delete an extension while it is installing")
	}
	targets := a.affectedRunningProfilesForExt(ext)

	if err := a.extMgr.Store.Delete(id); err != nil {
		return nil, err
	}
	if err := os.RemoveAll(a.extMgr.Paths.ExtensionDir(id)); err != nil {
		log.Warn("删除扩展目录失败", logger.F("extension_id", id), logger.F("error", err))
	}

	for _, t := range targets {
		ok, reason := t.CDP.Unload(ext.ChromeID)
		logCDPSync("Delete:unload", t, "Extensions.unload", ok, reason)
	}

	validGroups, validProfiles := a.extensionValidIDSets()
	return a.extMgr.BuildView(ext, validGroups, validProfiles), nil
}

// tryLiveSyncForInstalledExtension is invoked by the InstallWorker after a row
// transitions to 'succeeded'. It is the equivalent of the mutation-handler
// live-sync paths for newly-installed (or re-installed) extensions: best-effort
// CDP LoadUnpacked on every currently-running profile whose scope matches.
// All CDP failures are silently ignored by design.
func (a *App) tryLiveSyncForInstalledExtension(extID string) {
	if a.extMgr == nil {
		return
	}
	ext, err := a.extMgr.Store.GetByID(extID)
	if err != nil || ext == nil || !ext.Enabled {
		return
	}
	unpacked := filepath.Join(a.extMgr.Paths.ExtensionDir(extID), "unpacked")
	for _, t := range a.affectedRunningProfilesForExt(ext) {
		ok, reason := t.CDP.LoadUnpacked(unpacked)
		logCDPSync("InstallComplete:load", t, "Extensions.loadUnpacked", ok, reason)
	}
}

func (a *App) extensionValidIDSets() (groups, profiles map[string]struct{}) {
	groups = map[string]struct{}{}
	profiles = map[string]struct{}{}
	if a.browserMgr == nil {
		return
	}
	if a.browserMgr.GroupDAO != nil {
		if gs, err := a.browserMgr.GroupDAO.List(); err == nil {
			for _, g := range gs {
				groups[g.GroupId] = struct{}{}
			}
		}
	}
	if a.browserMgr.ProfileDAO != nil {
		if ps, err := a.browserMgr.ProfileDAO.List(); err == nil {
			for _, p := range ps {
				profiles[p.ProfileId] = struct{}{}
			}
		}
	}
	return
}

type affectedProfile struct {
	ProfileID string
	DebugPort int
	CoreID    string
	CDP       *extension.CDPClient
}

// affectedRunningProfilesForExt returns the live-CDP-ready targets whose scope
// resolves for the given extension. Profiles that aren't running / debug-ready
// are excluded — they'll already pick up the new state on their next start.
// Emits a DEBUG log with a per-bucket breakdown so "no targets" cases can be
// diagnosed (scope mismatch vs not running vs debug not ready).
func (a *App) affectedRunningProfilesForExt(ext *extension.Extension) []affectedProfile {
	if a.browserMgr == nil {
		return nil
	}
	// Running / DebugReady / DebugPort live only in the in-memory map
	// (browserMgr.Profiles), never persisted to SQLite. Snapshot under lock
	// and release before any CDP I/O.
	a.browserMgr.Mutex.Lock()
	snapshots := make([]BrowserProfile, 0, len(a.browserMgr.Profiles))
	for _, p := range a.browserMgr.Profiles {
		if p == nil {
			continue
		}
		snapshots = append(snapshots, *p)
	}
	a.browserMgr.Mutex.Unlock()

	var scopeMatch, notRunning, noDebug []string
	out := make([]affectedProfile, 0)
	for i := range snapshots {
		p := &snapshots[i]
		if !extension.Resolve(ext, p) {
			continue
		}
		scopeMatch = append(scopeMatch, p.ProfileId)
		if !p.Running {
			notRunning = append(notRunning, p.ProfileId)
			continue
		}
		if !p.DebugReady || p.DebugPort == 0 {
			noDebug = append(noDebug, p.ProfileId)
			continue
		}
		out = append(out, affectedProfile{
			ProfileID: p.ProfileId,
			DebugPort: p.DebugPort,
			CoreID:    p.CoreId,
			CDP:       extension.NewCDPClient(p.DebugPort, p.CoreId),
		})
	}
	logger.New("ExtCDP").Debug("affected running profiles",
		logger.F("extension_id", ext.ExtensionID),
		logger.F("scope_kind", ext.Scope.Kind),
		logger.F("scope_ids", ext.Scope.IDs),
		logger.F("total_profiles", len(snapshots)),
		logger.F("scope_match", scopeMatch),
		logger.F("not_running", notRunning),
		logger.F("no_debug", noDebug),
		logger.F("ready_targets", len(out)),
	)
	return out
}

// logCDPSync emits a per-target debug log for a live-sync CDP invocation.
// Captures kernel capability + call outcome so silent-fail cases can be traced.
func logCDPSync(op string, t affectedProfile, method string, ok bool, reason string) {
	cap := t.CDP.Capability()
	logger.New("ExtCDP").Debug(op,
		logger.F("profile_id", t.ProfileID),
		logger.F("debug_port", t.DebugPort),
		logger.F("method", method),
		logger.F("supports_load_unpacked", cap.SupportsLoadUnpacked),
		logger.F("supports_unload", cap.SupportsUnload),
		logger.F("probe_error", cap.ProbeError),
		logger.F("ok", ok),
		logger.F("reason", reason),
	)
}
