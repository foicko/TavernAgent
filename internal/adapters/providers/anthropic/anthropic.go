// Package anthropic 提供 Anthropic Messages API 适配器（POST /v1/messages，streaming）。
// 适配 Claude 官方接口及兼容网关。支持思考链增量与 max_tokens 输出截断识别。
package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"tavernagent/internal/adapters/providers/providerutil"
	"tavernagent/internal/ports"
)

const (
	defaultMaxTokens = 4096
	anthropicVersion = "2023-06-01"
	// minThinkingBudget 是 Anthropic 允许的最小思考预算（budget_tokens ≥ 1024，
	// 且必须严格小于 max_tokens）。
	minThinkingBudget = 1024
)

// effortThinkingPercent 把界面档位折算成占输出上限的思考预算比例：
// 强度是相对的，用户把最大输出调大时思考空间跟着变大。
var effortThinkingPercent = map[string]int{"low": 30, "medium": 50, "high": 75}

// Config 是 Anthropic 适配器配置。
type Config struct {
	BaseURL           string
	Model             string
	APIKey            string
	Temperature       *float64
	MaxTokens         int
	ReasoningEffort   string // low | medium | high；空 = 默认（不开启 extended thinking）
	TimeoutFirstToken time.Duration
	TimeoutIdle       time.Duration
	TimeoutTotal      time.Duration
	HTTPClient        *http.Client
}

// Provider 是 Anthropic 流式供应商。
type Provider struct {
	cfg Config
}

var _ ports.ModelProvider = (*Provider)(nil)

// 语义错误（供应用层分类：PROVIDER_UNAVAILABLE / awaiting_continuation）。
// ErrTruncated 复用 ports.ErrTruncatedStream，应用层依赖该哨兵进入续写态；
// 超时哨兵、空流判定与 HTTPError 由三条协议共享，见 providerutil。
var (
	ErrTruncated         = ports.ErrTruncatedStream
	ErrTimeoutFirstToken = providerutil.ErrTimeoutFirstToken
	ErrTimeoutIdle       = providerutil.ErrTimeoutIdle
	ErrTimeoutTotal      = providerutil.ErrTimeoutTotal
)

// HTTPError 是共享的供应商非 2xx 错误类型（保留包内名称以稳定调用方）。
type HTTPError = providerutil.HTTPError

// New 创建 Anthropic 适配器。
func New(cfg Config) *Provider {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{}
	}
	cfg.BaseURL = normalizeURL(cfg.BaseURL)
	if cfg.TimeoutFirstToken == 0 {
		cfg.TimeoutFirstToken = providerutil.DefaultTimeoutFirstToken
	}
	if cfg.TimeoutIdle == 0 {
		cfg.TimeoutIdle = providerutil.DefaultTimeoutIdle
	}
	if cfg.TimeoutTotal == 0 {
		cfg.TimeoutTotal = providerutil.DefaultTimeoutTotal
	}
	return &Provider{cfg: cfg}
}

func (p *Provider) Capabilities(ctx context.Context) (ports.ProviderCapabilities, error) {
	return ports.ProviderCapabilities{
		ID: "anthropic-messages", Streaming: true, StructuredOutput: false,
		Continuation: false, ContextWindow: 0, TokenizerKnown: false,
	}, nil
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	System      string             `json:"system,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	MaxTokens   int                `json:"max_tokens"`
	Stream      bool               `json:"stream"`
	Temperature *float64           `json:"temperature,omitempty"`
	// Thinking 只在用户显式设置思考强度且预算放得下时出现。
	Thinking *anthropicThinking `json:"thinking,omitempty"`
}

// anthropicThinking 是 extended thinking 配置（Anthropic 要求 budget_tokens ≥ 1024
// 且严格小于 max_tokens；开启时 temperature 只能保持默认的 1）。
type anthropicThinking struct {
	Type         string `json:"type"` // 固定 "enabled"
	BudgetTokens int    `json:"budget_tokens"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type sseEvent = providerutil.SSEEvent

// anthropicUsage 是 Anthropic Messages 的 usage 对象。
//
// cache_read_input_tokens 是**命中提示缓存**的输入 token，是判断提示词前缀是否
// 稳定的直接证据；cache_creation_input_tokens 是本次写入缓存的量（不算命中）。
type anthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}

type anthropicDeltaPayload struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Delta struct {
		Type         string `json:"type"`
		Text         string `json:"text"`
		Thinking     string `json:"thinking"`
		StopReason   string `json:"stop_reason"`
		StopSequence string `json:"stop_sequence"`
	} `json:"delta"`
	// Message 出现在 message_start 事件中，携带首帧 usage（输入与缓存读取量）。
	Message *struct {
		Usage *anthropicUsage `json:"usage"`
	} `json:"message"`
	// Usage 出现在 message_delta 事件中，携带累积的 output_tokens。
	Usage *anthropicUsage `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (p *Provider) Stream(ctx context.Context, req ports.ChatRequest, sink ports.StreamSink) error {
	body, err := p.buildBody(req)
	if err != nil {
		return err
	}

	streamCtx := ctx
	cancel := func() {}
	if p.cfg.TimeoutTotal > 0 {
		streamCtx, cancel = context.WithTimeout(ctx, p.cfg.TimeoutTotal)
	}
	defer cancel()

	httpReq, err := http.NewRequestWithContext(streamCtx, http.MethodPost, p.cfg.BaseURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("anthropic: 构造请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	if p.cfg.APIKey != "" {
		httpReq.Header.Set("x-api-key", p.cfg.APIKey)
		// 兼容一些接收标准 Bearer Token 的转发中转网关
		httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}

	resp, err := p.cfg.HTTPClient.Do(httpReq)
	if err != nil {
		if streamCtx.Err() != nil {
			return providerutil.StreamAbort(ctx)
		}
		return fmt.Errorf("anthropic: 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return providerutil.ReadHTTPError(resp, decodeAPIError)
	}

	eventCh := make(chan sseEvent, 16)
	bodyErrCh := make(chan error, 1)

	go func() {
		defer close(eventCh)
		br := bufio.NewReader(resp.Body)
		var currentEvent string
		for {
			line, err := br.ReadBytes('\n')
			if err != nil {
				if err != io.EOF {
					select {
					case bodyErrCh <- err:
					default:
					}
				}
				return
			}
			t := bytes.TrimSpace(line)
			if len(t) == 0 {
				currentEvent = ""
				continue
			}
			if bytes.HasPrefix(t, []byte("event:")) {
				currentEvent = strings.TrimSpace(string(t[len("event:"):]))
				continue
			}
			if bytes.HasPrefix(t, []byte("data:")) {
				data := bytes.TrimSpace(t[len("data:"):])
				if len(data) == 0 {
					continue
				}
				select {
				case eventCh <- sseEvent{Event: currentEvent, Data: data}:
				case <-streamCtx.Done():
					return
				}
			}
		}
	}()

	tokenSeen := false
	// eventSeen 区分『供应商没按 SSE 说话』与『模型写了一半被截断』。
	eventSeen := false
	finishLength := false

	var firstTimer *time.Timer
	var firstDone <-chan time.Time
	if p.cfg.TimeoutFirstToken > 0 {
		firstTimer = time.NewTimer(p.cfg.TimeoutFirstToken)
		firstDone = firstTimer.C
		defer firstTimer.Stop()
	}

	idleTimer := time.NewTimer(p.cfg.TimeoutIdle)
	idleTimer.Stop()
	defer idleTimer.Stop()

	thinkingFn := ports.ThinkingCallbackFromContext(ctx)

	// usage 在 Anthropic 是分帧上报的：message_start 给输入与缓存读取量，
	// message_delta 给累积输出量。这里合并成一份再上报，保证调用方收到的
	// 每一次回调都是「到目前为止的完整值」（取最后一次即最终结果）。
	var usagePrompt, usageCompletion, usageCached int
	absorbUsage := func(u *anthropicUsage) {
		if u == nil {
			return
		}
		if u.InputTokens > 0 {
			usagePrompt = u.InputTokens
		}
		if u.OutputTokens > 0 {
			usageCompletion = u.OutputTokens
		}
		if u.CacheReadInputTokens > 0 {
			usageCached = u.CacheReadInputTokens
		}
	}
	emitUsage := func() {
		if usagePrompt == 0 && usageCompletion == 0 && usageCached == 0 {
			return
		}
		sink.Usage(ports.TokenUsage{
			Prompt:     usagePrompt,
			Completion: usageCompletion,
			Cached:     usageCached,
			Reported:   true,
		})
	}

	for {
		select {
		case <-streamCtx.Done():
			return providerutil.StreamAbort(ctx)
		case err := <-bodyErrCh:
			if streamCtx.Err() != nil {
				return providerutil.StreamAbort(ctx)
			}
			return fmt.Errorf("anthropic: 读取流失败: %w", err)
		case <-firstDone:
			if !tokenSeen {
				return ErrTimeoutFirstToken
			}
			firstDone = nil
		case <-idleTimer.C:
			if tokenSeen {
				return ErrTimeoutIdle
			}
		case ev, ok := <-eventCh:
			if ok {
				eventSeen = true
			}
			if !ok {
				if !tokenSeen {
					return providerutil.EmptyStreamError(eventSeen)
				}
				if finishLength {
					return ErrTruncated
				}
				return nil
			}

			if ev.Event == "message_stop" {
				if !tokenSeen {
					return providerutil.EmptyStreamError(eventSeen)
				}
				if finishLength {
					return ErrTruncated
				}
				return nil
			}

			var payload anthropicDeltaPayload
			if err := json.Unmarshal(ev.Data, &payload); err != nil {
				continue
			}

			if payload.Error != nil {
				return fmt.Errorf("anthropic: 服务端错误 (%s): %s", payload.Error.Type, payload.Error.Message)
			}

			// 检查截断：message_delta 中的 stop_reason: max_tokens
			if payload.Delta.StopReason == "max_tokens" {
				finishLength = true
			}

			// usage：message_start 与 message_delta 各带一部分，合并后上报。
			if payload.Message != nil {
				absorbUsage(payload.Message.Usage)
			}
			absorbUsage(payload.Usage)
			emitUsage()

			// 思考链增量 (Claude 3.7 Extended Thinking)
			if payload.Delta.Thinking != "" {
				tokenSeen = true
				if firstDone != nil {
					firstDone = nil
				}
				idleTimer.Reset(p.cfg.TimeoutIdle)
				if thinkingFn != nil {
					thinkingFn(payload.Delta.Thinking)
				}
			}

			// 正文内容增量
			if payload.Delta.Text != "" {
				tokenSeen = true
				if firstDone != nil {
					firstDone = nil
				}
				idleTimer.Reset(p.cfg.TimeoutIdle)
				if err := sink.Chunk([]byte(payload.Delta.Text)); err != nil {
					return err
				}
			}
		}
	}
}

func (p *Provider) buildBody(req ports.ChatRequest) ([]byte, error) {
	var systemParts []string
	var rawMessages []anthropicMessage

	for _, m := range req.Messages {
		if strings.EqualFold(m.Role, "system") {
			if strings.TrimSpace(m.Content) != "" {
				systemParts = append(systemParts, m.Content)
			}
		} else {
			role := "user"
			if strings.EqualFold(m.Role, "assistant") {
				role = "assistant"
			}
			rawMessages = append(rawMessages, anthropicMessage{Role: role, Content: m.Content})
		}
	}

	// Anthropic 必须保证 user 和 assistant 严格交替，连续相同角色必须合并
	var mergedMessages []anthropicMessage
	for _, m := range rawMessages {
		if len(mergedMessages) > 0 && mergedMessages[len(mergedMessages)-1].Role == m.Role {
			mergedMessages[len(mergedMessages)-1].Content += "\n\n" + m.Content
		} else {
			mergedMessages = append(mergedMessages, m)
		}
	}

	if len(mergedMessages) == 0 {
		mergedMessages = append(mergedMessages, anthropicMessage{Role: "user", Content: "Hello"})
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = p.cfg.MaxTokens
	}
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	ar := anthropicRequest{
		Model:       p.cfg.Model,
		System:      strings.Join(systemParts, "\n\n"),
		Messages:    mergedMessages,
		MaxTokens:   maxTokens,
		Stream:      true,
		Temperature: p.cfg.Temperature,
	}
	if budget, ok := thinkingBudget(p.cfg.ReasoningEffort, maxTokens); ok {
		ar.Thinking = &anthropicThinking{Type: "enabled", BudgetTokens: budget}
		// Anthropic 规定开启 extended thinking 时 temperature 只能是默认的 1，
		// 省略比显式发送更稳妥（供应商默认即为 1）。
		ar.Temperature = nil
	}

	return json.Marshal(ar)
}

// thinkingBudget 计算 extended thinking 的 token 预算；放不下（max_tokens ≤ 1024）
// 时返回 ok=false，该连接上的档位退化为默认（不开启思考）。
func thinkingBudget(effort string, maxTokens int) (int, bool) {
	percent, known := effortThinkingPercent[effort]
	if !known {
		return 0, false
	}
	budget := maxTokens * percent / 100
	if budget < minThinkingBudget {
		budget = minThinkingBudget
	}
	if budget > maxTokens-1 {
		budget = maxTokens - 1
	}
	if budget < minThinkingBudget {
		return 0, false
	}
	return budget, true
}

func normalizeURL(baseURL string) string {
	raw := strings.TrimRight(baseURL, "/")
	raw = strings.TrimSuffix(raw, "/chat/completions")
	raw = strings.TrimSuffix(raw, "/responses")
	if strings.HasSuffix(raw, "/messages") {
		return raw
	}
	if strings.HasSuffix(raw, "/v1") {
		return raw + "/messages"
	}
	if !strings.Contains(raw, "/v1") {
		return raw + "/v1/messages"
	}
	return raw + "/messages"
}

// decodeAPIError 解析 Anthropic 的错误体：{"type":...,"error":{"type","message"}}。
// 错误码取 error.type（如 invalid_request_error），与 OpenAI 系取 error.code 不同。
func decodeAPIError(body []byte) (code, message string, ok bool) {
	var envelope struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &envelope) == nil && envelope.Error.Message != "" {
		return envelope.Error.Type, envelope.Error.Message, true
	}
	return "", "", false
}
