package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 契约测试：`/api/` 下的 stdlib 404/405 必须也是统一错误信封（docs/spec/04-errors.md）。
func TestAPIErrorEnvelopeWrapsStdlib404And405(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/thing", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	h := apiErrorEnvelope(mux)

	cases := []struct {
		method, path, code string
		status             int
	}{
		{"GET", "/api/v1/missing", "NOT_FOUND", 404},
		{"DELETE", "/api/v1/thing", "METHOD_NOT_ALLOWED", 405},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
		if rec.Code != tc.status {
			t.Fatalf("%s %s 状态 = %d, want %d", tc.method, tc.path, rec.Code, tc.status)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Fatalf("%s %s Content-Type = %q, want application/json", tc.method, tc.path, ct)
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s %s 体不是 JSON: %v (%q)", tc.method, tc.path, err, rec.Body.String())
		}
		if body["code"] != tc.code {
			t.Fatalf("%s %s code = %v, want %s", tc.method, tc.path, body["code"], tc.code)
		}
		for _, field := range []string{"message", "retryable", "turnId"} {
			if _, ok := body[field]; !ok {
				t.Fatalf("%s %s 信封缺字段 %q: %v", tc.method, tc.path, field, body)
			}
		}
	}

	// 405 必须保留 Allow 头（客户端据此判断可用方法）。
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("DELETE", "/api/v1/thing", nil))
	if allow := rec.Header().Get("Allow"); !strings.Contains(allow, "GET") {
		t.Fatalf("405 Allow = %q, want 含 GET", allow)
	}

	// 非 /api/ 路径保持 stdlib 行为（静态资源/healthz 不受影响）。
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/nope", nil))
	if rec.Code != 404 || strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("非 /api/ 404 不应被改写：status=%d ct=%q", rec.Code, rec.Header().Get("Content-Type"))
	}

	// 成功响应透传。
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/thing", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("成功响应被改动：status=%d body=%q", rec.Code, rec.Body.String())
	}
}
