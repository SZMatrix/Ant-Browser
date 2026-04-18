package extension

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}

func (s *SQLiteStore) Insert(ext *Extension) error {
	scopeJSON, err := json.Marshal(ext.Scope)
	if err != nil {
		return fmt.Errorf("marshal scope: %w", err)
	}
	now := time.Now().UTC()
	if ext.CreatedAt.IsZero() {
		ext.CreatedAt = now
	}
	ext.UpdatedAt = now
	if ext.InstallStatus == "" {
		ext.InstallStatus = InstallStatusSucceeded
	}
	_, err = s.db.Exec(`
		INSERT INTO extensions (
			extension_id, chrome_id, name, provider, description, version,
			icon_path, unpacked_path, source_type, store_vendor, source_url,
			enabled, scope_json, install_status, install_error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ext.ExtensionID, ext.ChromeID, ext.Name, ext.Provider, ext.Description, ext.Version,
		ext.IconPath, ext.UnpackedPath, string(ext.SourceType), string(ext.StoreVendor), ext.SourceURL,
		boolToInt(ext.Enabled), string(scopeJSON), string(ext.InstallStatus), ext.InstallError,
		ext.CreatedAt, ext.UpdatedAt,
	)
	return err
}

func (s *SQLiteStore) GetByID(id string) (*Extension, error) {
	row := s.db.QueryRow(`
		SELECT extension_id, chrome_id, name, provider, description, version,
		       icon_path, unpacked_path, source_type, store_vendor, source_url,
		       enabled, scope_json, install_status, install_error, created_at, updated_at
		FROM extensions WHERE extension_id = ?`, id)
	return scanExtension(row)
}

func (s *SQLiteStore) FindByChromeID(chromeID string) (string, error) {
	if chromeID == "" {
		return "", nil
	}
	var id string
	err := s.db.QueryRow(`SELECT extension_id FROM extensions WHERE chrome_id = ? LIMIT 1`, chromeID).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}

func (s *SQLiteStore) List() ([]*Extension, error) {
	rows, err := s.db.Query(`
		SELECT extension_id, chrome_id, name, provider, description, version,
		       icon_path, unpacked_path, source_type, store_vendor, source_url,
		       enabled, scope_json, install_status, install_error, created_at, updated_at
		FROM extensions ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Extension
	for rows.Next() {
		ext, err := scanExtension(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ext)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ListEnabled() ([]*Extension, error) {
	rows, err := s.db.Query(`
		SELECT extension_id, chrome_id, name, provider, description, version,
		       icon_path, unpacked_path, source_type, store_vendor, source_url,
		       enabled, scope_json, install_status, install_error, created_at, updated_at
		FROM extensions WHERE enabled = 1 ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Extension
	for rows.Next() {
		ext, err := scanExtension(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ext)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) Update(ext *Extension) error {
	scopeJSON, err := json.Marshal(ext.Scope)
	if err != nil {
		return fmt.Errorf("marshal scope: %w", err)
	}
	ext.UpdatedAt = time.Now().UTC()
	_, err = s.db.Exec(`
		UPDATE extensions SET
			chrome_id = ?, name = ?, provider = ?, description = ?, version = ?,
			icon_path = ?, unpacked_path = ?, source_type = ?, store_vendor = ?, source_url = ?,
			enabled = ?, scope_json = ?, install_status = ?, install_error = ?, updated_at = ?
		WHERE extension_id = ?`,
		ext.ChromeID, ext.Name, ext.Provider, ext.Description, ext.Version,
		ext.IconPath, ext.UnpackedPath, string(ext.SourceType), string(ext.StoreVendor), ext.SourceURL,
		boolToInt(ext.Enabled), string(scopeJSON), string(ext.InstallStatus), ext.InstallError, ext.UpdatedAt,
		ext.ExtensionID,
	)
	return err
}

func (s *SQLiteStore) SetEnabled(id string, enabled bool) error {
	_, err := s.db.Exec(`UPDATE extensions SET enabled = ?, updated_at = ? WHERE extension_id = ?`,
		boolToInt(enabled), time.Now().UTC(), id)
	return err
}

func (s *SQLiteStore) UpdateScope(id string, sc Scope) error {
	b, err := json.Marshal(sc)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE extensions SET scope_json = ?, updated_at = ? WHERE extension_id = ?`,
		string(b), time.Now().UTC(), id)
	return err
}

func (s *SQLiteStore) Rename(id string, name string) error {
	_, err := s.db.Exec(`UPDATE extensions SET name = ?, updated_at = ? WHERE extension_id = ?`,
		name, time.Now().UTC(), id)
	return err
}

func (s *SQLiteStore) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM extensions WHERE extension_id = ?`, id)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanExtension(row rowScanner) (*Extension, error) {
	var ext Extension
	var enabledInt int
	var scopeJSON string
	var sourceType, storeVendor, installStatus string
	err := row.Scan(
		&ext.ExtensionID, &ext.ChromeID, &ext.Name, &ext.Provider, &ext.Description, &ext.Version,
		&ext.IconPath, &ext.UnpackedPath, &sourceType, &storeVendor, &ext.SourceURL,
		&enabledInt, &scopeJSON, &installStatus, &ext.InstallError, &ext.CreatedAt, &ext.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	ext.SourceType = SourceType(sourceType)
	ext.StoreVendor = StoreVendor(storeVendor)
	ext.InstallStatus = InstallStatus(installStatus)
	ext.Enabled = enabledInt != 0
	if err := json.Unmarshal([]byte(scopeJSON), &ext.Scope); err != nil {
		return nil, fmt.Errorf("decode scope: %w", err)
	}
	if ext.Scope.IDs == nil {
		ext.Scope.IDs = []string{}
	}
	return &ext, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// PruneProfileFromAllScopes removes profileID from every extension whose scope.kind = instances.
func (s *SQLiteStore) PruneProfileFromAllScopes(profileID string) error {
	return s.pruneIDFromScopes(ScopeKindInstances, profileID)
}

// PruneGroupFromAllScopes removes groupID from every extension whose scope.kind = groups.
func (s *SQLiteStore) PruneGroupFromAllScopes(groupID string) error {
	return s.pruneIDFromScopes(ScopeKindGroups, groupID)
}

func (s *SQLiteStore) pruneIDFromScopes(kind ScopeKind, targetID string) error {
	if targetID == "" {
		return nil
	}
	exts, err := s.List()
	if err != nil {
		return err
	}
	remove := map[string]struct{}{targetID: {}}
	for _, ext := range exts {
		if ext.Scope.Kind != kind {
			continue
		}
		if !containsString(ext.Scope.IDs, targetID) {
			continue
		}
		pruned := PruneIDs(ext.Scope, remove)
		if err := s.UpdateScope(ext.ExtensionID, pruned); err != nil {
			return err
		}
	}
	return nil
}

func containsString(xs []string, target string) bool {
	for _, x := range xs {
		if x == target {
			return true
		}
	}
	return false
}

// UpdateInstallStatus flips an extension's install status + error message
// without touching any other column.
func (s *SQLiteStore) UpdateInstallStatus(id string, status InstallStatus, installErr string) error {
	_, err := s.db.Exec(`
		UPDATE extensions SET install_status = ?, install_error = ?, updated_at = ?
		WHERE extension_id = ?`,
		string(status), installErr, time.Now().UTC(), id,
	)
	return err
}

// ListInstalling returns every row still marked as installing. Used by
// startup recovery to flip interrupted installs to failed.
func (s *SQLiteStore) ListInstalling() ([]*Extension, error) {
	rows, err := s.db.Query(`
		SELECT extension_id, chrome_id, name, provider, description, version,
		       icon_path, unpacked_path, source_type, store_vendor, source_url,
		       enabled, scope_json, install_status, install_error, created_at, updated_at
		FROM extensions WHERE install_status = ?`, string(InstallStatusInstalling))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Extension
	for rows.Next() {
		ext, err := scanExtension(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ext)
	}
	return out, rows.Err()
}
