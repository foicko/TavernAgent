// Package httpbody 把上游 HTTP 错误体转成可安全放进错误信息与日志的摘要。
//
// 此前三处各有一份实现（图像/语音按字节截断、供应商层按 rune 截断并替换 HTML）：
// 同一响应会得到不同文案，且按字节截断会把多字节字符切坏。集中到一处后，
// 「长度上限、去空白、HTML 错误页替换、UTF-8 安全截断」只有一种语义。
package httpbody

import "strings"

// SnippetLimit 是错误体摘要的字符上限。
const SnippetLimit = 512

// HTMLBodyNotice 是网关 HTML 错误页替换后的提示（原样回显只会在 UI 里变成一坨标签）。
const HTMLBodyNotice = "上游返回了非预期的 HTML 响应（通常是网关或代理错误页），请检查接口地址与网关状态"

// Snippet 返回至多 limit 个字符的响应体摘要：
//   - 网关/代理的 HTML 错误页替换为可读提示；
//   - 去掉首尾空白；
//   - 按 rune 截断，不切坏 UTF-8；截断时补 "…" 以示省略。
//
// body 为空时返回空串，由调用方决定兜底文案（如"无响应体"）。
func Snippet(body []byte, limit int) string {
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		return ""
	}
	if looksLikeHTML(msg) {
		return HTMLBodyNotice
	}
	runes := []rune(msg)
	if limit > 0 && len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return msg
}

// looksLikeHTML 判断响应体是否是 HTML 错误页（大小写不敏感）。
func looksLikeHTML(s string) bool {
	lower := strings.ToLower(s)
	return strings.HasPrefix(lower, "<!doctype") || strings.HasPrefix(lower, "<html") || strings.Contains(lower, "<html")
}
