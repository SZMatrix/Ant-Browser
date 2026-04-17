package extension

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Manifest struct {
	ManifestVersion int               `json:"manifest_version"`
	Name            string            `json:"name"`
	ShortName       string            `json:"short_name"`
	Version         string            `json:"version"`
	Description     string            `json:"description"`
	Author          string            `json:"author"`
	Developer       *DeveloperField   `json:"developer,omitempty"`
	DefaultLocale   string            `json:"default_locale"`
	Icons           map[string]string `json:"icons"`
}

type DeveloperField struct {
	Name string `json:"name"`
}

func ParseManifest(raw []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	if m.Icons == nil {
		m.Icons = map[string]string{}
	}
	return &m, nil
}

type ResolvedManifest struct {
	Name        string
	Description string
	Version     string
	Provider    string
	IconRelPath string
}

var msgPlaceholder = regexp.MustCompile(`^__MSG_([A-Za-z0-9_@]+)__$`)

// ResolveManifestStrings expands __MSG_xxx__ placeholders and chooses sensible
// fallbacks for name / provider. unpackedDir must be the extension root that
// contains _locales/<default_locale>/messages.json.
func ResolveManifestStrings(m *Manifest, unpackedDir string) ResolvedManifest {
	msgs := loadLocaleMessages(m.DefaultLocale, unpackedDir)
	name := expand(m.Name, msgs)
	if name == "" {
		name = m.ShortName
	}
	desc := expand(m.Description, msgs)
	provider := ""
	if m.Developer != nil {
		provider = strings.TrimSpace(m.Developer.Name)
	}
	if provider == "" {
		provider = strings.TrimSpace(m.Author)
	}
	return ResolvedManifest{
		Name:        name,
		Description: desc,
		Version:     m.Version,
		Provider:    provider,
		IconRelPath: PickIconPath(m.Icons),
	}
}

func loadLocaleMessages(locale string, unpackedDir string) map[string]string {
	if locale == "" {
		return nil
	}
	p := filepath.Join(unpackedDir, "_locales", locale, "messages.json")
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return nil
	}
	var parsed map[string]struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(b, &parsed); err != nil {
		return nil
	}
	out := make(map[string]string, len(parsed))
	for k, v := range parsed {
		out[k] = v.Message
	}
	return out
}

func expand(s string, msgs map[string]string) string {
	m := msgPlaceholder.FindStringSubmatch(strings.TrimSpace(s))
	if len(m) != 2 {
		return s
	}
	if v, ok := msgs[m[1]]; ok {
		return v
	}
	return s
}

// PickIconPath picks 128 > 48 > 16 > any; returns "" if map is empty.
func PickIconPath(icons map[string]string) string {
	for _, k := range []string{"128", "48", "16"} {
		if v, ok := icons[k]; ok && v != "" {
			return v
		}
	}
	for _, v := range icons {
		if v != "" {
			return v
		}
	}
	return ""
}
