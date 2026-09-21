package main

import (
	"crypto/tls"
	"path/filepath"
	"testing"
)

func TestParseTLSMode(t *testing.T) {
	for _, tc := range []struct {
		spec string
		want tlsMode
	}{
		{"", tlsOff},
		{"off", tlsOff},
		{"OFF", tlsOff},
		{"auto", tlsAuto},
		{" auto ", tlsAuto},
		{"files", tlsFiles},
	} {
		got, err := parseTLSMode(tc.spec)
		if err != nil {
			t.Fatalf("parseTLSMode(%q): %v", tc.spec, err)
		}
		if got != tc.want {
			t.Fatalf("parseTLSMode(%q) = %q, want %q", tc.spec, got, tc.want)
		}
	}
	// 写错取值必须报错：静默退回明文是最危险的一种"宽容"。
	if _, err := parseTLSMode("on"); err == nil {
		t.Fatal("非法取值应当报错")
	}
}

func TestBuildTLSConfigOffByDefault(t *testing.T) {
	cfg, err := buildTLSConfig(tlsOptions{mode: tlsOff}, t.TempDir(), "0.0.0.0:8890", "")
	if err != nil {
		t.Fatalf("关闭 TLS 不该报错: %v", err)
	}
	if cfg != nil {
		t.Fatal("关闭 TLS 时应返回 nil 配置")
	}
}

func TestBuildTLSConfigRefusesLANWithoutPIN(t *testing.T) {
	// 局域网暴露 + 无配对码 = 同网段任何设备都能操作故事库；TLS 只解决"被看"。
	_, err := buildTLSConfig(tlsOptions{mode: tlsAuto}, t.TempDir(), "192.168.1.10:8890", "")
	if err == nil {
		t.Fatal("非回环地址 + 空配对码必须拒绝启动")
	}
}

func TestBuildTLSConfigAutoGeneratesCertificate(t *testing.T) {
	dir := t.TempDir()
	cfg, err := buildTLSConfig(tlsOptions{mode: tlsAuto}, dir, "192.168.1.10:8890", "123456")
	if err != nil {
		t.Fatalf("buildTLSConfig: %v", err)
	}
	if cfg == nil || len(cfg.Certificates) != 1 {
		t.Fatalf("应装载一份证书: %+v", cfg)
	}
	// 证书落在数据目录下的 tls/ 里，随数据一起被备份与迁移。
	if _, err := filepath.Glob(filepath.Join(dir, "tls", "server.*")); err != nil {
		t.Fatalf("证书目录: %v", err)
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Fatalf("最低版本应为 TLS 1.2")
	}
}

func TestBuildTLSConfigFilesRequiresBothPaths(t *testing.T) {
	_, err := buildTLSConfig(tlsOptions{mode: tlsFiles, cert: "only.crt"}, t.TempDir(), "127.0.0.1:8890", "123456")
	if err == nil {
		t.Fatal("只给证书不给私钥应当报错")
	}
}

func TestIsLoopbackAddr(t *testing.T) {
	for _, tc := range []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:8890", true},
		{"localhost:8890", true},
		{"[::1]:8890", true},
		{"0.0.0.0:8890", false},
		{"192.168.1.10:8890", false},
	} {
		if got := isLoopbackAddr(tc.addr); got != tc.want {
			t.Fatalf("isLoopbackAddr(%q) = %v, want %v", tc.addr, got, tc.want)
		}
	}
}
