// Package endpointguard 为"用户自带端点"（生图 / TTS / embedding 的 BaseURL 由请求体指定）
// 提供 SSRF 防护：阻断服务端把请求发往云元数据与链路本地地址。
//
// 设计取舍：本功能是 bring-your-own-endpoint，公网供应商与本地 LLM（回环/私网）都是
// 合法用法，因此不做"仅私网"的过度限制；只封堵 SSRF 的主要目标——云实例元数据端点
// 与链路本地网段（S5）。
package endpointguard

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

// metadataHosts 是已知云元数据主机名（小写）。
var metadataHosts = map[string]bool{
	"metadata":                 true,
	"metadata.google.internal": true,
	"metadata.goog":            true,
}

// Validate 校验端点 URL 是否可安全访问；为空表示使用默认端点，直接放行。
func Validate(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return errors.New("端点地址格式不合法")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("端点地址必须以 http(s) 开头")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return errors.New("端点地址缺少主机名")
	}
	if metadataHosts[host] || strings.HasPrefix(host, "metadata.") {
		return errors.New("端点主机被拒绝：云元数据地址不可访问")
	}
	if ip := net.ParseIP(host); ip != nil && isBlockedIP(ip) {
		return errors.New("端点地址被拒绝：链路本地/元数据网段不可访问")
	}
	return nil
}

// isBlockedIP 判定链路本地与已知元数据地址；回环与私网不在封禁之列。
func isBlockedIP(ip net.IP) bool {
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	// 100.100.100.200：阿里云元数据；fd00:ec2::254：AWS IPv6 元数据。
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] == 100 && v4[2] == 100 && v4[3] == 200 {
		return true
	}
	return ip.Equal(net.ParseIP("fd00:ec2::254"))
}
