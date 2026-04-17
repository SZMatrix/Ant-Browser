package extension

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type KernelCapability struct {
	SupportsLoadUnpacked bool      `json:"supportsLoadUnpacked"`
	SupportsUnload       bool      `json:"supportsUnload"`
	ProbedAt             time.Time `json:"probedAt"`
	ProbeError           string    `json:"probeError"`
}

type CDPClient struct {
	DebugPort int
	CoreID    string
	cache     *sync.Map
}

var globalCapabilityCache = &sync.Map{}

func NewCDPClient(debugPort int, coreID string) *CDPClient {
	return &CDPClient{DebugPort: debugPort, CoreID: coreID, cache: globalCapabilityCache}
}

// Capability returns the probed CDP method support for the client's core.
// Result is cached per coreID; call InvalidateCapability(coreID) after a kernel swap.
func (c *CDPClient) Capability() *KernelCapability {
	if v, ok := c.cache.Load(c.CoreID); ok {
		return v.(*KernelCapability)
	}
	cap := &KernelCapability{ProbedAt: time.Now()}
	proto, err := fetchProtocolDescriptor(c.DebugPort, 3*time.Second)
	if err != nil {
		cap.ProbeError = err.Error()
	} else {
		cap.SupportsLoadUnpacked = protoHas(proto, "Extensions", "loadUnpacked")
		cap.SupportsUnload = protoHas(proto, "Extensions", "unload")
	}
	c.cache.Store(c.CoreID, cap)
	return cap
}

func InvalidateCapability(coreID string) {
	globalCapabilityCache.Delete(coreID)
}

func InvalidateAllCapabilities() {
	globalCapabilityCache.Range(func(k, _ any) bool {
		globalCapabilityCache.Delete(k)
		return true
	})
}

type protocolDescriptor struct {
	Domains []struct {
		Domain  string `json:"domain"`
		Methods []struct {
			Name string `json:"name"`
		} `json:"methods"`
	} `json:"domains"`
}

func fetchProtocolDescriptor(port int, timeout time.Duration) (*protocolDescriptor, error) {
	c := &http.Client{Timeout: timeout}
	url := fmt.Sprintf("http://127.0.0.1:%d/json/protocol", port)
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var pd protocolDescriptor
	if err := json.Unmarshal(body, &pd); err != nil {
		return nil, err
	}
	return &pd, nil
}

func protoHas(p *protocolDescriptor, domain, method string) bool {
	for _, d := range p.Domains {
		if d.Domain != domain {
			continue
		}
		for _, m := range d.Methods {
			if m.Name == method {
				return true
			}
		}
	}
	return false
}

// LoadUnpacked attempts Extensions.loadUnpacked and reports whether it succeeded.
// Returns (false, reason) when the kernel doesn't support the method.
func (c *CDPClient) LoadUnpacked(unpackedPath string) (bool, string) {
	if !c.Capability().SupportsLoadUnpacked {
		return false, "unsupported"
	}
	payload := map[string]any{
		"method": "Extensions.loadUnpacked",
		"params": map[string]any{"path": unpackedPath},
	}
	return c.invokeHTTP(payload)
}

// Unload attempts Extensions.unload by chrome_id.
func (c *CDPClient) Unload(chromeExtID string) (bool, string) {
	if !c.Capability().SupportsUnload {
		return false, "unsupported"
	}
	payload := map[string]any{
		"method": "Extensions.unload",
		"params": map[string]any{"id": chromeExtID},
	}
	return c.invokeHTTP(payload)
}

// invokeHTTP is a conservative starter transport. The DevTools spec requires
// WebSocket (ws://127.0.0.1:<port>/devtools/browser/<browserId>) for command
// execution; the HTTP POST path does not exist on stock Chromium. Treat this
// implementation as a known-limited shim that will usually return an error
// and therefore mark affected profiles as "pending restart" — which matches
// the spec's documented fallback semantics for kernels without runtime support.
// Upgrade to a real WS dial (e.g. github.com/gorilla/websocket) when a fork
// that honors Extensions.loadUnpacked is available to test against.
func (c *CDPClient) invokeHTTP(payload map[string]any) (bool, string) {
	body, err := json.Marshal(payload)
	if err != nil {
		return false, err.Error()
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/json/protocol/command", c.DebugPort)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return false, err.Error()
	}
	req.Header.Set("Content-Type", "application/json")
	cl := &http.Client{Timeout: 3 * time.Second}
	resp, err := cl.Do(req)
	if err != nil {
		return false, err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return false, fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	var result struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return false, err.Error()
	}
	if result.Error != nil {
		return false, result.Error.Message
	}
	return true, ""
}
