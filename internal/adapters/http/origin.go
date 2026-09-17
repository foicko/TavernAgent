package http

import (
	"net/http"
	"net/url"
	"strings"
)

func (s *Server) allowedOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.User != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if strings.EqualFold(u.Scheme, scheme) && strings.EqualFold(u.Host, r.Host) {
		return true
	}
	for _, allowed := range s.allowedOrigins {
		if sameOrigin(origin, allowed) {
			return true
		}
	}
	return sameOrigin(origin, s.origin)
}
