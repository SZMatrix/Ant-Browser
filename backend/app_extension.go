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

// Wails-exported type aliases re-used on the frontend (wails codegen will emit TS).
type ExtensionView = extension.ExtensionView
type ExtensionPreview = extension.ExtensionPreview
type ExtensionChangeResult = extension.ExtensionChangeResult
type ExtensionScope = extension.Scope

func (a *App) ExtensionList() ([]*ExtensionView, error) {
	if a.extMgr == nil {
		return nil, fmt.Errorf("extension manager not ready")
	}
	exts, err := a.extMgr.Store.List()
	if err != nil {
		return nil, err
	}
	log := logger.New("Extension")
	ids := make([]string, 0, len(exts))
	for _, ext := range exts {
		ids = append(ids, ext.ExtensionID+"/"+ext.ChromeID)
	}
	log.Info("ExtensionList", logger.F("count", len(exts)), logger.F("ids", ids))
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

// ExtensionPreviewFromLocal opens a file dialog when path is empty, otherwise
// stages the file at the given path.
func (a *App) ExtensionPreviewFromLocal(path string) (*ExtensionPreview, error) {
	log := logger.New("Extension")
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
	raw, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	preview, err := a.extMgr.Installer.StageLocal(raw, p)
	if err != nil {
		return nil, err
	}
	if preview.ChromeID != "" {
		dup, _ := a.extMgr.Store.FindByChromeID(preview.ChromeID)
		preview.DuplicateOf = dup
	}
	log.Info("PreviewFromLocal", logger.F("chromeId", preview.ChromeID), logger.F("duplicateOf", preview.DuplicateOf), logger.F("name", preview.Name))
	return preview, nil
}

// ExtensionPreviewFromStore downloads from Chrome or Edge store and stages the CRX.
func (a *App) ExtensionPreviewFromStore(storeURL string) (*ExtensionPreview, error) {
	log := logger.New("Extension")
	if a.extMgr == nil {
		return nil, fmt.Errorf("extension manager not ready")
	}
	vendor, id, err := extension.ParseStoreURL(storeURL)
	if err != nil {
		return nil, err
	}
	downloadURL := extension.CRXDownloadURL(vendor, id)
	if downloadURL == "" {
		return nil, fmt.Errorf("unsupported vendor")
	}
	raw, err := extension.DownloadBytes(downloadURL, 15000)
	if err != nil {
		return nil, fmt.Errorf("store_unreachable: %w", err)
	}

	meta := extension.FetchStoreMetadata(vendor, id, 8000)

	preview, err := a.extMgr.Installer.StageStore(raw, vendor, storeURL, id, meta.Name, meta.Provider, meta.Description)
	if err != nil {
		return nil, err
	}
	if preview.ChromeID != "" {
		dup, _ := a.extMgr.Store.FindByChromeID(preview.ChromeID)
		preview.DuplicateOf = dup
	}
	log.Info("PreviewFromStore", logger.F("chromeId", preview.ChromeID), logger.F("duplicateOf", preview.DuplicateOf), logger.F("name", preview.Name), logger.F("urlId", id))
	return preview, nil
}

func (a *App) ExtensionCommit(stagingToken string, scope ExtensionScope, overrideName, duplicateOf string) (*ExtensionView, error) {
	if a.extMgr == nil {
		return nil, fmt.Errorf("extension manager not ready")
	}
	log := logger.New("Extension")
	if before, berr := a.extMgr.Store.List(); berr == nil {
		ids := make([]string, 0, len(before))
		for _, e := range before {
			ids = append(ids, e.ExtensionID+"/"+e.ChromeID)
		}
		log.Info("ExtensionCommit: before commit", logger.F("count", len(before)), logger.F("all", ids), logger.F("duplicateOf", duplicateOf))
	}
	var (
		ext *extension.Extension
		err error
	)
	if strings.TrimSpace(duplicateOf) != "" {
		log.Info("ExtensionCommit: overwrite", logger.F("target", duplicateOf), logger.F("token", stagingToken))
		ext, err = a.extMgr.Installer.CommitOverwrite(a.extMgr.Store, stagingToken, duplicateOf, scope, overrideName)
	} else {
		log.Info("ExtensionCommit: insert", logger.F("token", stagingToken))
		ext, err = a.extMgr.Installer.CommitInsert(a.extMgr.Store, stagingToken, scope, overrideName)
	}
	if err != nil {
		log.Error("ExtensionCommit failed", logger.F("error", err))
		return nil, err
	}
	if all, lerr := a.extMgr.Store.List(); lerr == nil {
		ids := make([]string, 0, len(all))
		for _, e := range all {
			ids = append(ids, e.ExtensionID+"/"+e.ChromeID)
		}
		log.Info("ExtensionCommit: after commit", logger.F("new_id", ext.ExtensionID), logger.F("all_count", len(all)), logger.F("all", ids))
	}
	validGroups, validProfiles := a.extensionValidIDSets()
	return a.extMgr.BuildView(ext, validGroups, validProfiles), nil
}

func (a *App) ExtensionCancelPreview(stagingToken string) error {
	if a.extMgr == nil {
		return fmt.Errorf("extension manager not ready")
	}
	return a.extMgr.Installer.CancelPreview(stagingToken)
}

func (a *App) ExtensionSetEnabled(id string, enabled bool) (*ExtensionChangeResult, error) {
	if a.extMgr == nil {
		return nil, fmt.Errorf("extension manager not ready")
	}
	if err := a.extMgr.Store.SetEnabled(id, enabled); err != nil {
		return nil, err
	}
	ext, err := a.extMgr.Store.GetByID(id)
	if err != nil {
		return nil, err
	}

	targets := a.affectedRunningProfilesForExt(ext)
	succ := []string{}
	pending := []string{}
	unpacked := filepath.Join(a.extMgr.Paths.ExtensionDir(ext.ExtensionID), "unpacked")
	kernelSupport := false
	for _, t := range targets {
		capab := t.CDP.Capability()
		if capab.SupportsLoadUnpacked || capab.SupportsUnload {
			kernelSupport = true
		}
		var ok bool
		if enabled {
			ok, _ = t.CDP.LoadUnpacked(unpacked)
		} else {
			ok, _ = t.CDP.Unload(ext.ChromeID)
		}
		if ok {
			succ = append(succ, t.ProfileID)
			a.extMgr.Tracker.ClearProfile(t.ProfileID)
		} else {
			pending = append(pending, t.ProfileID)
			a.extMgr.Tracker.Mark(ext.ExtensionID, t.ProfileID)
		}
	}
	validGroups, validProfiles := a.extensionValidIDSets()
	return &ExtensionChangeResult{
		Extension:            a.extMgr.BuildView(ext, validGroups, validProfiles),
		AffectedProfileIDs:   collectAffectedIDs(targets),
		CDPSucceededIDs:      succ,
		PendingRestartIDs:    pending,
		CDPSupportedByKernel: kernelSupport,
	}, nil
}

func (a *App) ExtensionUpdateScope(id string, scope ExtensionScope) (*ExtensionChangeResult, error) {
	if a.extMgr == nil {
		return nil, fmt.Errorf("extension manager not ready")
	}
	old, err := a.extMgr.Store.GetByID(id)
	if err != nil {
		return nil, err
	}

	validGroups, validProfiles := a.extensionValidIDSets()
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
	if err := a.extMgr.Store.UpdateScope(id, scope); err != nil {
		return nil, err
	}
	updated, err := a.extMgr.Store.GetByID(id)
	if err != nil {
		return nil, err
	}

	// Compute old vs new affected running profiles; apply symmetric difference.
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

	succ := []string{}
	pending := []string{}
	unpacked := filepath.Join(a.extMgr.Paths.ExtensionDir(id), "unpacked")
	kernelSupport := false

	// Newly-covered profiles → LoadUnpacked.
	for _, t := range newTargets {
		if _, was := oldSet[t.ProfileID]; was {
			continue
		}
		capab := t.CDP.Capability()
		if capab.SupportsLoadUnpacked || capab.SupportsUnload {
			kernelSupport = true
		}
		if ok, _ := t.CDP.LoadUnpacked(unpacked); ok {
			succ = append(succ, t.ProfileID)
			a.extMgr.Tracker.ClearProfile(t.ProfileID)
		} else {
			pending = append(pending, t.ProfileID)
			a.extMgr.Tracker.Mark(id, t.ProfileID)
		}
	}
	// Newly-uncovered profiles → Unload.
	for _, t := range oldTargets {
		if _, still := newSet[t.ProfileID]; still {
			continue
		}
		capab := t.CDP.Capability()
		if capab.SupportsLoadUnpacked || capab.SupportsUnload {
			kernelSupport = true
		}
		if ok, _ := t.CDP.Unload(updated.ChromeID); ok {
			succ = append(succ, t.ProfileID)
			a.extMgr.Tracker.ClearProfile(t.ProfileID)
		} else {
			pending = append(pending, t.ProfileID)
			a.extMgr.Tracker.Mark(id, t.ProfileID)
		}
	}

	affected := collectAffectedIDs(newTargets)
	return &ExtensionChangeResult{
		Extension:            a.extMgr.BuildView(updated, validGroups, validProfiles),
		AffectedProfileIDs:   affected,
		CDPSucceededIDs:      succ,
		PendingRestartIDs:    pending,
		CDPSupportedByKernel: kernelSupport,
	}, nil
}

func (a *App) ExtensionRename(id, name string) error {
	if a.extMgr == nil {
		return fmt.Errorf("extension manager not ready")
	}
	return a.extMgr.Store.Rename(id, strings.TrimSpace(name))
}

func (a *App) ExtensionDelete(id string) (*ExtensionChangeResult, error) {
	log := logger.New("Extension")
	if a.extMgr == nil {
		return nil, fmt.Errorf("extension manager not ready")
	}
	ext, err := a.extMgr.Store.GetByID(id)
	if err != nil {
		return nil, err
	}
	// Snapshot targets BEFORE deleting so we can unload them.
	targets := a.affectedRunningProfilesForExt(ext)

	if err := a.extMgr.Store.Delete(id); err != nil {
		return nil, err
	}
	if err := os.RemoveAll(a.extMgr.Paths.ExtensionDir(id)); err != nil {
		log.Warn("删除扩展目录失败", logger.F("extension_id", id), logger.F("error", err))
	}
	a.extMgr.Tracker.ClearExtension(id)

	succ := []string{}
	pending := []string{}
	kernelSupport := false
	for _, t := range targets {
		capab := t.CDP.Capability()
		if capab.SupportsLoadUnpacked || capab.SupportsUnload {
			kernelSupport = true
		}
		if ok, _ := t.CDP.Unload(ext.ChromeID); ok {
			succ = append(succ, t.ProfileID)
		} else {
			pending = append(pending, t.ProfileID)
		}
	}

	validGroups, validProfiles := a.extensionValidIDSets()
	return &ExtensionChangeResult{
		Extension:            a.extMgr.BuildView(ext, validGroups, validProfiles),
		AffectedProfileIDs:   collectAffectedIDs(targets),
		CDPSucceededIDs:      succ,
		PendingRestartIDs:    pending,
		CDPSupportedByKernel: kernelSupport,
	}, nil
}

func (a *App) ExtensionGetPendingRestarts() (map[string][]string, error) {
	if a.extMgr == nil {
		return nil, fmt.Errorf("extension manager not ready")
	}
	return a.extMgr.Tracker.Snapshot(), nil
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
func (a *App) affectedRunningProfilesForExt(ext *extension.Extension) []affectedProfile {
	if a.browserMgr == nil {
		return nil
	}
	profiles, _ := a.browserMgr.ProfileDAO.List()
	out := make([]affectedProfile, 0)
	for _, p := range profiles {
		if !extension.Resolve(ext, p) {
			continue
		}
		if !p.Running || !p.DebugReady || p.DebugPort == 0 {
			continue
		}
		out = append(out, affectedProfile{
			ProfileID: p.ProfileId,
			DebugPort: p.DebugPort,
			CoreID:    p.CoreId,
			CDP:       extension.NewCDPClient(p.DebugPort, p.CoreId),
		})
	}
	return out
}

func collectAffectedIDs(ts []affectedProfile) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.ProfileID)
	}
	return out
}
