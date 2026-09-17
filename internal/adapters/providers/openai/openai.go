// Package openai 提供 OpenAI 兼容 Chat Completions 适配器（streaming）。
// 适配任何实现了 HTTP /chat/completions + SSE 流式输出的供应商（OpenAI、DeepSeek、
// 本地 llama.cpp / vLLM / Ollama 兼容层等）。只读取增量文本并原样交付帧协议字节流。
package openai

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

// Config 是适配器配置（敏感字段由调用方注入，不落日志）。
type Config struct {
	BaseURL           string
	Model             string
	APIKey            string
	Temperature       *float64
	MaxTokens         int
	ReasoningEffort   string // low | medium | high；空 = 默认（不发 reasoning_effort）
	TimeoutFirstToken time.Duration
	TimeoutIdle       time.Duration
	TimeoutTotal      time.Duration
	HTTPClient        *http.Client
}

// Provider 是 OpenAI 兼容流式供应商。
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

// New 创建适配器。
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
		ID: "openai-compatible", Streaming: true, StructuredOutput: false,
		Continuation: false, ContextWindow: 0, TokenizerKnown: false,
	}, nil
}

// chatRequest 是对供应商的请求体（只发送 Content，无工具/函数，保证可移植）。
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
	// ReasoningEffort 只在用户显式设置时发送（"默认"档整个字段缺席），
	// 避免不支持的供应商因为多出的字段直接 400。
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	// StreamOptions 请求在流的末帧携带 usage（include_usage）。
	// 没有它，Chat Completions 流式响应不会回传统计，用量就永远测不到。
	StreamOptions *streamOptions `json:"stream_options,omitempty"`
}

// chatMessage 复用共享的 role/content 结构。
type chatMessage = providerutil.Message

// streamOptions 是 Chat Completions 的流式选项。
type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// sseChunk 是流式响应的一行 data 载荷的最小结构。
type sseChunk struct {
	// Error 是网关在流内上报的错误（`data: {"error":{...}}`）。此前它会被
	// 当成"空 choices"静默忽略，表现为无从解释的截断。
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	// Usage 只在开启 stream_options.include_usage 后出现，且通常位于独立的末帧
	// （该帧 Choices 为空）。不开启时字段缺失，Reported 保持 false。
	Usage *struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		PromptTokensDetails *struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
}

// Stream 调用 /chat/completions（stream=true），把增量内容原样交给 onChunk。
// 供应商按前述逐行 JSON 帧协议被提示词约束为"每行一个 JSON 对象"，
// 因此适配器不做内容再加工；截断以 finish_reason=length 识别。
// 超时实现：body 读取在独立 goroutine，主循环 select 事件（数据/首字/空闲/总时限）。
func (p *Provider) Stream(ctx context.Context, req ports.ChatRequest, sink ports.StreamSink) error {
	body, err := p.buildBody(req)
	if err != nil {
		return err
	}
	url := p.cfg.BaseURL

	streamCtx := ctx
	cancel := func() {}
	if p.cfg.TimeoutTotal > 0 {
		streamCtx, cancel = context.WithTimeout(ctx, p.cfg.TimeoutTotal)
	}
	defer cancel()

	httpReq, err := http.NewRequestWithContext(streamCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("openai: 构造请求失败: %w", err)
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
		return fmt.Errorf("openai: 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return providerutil.ReadHTTPError(resp, providerutil.DecodeOpenAIError)
	}

	// body 读取 goroutine：逐行提取 data: 载荷。读完（或错误）后关闭 lineCh，
	// 主循环先排空缓冲数据再收到关闭信号。错误经 bodyErrCh 单独上报。
	lineCh := make(chan []byte, 16)
	bodyErrCh := make(chan error, 1)
	go func() {
		defer close(lineCh)
		br := bufio.NewReader(resp.Body)
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
			if payload := ssePayload(line); payload != nil {
				select {
				case lineCh <- payload:
				case <-streamCtx.Done():
					return
				}
			}
		}
	}()

	tokenSeen := false
	finishLength := false
	// payloadSeen 区分"供应商根本没按 SSE 说话"（网关 HTML 错误页、JSON 错误体）
	// 与"模型写了一半被截断"：前者没有草稿可续写，必须归类为供应商不可用。
	payloadSeen := false

	// 首字时限。
	var firstTimer *time.Timer
	var firstDone <-chan time.Time
	if p.cfg.TimeoutFirstToken > 0 {
		firstTimer = time.NewTimer(p.cfg.TimeoutFirstToken)
		firstDone = firstTimer.C
		defer firstTimer.Stop()
	}
	// 空闲时限（token 间；有首个 token 后才生效）。
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
			return fmt.Errorf("openai: 读取流失败: %w", err)
		case <-firstDone:
			if !tokenSeen {
				return ErrTimeoutFirstToken
			}
			firstDone = nil
		case <-idleTimer.C:
			if tokenSeen {
				return ErrTimeoutIdle
			}
		case ln, ok := <-lineCh:
			if !ok {
				// 流结束（lineCh 关闭且已排空）。
				if !tokenSeen {
					return providerutil.EmptyStreamError(payloadSeen)
				}
				if finishLength {
					return ErrTruncated
				}
				return nil
			}
			if strings.TrimSpace(string(ln)) == "[DONE]" {
				if !tokenSeen {
					return providerutil.EmptyStreamError(payloadSeen)
				}
				if finishLength {
					return ErrTruncated
				}
				return nil
			}
			payloadSeen = true
			var chunk sseChunk
			if err := json.Unmarshal(ln, &chunk); err != nil {
				// 供应商扩展字段不影响解析；跳过无法解析的行。
				continue
			}
			// 流内错误事件：OpenAI 兼容网关会用 data: {"error":...} 报错，
			// 此前被当成"空 choices"静默忽略，表现为莫名其妙的截断。
			if chunk.Error != nil && chunk.Error.Message != "" {
				return fmt.Errorf("openai: 供应商流内错误: %s（type=%s code=%s）",
					chunk.Error.Message, chunk.Error.Type, chunk.Error.Code)
			}
			for _, c := range chunk.Choices {
				if c.Delta.ReasoningContent != "" || c.Delta.Content != "" {
					tokenSeen = true
					if firstDone != nil {
						firstDone = nil
					}
					idleTimer.Reset(p.cfg.TimeoutIdle)
				}
				if c.Delta.ReasoningContent != "" && thinkingFn != nil {
					thinkingFn(c.Delta.ReasoningContent)
				}
				if c.Delta.Content != "" {
					if err := sink.Chunk([]byte(c.Delta.Content)); err != nil {
						return err
					}
				}
				if c.FinishReason != nil && *c.FinishReason == "length" {
					finishLength = true
				}
			}
			// usage 通常位于 Choices 为空的独立末帧，因此放在 choices 循环之外。
			if u := chunk.Usage; u != nil {
				cached := 0
				if u.PromptTokensDetails != nil {
					cached = u.PromptTokensDetails.CachedTokens
				}
				sink.Usage(ports.TokenUsage{
					Prompt:     u.PromptTokens,
					Completion: u.CompletionTokens,
					Cached:     cached,
					Reported:   true,
				})
			}
		}
	}
}

// buildBody 构造请求体（角色消息映射，兼容供应商的 Chat Completions 约定）。
// max_tokens 优先取请求级预算（编译器的上下文预算），未指定时回退配置默认值；
// 请求级预算为 0 表示「不限制」，交由供应商默认行为处理。
func (p *Provider) buildBody(req ports.ChatRequest) ([]byte, error) {
	msgs := normalizeMessages(req.Messages)
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = p.cfg.MaxTokens
	}
	cr := chatRequest{
		Model: p.cfg.Model, Messages: msgs, Stream: true,
		MaxTokens: maxTokens, Temperature: p.cfg.Temperature,
		ReasoningEffort: p.cfg.ReasoningEffort,
		StreamOptions:   &streamOptions{IncludeUsage: true},
	}
	return json.Marshal(cr)
}

// normalizeMessages 复用共享的消息规范化（见 providerutil.NormalizeMessages）。
func normalizeMessages(messages []ports.ChatMessage) []chatMessage {
	return providerutil.NormalizeMessages(messages)
}

// ssePayload 提取一行 SSE：`data: ...`（空行/注释/event:/id: 返回 nil）。
func ssePayload(line []byte) []byte {
	t := bytes.TrimSpace(line)
	if len(t) == 0 || t[0] != 'd' {
		return nil
	}
	// 只接受以 data: 开头（不匹配 event:/id:/retry:）
	if !bytes.HasPrefix(t, []byte("data:")) {
		return nil
	}
	p := bytes.TrimSpace(t[len("data:"):])
	if len(p) == 0 {
		return nil
	}
	return p
}

func normalizeURL(baseURL string) string {
	raw := strings.TrimRight(baseURL, "/")
	raw = strings.TrimSuffix(raw, "/responses")
	if strings.HasSuffix(raw, "/chat/completions") {
		return raw
	}
	return raw + "/chat/completions"
}
