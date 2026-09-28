package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// apiHostGuard 是"新增路由漏包 Host 校验"的兜底：所有 /api/ 请求一律先过 Host 白名单。
func TestAPIHostGuardRejectsForeignHostOnAPIPaths(t *testing.T) {
	s := mustNew(t, Deps{Addr: "127.0.0.1:8890"})
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	guard := s.apiHostGuard(next)

	cases := []struct {
		name   string
		target string
		want   int
	}{
		{"非私有 Host 的 /api/ 请求被拒", "http://evil.example.com/api/v1/sessions", http.StatusForbidden},
		{"局域网私有地址放行", "http://192.168.1.20:8890/api/v1/sessions", http.StatusTeapot},
		{"回环放行", "http://127.0.0.1:8890/api/v1/sessions", http.StatusTeapot},
		{"非 /api/ 路径不受此层影响", "http://evil.example.com/healthz", http.StatusTeapot},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		guard.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.target, nil))
		if rec.Code != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.name, rec.Code, tc.want)
		}
	}
}
