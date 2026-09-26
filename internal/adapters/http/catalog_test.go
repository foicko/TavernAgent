package http

import (
	"encoding/json"
	"net/http"
	"testing"
)

// 展示目录端点：客户端启动时拉一次，命中 ETag 时只回 304。
func TestCatalogEndpoint(t *testing.T) {
	ts, _ := newTestServer(t)

	resp, err := http.Get(ts.URL + "/api/v1/catalog")
	if err != nil {
		t.Fatalf("GET catalog: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("状态码 = %d, 期望 200", resp.StatusCode)
	}
	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Fatal("缺少 ETag：客户端无法缓存目录")
	}
	var body struct {
		Version       string `json:"version"`
		DefaultProt   string `json:"defaultProtocol"`
		ProviderCount int    `json:"-"`
		Presets       []struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"providerPresets"`
		Slots []struct {
			Key string `json:"key"`
		} `json:"slots"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("解析目录: %v", err)
	}
	if body.Version == "" || len(body.Presets) == 0 || len(body.Slots) == 0 || body.DefaultProt == "" {
		t.Fatalf("目录内容不完整: %+v", body)
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/catalog", nil)
	req.Header.Set("If-None-Match", etag)
	again, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET catalog (304): %v", err)
	}
	defer again.Body.Close()
	if again.StatusCode != http.StatusNotModified {
		t.Fatalf("命中 ETag 时状态码 = %d, 期望 304", again.StatusCode)
	}
}
