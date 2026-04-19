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

// IdentifyFromStore fetches metadata from Chrome Web Store or Edge Add-ons.
// Chrome is scraped from HTML; Edge uses its public JSON product-details API
// because its store page is a client-rendered SPA with no server-side metadata.
func IdentifyFromStore(storeURL string) (*Metadata, error) {
	vendor, id, err := ParseStoreURL(storeURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrIdentifyFailed, err)
	}
	if storeURL == "" {
		return nil, fmt.Errorf("%w: empty url", ErrIdentifyFailed)
	}
	switch vendor {
	case VendorEdge:
		return identifyFromEdgeAPI(storeURL, id)
	case VendorChrome:
		return identifyFromChromeHTML(storeURL, id)
	default:
		return nil, fmt.Errorf("%w: unsupported vendor", ErrIdentifyFailed)
	}
}

func identifyFromChromeHTML(storeURL, id string) (*Metadata, error) {
	body, err := httpGetBody(storeURL, 15000)
	if err != nil {
		return nil, fmt.Errorf("%w: fetch page: %v", ErrIdentifyFailed, err)
	}
	meta := parseChromeHTML(body, id)
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
		StoreVendor: VendorChrome,
		SourceURL:   storeURL,
	}, nil
}

// The Edge Add-ons page is a client-rendered SPA — server HTML contains no
// og:* tags or inline data for name/version/developer, so scraping the page
// yields nothing. The store exposes a JSON endpoint that the SPA itself calls
// to hydrate the detail view; we call it directly.
func identifyFromEdgeAPI(storeURL, id string) (*Metadata, error) {
	apiURL := "https://microsoftedge.microsoft.com/addons/getproductdetailsbycrxid/" + id
	body, err := httpGetBody(apiURL, 15000)
	if err != nil {
		return nil, fmt.Errorf("%w: fetch edge api: %v", ErrIdentifyFailed, err)
	}
	var payload struct {
		Name             string `json:"name"`
		Developer        string `json:"developer"`
		Version          string `json:"version"`
		LogoURL          string `json:"logoUrl"`
		ShortDescription string `json:"shortDescription"`
		Description      string `json:"description"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("%w: edge json: %v", ErrIdentifyFailed, err)
	}
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		name = "未命名扩展"
	}
	provider := strings.TrimSpace(payload.Developer)
	if provider == "" {
		provider = "未知"
	}
	desc := strings.TrimSpace(payload.ShortDescription)
	if desc == "" {
		desc = strings.TrimSpace(payload.Description)
	}
	iconDataURL := ""
	if payload.LogoURL != "" {
		imgURL := payload.LogoURL
		if strings.HasPrefix(imgURL, "//") {
			imgURL = "https:" + imgURL
		}
		if b, mime, ok := fetchImage(imgURL, 8000); ok {
			iconDataURL = "data:" + mime + ";base64," + stdEncodeBase64(b)
		}
	}
	return &Metadata{
		ChromeID:    id,
		Name:        name,
		Provider:    provider,
		Description: desc,
		Version:     strings.TrimSpace(payload.Version),
		IconDataURL: iconDataURL,
		SourceType:  SourceTypeStore,
		StoreVendor: VendorEdge,
		SourceURL:   storeURL,
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

func parseChromeHTML(body []byte, id string) storeScrape {
	s := string(body)
	out := storeScrape{}
	if m := reOGTitle.FindStringSubmatch(s); len(m) == 2 {
		out.Name = stripStoreTitleSuffix(m[1], VendorChrome)
	}
	if m := reOGDesc.FindStringSubmatch(s); len(m) == 2 {
		out.Description = m[1]
	}
	if m := reOGImage.FindStringSubmatch(s); len(m) == 2 {
		if b, mime, ok := fetchImage(m[1], 8000); ok {
			out.IconDataURL = "data:" + mime + ";base64," + stdEncodeBase64(b)
		}
	}
	out.Version = scrapeChromeVersion(s)
	out.Provider = scrapeChromeProvider(s, id)
	return out
}

func stripStoreTitleSuffix(title string, vendor StoreVendor) string {
	for _, suffix := range []string{
		" - Chrome Web Store",
		" - Microsoft Edge Addons",
		" - Microsoft Edge Add-ons",
		" - Chrome 线上应用店",
	} {
		if strings.HasSuffix(title, suffix) {
			return strings.TrimSpace(strings.TrimSuffix(title, suffix))
		}
	}
	_ = vendor
	return strings.TrimSpace(title)
}

var (
	// Label/value card rendered on the Chrome Web Store detail page.
	// Matches both English ("Version") and Simplified Chinese ("版本").
	reVersionLabel = regexp.MustCompile(`>(?:Version|版本)</div>\s*<div[^>]*>\s*([^<]+?)\s*</div>`)
	// Inline manifest.json in the ds:0 AF_initDataCallback blob is embedded as
	// a JS string literal, so its JSON quotes arrive escaped as \".
	reVersionEscapedJSON = regexp.MustCompile(`\\"version\\"\s*:\s*\\"([0-9][0-9A-Za-z.\-]+)\\"`)
	reVersionJSON        = regexp.MustCompile(`"version"\s*:\s*"([0-9][0-9A-Za-z.\-]+)"`)
)

func scrapeChromeVersion(body string) string {
	if m := reVersionLabel.FindStringSubmatch(body); len(m) == 2 {
		if v := strings.TrimSpace(m[1]); v != "" {
			return v
		}
	}
	if m := reVersionEscapedJSON.FindStringSubmatch(body); len(m) == 2 {
		return m[1]
	}
	if m := reVersionJSON.FindStringSubmatch(body); len(m) == 2 {
		return m[1]
	}
	return ""
}

// The publisher display name shown next to the extension title lives in the
// ds:0 inline-data array. We anchor on the known extension id because class
// names are minified and names are not unique.
// Array shape: [id, iconURL, name, rating, reviewCount, promoURL, shortDesc, publisher, ...]
var rePublisherNearTitle = regexp.MustCompile(`<a[^>]*class="[^"]*cJI8ee[^"]*"[^>]*>([^<]+)</a>`)

func scrapeChromeProvider(body, id string) string {
	if id != "" {
		pat := `"` + regexp.QuoteMeta(id) + `","[^"]+","[^"]+",[0-9.]+,[0-9]+,"[^"]+","[^"]*","([^"]+)"`
		if re, err := regexp.Compile(pat); err == nil {
			if m := re.FindStringSubmatch(body); len(m) == 2 {
				if v := strings.TrimSpace(m[1]); v != "" {
					return v
				}
			}
		}
	}
	if m := rePublisherNearTitle.FindStringSubmatch(body); len(m) == 2 {
		return strings.TrimSpace(m[1])
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
