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
