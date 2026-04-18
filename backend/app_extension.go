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
func (a *App) ExtensionCreateInstalling(meta extension.Metadata, scope ExtensionScope, overrideName string) (*ExtensionView, error) {
	if a.extMgr == nil {
		return nil, fmt.Errorf("extension manager not ready")
	}
	if a.extMgr.Worker == nil {
		return nil, fmt.Errorf("install worker not ready")
	}
	ext, err := a.extMgr.Installer.CreatePlaceholder(a.extMgr.Store, meta, scope, overrideName)
	if err != nil {
		return nil, err
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

func (a *App) ExtensionSetEnabled(id string, enabled bool) (*ExtensionChangeResult, error) {
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
	if old.InstallStatus != extension.InstallStatusSucceeded {
		return nil, fmt.Errorf("extension not installed yet")
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
	ext, err := a.extMgr.Store.GetByID(id)
	if err != nil {
		return err
	}
	if ext.InstallStatus == extension.InstallStatusInstalling {
		return fmt.Errorf("extension not installed yet")
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
