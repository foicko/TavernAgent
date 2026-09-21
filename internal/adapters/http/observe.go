// 观测中间件：访问日志、状态码捕获，以及请求级关联 ID。
//
// 从 server.go 抽出来：观测是横切关注点，与业务路由无关。
package http

import (
	"log"
	"net/http"
	"strings"
	"time"

	"tavernagent/internal/ports"
	"tavernagent/internal/util/id"
)

// requestIDHeader 是请求级关联 ID 的头名：进来时若带上就沿用，出去时一定回显。
//
// 关联 ID 只用于把"客户端看到的那次失败"和"服务端日志里的那几行"对上，
// 因此它**不进任何持久化存储**，也不允许携带业务数据。
const requestIDHeader = "X-Request-ID"

// maxRequestIDLen 限制客户端自带的 ID 长度：它会被写进日志，不能无界。
const maxRequestIDLen = 64

// withLogging 是唯一的入口中间件：分配请求 ID → 开一个请求级 span → 记录访问日志。
//
// 之前只有一行访问日志，且不带任何关联信息：一次慢请求只能看到"总耗时 3.2s"，
// 无法回答"慢在编译还是慢在模型"，也没法把客户端的报错与服务端日志对上。
func withLogging(next http.Handler, tracer ports.Tracer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 健康检查是高频探活，跳过避免日志噪音。
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		requestID := sanitizeRequestID(r.Header.Get(requestIDHeader))
		if requestID == "" {
			requestID = id.New()
		}
		// 回显给客户端：出问题时用户能直接报出这个 ID。
		w.Header().Set(requestIDHeader, requestID)

		start := time.Now()
		ctx, span := tracer.Start(r.Context(), "http.request")
		span.SetAttr("requestId", requestID)
		span.SetAttr("method", r.Method)
		span.SetAttr("path", r.URL.Path)

		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r.WithContext(ctx))

		duration := time.Since(start)
		span.SetAttr("status", rec.status)
		span.End(nil)
		log.Printf("%s %s -> %d (%s) req=%s", r.Method, r.URL.Path, rec.status, duration.Round(time.Millisecond), requestID)
	})
}

// sanitizeRequestID 只接受短的可打印 ASCII 串；其余（空、超长、含控制字符）一律丢弃，
// 由调用方重新生成。客户端给什么就写什么，等于把日志注入的入口交给外部。
func sanitizeRequestID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxRequestIDLen {
		return ""
	}
	for _, ch := range raw {
		if ch < 0x21 || ch > 0x7e {
			return ""
		}
	}
	return raw
}

// statusRecorder 包装 ResponseWriter 捕获状态码（访问日志用）。
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Flush 透传：SSE 路径依赖 w.(http.Flusher) 断言，包装层必须保持该能力。
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
