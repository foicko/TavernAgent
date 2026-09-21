package tlscert

import (
	"crypto/tls"
	"fmt"
	"net"
	"path/filepath"
	"strings"
)

// Mode 是加密传输的开关形态。
type Mode string

const (
	// ModeOff 保持明文（仅回环或可信局域网使用）。
	ModeOff Mode = "off"
	// ModeAuto 使用 <dataDir>/tls 下的自签证书，缺失或地址变化时自动重签。
	ModeAuto Mode = "auto"
	// ModeFiles 使用外置证书。
	ModeFiles Mode = "files"
)

// Options 是 Resolve 的输入。
type Options struct {
	Mode Mode
	// DataDir 是自签证书的落点（<dataDir>/tls）。
	DataDir string
	// Addr 是即将绑定的监听地址；其中的主机名/IP 会被写进证书 SAN。
	Addr string
	// CertFile / KeyFile 在 ModeFiles 时必填。
	CertFile string
	KeyFile  string
}

// ParseMode 解析取值；非法值直接报错，不做静默降级。
func ParseMode(spec string) (Mode, error) {
	switch Mode(strings.ToLower(strings.TrimSpace(spec))) {
	case "", ModeOff:
		return ModeOff, nil
	case ModeAuto:
		return ModeAuto, nil
	case ModeFiles:
		return ModeFiles, nil
	default:
		return "", fmt.Errorf("不支持的加密传输取值 %q；可用 off / auto / files", spec)
	}
}

// Resolve 把选项解析成一份可直接交给 tls.NewListener 的配置；关闭时返回 nil。
//
// 两个入口（无头服务与桌面壳）共用这一份：加密开关的语义不能因为"从哪个程序启动"
// 而不同，否则会出现"命令行版是加密的、桌面版是明文的"这种最难解释的差异。
func Resolve(opts Options) (*tls.Config, Paths, error) {
	if opts.Mode == ModeOff || opts.Mode == "" {
		return nil, Paths{}, nil
	}
	paths := Paths{}
	switch opts.Mode {
	case ModeAuto:
		hosts, err := LocalHosts()
		if err != nil {
			// 拿不到网卡列表不致命：至少把回环与主机名写进证书。
			hosts = nil
		}
		// 绑定地址若明确指向某个 IP，也必须进 SAN，否则用该 IP 访问会证书不匹配。
		if host := hostOnly(opts.Addr); host != "" {
			hosts = append(hosts, host)
		}
		generated, err := Ensure(filepath.Join(opts.DataDir, "tls"), hosts)
		if err != nil {
			return nil, Paths{}, err
		}
		paths = generated
	case ModeFiles:
		if strings.TrimSpace(opts.CertFile) == "" || strings.TrimSpace(opts.KeyFile) == "" {
			return nil, Paths{}, fmt.Errorf("加密传输=files 需要同时给出证书与私钥路径")
		}
		paths = Paths{Cert: opts.CertFile, Key: opts.KeyFile}
	}
	cfg, err := Load(paths)
	if err != nil {
		return nil, Paths{}, err
	}
	return cfg, paths, nil
}

// hostOnly 从 "host:port" 里取出 host；解析失败时原样返回（可能是裸主机名）。
func hostOnly(addr string) string {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return strings.Trim(host, "[]")
	}
	return strings.Trim(strings.TrimSpace(addr), "[]")
}
