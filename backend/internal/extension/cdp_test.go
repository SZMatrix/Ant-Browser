package extension

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCapabilityProbeParsesProtocol(t *testing.T) {
	body := `{"domains":[{"domain":"Extensions","methods":[{"name":"loadUnpacked"},{"name":"unload"}]}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/json/protocol") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	// Parse out the port from httptest server URL.
	var port int
	_, err := Sscanf_port(srv.URL, &port)
	if err != nil {
		t.Fatal(err)
	}
	InvalidateAllCapabilities()
	c := NewCDPClient(port, "core-a")
	cap := c.Capability()
	if !cap.SupportsLoadUnpacked || !cap.SupportsUnload {
		t.Fatalf("capability wrong: %+v", cap)
	}
}

func TestCapabilityCached(t *testing.T) {
	InvalidateAllCapabilities()
	// Probe against a clearly dead port; result should still be cached.
	c := NewCDPClient(1, "core-b")
	first := c.Capability()
	if first.ProbeError == "" {
		t.Fatal("expected probe error on port 1")
	}
	second := c.Capability()
	if first != second {
		t.Fatal("capability should be the same cached pointer")
	}
	time.Sleep(10 * time.Millisecond)
	InvalidateCapability("core-b")
	third := c.Capability()
	if first == third {
		t.Fatal("after invalidate, a fresh probe pointer should be returned")
	}
}

// Sscanf_port extracts the port from an URL like http://127.0.0.1:12345.
func Sscanf_port(url string, outPort *int) (int, error) {
	// Naive parse: locate last ':' then parse int until end.
	i := strings.LastIndex(url, ":")
	if i < 0 {
		return 0, nil
	}
	p := url[i+1:]
	_, err := __parsePort(p, outPort)
	return 0, err
}

func __parsePort(s string, out *int) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	*out = n
	return n, nil
}
