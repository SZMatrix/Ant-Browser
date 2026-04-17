package extension

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseStoreURLChrome(t *testing.T) {
	cases := []string{
		"https://chromewebstore.google.com/detail/my-ext/abcdefghijklmnopabcdefghijklmnop",
		"https://chrome.google.com/webstore/detail/my-ext/abcdefghijklmnopabcdefghijklmnop",
		"https://chromewebstore.google.com/detail/abcdefghijklmnopabcdefghijklmnop?hl=en",
	}
	for _, u := range cases {
		vendor, id, err := ParseStoreURL(u)
		if err != nil || vendor != VendorChrome || len(id) != 32 {
			t.Fatalf("case %q: vendor=%q id=%q err=%v", u, vendor, id, err)
		}
	}
}

func TestParseStoreURLEdge(t *testing.T) {
	u := "https://microsoftedge.microsoft.com/addons/detail/my-ext/mpognobbkildjkofajifpdfhcoklimli"
	vendor, id, err := ParseStoreURL(u)
	if err != nil || vendor != VendorEdge || id != "mpognobbkildjkofajifpdfhcoklimli" {
		t.Fatalf("vendor=%q id=%q err=%v", vendor, id, err)
	}
}

func TestParseStoreURLRejectsOthers(t *testing.T) {
	if _, _, err := ParseStoreURL("https://example.com/whatever"); err == nil {
		t.Fatal("expected error")
	}
}

func TestDownloadCRXFollowsRedirect(t *testing.T) {
	body := append([]byte("Cr24"), make([]byte, 32)...)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/final" {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(body)
			return
		}
		w.Header().Set("Location", "/final")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	raw, err := DownloadBytes(srv.URL+"/start", 5_000)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(raw[:4]), "Cr24") {
		t.Fatalf("want Cr24 magic, got %q", string(raw[:4]))
	}
}
