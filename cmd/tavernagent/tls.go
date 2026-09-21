// 局域网 HTTPS 的命令行装配。
//
// 单独成文件而不是写进 main.go：run() 有 120 行的长度上限（架构门禁），
// 而 TLS 的解析、校验与日志本身就有几条分支。
package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"strings"

	"tavernagent/internal/util/tlscert"
)

// tlsOptions 是 TLS 相关的命令行选项。
type tlsOptions struct {
	mode tlsMode
	cert string
	key  string
}

// tlsMode 复用 tlscert.Mode 的取值，避免两处各定义一套字符串。
type tlsMode = tlscert.Mode

const (
	tlsOff   = tlscert.ModeOff
	tlsAuto  = tlscert.ModeAuto
	tlsFiles = tlscert.ModeFiles
)

// parseTLSMode 解析 -tls 的取值。
func parseTLSMode(spec string) (tlsMode, error) { return tlscert.ParseMode(spec) }

// buildTLSConfig 按选项装配 TLS 配置；关闭时返回 nil。
//
// 安全约束：绑定非回环地址且启用 TLS 时**必须有配对码**。没有配对码的局域网暴露
// 等于把故事库交给同网段的任何设备——TLS 只解决"传输被看"，不解决"谁能进"。
func buildTLSConfig(opts tlsOptions, dataDir, addr, pin string) (*tls.Config, error) {
	if opts.mode == tlsOff {
		return nil, nil
	}
	if !isLoopbackAddr(addr) && strings.TrimSpace(pin) == "" {
		return nil, fmt.Errorf("绑定 %s 且启用加密传输时必须提供配对码（-pin），否则局域网内任何设备都能直接操作故事库", addr)
	}
	cfg, paths, err := tlscert.Resolve(tlscert.Options{
		Mode: opts.mode, DataDir: dataDir, Addr: addr, CertFile: opts.cert, KeyFile: opts.key,
	})
	if err != nil {
		return nil, err
	}
	if fingerprint, err := tlscert.Fingerprint(paths); err == nil {
		log.Printf("🔐 局域网 HTTPS 已启用（证书 %s，SHA-256 指纹 %s）", paths.Cert, fingerprint)
		log.Printf("   首次在手机/平板上访问需要信任该证书；指纹应与上面这一行逐字一致")
	}
	return cfg, nil
}

// isLoopbackAddr 判断监听地址是否只对本机可见。
func isLoopbackAddr(addr string) bool {
	host := hostOnly(addr)
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// hostOnly 从 "host:port" 里取出 host；解析失败时原样返回（可能是裸主机名）。
func hostOnly(addr string) string {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return strings.Trim(host, "[]")
	}
	return strings.Trim(strings.TrimSpace(addr), "[]")
}
