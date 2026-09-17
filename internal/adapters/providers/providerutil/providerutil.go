// Package providerutil 汇集三条供应商协议适配器共享的错误分类与 HTTP 小工具。
//
// 各适配器只在请求构造与响应映射上不同；「超时归类、空流判定、错误体解析」
// 的语义完全一致。集中在这里避免三份拷贝各自漂移——历史上取消曾被误记为
// 供应商超时，正是因为这段逻辑散落在每个适配器里。
package providerutil

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"tavernagent/internal/ports"
)

// 默认超时：首字 / 空闲 / 总时限。各适配器的 Config 可覆盖。
const (
	DefaultTimeoutFirstToken = 60 * time.Second
	DefaultTimeoutIdle       = 30 * time.Second
	DefaultTimeoutTotal      = 5 * time.Minute
)

// 语义错误：应用层据此区分重试、续写与取消。
var (
	ErrTimeoutFirstToken = errors.New("超时：等待首个 token 超时")
	ErrTimeoutIdle       = errors.New("超时：token 间空闲超时")
	ErrTimeoutTotal      = errors.New("超时：超出总时限")
)

// HTTPError 是供应商返回的非 2xx 错误。
type HTTPError struct {
	StatusCode int
	Code       string // 供应商错误码（如有）
	Message    string
}

func (e *HTTPError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("供应商返回 %d (%s): %s", e.StatusCode, e.Code, e.Message)
	}
	return fmt.Sprintf("供应商返回 %d: %s", e.StatusCode, e.Message)
}

// EmptyStreamError 在没有产生任何 token 时给出正确归类。
//
// 收到过流式事件/载荷却没有任何内容 = 供应商明确返回了空回复（按截断处理，
// 应用层会带着已有草稿进入续写）；一个事件都没收到 = 对方没按流式协议说话
// （典型是网关 200 + HTML 错误页），续写无从谈起，必须归类为供应商不可用。
func EmptyStreamError(eventSeen bool) error {
	if eventSeen {
		return ports.ErrTruncatedStream
	}
	return ports.ErrNonStreamingResponse
}

// StreamAbort 区分「调用方取消」与「本适配器总时限到期」。
//
// 两者都会让 streamCtx 结束，但含义完全不同：前者是后台任务被新版本取代
// （排队合并主动取消）或用户停止生成，用量账本应记为 cancelled；后者才是
// 供应商超时。此前一律返回 ErrTimeoutTotal，导致取消被记成失败、
// 排障时看到「20 次失败」却不知道其中大多数根本没打算跑完。
func StreamAbort(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrTimeoutTotal
}

// Message 是 role/content 对；Chat Completions 与 Responses 的消息数组结构一致。
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// SSEEvent 是一行 SSE 事件（event 名 + data 载荷），Responses 与 Messages 协议共用。
type SSEEvent struct {
	Event string
	Data  []byte
}

// NormalizeMessages 规范化消息列表以兼容各类下游供应商（如通过代理转接
// Gemini / Claude 的兼容层）：
//  1. 去除空白 assistant/system 消息；
//  2. 保证消息内容不为空白，避免代理静默丢弃后导致角色顺序错乱；
//  3. 严格保证对话必须以非空 user 消息结尾
//     （避免 "Requests ending with a model turn are not supported" 400）。
func NormalizeMessages(messages []ports.ChatMessage) []Message {
	msgs := make([]Message, 0, len(messages)+1)
	for _, m := range messages {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		if role == "" {
			role = "user"
		}
		content := m.Content
		if strings.TrimSpace(content) == "" {
			if role == "assistant" || role == "system" {
				continue
			}
			content = "..."
		}
		msgs = append(msgs, Message{Role: role, Content: content})
	}

	if len(msgs) == 0 {
		return []Message{{Role: "user", Content: "..."}}
	}

	lastIdx := len(msgs) - 1
	if msgs[lastIdx].Role == "system" {
		msgs[lastIdx].Role = "user"
	} else if msgs[lastIdx].Role == "assistant" {
		msgs = append(msgs, Message{Role: "user", Content: "请继续"})
	}

	if strings.TrimSpace(msgs[len(msgs)-1].Content) == "" {
		msgs[len(msgs)-1].Content = "..."
	}

	return msgs
}

// APIErrorDecoder 把供应商特有的错误 JSON 解码为错误码与消息；ok=false 表示
// 该 JSON 不匹配（调用方随后回退到纯文本错误体）。
type APIErrorDecoder func(body []byte) (code, message string, ok bool)

// ReadHTTPError 读取并解析非 2xx 响应体：协议特有的 JSON 形状由 decoder 提供，
// 其余处理（限长、纯文本兜底、空体兜底）三协议一致。
func ReadHTTPError(resp *http.Response, decoder APIErrorDecoder) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if decoder != nil {
		if code, message, ok := decoder(body); ok && message != "" {
			return &HTTPError{StatusCode: resp.StatusCode, Code: code, Message: message}
		}
	}
	msg := strings.TrimSpace(string(body))
	if len(msg) > 512 {
		msg = msg[:512]
	}
	if msg == "" {
		msg = "无响应体"
	}
	return &HTTPError{StatusCode: resp.StatusCode, Message: msg}
}

// DecodeOpenAIError 解析 OpenAI 系（Chat Completions 与 Responses）的错误体：
// {"error":{"message","type","code"}}。
func DecodeOpenAIError(body []byte) (code, message string, ok bool) {
	var envelope struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &envelope) == nil && envelope.Error.Message != "" {
		return envelope.Error.Code, envelope.Error.Message, true
	}
	return "", "", false
}
