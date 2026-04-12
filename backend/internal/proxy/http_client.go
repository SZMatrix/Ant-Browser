package proxy

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ant-chrome/backend/internal/config"

	xproxy "golang.org/x/net/proxy"
)

// buildProxyHTTPClient 根据代理配置构建 HTTP 客户端，统一用于测速/健康检测场景。
func buildProxyHTTPClient(
	src string,
	proxyId string,
	proxies []config.BrowserProxy,
	xrayMgr *XrayManager,
	singboxMgr *SingBoxManager,
	timeout time.Duration,
) (*http.Client, error) {
	l := strings.ToLower(strings.TrimSpace(src))
	if l == "" || l == "direct://" {
		return &http.Client{Timeout: timeout}, nil
	}

	if IsSingBoxProtocol(src) {
		if singboxMgr == nil {
			return nil, fmt.Errorf("sing-box 管理器未初始化")
		}
		socks5Addr, err := singboxMgr.EnsureBridge(src, proxies, proxyId)
		if err != nil {
			return nil, fmt.Errorf("sing-box 桥接启动失败: %w", err)
		}
		return buildSocks5HTTPClient(strings.TrimPrefix(socks5Addr, "socks5://"), timeout)
	}

	if RequiresBridge(src, proxies, proxyId) {
		if xrayMgr == nil {
			return nil, fmt.Errorf("xray 管理器未初始化")
		}
		socks5Addr, err := xrayMgr.EnsureBridge(src, proxies, proxyId)
		if err != nil {
			return nil, fmt.Errorf("xray 桥接启动失败: %w", err)
		}
		return buildSocks5HTTPClient(strings.TrimPrefix(socks5Addr, "socks5://"), timeout)
	}

	if strings.HasPrefix(l, "socks5://") {
		u, err := url.Parse(src)
		if err != nil {
			return nil, fmt.Errorf("SOCKS5 地址解析失败: %w", err)
		}
		var auth *xproxy.Auth
		if u.User != nil {
			pass, _ := u.User.Password()
			auth = &xproxy.Auth{
				User:     u.User.Username(),
				Password: pass,
			}
		}
		dialer, err := xproxy.SOCKS5("tcp", u.Host, auth, xproxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("SOCKS5 dialer 创建失败: %w", err)
		}
		contextDialer, ok := dialer.(xproxy.ContextDialer)
		if !ok {
			return nil, fmt.Errorf("SOCKS5 dialer 不支持 ContextDialer")
		}
		transport := &http.Transport{DialContext: contextDialer.DialContext}
		return &http.Client{Transport: transport, Timeout: timeout}, nil
	}

	transport, err := HTTPProxyTransport(src)
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: transport, Timeout: timeout}, nil
}

// httpProxyTransport 解析代理地址并返回 http.Transport。
// HTTP 代理即使代理 HTTPS 流量，代理连接本身也是明文 HTTP（通过 CONNECT 隧道）。
// 如果 scheme 为 https，Go 会尝试与代理服务器做 TLS 握手，导致握手失败，
// 因此统一将 https scheme 转为 http。
func HTTPProxyTransport(raw string) (*http.Transport, error) {
	proxyURL, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("代理地址解析失败: %w", err)
	}
	if proxyURL.Scheme == "https" {
		proxyURL.Scheme = "http"
	}
	return &http.Transport{Proxy: http.ProxyURL(proxyURL)}, nil
}

func buildSocks5HTTPClient(socks5Host string, timeout time.Duration) (*http.Client, error) {
	dialer, err := xproxy.SOCKS5("tcp", socks5Host, nil, xproxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("SOCKS5 dialer 创建失败: %w", err)
	}
	contextDialer, ok := dialer.(xproxy.ContextDialer)
	if !ok {
		return nil, fmt.Errorf("SOCKS5 dialer 不支持 ContextDialer")
	}
	transport := &http.Transport{DialContext: contextDialer.DialContext}
	return &http.Client{Transport: transport, Timeout: timeout}, nil
}
