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

type Extension struct {
	ExtensionID  string     `json:"extensionId"`
	ChromeID     string     `json:"chromeId"`
	Name         string     `json:"name"`
	Provider     string     `json:"provider"`
	Description  string     `json:"description"`
	Version      string     `json:"version"`
	IconPath     string     `json:"iconPath"`
	UnpackedPath string     `json:"unpackedPath"`
	SourceType   SourceType `json:"sourceType"`
	StoreVendor  StoreVendor `json:"storeVendor"`
	SourceURL    string     `json:"sourceUrl"`
	Enabled      bool       `json:"enabled"`
	Scope        Scope      `json:"scope"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

type ExtensionView struct {
	ExtensionID              string      `json:"extensionId"`
	ChromeID                 string      `json:"chromeId"`
	Name                     string      `json:"name"`
	Provider                 string      `json:"provider"`
	Description              string      `json:"description"`
	Version                  string      `json:"version"`
	SourceType               SourceType  `json:"sourceType"`
	StoreVendor              StoreVendor `json:"storeVendor"`
	SourceURL                string      `json:"sourceUrl"`
	Enabled                  bool        `json:"enabled"`
	Scope                    Scope       `json:"scope"`
	IconDataURL              string      `json:"iconDataURL"`
	PendingRestartProfileIDs []string    `json:"pendingRestartProfileIds"`
	StaleScopeIDs            []string    `json:"staleScopeIds"`
}

type ExtensionPreview struct {
	StagingToken string      `json:"stagingToken"`
	ChromeID     string      `json:"chromeId"`
	Name         string      `json:"name"`
	Provider     string      `json:"provider"`
	Description  string      `json:"description"`
	Version      string      `json:"version"`
	SourceType   SourceType  `json:"sourceType"`
	StoreVendor  StoreVendor `json:"storeVendor"`
	SourceURL    string      `json:"sourceUrl"`
	IconDataURL  string      `json:"iconDataURL"`
	DuplicateOf  string      `json:"duplicateOf"`
}

type ExtensionChangeResult struct {
	// Extension is a pointer so a failed mutation (e.g. delete after the row
	// is already gone) can serialize as null without a zero-valued placeholder.
	Extension            *ExtensionView `json:"extension"`
	AffectedProfileIDs   []string       `json:"affectedProfileIds"`
	CDPSucceededIDs      []string       `json:"cdpSucceededIds"`
	PendingRestartIDs    []string       `json:"pendingRestartIds"`
	NotRunningIDs        []string       `json:"notRunningIds"`
	CDPSupportedByKernel bool           `json:"cdpSupportedByKernel"`
}

// StagedMeta is persisted under _staging/<token>/preview.json, the
// source of truth during Commit.
type StagedMeta struct {
	ChromeID     string      `json:"chromeId"`
	Name         string      `json:"name"`
	Provider     string      `json:"provider"`
	Description  string      `json:"description"`
	Version      string      `json:"version"`
	SourceType   SourceType  `json:"sourceType"`
	StoreVendor  StoreVendor `json:"storeVendor"`
	SourceURL    string      `json:"sourceUrl"`
	HasIcon      bool        `json:"hasIcon"`
}
