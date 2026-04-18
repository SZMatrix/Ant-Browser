package extension

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"time"
)

var ErrIdentifyFailed = errors.New("identify failed")

// IdentifyFromLocal parses the given file's CRX header (or ZIP) and its
// manifest.json to build a Metadata. Nothing is copied or staged.
func IdentifyFromLocal(filePath string) (*Metadata, error) {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrIdentifyFailed, err)
	}
	kind := DetectKind(raw)
	zipBytes := raw
	chromeID := ""
	var sourceType SourceType
	if kind == "crx" {
		z, id, err := StripCRXHeader(raw)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid crx: %v", ErrIdentifyFailed, err)
		}
		zipBytes = z
		chromeID = id
		sourceType = SourceTypeLocalCRX
	} else {
		sourceType = SourceTypeLocalZIP
	}

	manifestBytes, iconPath, err := readManifestFromZip(zipBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrIdentifyFailed, err)
	}
	manifest, err := ParseManifest(manifestBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: manifest parse: %v", ErrIdentifyFailed, err)
	}
	name := strings.TrimSpace(manifest.Name)
	if strings.HasPrefix(name, "__MSG_") {
		name = resolveMessageFromZip(zipBytes, manifest.DefaultLocale, manifest.Name)
	}
	if name == "" {
		name = "未命名扩展"
	}
	provider := ""
	if manifest.Developer != nil {
		provider = strings.TrimSpace(manifest.Developer.Name)
	}
	if provider == "" {
		provider = strings.TrimSpace(manifest.Author)
	}
	if provider == "" {
		provider = "未知"
	}
	desc := strings.TrimSpace(manifest.Description)
	if strings.HasPrefix(desc, "__MSG_") {
		desc = resolveMessageFromZip(zipBytes, manifest.DefaultLocale, manifest.Description)
	}

	iconDataURL := ""
	if iconPath != "" {
		if b, mime, ok := readFromZip(zipBytes, iconPath); ok {
			iconDataURL = "data:" + mime + ";base64," + stdEncodeBase64(b)
		}
	}

	return &Metadata{
		ChromeID:    chromeID,
		Name:        name,
		Provider:    provider,
		Description: desc,
		Version:     manifest.Version,
		IconDataURL: iconDataURL,
		SourceType:  sourceType,
		StoreVendor: "",
		SourceURL:   filePath,
	}, nil
}

// IdentifyFromStore fetches the Chrome Web Store / Edge Add-ons detail page
// and parses metadata out of the HTML. No CRX is downloaded.
func IdentifyFromStore(storeURL string) (*Metadata, error) {
	vendor, id, err := ParseStoreURL(storeURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrIdentifyFailed, err)
	}

	pageURL := storeURL
	if pageURL == "" {
		return nil, fmt.Errorf("%w: empty url", ErrIdentifyFailed)
	}
	body, err := httpGetBody(pageURL, 15000)
	if err != nil {
		return nil, fmt.Errorf("%w: fetch page: %v", ErrIdentifyFailed, err)
	}

	meta := parseStoreHTML(body, vendor)
	name := strings.TrimSpace(meta.Name)
	if name == "" {
		name = "未命名扩展"
	}
	provider := strings.TrimSpace(meta.Provider)
	if provider == "" {
		provider = "未知"
	}

	return &Metadata{
		ChromeID:    id,
		Name:        name,
		Provider:    provider,
		Description: meta.Description,
		Version:     meta.Version,
		IconDataURL: meta.IconDataURL,
		SourceType:  SourceTypeStore,
		StoreVendor: vendor,
		SourceURL:   pageURL,
	}, nil
}

type storeScrape struct {
	Name        string
	Provider    string
	Description string
	Version     string
	IconDataURL string
}

var (
	reOGTitle = regexp.MustCompile(`(?i)<meta[^>]+property=["']og:title["'][^>]+content=["']([^"']+)["']`)
	reOGImage = regexp.MustCompile(`(?i)<meta[^>]+property=["']og:image["'][^>]+content=["']([^"']+)["']`)
	reOGDesc  = regexp.MustCompile(`(?i)<meta[^>]+property=["']og:description["'][^>]+content=["']([^"']+)["']`)
)

func parseStoreHTML(body []byte, vendor StoreVendor) storeScrape {
	s := string(body)
	out := storeScrape{}
	if m := reOGTitle.FindStringSubmatch(s); len(m) == 2 {
		out.Name = stripStoreTitleSuffix(m[1], vendor)
	}
	if m := reOGDesc.FindStringSubmatch(s); len(m) == 2 {
		out.Description = m[1]
	}
	if m := reOGImage.FindStringSubmatch(s); len(m) == 2 {
		if b, mime, ok := fetchImage(m[1], 8000); ok {
			out.IconDataURL = "data:" + mime + ";base64," + stdEncodeBase64(b)
		}
	}
	out.Version = scrapeVersion(s, vendor)
	out.Provider = scrapeProvider(s, vendor)
	return out
}

func stripStoreTitleSuffix(title string, vendor StoreVendor) string {
	for _, suffix := range []string{" - Chrome Web Store", " - Microsoft Edge Addons", " - Chrome 线上应用店"} {
		if strings.HasSuffix(title, suffix) {
			return strings.TrimSpace(strings.TrimSuffix(title, suffix))
		}
	}
	_ = vendor
	return strings.TrimSpace(title)
}

var reVersion = regexp.MustCompile(`"version"\s*:\s*"([0-9][0-9A-Za-z.\-]+)"`)

func scrapeVersion(body string, _ StoreVendor) string {
	if m := reVersion.FindStringSubmatch(body); len(m) == 2 {
		return m[1]
	}
	return ""
}

var (
	reAuthor    = regexp.MustCompile(`"author"\s*:\s*\{[^}]*"name"\s*:\s*"([^"]+)"`)
	rePublisher = regexp.MustCompile(`"publisher"\s*:\s*"([^"]+)"`)
)

func scrapeProvider(body string, _ StoreVendor) string {
	if m := reAuthor.FindStringSubmatch(body); len(m) == 2 {
		return m[1]
	}
	if m := rePublisher.FindStringSubmatch(body); len(m) == 2 {
		return m[1]
	}
	return ""
}

func httpGetBody(raw string, timeoutMS int) ([]byte, error) {
	if _, err := url.Parse(raw); err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: time.Duration(timeoutMS) * time.Millisecond}
	req, err := http.NewRequest("GET", raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0 Safari/537.36")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
}

func fetchImage(raw string, timeoutMS int) ([]byte, string, bool) {
	b, err := httpGetBody(raw, timeoutMS)
	if err != nil {
		return nil, "", false
	}
	return b, http.DetectContentType(b), true
}

// zip helpers ------------------------------------------------------------

func readManifestFromZip(zipBytes []byte) (manifestBytes []byte, iconPath string, err error) {
	r, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, "", err
	}
	for _, zf := range r.File {
		if path.Clean(zf.Name) != "manifest.json" {
			continue
		}
		rc, oerr := zf.Open()
		if oerr != nil {
			return nil, "", oerr
		}
		manifestBytes, err = io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, "", err
		}
		break
	}
	if manifestBytes == nil {
		return nil, "", fmt.Errorf("manifest.json missing")
	}
	m, _ := ParseManifest(manifestBytes)
	if m != nil {
		iconPath = pickBestIcon(m)
	}
	return manifestBytes, iconPath, nil
}

func readFromZip(zipBytes []byte, rel string) (data []byte, mime string, ok bool) {
	if rel == "" {
		return nil, "", false
	}
	r, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, "", false
	}
	want := path.Clean(rel)
	for _, zf := range r.File {
		if path.Clean(zf.Name) != want {
			continue
		}
		rc, oerr := zf.Open()
		if oerr != nil {
			return nil, "", false
		}
		b, rerr := io.ReadAll(rc)
		rc.Close()
		if rerr != nil {
			return nil, "", false
		}
		return b, http.DetectContentType(b), true
	}
	return nil, "", false
}

func resolveMessageFromZip(zipBytes []byte, defaultLocale, placeholder string) string {
	if !strings.HasPrefix(placeholder, "__MSG_") || !strings.HasSuffix(placeholder, "__") {
		return placeholder
	}
	key := strings.TrimSuffix(strings.TrimPrefix(placeholder, "__MSG_"), "__")
	locale := defaultLocale
	if locale == "" {
		locale = "en"
	}
	b, _, ok := readFromZip(zipBytes, "_locales/"+locale+"/messages.json")
	if !ok {
		return placeholder
	}
	msgs := map[string]struct {
		Message string `json:"message"`
	}{}
	if err := json.Unmarshal(b, &msgs); err != nil {
		return placeholder
	}
	for k, v := range msgs {
		if strings.EqualFold(k, key) {
			return v.Message
		}
	}
	return placeholder
}
