// Package responses 提供 OpenAI Responses API 适配器（POST /v1/responses，streaming）。
// 适配遵循 OpenAI 新版 Responses API 规范的端点。支持思考链增量与输出截断识别。
package responses

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"tavernagent/internal/adapters/providers/providerutil"
	"tavernagent/internal/ports"
)

// Config 是 Responses API 适配器配置。
type Config struct {
	BaseURL           string
	Model             string
	APIKey            string
	Temperature       *float64
	MaxTokens         int
	ReasoningEffort   string // low | medium | high；空 = 默认（不发 reasoning 对象）
	TimeoutFirstToken time.Duration
	TimeoutIdle       time.Duration
	TimeoutTotal      time.Duration
	HTTPClient        *http.Client
}

// Provider 是 Responses API 流式供应商。
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

// New 创建 Responses API 适配器。
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
		ID: "openai-responses", Streaming: true, StructuredOutput: false,
		Continuation: false, ContextWindow: 0, TokenizerKnown: false,
	}, nil
}

type responsesRequest struct {
	Model           string             `json:"model"`
	Input           []responsesMessage `json:"input"`
	Stream          bool               `json:"stream"`
	MaxOutputTokens int                `json:"max_output_tokens,omitempty"`
	Temperature     *float64           `json:"temperature,omitempty"`
	// Reasoning 只在用户显式设置思考强度时出现（"默认"档整个对象缺席），
	// 避免不支持的供应商因为多出的字段直接 400。
	Reasoning *responsesReasoning `json:"reasoning,omitempty"`
}

type responsesReasoning struct {
	Effort string `json:"effort"` // low | medium | high
}

// responsesMessage 复用共享的 role/content 结构。
type responsesMessage = providerutil.Message

type sseEvent = providerutil.SSEEvent

type responsesPayload struct {
	Type           string `json:"type"`
	Delta          string `json:"delta"`
	ReasoningDelta string `json:"reasoning_delta"`
	// Message/Code 是 type=="error" 事件的顶层字段（Responses 流式错误事件不嵌套 error 对象）。
	Message  string `json:"message"`
	Code     string `json:"code"`
	Response *struct {
		Status string `json:"status"`
		// IncompleteDetails 在 status=="incomplete" 时说明原因：
		// max_output_tokens 才是可续写的截断；content_filter 续写只会再被过滤。
		IncompleteDetails *struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
		// Error 出现在 response.failed 事件中。
		Error *struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
		// Usage 出现在 response.completed / response.done 事件中。
		// Responses API 默认回传统计，无需额外开启选项。
		Usage *struct {
			InputTokens        int `json:"input_tokens"`
			OutputTokens       int `json:"output_tokens"`
			InputTokensDetails *struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"input_tokens_details"`
		} `json:"usage"`
	} `json:"response"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

func (p *Provider) Stream(ctx context.Context, req ports.ChatRequest, sink ports.StreamSink) error {
	body, err := p.buildBody(req)
	if err != nil {
		return err
	}

	// 始终派生可取消的上下文：保证 sink 提前返回时能唤醒 body 读取 goroutine。
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if p.cfg.TimeoutTotal > 0 {
		var timeoutCancel context.CancelFunc
		streamCtx, timeoutCancel = context.WithTimeout(streamCtx, p.cfg.TimeoutTotal)
		defer timeoutCancel()
	}

	httpReq, err := http.NewRequestWithContext(streamCtx, http.MethodPost, p.cfg.BaseURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("responses: 构造请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}

	resp, firstBudget, headerTimedOut, err := providerutil.DoWithinFirstTokenBudget(p.cfg.HTTPClient, httpReq, p.cfg.TimeoutFirstToken)
	if headerTimedOut {
		return ErrTimeoutFirstToken
	}
	if err != nil {
		if streamCtx.Err() != nil {
			return providerutil.StreamAbort(ctx)
		}
		return fmt.Errorf("responses: 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return providerutil.ReadHTTPError(resp, providerutil.DecodeOpenAIError)
	}

	// body 读取 goroutine（共享实现见 providerutil.ReadSSE）。
	eventCh, bodyErrCh := providerutil.ReadSSE(streamCtx, resp.Body)

	tokenSeen := false
	// eventSeen 区分『供应商没按 SSE 说话』与『模型写了一半被截断』。
	eventSeen := false
	finishLength := false

	var firstTimer *time.Timer
	var firstDone <-chan time.Time
	if p.cfg.TimeoutFirstToken > 0 {
		// 响应头恰好卡在首字时限上到达时 firstBudget 可能 ≤0：给一个下限，
		// 否则 NewTimer(≤0) 会立即触发并误报"等待首个 token 超时"。
		budget := firstBudget
		if budget <= 0 {
			budget = p.cfg.TimeoutIdle
		}
		if budget <= 0 {
			budget = providerutil.DefaultTimeoutFirstToken
		}
		firstTimer = time.NewTimer(budget)
		firstDone = firstTimer.C
		defer firstTimer.Stop()
	}

	idleTimer := time.NewTimer(p.cfg.TimeoutIdle)
	idleTimer.Stop()
	defer idleTimer.Stop()

	thinkingFn := ports.ThinkingCallbackFromContext(ctx)

	for {
		select {
		case <-streamCtx.Done():
			return providerutil.StreamAbort(ctx)
		case err := <-bodyErrCh:
			if streamCtx.Err() != nil {
				return providerutil.StreamAbort(ctx)
			}
			return fmt.Errorf("responses: 读取流失败: %w", err)
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

			if strings.TrimSpace(string(ev.Data)) == "[DONE]" {
				if !tokenSeen {
					return providerutil.EmptyStreamError(eventSeen)
				}
				if finishLength {
					return ErrTruncated
				}
				return nil
			}

			var payload responsesPayload
			if err := json.Unmarshal(ev.Data, &payload); err != nil {
				continue
			}

			if payload.Error != nil {
				return fmt.Errorf("responses: 服务端错误: %s", payload.Error.Message)
			}
			if payload.Type == "error" {
				return fmt.Errorf("responses: 服务端错误: %s", firstNonEmpty(payload.Message, payload.Code, "未知错误"))
			}

			if payload.Response != nil {
				switch payload.Response.Status {
				case "failed", "cancelled":
					msg := payload.Response.Status
					if e := payload.Response.Error; e != nil {
						msg = firstNonEmpty(e.Message, e.Code, msg)
					}
					return fmt.Errorf("responses: 生成失败: %s", msg)
				case "incomplete":
					// 截断检查：只有输出上限导致的截断才进入续写态。
					reason := ""
					if d := payload.Response.IncompleteDetails; d != nil {
						reason = d.Reason
					}
					if reason != "" && reason != "max_output_tokens" {
						return fmt.Errorf("responses: 输出未完成: %s", reason)
					}
					finishLength = true
				}
				if u := payload.Response.Usage; u != nil {
					cached := 0
					if u.InputTokensDetails != nil {
						cached = u.InputTokensDetails.CachedTokens
					}
					sink.Usage(ports.TokenUsage{
						Prompt:     u.InputTokens,
						Completion: u.OutputTokens,
						Cached:     cached,
						Reported:   true,
					})
				}
			}

			// 事件类型：payload.type 与 SSE 的 event: 行名二选一（不同网关填法不一），
			// 优先 payload.type。真实 Responses 事件名是 response.output_text.delta
			// 与 response.reasoning_summary_text.delta / response.reasoning_text.delta；
			// 此前只显式认 response.text.delta（并不存在），思考摘要因此被当成正文
			// 塞给帧解析器，污染正文并可能整轮失败。
			evType := payload.Type
			if evType == "" {
				evType = ev.Event
			}
			isReasoning := strings.Contains(evType, "reasoning")

			// 思考链增量（reasoning 事件，或供应商自带的 reasoning_delta 字段）。
			reasoningText := payload.ReasoningDelta
			if reasoningText == "" && isReasoning {
				reasoningText = payload.Delta
			}
			if reasoningText != "" {
				tokenSeen = true
				if firstDone != nil {
					firstDone = nil
				}
				idleTimer.Reset(p.cfg.TimeoutIdle)
				if thinkingFn != nil {
					thinkingFn(reasoningText)
				}
			}

			// 正文内容增量：非 reasoning 事件携带 delta 的即为正文。
			// response.output_text.done 等终帧只带 text、不带 delta，因此不会重复。
			contentText := ""
			if !isReasoning && payload.Delta != "" {
				contentText = payload.Delta
			}

			if contentText != "" {
				tokenSeen = true
				if firstDone != nil {
					firstDone = nil
				}
				idleTimer.Reset(p.cfg.TimeoutIdle)
				if err := sink.Chunk([]byte(contentText)); err != nil {
					return err
				}
			}
		}
	}
}

func (p *Provider) buildBody(req ports.ChatRequest) ([]byte, error) {
	msgs := providerutil.NormalizeMessages(req.Messages)
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = p.cfg.MaxTokens
	}
	cr := responsesRequest{
		Model:           p.cfg.Model,
		Input:           msgs,
		Stream:          true,
		MaxOutputTokens: maxTokens,
		Temperature:     p.cfg.Temperature,
	}
	if p.cfg.ReasoningEffort != "" {
		cr.Reasoning = &responsesReasoning{Effort: p.cfg.ReasoningEffort}
	}
	return json.Marshal(cr)
}

func normalizeURL(baseURL string) string {
	raw := strings.TrimRight(baseURL, "/")
	raw = strings.TrimSuffix(raw, "/chat/completions")
	if strings.HasSuffix(raw, "/responses") {
		return raw
	}
	return raw + "/responses"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
