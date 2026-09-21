package http

import (
	"net/http/httptest"
	"strings"
	"testing"

	"tavernagent/internal/util/trace"
)

// requestID 相关的行为都在中间件里，所以用真实 Handler 打一次请求来验证，
// 而不是单独测 sanitize（那是实现细节，回显与生成才是对外契约）。
func TestRequestIDIsGeneratedEchoedAndSanitized(t *testing.T) {
	recorder := trace.NewRecorder(8)
	srv := mustNew(t, Deps{Addr: "127.0.0.1:8890", Tracer: recorder})
	handler := srv.Handler()

	for _, tc := range []struct {
		name      string
		sent      string
		wantEcho  bool
		expectLog string
	}{
		{name: "未携带则服务端生成", sent: "", wantEcho: false},
		{name: "合法的客户端 ID 原样回显", sent: "trace-abc-123", wantEcho: true},
		{name: "超长 ID 一律丢弃", sent: strings.Repeat("x", maxRequestIDLen+1), wantEcho: false},
		{name: "含控制字符的 ID 一律丢弃", sent: "bad\nid", wantEcho: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://127.0.0.1:8890/api/status", nil)
			if tc.sent != "" {
				req.Header.Set(requestIDHeader, tc.sent)
			}
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)

			got := res.Header().Get(requestIDHeader)
			if got == "" {
				t.Fatal("响应必须回显请求 ID（用户报错时要能对上服务端日志）")
			}
			if tc.wantEcho && got != tc.sent {
				t.Fatalf("合法 ID 应原样回显：got %q want %q", got, tc.sent)
			}
			if !tc.wantEcho && got == tc.sent {
				t.Fatalf("非法 ID 不应被回显：%q", got)
			}
		})
	}
}

// 健康检查是高频探活，不该被记录进追踪读数，也不该回显 ID。
func TestHealthzIsExcludedFromTracing(t *testing.T) {
	recorder := trace.NewRecorder(8)
	srv := mustNew(t, Deps{Addr: "127.0.0.1:8890", Tracer: recorder})

	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, httptest.NewRequest("GET", "http://127.0.0.1:8890/healthz", nil))

	if got := res.Header().Get(requestIDHeader); got != "" {
		t.Fatalf("healthz 不应回显请求 ID，得到 %q", got)
	}
	if summary := recorder.TraceSnapshot(); summary.Total != 0 {
		t.Fatalf("healthz 不应进入追踪读数，得到 %+v", summary)
	}
}

// /api/status 的 trace 段：请求 ID 要能被关联回具体那一次请求。
func TestStatusExposesTraceSection(t *testing.T) {
	recorder := trace.NewRecorder(8)
	srv := mustNew(t, Deps{Addr: "127.0.0.1:8890", Tracer: recorder})

	req := httptest.NewRequest("GET", "http://127.0.0.1:8890/api/status", nil)
	req.Header.Set(requestIDHeader, "status-probe-1")
	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, req)

	if res.Code != 200 {
		t.Fatalf("status = %d", res.Code)
	}
	if !strings.Contains(res.Body.String(), `"trace"`) {
		t.Fatalf("status 应包含 trace 段: %s", res.Body.String())
	}
	// 这一条状态请求本身也应当被记录（中间件在 handler 之后收尾）。
	summary := recorder.TraceSnapshot()
	if summary.Total < 1 || len(summary.Recent) == 0 {
		t.Fatalf("状态请求本身应被记录: %+v", summary)
	}
	if summary.Recent[0].Path != "/api/status" || summary.Recent[0].Status != 200 {
		t.Fatalf("记录内容不对: %+v", summary.Recent[0])
	}
}

// 没配记录器时状态端点仍要可用，只是少一段——观测不得影响服务可用性。
func TestStatusWorksWithoutRecorder(t *testing.T) {
	srv := mustNew(t, Deps{Addr: "127.0.0.1:8890"})
	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, httptest.NewRequest("GET", "http://127.0.0.1:8890/api/status", nil))

	if res.Code != 200 {
		t.Fatalf("status = %d", res.Code)
	}
	if strings.Contains(res.Body.String(), `"trace"`) {
		t.Fatalf("没有记录器时不该出现 trace 段: %s", res.Body.String())
	}
}
