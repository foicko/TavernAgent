package tlscert

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestEnsureGeneratesAndReusesCertificate(t *testing.T) {
	dir := t.TempDir()
	hosts := []string{"localhost", "127.0.0.1", "192.168.7.7", "tavern-host"}

	paths, err := Ensure(dir, hosts)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	first, err := os.ReadFile(paths.Cert)
	if err != nil {
		t.Fatalf("读取证书: %v", err)
	}

	// 私钥必须只有属主可读：它一旦泄漏，局域网里任何人都能冒充本服务。
	// Windows 上 Go 的 chmod 只能切换只读位（os.Stat 一律报 0666），实际权限由
	// 用户目录 ACL 决定，因此这一条只在类 Unix 上断言。
	info, err := os.Stat(paths.Key)
	if err != nil {
		t.Fatalf("读取私钥: %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != keyMode {
			t.Fatalf("私钥权限 = %o, want %o", perm, keyMode)
		}
	}

	// 再调一次：证书仍然覆盖这些地址，就不该重签（否则用户每次重启都要重装信任）。
	if _, err := Ensure(dir, hosts); err != nil {
		t.Fatalf("二次 Ensure: %v", err)
	}
	second, err := os.ReadFile(paths.Cert)
	if err != nil {
		t.Fatalf("二次读取证书: %v", err)
	}
	if string(first) != string(second) {
		t.Fatal("地址未变时不应重新签发证书")
	}

	// 地址变了（换 Wi-Fi）：必须重签，否则新地址访问会证书不匹配。
	if _, err := Ensure(dir, append(hosts, "10.1.2.3")); err != nil {
		t.Fatalf("地址变化后 Ensure: %v", err)
	}
	third, err := os.ReadFile(paths.Cert)
	if err != nil {
		t.Fatalf("三次读取证书: %v", err)
	}
	if string(second) == string(third) {
		t.Fatal("新增地址后应重新签发证书")
	}
}

func TestGeneratedCertificateCoversHostsAndIsUsable(t *testing.T) {
	dir := t.TempDir()
	paths, err := Ensure(dir, []string{"localhost", "127.0.0.1", "10.0.0.5"})
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	raw, err := os.ReadFile(paths.Cert)
	if err != nil {
		t.Fatalf("读取证书: %v", err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		t.Fatal("证书不是 PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("解析证书: %v", err)
	}

	dns := map[string]bool{}
	for _, name := range cert.DNSNames {
		dns[name] = true
	}
	if !dns["localhost"] {
		t.Fatalf("DNS SAN 缺少 localhost: %v", cert.DNSNames)
	}
	ips := map[string]bool{}
	for _, ip := range cert.IPAddresses {
		ips[ip.String()] = true
	}
	for _, want := range []string{"127.0.0.1", "10.0.0.5"} {
		if !ips[want] {
			t.Fatalf("IP SAN 缺少 %s: %v", want, cert.IPAddresses)
		}
	}
	if cert.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
		t.Fatalf("用途应为服务端认证: %v", cert.ExtKeyUsage)
	}

	// 能被 tls 包直接加载，并且指纹可复现（用户加信任时靠它比对）。
	cfg, err := Load(paths)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Fatalf("最低版本应为 TLS 1.2，得到 %x", cfg.MinVersion)
	}
	fingerprint, err := Fingerprint(paths)
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	if len(fingerprint) != 95 { // 32 字节 → 95 个字符（64 个十六进制 + 31 个冒号）
		t.Fatalf("指纹形状不对: %q", fingerprint)
	}
}

func TestLocalHostsIncludesLoopback(t *testing.T) {
	hosts, _ := LocalHosts()
	seen := map[string]bool{}
	for _, host := range hosts {
		seen[host] = true
		if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() && !ip.IsPrivate() {
			t.Fatalf("只应收集回环与私网地址，出现 %s", host)
		}
	}
	for _, want := range []string{"localhost", "127.0.0.1"} {
		if !seen[want] {
			t.Fatalf("缺少 %s: %v", want, hosts)
		}
	}
}

func TestLoadRejectsMissingFiles(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(Paths{Cert: filepath.Join(dir, "nope.crt"), Key: filepath.Join(dir, "nope.key")}); err == nil {
		t.Fatal("缺文件时应当报错，而不是给出一份空配置")
	}
}
