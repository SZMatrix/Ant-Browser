package extension

import "path/filepath"

// Paths encapsulates filesystem locations used by the extension subsystem.
// All paths are derived from dataDir (equivalent of resolveAppPath("data")).
type Paths struct {
	ExtRoot     string // <data>/extensions
	StagingRoot string // <data>/extensions/_staging
}

func NewPaths(dataDir string) Paths {
	root := filepath.Join(dataDir, "extensions")
	return Paths{
		ExtRoot:     root,
		StagingRoot: filepath.Join(root, "_staging"),
	}
}

func (p Paths) ExtensionDir(id string) string  { return filepath.Join(p.ExtRoot, id) }
func (p Paths) StagingDir(token string) string { return filepath.Join(p.StagingRoot, token) }
