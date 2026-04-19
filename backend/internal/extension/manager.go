package extension

import (
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
)

// Manager wires Store + Installer and resolves ExtensionView data for the API
// layer. The CDP client factory is deferred to the app layer and lives outside
// this struct so this package stays free of CDP imports for now.
type Manager struct {
	Store     *SQLiteStore
	Installer *Installer
	Paths     Paths
	Worker    *InstallWorker // attached after Wails context is ready; nil until then
}

func NewManager(store *SQLiteStore, inst *Installer, paths Paths) *Manager {
	return &Manager{Store: store, Installer: inst, Paths: paths}
}

// ListEnabled returns all enabled extensions. Used by launch integration.
func (m *Manager) ListEnabled() []*Extension {
	xs, _ := m.Store.ListEnabled()
	return xs
}

// BuildView assembles an ExtensionView from a stored extension plus live state.
// validGroupIDs and validProfileIDs are used to compute StaleScopeIDs.
func (m *Manager) BuildView(ext *Extension, validGroupIDs, validProfileIDs map[string]struct{}) *ExtensionView {
	iconDataURL := ""
	if ext.IconPath != "" {
		p := filepath.Join(m.Paths.ExtensionDir(ext.ExtensionID), ext.IconPath)
		if b, err := os.ReadFile(p); err == nil {
			// Sniff content: Stage() always writes to icon.png regardless of the
			// source icon's real format (JPEG/SVG/PNG), so the filename is not
			// authoritative for mime.
			mime := http.DetectContentType(b)
			iconDataURL = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b)
		}
	}
	stale := staleIDs(ext.Scope, validGroupIDs, validProfileIDs)
	if stale == nil {
		stale = []string{}
	}
	scope := ext.Scope
	if scope.IDs == nil {
		scope.IDs = []string{}
	}
	return &ExtensionView{
		ExtensionID:   ext.ExtensionID,
		ChromeID:      ext.ChromeID,
		Name:          ext.Name,
		Provider:      ext.Provider,
		Description:   ext.Description,
		Version:       ext.Version,
		SourceType:    ext.SourceType,
		StoreVendor:   ext.StoreVendor,
		SourceURL:     ext.SourceURL,
		Enabled:       ext.Enabled,
		Scope:         scope,
		IconDataURL:   iconDataURL,
		StaleScopeIDs: stale,
		InstallStatus: ext.InstallStatus,
		InstallError:  ext.InstallError,
	}
}

func staleIDs(sc Scope, validGroupIDs, validProfileIDs map[string]struct{}) []string {
	var ref map[string]struct{}
	switch sc.Kind {
	case ScopeKindGroups:
		ref = validGroupIDs
	case ScopeKindInstances:
		ref = validProfileIDs
	default:
		return nil
	}
	var out []string
	for _, id := range sc.IDs {
		if _, ok := ref[id]; !ok {
			out = append(out, id)
		}
	}
	return out
}
