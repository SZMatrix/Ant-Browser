package extension

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseManifestBasic(t *testing.T) {
	data, err := os.ReadFile("testdata/manifest_basic.json")
	if err != nil {
		t.Fatal(err)
	}
	m, err := ParseManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "Example" || m.Version != "1.2.3" || m.ShortName != "Ex" {
		t.Fatalf("unexpected: %+v", m)
	}
	if m.Author != "Acme Inc." {
		t.Fatalf("author: %q", m.Author)
	}
	if m.Icons["128"] != "icons/128.png" {
		t.Fatalf("icons: %+v", m.Icons)
	}
}

func TestResolveI18n(t *testing.T) {
	dir := t.TempDir()
	localesSrc := "testdata/_locales/en/messages.json"
	localesDst := filepath.Join(dir, "_locales", "en", "messages.json")
	if err := os.MkdirAll(filepath.Dir(localesDst), 0o755); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(localesSrc)
	if err := os.WriteFile(localesDst, b, 0o644); err != nil {
		t.Fatal(err)
	}

	mb, _ := os.ReadFile("testdata/manifest_i18n.json")
	m, _ := ParseManifest(mb)
	resolved := ResolveManifestStrings(m, dir)
	if resolved.Name != "Locale-resolved Name" {
		t.Fatalf("name: %q", resolved.Name)
	}
	if resolved.Description != "Locale-resolved Description" {
		t.Fatalf("desc: %q", resolved.Description)
	}
}

func TestPickIconPath(t *testing.T) {
	icons := map[string]string{"16": "a.png", "48": "b.png", "128": "c.png"}
	if PickIconPath(icons) != "c.png" {
		t.Fatal("want 128")
	}
	if PickIconPath(map[string]string{"48": "b.png"}) != "b.png" {
		t.Fatal("want 48")
	}
	if PickIconPath(map[string]string{"any": "x.png"}) == "" {
		t.Fatal("want any non-empty fallback")
	}
	if PickIconPath(nil) != "" {
		t.Fatal("empty case")
	}
}
