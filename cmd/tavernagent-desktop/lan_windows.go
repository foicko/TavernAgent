//go:build windows

// 局域网第二屏：为"手机/平板连 PC 游玩"提供一个显式开关。
//
// 设计要点（对应 docs/DESKTOP.md 的待办）：
//  1. 默认关闭——桌面端默认只监听 loopback，任何本机进程之外的访问都不存在。
//  2. 开启后**强制配对码**：没有配对码的局域网暴露等于把故事库交给同网段所有设备。
//  3. 独立监听端口而不是复用窗口那个：窗口的源必须稳定在 127.0.0.1（localStorage
//     按源隔离），而且自签证书会让 WebView 弹证书警告——第二屏单独一个端口，
//     加密只作用在它上面，桌面窗口完全不受影响。
package main

import (
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"log"
	"math/big"
	"net"
	"strings"

	"tavernagent/internal/util/tlscert"
)

// defaultLANAddr 是第二屏的默认监听地址。与窗口端口 8891 相邻便于记忆。
const defaultLANAddr = "0.0.0.0:8892"

// lanListener 是一次已就绪的第二屏监听。
type lanListener struct {
	listener net.Listener
	scheme   string
	addr     string
	pin      string
}

// Close 释放监听。
func (l *lanListener) Close() {
	if l != nil && l.listener != nil {
		_ = l.listener.Close()
	}
}

// URLs 列出可以在手机上直接输入的地址（只列私网 IPv4，公网地址没有意义）。
func (l *lanListener) URLs() []string {
	if l == nil {
		return nil
	}
	_, port, err := net.SplitHostPort(l.addr)
	if err != nil {
		return nil
	}
	hosts, err := tlscert.LocalHosts()
	if err != nil {
		return nil
	}
	urls := make([]string, 0, len(hosts))
	for _, host := range hosts {
		ip := net.ParseIP(host)
		if ip == nil || ip.To4() == nil || ip.IsLoopback() {
			continue
		}
		urls = append(urls, fmt.Sprintf("%s://%s:%s", l.scheme, host, port))
	}
	return urls
}

// startLAN 按选项起第二屏监听；未启用时返回 nil。
func startLAN(enabled bool, dataDir, addr, pinSpec, tlsSpec, certFile, keyFile string) (*lanListener, error) {
	if !enabled {
		return nil, nil
	}
	pin := strings.TrimSpace(pinSpec)
	if pin == "" {
		generated, err := randomPIN()
		if err != nil {
			return nil, err
		}
		pin = generated
	}

	mode, err := tlscert.ParseMode(tlsSpec)
	if err != nil {
		return nil, err
	}
	cfg, paths, err := tlscert.Resolve(tlscert.Options{
		Mode: mode, DataDir: dataDir, Addr: addr, CertFile: certFile, KeyFile: keyFile,
	})
	if err != nil {
		return nil, err
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("第二屏监听 %s 失败: %w", addr, err)
	}
	scheme := "http"
	if cfg != nil {
		listener = tls.NewListener(listener, cfg)
		scheme = "https"
		if fingerprint, err := tlscert.Fingerprint(paths); err == nil {
			log.Printf("🔐 第二屏已启用加密传输（证书 %s，SHA-256 指纹 %s）", paths.Cert, fingerprint)
		}
	}
	return &lanListener{listener: listener, scheme: scheme, addr: addr, pin: pin}, nil
}

// randomPIN 生成 6 位数字配对码（与无头入口同一套规则）。
func randomPIN() (string, error) {
	number, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("生成配对码失败: %w", err)
	}
	return fmt.Sprintf("%06d", number.Int64()), nil
}
