// Package responses 提供 OpenAI Responses API 适配器（POST /v1/responses，streaming）。
// 适配遵循 OpenAI 新版 Responses API 规范的端点。支持思考链增量与输出截断识别。
package responses

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
	Response       *struct {
		Status        string `json:"status"`
		StatusDetails *struct {
			Reason string `json:"reason"`
		} `json:"status_details"`
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

	streamCtx := ctx
	cancel := func() {}
	if p.cfg.TimeoutTotal > 0 {
		streamCtx, cancel = context.WithTimeout(ctx, p.cfg.TimeoutTotal)
	}
	defer cancel()

	httpReq, err := http.NewRequestWithContext(streamCtx, http.MethodPost, p.cfg.BaseURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("responses: 构造请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}

	resp, err := p.cfg.HTTPClient.Do(httpReq)
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

			// 截断检查：response.done 中 status 为 incomplete 或 max_output_tokens
			if payload.Response != nil {
				if payload.Response.Status == "incomplete" {
					finishLength = true
				}
				if payload.Response.StatusDetails != nil && payload.Response.StatusDetails.Reason == "max_output_tokens" {
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

			// 思考链增量
			reasoningText := payload.ReasoningDelta
			if reasoningText == "" && (ev.Event == "response.reasoning_text.delta" || ev.Event == "response.reasoning.delta" || payload.Type == "response.reasoning_text.delta" || payload.Type == "response.reasoning.delta") {
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

			// 正文内容增量
			contentText := ""
			if ev.Event == "response.text.delta" || payload.Type == "response.text.delta" || (payload.Delta != "" && reasoningText == "") {
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
