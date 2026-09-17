package http

import (
	"net/http/httptest"
	"testing"
)

func TestOriginRequiresSameAuthorityOrExplicitAllowlist(t *testing.T) {
	s := mustNew(t, Deps{Addr: "0.0.0.0:8890", AllowedOrigins: []string{"http://localhost:5173"}})
	for _, tc := range []struct {
		origin string
		want   bool
	}{
		{"http://192.168.1.20:8890", true},
		{"http://192.168.1.20:5173", false},
		{"http://192.168.1.21:8890", false},
		{"http://localhost:5173", true},
		{"http://attacker.example", false},
		{"null", false},
		{"", true},
	} {
		r := httptest.NewRequest("POST", "http://192.168.1.20:8890/api/v1/sessions", nil)
		r.Header.Set("Origin", tc.origin)
		if got := s.allowedOrigin(r); got != tc.want {
			t.Errorf("origin %q: got %v", tc.origin, got)
		}
	}
}
