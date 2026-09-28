// 鉴权与来源校验：本机免检、局域网配对 Token、写请求来源白名单。
//
// 从 server.go 抽出来：这几条规则是**安全边界**，混在路由与 handler 之间很难被
// 完整审阅。安全相关的改动应当只动这一个文件，评审范围因此可枚举。
package http

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ---- 安全：回环与局域网私有地址绑定 + 来源校验（T28 相关，支持 LAN 访问）----

func (s *Server) security(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		// 允许本机/私有局域网。
		if !isLANHost(host) {
			writeError(w, 403, "FORBIDDEN", "只允许本机或局域网私有网络访问", false, "")
			return
		}

		// 局域网鉴权（SEC-01）：本机免检；非本机 LAN 访问若启用了 AuthPIN，则必须携带合法授权 Token
		if !isLoopbackPeer(r) && s.authPIN != "" {
			token := extractToken(r)
			if token == "" || !secureEqual(token, s.authToken) {
				writeError(w, 401, "AUTH_REQUIRED", "局域网访问需要配对码授权", false, "")
				return
			}
		}

		if isWrite(r) {
			o := r.Header.Get("Origin")
			if o != "" && !s.allowedOrigin(r) {
				writeError(w, 403, "FORBIDDEN", "跨来源写请求被拒绝", false, "")
				return
			}
		}
		h(w, r)
	}
}

// hostOnly 只做 Host 白名单（防 DNS rebinding），不做 Token 鉴权。
//
// 用于浏览器 <img>/<audio> 无法携带 Bearer Token 的只读资源路由：保持"可公开读取"，
// 但仍拒绝非本机/非私有网段的 Host，避免恶意网页把自身来源 rebinding 到 127.0.0.1
// 后读走本机的卡资源与生成图（SEC-REBIND-01）。注意 Host 头可被客户端伪造，
// 因此它只是 rebinding 防线，**不是**授权边界——授权仍由 security 的 Token 判定。
func (s *Server) hostOnly(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isLANHost(r.Host) {
			writeError(w, 403, "FORBIDDEN", "只允许本机或局域网私有网络访问", false, "")
			return
		}
		h(w, r)
	}
}

func extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	if tok := r.Header.Get("X-Auth-Token"); tok != "" {
		return tok
	}
	return ""
}

func (s *Server) pairAuth(w http.ResponseWriter, r *http.Request) {
	if !isLANHost(r.Host) || (isWrite(r) && !s.allowedOrigin(r)) {
		writeError(w, 403, "FORBIDDEN", "请求来源不受支持", false, "")
		return
	}
	if s.authPIN == "" {
		writeJSON(w, 200, map[string]any{"ok": true, "token": ""})
		return
	}
	ip := clientIP(r)

	var req struct {
		PIN string `json:"pin"`
	}
	if err := decodeJSON(w, r, &req, 1024); err != nil {
		writeBodyError(w, err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for address, failure := range s.pairFails {
		if !now.Before(failure.Expires) {
			delete(s.pairFails, address)
		}
	}
	// 防滥用：失败表键是客户端可控的源地址，不能无界增长。过期清理后仍超上限就逐出若干条。
	if len(s.pairFails) > maxPairFailEntries {
		for address := range s.pairFails {
			delete(s.pairFails, address)
			if len(s.pairFails) <= maxPairFailEntries {
				break
			}
		}
	}
	failure := s.pairFails[ip]
	if failure.Count >= 10 {
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(failure.Expires.Sub(now).Seconds()))))
		writeError(w, 429, "TOO_MANY_ATTEMPTS", "尝试次数过多，请五分钟后重试", false, "")
		return
	}
	if !secureEqual(strings.TrimSpace(req.PIN), s.authPIN) {
		if failure.Count == 0 {
			failure.Expires = now.Add(5 * time.Minute)
		}
		failure.Count++
		s.pairFails[ip] = failure
		writeError(w, 401, "INVALID_PIN", "配对码错误", false, "")
		return
	}

	delete(s.pairFails, ip)

	writeJSON(w, 200, map[string]any{"ok": true, "token": s.authToken})
}

type pairFailure struct {
	Count   int
	Expires time.Time
}

// maxPairFailEntries 限制配对失败表的容量，避免源地址伪造把内存撑爆。
const maxPairFailEntries = 2048

func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) {
	if !isLANHost(r.Host) {
		writeError(w, 403, "FORBIDDEN", "只允许本机或局域网私有网络访问", false, "")
		return
	}
	if isLoopbackPeer(r) || s.authPIN == "" {
		writeJSON(w, 200, map[string]any{"authenticated": true, "required": false})
		return
	}
	token := extractToken(r)
	authed := token != "" && secureEqual(token, s.authToken)
	writeJSON(w, 200, map[string]any{"authenticated": authed, "required": true})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func isWrite(r *http.Request) bool {
	switch r.Method {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	}
	return false
}

func isLoopbackHost(host string) bool {
	h := hostname(host)
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func hostname(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		return host[1 : len(host)-1]
	}
	return host
}

// Only the transport peer can grant the local exemption. Host and forwarded
// headers are supplied by the caller and must never establish its identity.
func isLoopbackPeer(r *http.Request) bool {
	ip := net.ParseIP(clientIP(r))
	return ip != nil && ip.IsLoopback()
}

func isLANHost(host string) bool {
	h := hostname(host)
	if isLoopbackHost(h) {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && (ip.IsPrivate() || ip.IsLoopback())
}

func sameOrigin(a, b string) bool { return strings.TrimSuffix(a, "/") == strings.TrimSuffix(b, "/") }
