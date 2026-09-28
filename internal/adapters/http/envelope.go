// 错误信封中间件：把 net/http 自己产生的纯文本 404/405 统一改写为规范信封。
//
// stdlib 的 NotFoundHandler / 405 不经过 writeError，直接透传会破坏
// "所有 /api/ 错误都是同一形状"（docs/spec/04-errors.md）的契约。此中间件只
// 拦截 `/api/` 路径上的 404/405：头不转发、体丢弃，交由本层统一写信封；
// 其余状态码（含 SSE 的 200 + Flush）完全透传。
package http

import (
	"net/http"
	"strings"
)

func apiErrorEnvelope(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ew := &apiEnvelopeWriter{ResponseWriter: w, api: strings.HasPrefix(r.URL.Path, "/api/")}
		next.ServeHTTP(ew, r)
		if !ew.intercepted {
			return
		}
		code, message := "NOT_FOUND", "资源不存在"
		if ew.status == http.StatusMethodNotAllowed {
			code, message = "METHOD_NOT_ALLOWED", "请求方法不允许"
		}
		// w 已带上 withLogging 写入的 X-Request-ID，信封里会带 traceId；
		// 405 的 Allow 头由 stdlib 预先写入 w.Header()，这里保持不变。
		writeError(w, ew.status, code, message, false, "")
	})
}

// apiEnvelopeWriter 只在 `/api/` 路径上拦截 404/405。
type apiEnvelopeWriter struct {
	http.ResponseWriter
	api         bool
	status      int
	intercepted bool
	wroteHeader bool
}

// Unwrap 让 http.ResponseController 能穿透到真实 writer。
func (w *apiEnvelopeWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *apiEnvelopeWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = code
	if w.api && (code == http.StatusNotFound || code == http.StatusMethodNotAllowed) {
		w.intercepted = true
		return // 不转发：稍后由 apiErrorEnvelope 统一写信封
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *apiEnvelopeWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.intercepted {
		return len(p), nil // 丢弃 stdlib 的纯文本体
	}
	return w.ResponseWriter.Write(p)
}

// Flush 透传：SSE 路径依赖 w.(http.Flusher) 断言，包装层必须保持该能力。
func (w *apiEnvelopeWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
