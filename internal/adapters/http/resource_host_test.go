package http

import (
	"net/http"
	"testing"
)

// 只读资源路由（tts/voices、images/{filename}、cards assets）也必须做 Host 白名单，
// 否则恶意网页可 DNS rebinding 读取本机数据（SEC-REBIND-01）。
func TestResourceRoutesRejectForeignHost(t *testing.T) {
	ts := loopbackTestServer(t)
	paths := []string{
		"/api/v1/tts/voices",
		"/api/v1/images/sample.png",
		"/api/v1/cards/does-not-matter/assets/x.png",
	}
	for _, p := range paths {
		req, err := http.NewRequest(http.MethodGet, ts.URL+p, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = "evil.example"
		res := doRequest(t, req)
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("%s with foreign host: got %d want %d", p, res.StatusCode, http.StatusForbidden)
		}
	}
}
