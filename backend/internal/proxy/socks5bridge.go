package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"ant-chrome/backend/internal/config"
	"ant-chrome/backend/internal/logger"

	xproxy "golang.org/x/net/proxy"
)

// Socks5 桥接用于解决 Chrome --proxy-server 不支持 SOCKS5 用户名/密码内联认证的问题。
// 做法：本地监听一个无认证 SOCKS5 服务，收到的 CONNECT 请求由本进程再以带认证的
// 方式转发到上游 SOCKS5。对 Chrome 而言只看到一个本地无认证代理。
//
// 仅支持 TCP CONNECT。UDP ASSOCIATE 与 BIND 未实现，常规浏览场景不会使用。

const (
	socks5BridgeIdleTTL         = 45 * time.Second
	socks5BridgeCleanupInterval = 15 * time.Second
	socks5BridgeHandshakeTimeout = 10 * time.Second
	socks5BridgeDialTimeout     = 30 * time.Second
)

// Socks5Bridge 代表一个本地 SOCKS5 无认证监听 + 上游带认证转发实例
type Socks5Bridge struct {
	NodeKey      string
	Port         int
	Upstream     string // host:port
	UpstreamAuth *xproxy.Auth
	dialer       xproxy.Dialer

	Listener   net.Listener
	Running    bool
	Stopping   bool
	RefCount   int
	LastUsedAt time.Time
	LastError  string
}

// Socks5BridgeManager Socks5 桥接管理器
type Socks5BridgeManager struct {
	Bridges      map[string]*Socks5Bridge
	OnBridgeDied func(key string, err error)
	mu           sync.Mutex
	stopCh       chan struct{}
	stopOnce     sync.Once
}

// NewSocks5BridgeManager 创建 Socks5 桥接管理器
func NewSocks5BridgeManager() *Socks5BridgeManager {
	manager := &Socks5BridgeManager{
		Bridges: make(map[string]*Socks5Bridge),
		stopCh:  make(chan struct{}),
	}
	go manager.cleanupLoop()
	return manager
}

// NeedsSocks5AuthBridge 判断给定代理配置是否为带认证的 socks5://。
// 仅当 proxyConfig 为 socks5:// 且 userinfo 非空时返回 true。
func NeedsSocks5AuthBridge(proxyConfig string, proxies []config.BrowserProxy, proxyId string) bool {
	src := strings.TrimSpace(proxyConfig)
	if proxyId != "" {
		for _, item := range proxies {
			if strings.EqualFold(item.ProxyId, proxyId) {
				src = strings.TrimSpace(item.ProxyConfig)
				break
			}
		}
	}
	if !strings.HasPrefix(strings.ToLower(src), "socks5://") {
		return false
	}
	u, err := url.Parse(src)
	if err != nil {
		return false
	}
	if u.User == nil {
		return false
	}
	return strings.TrimSpace(u.User.Username()) != ""
}

// EnsureBridge 返回本地无认证 socks5://127.0.0.1:port，无引用计数。
func (m *Socks5BridgeManager) EnsureBridge(proxyConfig string, proxies []config.BrowserProxy, proxyId string) (string, error) {
	socksURL, _, err := m.ensureBridge(proxyConfig, proxies, proxyId, false)
	return socksURL, err
}

// AcquireBridge 获取一个带引用计数的桥接，用于浏览器实例等长生命周期场景。
func (m *Socks5BridgeManager) AcquireBridge(proxyConfig string, proxies []config.BrowserProxy, proxyId string) (string, string, error) {
	return m.ensureBridge(proxyConfig, proxies, proxyId, true)
}

// ReleaseBridge 释放一个已占用的桥接引用。
func (m *Socks5BridgeManager) ReleaseBridge(key string) {
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
func (m *Socks5BridgeManager) StopAll() {
	m.stopOnce.Do(func() {
		close(m.stopCh)
	})

	m.mu.Lock()
	bridges := make([]*Socks5Bridge, 0, len(m.Bridges))
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

func (m *Socks5BridgeManager) ensureBridge(proxyConfig string, proxies []config.BrowserProxy, proxyId string, pin bool) (string, string, error) {
	log := logger.New("Socks5Bridge")
	src := strings.TrimSpace(proxyConfig)
	if proxyId != "" {
		for _, item := range proxies {
			if strings.EqualFold(item.ProxyId, proxyId) {
				src = strings.TrimSpace(item.ProxyConfig)
				break
			}
		}
	}
	if src == "" {
		return "", "", fmt.Errorf("未找到代理节点")
	}
	if !strings.HasPrefix(strings.ToLower(src), "socks5://") {
		return "", "", fmt.Errorf("非 socks5 代理，无法通过 socks5 桥接")
	}

	u, err := url.Parse(src)
	if err != nil {
		return "", "", fmt.Errorf("socks5 地址解析失败: %w", err)
	}
	if u.Host == "" {
		return "", "", fmt.Errorf("socks5 地址缺少 host:port")
	}

	var auth *xproxy.Auth
	if u.User != nil {
		username := strings.TrimSpace(u.User.Username())
		password, _ := u.User.Password()
		if username != "" {
			auth = &xproxy.Auth{User: username, Password: password}
		}
	}

	key := computeNodeKey(src)

	if socksURL, reused := m.tryReuseBridge(key, pin); reused {
		log.Info("复用 socks5 桥接", logger.F("key", key[:8]), logger.F("socks_url", socksURL))
		return socksURL, key, nil
	}

	// 创建 upstream dialer（xproxy.SOCKS5 本身不发起连接，只保存参数）
	dialer, err := xproxy.SOCKS5("tcp", u.Host, auth, xproxy.Direct)
	if err != nil {
		return "", "", fmt.Errorf("socks5 upstream dialer 创建失败: %w", err)
	}
	if _, ok := dialer.(xproxy.ContextDialer); !ok {
		return "", "", fmt.Errorf("socks5 upstream dialer 不支持 ContextDialer")
	}

	// 本地监听端口（由 OS 分配）
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", "", fmt.Errorf("socks5 本地监听失败: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	bridge := &Socks5Bridge{
		NodeKey:      key,
		Port:         port,
		Upstream:     u.Host,
		UpstreamAuth: auth,
		dialer:       dialer,
		Listener:     listener,
		Running:      true,
		LastUsedAt:   time.Now(),
	}

	if socksURL, reused := m.registerBridge(key, bridge, pin); reused {
		// 已有可用桥接，关闭本次新建的
		bridge.Stopping = true
		m.stopBridge(bridge)
		log.Info("复用已注册 socks5 桥接", logger.F("key", key[:8]), logger.F("socks_url", socksURL))
		return socksURL, key, nil
	}

	go m.serve(bridge)
	log.Info("socks5 桥接启动",
		logger.F("key", key[:8]),
		logger.F("port", bridge.Port),
		logger.F("upstream", u.Host),
		logger.F("with_auth", auth != nil),
	)

	return fmt.Sprintf("socks5://127.0.0.1:%d", port), key, nil
}

func (m *Socks5BridgeManager) tryReuseBridge(key string, pin bool) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bridge, ok := m.Bridges[key]
	if !ok || bridge == nil {
		return "", false
	}
	if !bridge.Running || bridge.Stopping || bridge.Listener == nil {
		// 死桥接，清理掉让调用方重建
		delete(m.Bridges, key)
		return "", false
	}
	if pin {
		bridge.RefCount++
	}
	bridge.LastUsedAt = time.Now()
	return fmt.Sprintf("socks5://127.0.0.1:%d", bridge.Port), true
}

func (m *Socks5BridgeManager) registerBridge(key string, bridge *Socks5Bridge, pin bool) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.Bridges[key]; ok && existing != nil && existing.Running && !existing.Stopping {
		if pin {
			existing.RefCount++
		}
		existing.LastUsedAt = time.Now()
		return fmt.Sprintf("socks5://127.0.0.1:%d", existing.Port), true
	}

	if pin {
		bridge.RefCount = 1
	}
	bridge.LastUsedAt = time.Now()
	m.Bridges[key] = bridge
	return "", false
}

// serve 循环接受客户端连接，为每个连接启动处理协程
func (m *Socks5BridgeManager) serve(bridge *Socks5Bridge) {
	log := logger.New("Socks5Bridge")
	for {
		conn, err := bridge.Listener.Accept()
		if err != nil {
			// 主动关闭或异常退出
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
		go m.handleConn(bridge, conn)
	}
}

// handleConn 处理一个客户端 SOCKS5 请求并转发到上游
func (m *Socks5BridgeManager) handleConn(bridge *Socks5Bridge, client net.Conn) {
	defer client.Close()

	// 握手阶段设置一个较短的 deadline，防止慢客户端卡住 goroutine
	_ = client.SetDeadline(time.Now().Add(socks5BridgeHandshakeTimeout))

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
		// 仅支持 CONNECT
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

	// 握手完成，解除 deadline（之后靠上游连接超时或远端关闭来收敛）
	_ = client.SetDeadline(time.Time{})

	// === 通过带认证的上游 socks5 拨号目标 ===
	ctxDialer, _ := bridge.dialer.(xproxy.ContextDialer)
	dialCtx, cancel := context.WithTimeout(context.Background(), socks5BridgeDialTimeout)
	upstream, err := ctxDialer.DialContext(dialCtx, "tcp", target)
	cancel()
	if err != nil {
		writeSocks5Reply(client, socks5DialErrorCode(err))
		return
	}
	defer upstream.Close()

	// 回成功响应，BND.ADDR/BND.PORT 填 0 即可
	if _, err := client.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}

	// === 双向透传 ===
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(upstream, client)
		_ = upstream.Close()
		_ = client.Close()
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(client, upstream)
		_ = client.Close()
		_ = upstream.Close()
	}()
	wg.Wait()
}

func writeSocks5Reply(c net.Conn, code byte) {
	_, _ = c.Write([]byte{0x05, code, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
}

// socks5DialErrorCode 将 Go 错误映射为 SOCKS5 REP 字段
func socks5DialErrorCode(err error) byte {
	if err == nil {
		return 0x00
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "refused"):
		return 0x05 // Connection refused
	case strings.Contains(msg, "network is unreachable"):
		return 0x03 // Network unreachable
	case strings.Contains(msg, "host is unreachable"), strings.Contains(msg, "no such host"):
		return 0x04 // Host unreachable
	case strings.Contains(msg, "timeout"), strings.Contains(msg, "deadline"):
		return 0x06 // TTL expired (最接近 timeout)
	}
	return 0x01 // general SOCKS server failure
}

func (m *Socks5BridgeManager) stopBridge(bridge *Socks5Bridge) {
	if bridge == nil {
		return
	}
	bridge.Running = false
	if bridge.Listener != nil {
		_ = bridge.Listener.Close()
	}
}

func (m *Socks5BridgeManager) cleanupLoop() {
	ticker := time.NewTicker(socks5BridgeCleanupInterval)
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

func (m *Socks5BridgeManager) recycleIdle() {
	now := time.Now()
	var stale []*Socks5Bridge

	m.mu.Lock()
	for key, bridge := range m.Bridges {
		if bridge == nil {
			delete(m.Bridges, key)
			continue
		}
		if bridge.RefCount > 0 {
			continue
		}
		if now.Sub(bridge.LastUsedAt) < socks5BridgeIdleTTL {
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

	log := logger.New("Socks5Bridge")
	for _, bridge := range stale {
		log.Info("回收空闲 socks5 桥接", logger.F("key", bridge.NodeKey[:8]), logger.F("port", bridge.Port))
		m.stopBridge(bridge)
	}
}
