package extension

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var chromeIDPattern = regexp.MustCompile(`^[a-p]{32}$`)

func chromeHostOK(h string) bool {
	h = strings.ToLower(h)
	return h == "chromewebstore.google.com" || h == "chrome.google.com"
}

func edgeHostOK(h string) bool {
	return strings.ToLower(h) == "microsoftedge.microsoft.com"
}

// ParseStoreURL extracts (vendor, chrome_id) from Chrome or Edge store detail URLs.
//
// Recognised formats:
//
//	https://chromewebstore.google.com/detail/<slug>/<id>
//	https://chrome.google.com/webstore/detail/<slug>/<id>
//	https://microsoftedge.microsoft.com/addons/detail/<slug>/<id>
//
// The chrome_id is always the last path segment and must match [a-p]{32}.
func ParseStoreURL(raw string) (StoreVendor, string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", "", err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) == 0 {
		return "", "", fmt.Errorf("cannot parse path")
	}
	last := parts[len(parts)-1]
	switch {
	case chromeHostOK(u.Host):
		if !chromeIDPattern.MatchString(last) {
			return "", "", fmt.Errorf("chrome extension id not found in URL")
		}
		return VendorChrome, last, nil
	case edgeHostOK(u.Host):
		if !chromeIDPattern.MatchString(last) {
			return "", "", fmt.Errorf("edge extension id not found in URL")
		}
		return VendorEdge, last, nil
	}
	return "", "", fmt.Errorf("unsupported store host: %s", u.Host)
}

// CRXDownloadURL returns the vendor's CRX-fetch endpoint for a given extension id.
func CRXDownloadURL(vendor StoreVendor, id string) string {
	switch vendor {
	case VendorChrome:
		return "https://clients2.google.com/service/update2/crx?response=redirect&prodversion=120.0&acceptformat=crx2,crx3&x=id%3D" + id + "%26uc"
	case VendorEdge:
		return "https://edge.microsoft.com/extensionwebstorebase/v1/crx?response=redirect&x=id%3D" + id + "%26installsource%3Dondemand%26uc"
	}
	return ""
}

// DownloadBytes fetches a URL (following redirects) with a single-shot timeout in ms.
func DownloadBytes(u string, timeoutMs int) ([]byte, error) {
	if timeoutMs <= 0 {
		timeoutMs = 15000
	}
	c := &http.Client{Timeout: time.Duration(timeoutMs) * time.Millisecond}
	resp, err := c.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("store unreachable: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// StoreMetadata is a best-effort abstraction filled from store page parsing.
// Any field may be empty; installer falls back to manifest/default values.
type StoreMetadata struct {
	Name        string
	Provider    string
	Description string
}

// FetchStoreMetadata attempts to read name/provider/description from the vendor page.
// Implementation is deliberately tolerant: any error returns an empty metadata struct
// so the installer falls back to manifest-derived values.
func FetchStoreMetadata(vendor StoreVendor, id string, timeoutMs int) StoreMetadata {
	switch vendor {
	case VendorChrome:
		return scrapeChrome(id, timeoutMs)
	case VendorEdge:
		return scrapeEdge(id, timeoutMs)
	}
	return StoreMetadata{}
}

func scrapeChrome(id string, timeoutMs int) StoreMetadata {
	u := "https://chromewebstore.google.com/detail/" + id
	body, err := DownloadBytes(u, timeoutMs)
	if err != nil {
		return StoreMetadata{}
	}
	return StoreMetadata{
		Name:        firstRegexGroup(`<meta property="og:title" content="([^"]+)">`, body),
		Description: firstRegexGroup(`<meta property="og:description" content="([^"]+)">`, body),
		Provider:    firstRegexGroup(`"offered_by_name":"([^"]+)"`, body),
	}
}

func scrapeEdge(id string, timeoutMs int) StoreMetadata {
	u := "https://microsoftedge.microsoft.com/addons/getproductdetailsbycrxid/" + id
	if body, err := DownloadBytes(u, timeoutMs); err == nil {
		return StoreMetadata{
			Name:        firstRegexGroup(`"name":"([^"]+)"`, body),
			Description: firstRegexGroup(`"shortDescription":"([^"]+)"`, body),
			Provider:    firstRegexGroup(`"publisher":"([^"]+)"`, body),
		}
	}
	htmlURL := "https://microsoftedge.microsoft.com/addons/detail/" + id
	html, err := DownloadBytes(htmlURL, timeoutMs)
	if err != nil {
		return StoreMetadata{}
	}
	return StoreMetadata{
		Name:        firstRegexGroup(`<meta property="og:title" content="([^"]+)">`, html),
		Description: firstRegexGroup(`<meta property="og:description" content="([^"]+)">`, html),
	}
}

func firstRegexGroup(pattern string, body []byte) string {
	re := regexp.MustCompile(pattern)
	m := re.FindSubmatch(body)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(string(m[1]))
}
