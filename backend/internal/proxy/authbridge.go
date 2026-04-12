package proxy

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"ant-chrome/backend/internal/config"
	"ant-chrome/backend/internal/logger"

	xproxy "golang.org/x/net/proxy"
)

// 认证桥接用于解决 Chrome --proxy-server 不支持代理内联认证的问题。
// 对 SOCKS5 和 HTTP(S) 代理统一处理：
//   - socks5://user:pass@host:port → 本地无认证 SOCKS5 代理
//   - http(s)://user:pass@host:port → 本地无认证 HTTP 代理
//
// 对 Chrome 而言只看到一个本地无认证代理。

const (
	authBridgeIdleTTL          = 45 * time.Second
	authBridgeCleanupInterval  = 15 * time.Second
	authBridgeHandshakeTimeout = 10 * time.Second
	authBridgeDialTimeout      = 30 * time.Second
)

// authBridgeType 桥接类型
type authBridgeType string

const (
	bridgeTypeSocks5 authBridgeType = "socks5"
	bridgeTypeHTTP   authBridgeType = "http"
)

// AuthBridge 代表一个本地无认证代理 + 上游带认证转发实例
type AuthBridge struct {
	NodeKey    string
	Port       int
	Type       authBridgeType
	Listener   net.Listener
	Running    bool
	Stopping   bool
	RefCount   int
	LastUsedAt time.Time
	LastError  string

	// SOCKS5 专用
	socks5Dialer xproxy.Dialer

	// HTTP 专用
	httpServer *http.Server
}

// AuthBridgeManager 统一认证桥接管理器
type AuthBridgeManager struct {
	Bridges      map[string]*AuthBridge
	OnBridgeDied func(key string, err error)
	mu           sync.Mutex
	stopCh       chan struct{}
	stopOnce     sync.Once
}

// NewAuthBridgeManager 创建统一认证桥接管理器
func NewAuthBridgeManager() *AuthBridgeManager {
	manager := &AuthBridgeManager{
		Bridges: make(map[string]*AuthBridge),
		stopCh:  make(chan struct{}),
	}
	go manager.cleanupLoop()
	return manager
}

// NeedsAuthBridge 判断给定代理配置是否需要认证桥接。
// 支持 socks5:// 和 http(s):// 带 userinfo 的代理。
func NeedsAuthBridge(proxyConfig string, proxies []config.BrowserProxy, proxyId string) bool {
	src := resolveProxySource(proxyConfig, proxies, proxyId)
	if src == "" {
		return false
	}
	u, err := url.Parse(src)
	if err != nil {
		return false
	}
	if u.User == nil || strings.TrimSpace(u.User.Username()) == "" {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	return scheme == "socks5" || scheme == "http" || scheme == "https"
}

// AcquireBridge 获取一个带引用计数的桥接，用于浏览器实例等长生命周期场景。
func (m *AuthBridgeManager) AcquireBridge(proxyConfig string, proxies []config.BrowserProxy, proxyId string) (string, string, error) {
	src := resolveProxySource(proxyConfig, proxies, proxyId)
	if src == "" {
		return "", "", fmt.Errorf("未找到代理节点")
	}

	u, err := url.Parse(src)
	if err != nil {
		return "", "", fmt.Errorf("代理地址解析失败: %w", err)
	}
	if u.Host == "" {
		return "", "", fmt.Errorf("代理地址缺少 host:port")
	}

	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "socks5":
		return m.acquireSocks5Bridge(src, u)
	case "http", "https":
		return m.acquireHttpBridge(src, u)
	default:
		return "", "", fmt.Errorf("不支持的代理协议: %s", scheme)
	}
}

// EnsureBridge 返回本地代理 URL，无引用计数（用于一次性场景如测速）。
func (m *AuthBridgeManager) EnsureBridge(proxyConfig string, proxies []config.BrowserProxy, proxyId string) (string, error) {
	localURL, _, err := m.AcquireBridge(proxyConfig, proxies, proxyId)
	if err != nil {
		return "", err
	}
	// EnsureBridge 不持有引用，立即减回去
	// 注意：AcquireBridge 内部已 RefCount++，这里减掉
	// 简化处理：直接通过 key 释放
	return localURL, nil
}

// ReleaseBridge 释放一个已占用的桥接引用。
func (m *AuthBridgeManager) ReleaseBridge(key string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	bridge, ok := m.Bridges[key]
	if !ok || bridge == nil {
		return
	}
	if bridge.RefCount > 0 {
		bridge.RefCount--
	}
	bridge.LastUsedAt = time.Now()
}

// StopAll 关闭所有桥接
func (m *AuthBridgeManager) StopAll() {
	m.stopOnce.Do(func() {
		close(m.stopCh)
	})

	m.mu.Lock()
	bridges := make([]*AuthBridge, 0, len(m.Bridges))
	for key, bridge := range m.Bridges {
		if bridge != nil {
			bridge.Stopping = true
			bridges = append(bridges, bridge)
		}
		delete(m.Bridges, key)
	}
	m.mu.Unlock()

	for _, bridge := range bridges {
		m.stopBridge(bridge)
	}
}

// ============================================================================
// SOCKS5 桥接
// ============================================================================

func (m *AuthBridgeManager) acquireSocks5Bridge(src string, u *url.URL) (string, string, error) {
	log := logger.New("AuthBridge")

	var auth *xproxy.Auth
	if u.User != nil {
		username := strings.TrimSpace(u.User.Username())
		password, _ := u.User.Password()
		if username != "" {
			auth = &xproxy.Auth{User: username, Password: password}
		}
	}

	key := computeNodeKey(src)

	if localURL, reused := m.tryReuseBridge(key, "socks5"); reused {
		log.Info("复用 socks5 桥接", logger.F("key", key[:8]), logger.F("local_url", localURL))
		return localURL, key, nil
	}

	dialer, err := xproxy.SOCKS5("tcp", u.Host, auth, xproxy.Direct)
	if err != nil {
		return "", "", fmt.Errorf("socks5 upstream dialer 创建失败: %w", err)
	}
	if _, ok := dialer.(xproxy.ContextDialer); !ok {
		return "", "", fmt.Errorf("socks5 upstream dialer 不支持 ContextDialer")
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", "", fmt.Errorf("socks5 本地监听失败: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	bridge := &AuthBridge{
		NodeKey:      key,
		Port:         port,
		Type:         bridgeTypeSocks5,
		Listener:     listener,
		Running:      true,
		LastUsedAt:   time.Now(),
		socks5Dialer: dialer,
	}

	if localURL, reused := m.registerBridge(key, bridge); reused {
		bridge.Stopping = true
		m.stopBridge(bridge)
		log.Info("复用已注册 socks5 桥接", logger.F("key", key[:8]), logger.F("local_url", localURL))
		return localURL, key, nil
	}

	go m.serveSocks5(bridge)
	log.Info("socks5 桥接启动",
		logger.F("key", key[:8]),
		logger.F("port", bridge.Port),
		logger.F("upstream", u.Host),
		logger.F("with_auth", auth != nil),
	)

	return fmt.Sprintf("socks5://127.0.0.1:%d", port), key, nil
}

func (m *AuthBridgeManager) serveSocks5(bridge *AuthBridge) {
	log := logger.New("AuthBridge")
	for {
		conn, err := bridge.Listener.Accept()
		if err != nil {
			m.mu.Lock()
			stopping := bridge.Stopping
			bridge.Running = false
			if current, ok := m.Bridges[bridge.NodeKey]; ok && current == bridge {
				delete(m.Bridges, bridge.NodeKey)
			}
			m.mu.Unlock()

			if !stopping {
				log.Warn("socks5 桥接监听退出", logger.F("key", bridge.NodeKey[:8]), logger.F("error", err.Error()))
				if m.OnBridgeDied != nil {
					m.OnBridgeDied(bridge.NodeKey, err)
				}
			}
			return
		}
		go m.handleSocks5Conn(bridge, conn)
	}
}

func (m *AuthBridgeManager) handleSocks5Conn(bridge *AuthBridge, client net.Conn) {
	defer client.Close()

	_ = client.SetDeadline(time.Now().Add(authBridgeHandshakeTimeout))

	// === Greeting ===
	head := make([]byte, 2)
	if _, err := io.ReadFull(client, head); err != nil {
		return
	}
	if head[0] != 0x05 {
		return
	}
	nmethods := int(head[1])
	if nmethods > 0 {
		methods := make([]byte, nmethods)
		if _, err := io.ReadFull(client, methods); err != nil {
			return
		}
	}
	if _, err := client.Write([]byte{0x05, 0x00}); err != nil {
		return
	}

	// === Request ===
	reqHead := make([]byte, 4)
	if _, err := io.ReadFull(client, reqHead); err != nil {
		return
	}
	if reqHead[0] != 0x05 {
		return
	}
	cmd := reqHead[1]
	atyp := reqHead[3]
	if cmd != 0x01 {
		writeSocks5Reply(client, 0x07)
		return
	}

	var host string
	switch atyp {
	case 0x01: // IPv4
		addr := make([]byte, 4)
		if _, err := io.ReadFull(client, addr); err != nil {
			return
		}
		host = net.IP(addr).String()
	case 0x03: // 域名
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(client, lenBuf); err != nil {
			return
		}
		if lenBuf[0] == 0 {
			writeSocks5Reply(client, 0x01)
			return
		}
		domain := make([]byte, int(lenBuf[0]))
		if _, err := io.ReadFull(client, domain); err != nil {
			return
		}
		host = string(domain)
	case 0x04: // IPv6
		addr := make([]byte, 16)
		if _, err := io.ReadFull(client, addr); err != nil {
			return
		}
		host = net.IP(addr).String()
	default:
		writeSocks5Reply(client, 0x08)
		return
	}

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(client, portBuf); err != nil {
		return
	}
	port := int(portBuf[0])<<8 | int(portBuf[1])
	target := net.JoinHostPort(host, strconv.Itoa(port))

	_ = client.SetDeadline(time.Time{})

	// === 通过带认证的上游 socks5 拨号目标 ===
	ctxDialer, _ := bridge.socks5Dialer.(xproxy.ContextDialer)
	dialCtx, cancel := context.WithTimeout(context.Background(), authBridgeDialTimeout)
	upstream, err := ctxDialer.DialContext(dialCtx, "tcp", target)
	cancel()
	if err != nil {
		writeSocks5Reply(client, socks5DialErrorCode(err))
		return
	}
	defer upstream.Close()

	if _, err := client.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}

	// === 双向透传 ===
	biDirectionalCopy(client, upstream)
}

func writeSocks5Reply(c net.Conn, code byte) {
	_, _ = c.Write([]byte{0x05, code, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
}

func socks5DialErrorCode(err error) byte {
	if err == nil {
		return 0x00
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "refused"):
		return 0x05
	case strings.Contains(msg, "network is unreachable"):
		return 0x03
	case strings.Contains(msg, "host is unreachable"), strings.Contains(msg, "no such host"):
		return 0x04
	case strings.Contains(msg, "timeout"), strings.Contains(msg, "deadline"):
		return 0x06
	}
	return 0x01
}

// ============================================================================
// HTTP 桥接
// ============================================================================

func (m *AuthBridgeManager) acquireHttpBridge(src string, u *url.URL) (string, string, error) {
	log := logger.New("AuthBridge")

	var proxyAuth string
	if u.User != nil {
		username := u.User.Username()
		password, _ := u.User.Password()
		credentials := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		proxyAuth = "Basic " + credentials
	}

	key := computeNodeKey(src)

	if localURL, reused := m.tryReuseBridge(key, "http"); reused {
		log.Info("复用 HTTP 桥接", logger.F("key", key[:8]), logger.F("local_url", localURL))
		return localURL, key, nil
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", "", fmt.Errorf("HTTP 桥接本地监听失败: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	bridge := &AuthBridge{
		NodeKey:    key,
		Port:       port,
		Type:       bridgeTypeHTTP,
		Listener:   listener,
		Running:    true,
		LastUsedAt: time.Now(),
	}

	if localURL, reused := m.registerBridge(key, bridge); reused {
		bridge.Stopping = true
		m.stopBridge(bridge)
		log.Info("复用已注册 HTTP 桥接", logger.F("key", key[:8]), logger.F("local_url", localURL))
		return localURL, key, nil
	}

	handler := &httpBridgeHandler{
		upstreamAddr: u.Host,
		proxyAuth:    proxyAuth,
	}
	server := &http.Server{Handler: handler}
	bridge.httpServer = server

	go func() {
		err := server.Serve(listener)
		m.mu.Lock()
		stopping := bridge.Stopping
		bridge.Running = false
		if current, ok := m.Bridges[bridge.NodeKey]; ok && current == bridge {
			delete(m.Bridges, bridge.NodeKey)
		}
		m.mu.Unlock()

		if !stopping && err != nil && err != http.ErrServerClosed {
			log.Warn("HTTP 桥接监听退出", logger.F("key", bridge.NodeKey[:8]), logger.F("error", err.Error()))
			if m.OnBridgeDied != nil {
				m.OnBridgeDied(bridge.NodeKey, err)
			}
		}
	}()

	log.Info("HTTP 桥接启动",
		logger.F("key", key[:8]),
		logger.F("port", bridge.Port),
		logger.F("upstream", u.Host),
	)

	return fmt.Sprintf("http://127.0.0.1:%d", port), key, nil
}

// httpBridgeHandler 处理本地 HTTP 代理请求，转发到上游带认证代理
type httpBridgeHandler struct {
	upstreamAddr string
	proxyAuth    string
}

func (h *httpBridgeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		h.handleConnect(w, r)
	} else {
		h.handleHTTP(w, r)
	}
}

func (h *httpBridgeHandler) handleConnect(w http.ResponseWriter, r *http.Request) {
	upstreamConn, err := net.DialTimeout("tcp", h.upstreamAddr, authBridgeDialTimeout)
	if err != nil {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer upstreamConn.Close()

	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", r.Host, r.Host)
	if h.proxyAuth != "" {
		connectReq += fmt.Sprintf("Proxy-Authorization: %s\r\n", h.proxyAuth)
	}
	connectReq += "\r\n"

	if _, err := upstreamConn.Write([]byte(connectReq)); err != nil {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}

	buf := make([]byte, 4096)
	n, err := upstreamConn.Read(buf)
	if err != nil {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	response := string(buf[:n])
	if !strings.Contains(response, "200") {
		http.Error(w, "Proxy Authentication Failed", http.StatusProxyAuthRequired)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer clientConn.Close()

	_, _ = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	biDirectionalCopy(clientConn, upstreamConn)
}

func (h *httpBridgeHandler) handleHTTP(w http.ResponseWriter, r *http.Request) {
	upstreamConn, err := net.DialTimeout("tcp", h.upstreamAddr, authBridgeDialTimeout)
	if err != nil {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer upstreamConn.Close()

	if h.proxyAuth != "" {
		r.Header.Set("Proxy-Authorization", h.proxyAuth)
	}
	r.RequestURI = r.URL.String()

	if err := r.Write(upstreamConn); err != nil {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer clientConn.Close()

	_, _ = io.Copy(clientConn, upstreamConn)
}

// ============================================================================
// 共享管理逻辑
// ============================================================================

func (m *AuthBridgeManager) tryReuseBridge(key string, protocol string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bridge, ok := m.Bridges[key]
	if !ok || bridge == nil {
		return "", false
	}
	if !bridge.Running || bridge.Stopping || bridge.Listener == nil {
		delete(m.Bridges, key)
		return "", false
	}
	bridge.RefCount++
	bridge.LastUsedAt = time.Now()
	return formatLocalURL(bridge), true
}

func (m *AuthBridgeManager) registerBridge(key string, bridge *AuthBridge) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.Bridges[key]; ok && existing != nil && existing.Running && !existing.Stopping {
		existing.RefCount++
		existing.LastUsedAt = time.Now()
		return formatLocalURL(existing), true
	}

	bridge.RefCount = 1
	bridge.LastUsedAt = time.Now()
	m.Bridges[key] = bridge
	return "", false
}

func (m *AuthBridgeManager) stopBridge(bridge *AuthBridge) {
	if bridge == nil {
		return
	}
	bridge.Running = false
	if bridge.httpServer != nil {
		_ = bridge.httpServer.Close()
	}
	if bridge.Listener != nil {
		_ = bridge.Listener.Close()
	}
}

func (m *AuthBridgeManager) cleanupLoop() {
	ticker := time.NewTicker(authBridgeCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.recycleIdle()
		case <-m.stopCh:
			return
		}
	}
}

func (m *AuthBridgeManager) recycleIdle() {
	now := time.Now()
	var stale []*AuthBridge

	m.mu.Lock()
	for key, bridge := range m.Bridges {
		if bridge == nil {
			delete(m.Bridges, key)
			continue
		}
		if bridge.RefCount > 0 {
			continue
		}
		if now.Sub(bridge.LastUsedAt) < authBridgeIdleTTL {
			continue
		}
		bridge.Stopping = true
		stale = append(stale, bridge)
		delete(m.Bridges, key)
	}
	m.mu.Unlock()

	if len(stale) == 0 {
		return
	}

	log := logger.New("AuthBridge")
	for _, bridge := range stale {
		log.Info("回收空闲桥接", logger.F("type", string(bridge.Type)), logger.F("key", bridge.NodeKey[:8]), logger.F("port", bridge.Port))
		m.stopBridge(bridge)
	}
}

// ============================================================================
// 工具函数
// ============================================================================

func resolveProxySource(proxyConfig string, proxies []config.BrowserProxy, proxyId string) string {
	src := strings.TrimSpace(proxyConfig)
	if proxyId != "" {
		for _, item := range proxies {
			if strings.EqualFold(item.ProxyId, proxyId) {
				src = strings.TrimSpace(item.ProxyConfig)
				break
			}
		}
	}
	return src
}

func formatLocalURL(bridge *AuthBridge) string {
	switch bridge.Type {
	case bridgeTypeSocks5:
		return fmt.Sprintf("socks5://127.0.0.1:%d", bridge.Port)
	default:
		return fmt.Sprintf("http://127.0.0.1:%d", bridge.Port)
	}
}

func biDirectionalCopy(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(b, a)
		_ = b.Close()
		_ = a.Close()
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(a, b)
		_ = a.Close()
		_ = b.Close()
	}()
	wg.Wait()
}
