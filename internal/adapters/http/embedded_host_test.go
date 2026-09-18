package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/application"
	"tavernagent/internal/ports"
)

// 桌面壳的虚拟主机名（Wails 的 wails.localhost）应被接受，
// 但它不该把其它非本机 Host 一起放行——那是局域网鉴权的边界。
func TestEmbeddedHostIsTheOnlyExtraAllowedHost(t *testing.T) {
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	srv := mustNew(t, Deps{
		Sessions:     application.NewSessionService(st),
		Addr:         "wails.localhost",
		EmbeddedHost: "wails.localhost",
	})
	handler := srv.Handler()

	cases := []struct {
		host string
		want int
	}{
		{"wails.localhost", http.StatusOK},
		{"127.0.0.1:8890", http.StatusOK},
		{"evil.example", http.StatusForbidden},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "http://"+tc.host+"/api/v1/sessions", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("host %s: got %d want %d (%s)", tc.host, rec.Code, tc.want, rec.Body.String())
		}
	}
}

// 未配置 EmbeddedHost 时，wails.localhost 不得被当作可信来源。
func TestEmbeddedHostDisabledByDefault(t *testing.T) {
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	srv := mustNew(t, Deps{Sessions: application.NewSessionService(st), Addr: "127.0.0.1:8890"})
	req := httptest.NewRequest(http.MethodGet, "http://wails.localhost/api/v1/sessions", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d want 403 (%s)", rec.Code, rec.Body.String())
	}
}

// authStatus 自带 Host 校验（未走 security 包装），也必须认嵌入式主机：
// 否则桌面端会出现“大部分接口正常、登录状态接口 403”的不一致。
func TestEmbeddedHostAuthStatusIsTrusted(t *testing.T) {
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	srv := mustNew(t, Deps{
		Sessions:     application.NewSessionService(st),
		Addr:         "wails.localhost",
		EmbeddedHost: "wails.localhost",
	})
	handler := srv.Handler()
	for _, host := range []string{"wails.localhost", "127.0.0.1:8890"} {
		req := httptest.NewRequest(http.MethodGet, "http://"+host+"/api/v1/auth/status", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("host %s: got %d want 200 (%s)", host, rec.Code, rec.Body.String())
		}
	}
	req := httptest.NewRequest(http.MethodGet, "http://evil.example/api/v1/auth/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d want 403 (%s)", rec.Code, rec.Body.String())
	}
}
