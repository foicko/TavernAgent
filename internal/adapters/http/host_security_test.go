package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/application"
	"tavernagent/internal/ports"
)

func loopbackTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	// 服务固定在 loopback 上监听；这里用与它一致的地址形态。
	srv := mustNew(t, Deps{
		Sessions: application.NewSessionService(st),
		Addr:     "127.0.0.1:8891",
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// 窗口跑在 loopback 上，Host 就是它的监听地址；非本机 Host 仍须被拒。
func TestLoopbackHostIsTrusted(t *testing.T) {
	ts := loopbackTestServer(t)
	for _, tc := range []struct {
		host string
		want int
	}{
		{"127.0.0.1:8891", http.StatusOK},
		{"localhost:8891", http.StatusOK},
		{"evil.example", http.StatusForbidden},
	} {
		req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/sessions", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = tc.host
		res := doRequest(t, req)
		if res.StatusCode != tc.want {
			t.Fatalf("host %s: got %d want %d", tc.host, res.StatusCode, tc.want)
		}
	}
}

// authStatus 自带 Host 校验（未走 security 包装），口径必须与 security 一致，
// 否则会出现"大部分接口正常、登录状态接口 403"的不一致。
func TestLoopbackAuthStatusUsesSameHostRule(t *testing.T) {
	ts := loopbackTestServer(t)
	for _, tc := range []struct {
		host string
		want int
	}{
		{"127.0.0.1:8891", http.StatusOK},
		{"evil.example", http.StatusForbidden},
	} {
		req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/auth/status", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = tc.host
		res := doRequest(t, req)
		if res.StatusCode != tc.want {
			t.Fatalf("host %s: got %d want %d", tc.host, res.StatusCode, tc.want)
		}
	}
}

func doRequest(t *testing.T, req *http.Request) *http.Response {
	t.Helper()
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}
