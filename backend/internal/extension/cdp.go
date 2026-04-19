package extension

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
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
	return c.invokeBrowserCommand("Extensions.loadUnpacked", map[string]any{"path": unpackedPath})
}

// Unload attempts Extensions.unload by chrome_id.
func (c *CDPClient) Unload(chromeExtID string) (bool, string) {
	if !c.Capability().SupportsUnload {
		return false, "unsupported"
	}
	return c.invokeBrowserCommand("Extensions.unload", map[string]any{"id": chromeExtID})
}

var cdpCmdIDSeq int64

// invokeBrowserCommand dials the browser-level DevTools WebSocket
// (webSocketDebuggerUrl from /json/version), sends one command, and waits for
// the matching response. One connection per call keeps state trivial; 3 s
// bounds all IO so a wedged kernel can't stall the caller.
func (c *CDPClient) invokeBrowserCommand(method string, params map[string]any) (bool, string) {
	wsURL, err := c.fetchBrowserWSURL(3 * time.Second)
	if err != nil {
		return false, err.Error()
	}
	dialer := &websocket.Dialer{HandshakeTimeout: 3 * time.Second}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return false, err.Error()
	}
	defer conn.Close()

	id := atomic.AddInt64(&cdpCmdIDSeq, 1)
	_ = conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	if err := conn.WriteJSON(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return false, err.Error()
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(deadline)
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return false, err.Error()
		}
		var resp struct {
			ID    int64 `json:"id"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			// Ignore unparseable / event frames.
			continue
		}
		if resp.ID != id {
			continue
		}
		if resp.Error != nil {
			return false, resp.Error.Message
		}
		return true, ""
	}
	return false, "timeout"
}

func (c *CDPClient) fetchBrowserWSURL(timeout time.Duration) (string, error) {
	cl := &http.Client{Timeout: timeout}
	resp, err := cl.Get(fmt.Sprintf("http://127.0.0.1:%d/json/version", c.DebugPort))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var v struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return "", err
	}
	if v.WebSocketDebuggerURL == "" {
		return "", fmt.Errorf("/json/version missing webSocketDebuggerUrl")
	}
	return v.WebSocketDebuggerURL, nil
}
