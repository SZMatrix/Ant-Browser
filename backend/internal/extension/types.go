package extension

import "time"

type ScopeKind string

const (
	ScopeKindInstances ScopeKind = "instances"
	ScopeKindGroups    ScopeKind = "groups"
)

type Scope struct {
	Kind ScopeKind `json:"kind"`
	IDs  []string  `json:"ids"`
}

type SourceType string

const (
	SourceTypeStore    SourceType = "store"
	SourceTypeLocalCRX SourceType = "local_crx"
	SourceTypeLocalZIP SourceType = "local_zip"
)

type StoreVendor string

const (
	VendorChrome StoreVendor = "chrome"
	VendorEdge   StoreVendor = "edge"
)

type InstallStatus string

const (
	InstallStatusSucceeded  InstallStatus = "succeeded"
	InstallStatusInstalling InstallStatus = "installing"
	InstallStatusFailed     InstallStatus = "failed"
)

type Extension struct {
	ExtensionID   string        `json:"extensionId"`
	ChromeID      string        `json:"chromeId"`
	Name          string        `json:"name"`
	Provider      string        `json:"provider"`
	Description   string        `json:"description"`
	Version       string        `json:"version"`
	IconPath      string        `json:"iconPath"`
	UnpackedPath  string        `json:"unpackedPath"`
	SourceType    SourceType    `json:"sourceType"`
	StoreVendor   StoreVendor   `json:"storeVendor"`
	SourceURL     string        `json:"sourceUrl"`
	Enabled       bool          `json:"enabled"`
	Scope         Scope         `json:"scope"`
	InstallStatus InstallStatus `json:"installStatus"`
	InstallError  string        `json:"installError"`
	CreatedAt     time.Time     `json:"createdAt"`
	UpdatedAt     time.Time     `json:"updatedAt"`
}

type ExtensionView struct {
	ExtensionID              string        `json:"extensionId"`
	ChromeID                 string        `json:"chromeId"`
	Name                     string        `json:"name"`
	Provider                 string        `json:"provider"`
	Description              string        `json:"description"`
	Version                  string        `json:"version"`
	SourceType               SourceType    `json:"sourceType"`
	StoreVendor              StoreVendor   `json:"storeVendor"`
	SourceURL                string        `json:"sourceUrl"`
	Enabled                  bool          `json:"enabled"`
	Scope                    Scope         `json:"scope"`
	IconDataURL              string        `json:"iconDataURL"`
	PendingRestartProfileIDs []string      `json:"pendingRestartProfileIds"`
	StaleScopeIDs            []string      `json:"staleScopeIds"`
	InstallStatus            InstallStatus `json:"installStatus"`
	InstallError             string        `json:"installError"`
}

// Metadata is the identify-result shape. It carries everything the frontend
// needs to render the scope step AND everything the install worker needs to
// actually install — no staging token, nothing on disk yet.
type Metadata struct {
	ChromeID    string      `json:"chromeId"`
	Name        string      `json:"name"`
	Provider    string      `json:"provider"`
	Description string      `json:"description"`
	Version     string      `json:"version"`
	IconDataURL string      `json:"iconDataURL"`
	SourceType  SourceType  `json:"sourceType"`
	StoreVendor StoreVendor `json:"storeVendor"`
	SourceURL   string      `json:"sourceUrl"` // store URL OR absolute local file path
	DuplicateOf string      `json:"duplicateOf"`
}

type ExtensionChangeResult struct {
	Extension            *ExtensionView `json:"extension"`
	AffectedProfileIDs   []string       `json:"affectedProfileIds"`
	CDPSucceededIDs      []string       `json:"cdpSucceededIds"`
	PendingRestartIDs    []string       `json:"pendingRestartIds"`
	NotRunningIDs        []string       `json:"notRunningIds"`
	CDPSupportedByKernel bool           `json:"cdpSupportedByKernel"`
}
