package http

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/application"
	"tavernagent/internal/ports"
)

func desktopTestServer(t *testing.T, theme func(string)) *httptest.Server {
	t.Helper()
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	// 桌面壳固定在 loopback 上监听；这里用与它一致的地址形态。
	srv := mustNew(t, Deps{
		Sessions:       application.NewSessionService(st),
		Addr:           "127.0.0.1:8891",
		SetNativeTheme: theme,
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// 桌面端的窗口跑在 loopback 上，Host 就是它的监听地址；非本机 Host 仍须被拒。
func TestDesktopLoopbackHostIsTrusted(t *testing.T) {
	ts := desktopTestServer(t, nil)
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
func TestDesktopAuthStatusUsesSameHostRule(t *testing.T) {
	ts := desktopTestServer(t, nil)
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

// 前端据此判断自己是否跑在桌面壳里（浏览器部署必须拿到 false，才不会去调不存在的窗口接口）。
func TestDesktopInfoReflectsBridgePresence(t *testing.T) {
	for _, tc := range []struct {
		name string
		hook func(string)
		want bool
	}{
		{"桌面壳", func(string) {}, true},
		{"纯浏览器", nil, false},
	} {
		ts := desktopTestServer(t, tc.hook)
		res := doRequest(t, mustRequest(t, http.MethodGet, ts.URL+"/api/v1/desktop", ""))
		if res.StatusCode != http.StatusOK {
			t.Fatalf("%s: got %d", tc.name, res.StatusCode)
		}
		body := readBody(t, res)
		want := `"desktop":false`
		if tc.want {
			want = `"desktop":true`
		}
		if !strings.Contains(body, want) {
			t.Fatalf("%s: body = %s, want 含 %s", tc.name, body, want)
		}
	}
}

// 窗口配色桥：合法取值透传给桌面壳，非法取值与"没有原生窗口"都要明确拒绝。
func TestDesktopThemeBridge(t *testing.T) {
	var got []string
	ts := desktopTestServer(t, func(mode string) { got = append(got, mode) })

	res := doRequest(t, mustRequest(t, http.MethodPost, ts.URL+"/api/v1/desktop/theme", `{"mode":"dark"}`))
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("dark: got %d want 204 (%s)", res.StatusCode, readBody(t, res))
	}
	res = doRequest(t, mustRequest(t, http.MethodPost, ts.URL+"/api/v1/desktop/theme", `{"mode":"Light"}`))
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("Light: got %d want 204（大小写应被规整）", res.StatusCode)
	}
	if len(got) != 2 || got[0] != "dark" || got[1] != "light" {
		t.Fatalf("回调收到 %v，want [dark light]", got)
	}

	res = doRequest(t, mustRequest(t, http.MethodPost, ts.URL+"/api/v1/desktop/theme", `{"mode":"neon"}`))
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("未知模式: got %d want 400", res.StatusCode)
	}
	if len(got) != 2 {
		t.Fatalf("非法模式不得触发回调，got %v", got)
	}
}

// 浏览器部署没有原生窗口：这条桥必须明确不可用，而不是静默成功。
func TestDesktopThemeAbsentInBrowser(t *testing.T) {
	ts := desktopTestServer(t, nil)
	res := doRequest(t, mustRequest(t, http.MethodPost, ts.URL+"/api/v1/desktop/theme", `{"mode":"dark"}`))
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("got %d want 404 (%s)", res.StatusCode, readBody(t, res))
	}
}

func mustRequest(t *testing.T, method, url, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	return req
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

func readBody(t *testing.T, res *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
