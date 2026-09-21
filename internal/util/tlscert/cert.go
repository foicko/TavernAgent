// Package tlscert 生成并持久化本机自签证书，供局域网 HTTPS 使用。
//
// 为什么是自签：局域网场景没有域名，也不该为了内网访问去申请公网证书。信任由用户
// 一次性完成（把 server.crt 装进设备信任库），此后手机/平板访问就是加密的。
// 明文的局域网 HTTP 等于把剧情正文与配对 Token 摊开给同一个网络里的人看。
//
// 证书覆盖本机的回环地址、私网地址与主机名；这些地址会随网络环境变化（换 Wi-Fi、
// 插网线），因此 Ensure 在发现现有证书"不再覆盖当前地址"时会重新签发——
// 否则用户会遇到"昨天还能用、今天提示证书不匹配"，而重装信任的代价远大于重签。
package tlscert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"tavernagent/internal/util/atomicfile"
)

const (
	certFileName = "server.crt"
	keyFileName  = "server.key"
	// validity 约 27 个月：长到不必频繁重装信任，又短于"永不过期"的坏习惯。
	validity = 825 * 24 * time.Hour
	// certMode / keyMode：私钥只给属主读写，证书可以放开读。
	keyMode  = 0o600
	certMode = 0o644
)

// Paths 是证书文件的位置。
type Paths struct {
	Cert string
	Key  string
}

// Ensure 确保 dir 下存在一份覆盖 hosts 的自签证书，返回其路径。
//
// 已存在且仍然覆盖 hosts 时直接复用：重启不该换证书，否则用户每次重启都要重装信任。
func Ensure(dir string, hosts []string) (Paths, error) {
	paths := Paths{Cert: filepath.Join(dir, certFileName), Key: filepath.Join(dir, keyFileName)}
	if _, ok := loadIfCovers(paths, hosts); ok {
		return paths, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return paths, fmt.Errorf("创建证书目录失败: %w", err)
	}
	certPEM, keyPEM, err := generate(hosts)
	if err != nil {
		return paths, err
	}
	if err := atomicfile.Write(paths.Cert, certPEM, certMode); err != nil {
		return paths, fmt.Errorf("写入证书失败: %w", err)
	}
	if err := atomicfile.Write(paths.Key, keyPEM, keyMode); err != nil {
		return paths, fmt.Errorf("写入私钥失败: %w", err)
	}
	return paths, nil
}

// Load 读取证书与私钥，返回可直接交给 tls.NewListener 的配置。
func Load(paths Paths) (*tls.Config, error) {
	pair, err := tls.LoadX509KeyPair(paths.Cert, paths.Key)
	if err != nil {
		return nil, fmt.Errorf("加载证书失败: %w", err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{pair},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// Fingerprint 返回证书的 SHA-256 指纹（冒号分隔的大写十六进制）。
//
// 用途：用户第一次在手机上加信任时，可以拿它和终端里打印的那一行对照——
// 自签证书的信任动作如果没有可比对的指纹，"装的是不是这台机器的证书"就只能靠猜。
func Fingerprint(paths Paths) (string, error) {
	raw, err := os.ReadFile(paths.Cert)
	if err != nil {
		return "", err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return "", fmt.Errorf("证书不是 PEM 格式")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(cert.Raw)
	parts := make([]string, 0, len(sum))
	for _, b := range sum {
		parts = append(parts, strings.ToUpper(hex.EncodeToString([]byte{b})))
	}
	return strings.Join(parts, ":"), nil
}

// LocalHosts 收集本机可用于证书 SAN 的标识：主机名 + 全部回环与私网地址。
//
// 只收私网地址：把公网地址写进证书没有意义，反而会把本机的公网位置固化进文件。
func LocalHosts() ([]string, error) {
	hosts := make([]string, 0, 8)
	if name, err := os.Hostname(); err == nil && strings.TrimSpace(name) != "" {
		hosts = append(hosts, name)
	}
	hosts = append(hosts, "localhost", "127.0.0.1", "::1")

	ifaces, err := net.Interfaces()
	if err != nil {
		return hosts, err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil || ip == nil {
				continue
			}
			if ip.IsLoopback() || ip.IsPrivate() {
				hosts = append(hosts, ip.String())
			}
		}
	}
	return dedupe(hosts), nil
}

// loadIfCovers 在证书存在、可解析且 SAN 覆盖 hosts 时返回 true。
func loadIfCovers(paths Paths, hosts []string) (bool, bool) {
	raw, err := os.ReadFile(paths.Cert)
	if err != nil {
		return false, false
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return false, false
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false, false
	}
	if _, err := os.Stat(paths.Key); err != nil {
		return false, false
	}
	if time.Now().After(cert.NotAfter) {
		return false, false
	}
	have := map[string]bool{}
	for _, name := range cert.DNSNames {
		have[strings.ToLower(name)] = true
	}
	for _, ip := range cert.IPAddresses {
		have[ip.String()] = true
	}
	for _, host := range hosts {
		if !have[strings.ToLower(host)] {
			return false, false
		}
	}
	return true, true
}

// generate 生成自签证书与私钥（PEM 编码）。
func generate(hosts []string) (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("生成私钥失败: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("生成序列号失败: %w", err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "TavernAgent 本地服务", Organization: []string{"TavernAgent"}},
		// 回拨一小时：设备与主机时钟不同步时，"尚未生效"是最难排查的一类握手失败。
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(validity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		// IsCA 为真使它可以被直接装进设备信任库当作根证书使用（自签场景的常规做法）。
		IsCA: true,
	}
	for _, host := range hosts {
		if ip := net.ParseIP(host); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
			continue
		}
		template.DNSNames = append(template.DNSNames, host)
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("签发证书失败: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("编码私钥失败: %w", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

func dedupe(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
