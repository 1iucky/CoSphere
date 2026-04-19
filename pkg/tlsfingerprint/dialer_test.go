package tlsfingerprint

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestDefaultProfile(t *testing.T) {
	profile := DefaultProfile()
	if profile == nil {
		t.Fatal("DefaultProfile() returned nil")
	}
	if profile.Name != "Node.js 24.x (Claude Code)" {
		t.Errorf("DefaultProfile name = %v, want 'Node.js 24.x (Claude Code)'", profile.Name)
	}
}

func TestBuildClientHelloSpecFromProfile_NilProfile(t *testing.T) {
	spec := buildClientHelloSpecFromProfile(nil)
	if spec == nil {
		t.Fatal("buildClientHelloSpecFromProfile(nil) returned nil")
	}
	if len(spec.CipherSuites) == 0 {
		t.Error("spec should have default cipher suites")
	}
	if len(spec.Extensions) == 0 {
		t.Error("spec should have default extensions")
	}
}

func TestBuildClientHelloSpecFromProfile_DefaultProfile(t *testing.T) {
	profile := DefaultProfile()
	spec := buildClientHelloSpecFromProfile(profile)
	if spec == nil {
		t.Fatal("buildClientHelloSpecFromProfile returned nil for default profile")
	}
	// 验证默认密码套件数量
	if len(spec.CipherSuites) != len(defaultCipherSuites) {
		t.Errorf("CipherSuites count = %d, want %d", len(spec.CipherSuites), len(defaultCipherSuites))
	}
}

func TestBuildClientHelloSpecFromProfile_CustomProfile(t *testing.T) {
	profile := &Profile{
		Name:         "Test Profile",
		CipherSuites: []uint16{0x1301, 0x1302},
		Curves:       []uint16{uint16(0x001d), uint16(0x0017)},
		EnableGREASE: false,
		ALPNProtocols: []string{"h2", "http/1.1"},
	}
	spec := buildClientHelloSpecFromProfile(profile)
	if spec == nil {
		t.Fatal("buildClientHelloSpecFromProfile returned nil for custom profile")
	}
	if len(spec.CipherSuites) != 2 {
		t.Errorf("CipherSuites count = %d, want 2", len(spec.CipherSuites))
	}
}

func TestIsGREASEValue(t *testing.T) {
	tests := []struct {
		value uint16
		want  bool
	}{
		{0x0a0a, true},
		{0x1a1a, true},
		{0x2a2a, true},
		{0x00ff, false},
		{0x1301, false},
		{0x0000, false},
	}
	for _, tt := range tests {
		if got := isGREASEValue(tt.value); got != tt.want {
			t.Errorf("isGREASEValue(0x%04x) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestToUint8s(t *testing.T) {
	input := []uint16{0, 1, 2, 255}
	expected := []uint8{0, 1, 2, 255}
	result := toUint8s(input)
	if len(result) != len(expected) {
		t.Fatalf("toUint8s length = %d, want %d", len(result), len(expected))
	}
	for i, v := range result {
		if v != expected[i] {
			t.Errorf("toUint8s[%d] = %d, want %d", i, v, expected[i])
		}
	}
}

func TestToUTLSCurves(t *testing.T) {
	input := []uint16{0x001d, 0x0017, 0x0018}
	result := toUTLSCurves(input)
	if len(result) != 3 {
		t.Fatalf("toUTLSCurves length = %d, want 3", len(result))
	}
}

// TestDialer_TLSHandshake_LocalServer 测试 TLS 握手与本地 TLS 服务器
func TestDialer_TLSHandshake_LocalServer(t *testing.T) {
	// 创建本地 TLS 服务器
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "hello")
	}))
	defer server.Close()

	// 从服务器 URL 提取 host:port
	addr := server.Listener.Addr().String()

	// 使用默认 profile 的 Dialer
	profile := DefaultProfile()
	dialer := NewDialer(profile, nil)
	dialer.InsecureSkipVerify = true

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := dialer.DialTLSContext(ctx, "tcp", addr)
	if err != nil {
		t.Fatalf("DialTLSContext failed: %v", err)
	}
	defer conn.Close()

	// 验证连接状态
	if conn == nil {
		t.Fatal("connection is nil")
	}
}

// TestNewTLSFingerprintClient_Integration 测试完整的 TLS 指纹 HTTP 客户端
func TestNewTLSFingerprintClient_Integration(t *testing.T) {
	// 创建本地 TLS 服务器
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	}))
	defer server.Close()

	// 创建自定义 transport 使用自定义 DialTLSContext
	profile := DefaultProfile()
	dialer := NewDialer(profile, nil)
	dialer.InsecureSkipVerify = true

	transport := &http.Transport{
		DialTLSContext: dialer.DialTLSContext,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Status code = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

// TestNewDialer_NilBaseDialer 测试 baseDialer 为 nil 时使用默认
func TestNewDialer_NilBaseDialer(t *testing.T) {
	dialer := NewDialer(DefaultProfile(), nil)
	if dialer == nil {
		t.Fatal("NewDialer returned nil")
	}
	if dialer.baseDialer == nil {
		t.Error("baseDialer should not be nil (should use default)")
	}
}

// TestNewHTTPProxyDialer 测试创建 HTTP 代理拨号器
func TestNewHTTPProxyDialer(t *testing.T) {
	proxyURL, _ := url.Parse("http://proxy:8080")
	dialer := NewHTTPProxyDialer(DefaultProfile(), proxyURL)
	if dialer == nil {
		t.Fatal("NewHTTPProxyDialer returned nil")
	}
	if dialer.proxyURL != proxyURL {
		t.Error("proxyURL mismatch")
	}
}

// TestNewSOCKS5ProxyDialer 测试创建 SOCKS5 代理拨号器
func TestNewSOCKS5ProxyDialer(t *testing.T) {
	proxyURL, _ := url.Parse("socks5://proxy:1080")
	dialer := NewSOCKS5ProxyDialer(DefaultProfile(), proxyURL)
	if dialer == nil {
		t.Fatal("NewSOCKS5ProxyDialer returned nil")
	}
	if dialer.proxyURL != proxyURL {
		t.Error("proxyURL mismatch")
	}
}
