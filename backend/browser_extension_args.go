package backend

import (
	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/extension"
	"ant-chrome/backend/internal/logger"
	"os"
	"path/filepath"
	"strings"
)

// resolveExtensionsForProfile picks enabled extensions whose scope resolves
// for this profile and returns absolute paths to their unpacked directories.
// Paths that don't exist on disk are skipped with a warning.
func (a *App) resolveExtensionsForProfile(profile *browser.Profile) []string {
	if a.extMgr == nil || profile == nil {
		return nil
	}
	log := logger.New("Extension")
	exts := a.extMgr.ListEnabled()
	out := make([]string, 0, len(exts))
	for _, ext := range exts {
		if !extension.Resolve(ext, profile) {
			continue
		}
		abs := filepath.Join(a.extMgr.Paths.ExtensionDir(ext.ExtensionID), "unpacked")
		if _, err := os.Stat(abs); err != nil {
			log.Warn("扩展目录丢失，跳过",
				logger.F("extension_id", ext.ExtensionID),
				logger.F("path", abs),
				logger.F("error", err),
			)
			continue
		}
		out = append(out, abs)
	}
	return out
}

func buildLoadExtensionArg(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	return "--load-extension=" + strings.Join(paths, ",")
}
