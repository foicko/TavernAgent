package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tavernagent/internal/ports"
)

func TestAnthropic_BuildBody(t *testing.T) {
	prov := New(Config{
		BaseURL: "https://api.anthropic.com",
		Model:   "claude-3-7-sonnet",
	})

	req := ports.ChatRequest{
		Messages: []ports.ChatMessage{
			{Role: "system", Content: "System rule 1"},
			{Role: "system", Content: "System rule 2"},
			{Role: "user", Content: "User part 1"},
			{Role: "user", Content: "User part 2"},
			{Role: "assistant", Content: "Assistant reply"},
		},
	}

	bodyBytes, err := prov.buildBody(req)
	if err != nil {
		t.Fatalf("buildBody failed: %v", err)
	}

	var reqBody anthropicRequest
	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		t.Fatalf("unmarshal request failed: %v", err)
	}

	// system 走块数组：只有块形态能挂 cache_control（Anthropic 不做自动前缀缓存）。
	if len(reqBody.System) != 2 {
		t.Fatalf("expected 2 system blocks, got %d", len(reqBody.System))
	}
	if reqBody.System[0].Text != "System rule 1" || reqBody.System[1].Text != "System rule 2" {
		t.Errorf("unexpected system blocks: %+v", reqBody.System)
	}
	// 前缀缓存断点必须打在**静态前缀末尾**（最后一块 system）。
	if reqBody.System[0].CacheControl != nil {
		t.Error("缓存断点不应打在非末尾的 system 块上")
	}
	if cc := reqBody.System[1].CacheControl; cc == nil || cc.Type != "ephemeral" {
		t.Errorf("静态前缀末尾缺少 cache_control 断点: %+v", cc)
	}

	if len(reqBody.Messages) != 2 {
		t.Fatalf("expected 2 merged messages, got %d", len(reqBody.Messages))
	}
	if reqBody.Messages[0].Content != "User part 1\n\nUser part 2" {
		t.Errorf("unexpected user content: %q", reqBody.Messages[0].Content)
	}
	if reqBody.Messages[1].Role != "assistant" {
		t.Errorf("unexpected assistant role: %q", reqBody.Messages[1].Role)
	}
	// 第二个断点打在历史前沿（最后一条 assistant）：它之后只会追加新回合，
	// 因此后续每轮只需为新增的那一轮付一次写缓存成本。
	frontier, ok := reqBody.Messages[1].Content.([]any)
	if !ok || len(frontier) != 1 {
		t.Fatalf("历史前沿应转成内容块形态以携带断点: %#v", reqBody.Messages[1].Content)
	}
	block, _ := frontier[0].(map[string]any)
	if block["type"] != "text" || block["text"] != "Assistant reply" {
		t.Errorf("历史前沿内容块不正确: %+v", block)
	}
	cc, _ := block["cache_control"].(map[string]any)
	if cc == nil || cc["type"] != "ephemeral" {
		t.Errorf("历史前沿缺少 cache_control 断点: %+v", block)
	}
	if reqBody.MaxTokens <= 0 {
		t.Errorf("expected positive max_tokens, got %d", reqBody.MaxTokens)
	}
}

func TestAnthropic_Stream_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/v1/messages") && !strings.HasSuffix(r.URL.Path, "/messages") {
			t.Errorf("expected messages endpoint, got %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "anthropic-key" {
			t.Errorf("unexpected api key: %s", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("unexpected version header: %s", r.Header.Get("anthropic-version"))
		}

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}

		events := []string{
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"deep thought\"}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"{\\\"v\\\":1}\\n\"}}\n\n",
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\n",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
		}

		for _, ev := range events {
			_, _ = fmt.Fprint(w, ev)
			flusher.Flush()
		}
	}))
	defer ts.Close()

	var thinkingLog strings.Builder
	ctx := ports.WithThinkingCallback(context.Background(), func(d string) {
		thinkingLog.WriteString(d)
	})

	prov := New(Config{
		BaseURL:           ts.URL,
		Model:             "claude-3-7-sonnet",
		APIKey:            "anthropic-key",
		TimeoutFirstToken: 2 * time.Second,
		TimeoutIdle:       2 * time.Second,
		TimeoutTotal:      5 * time.Second,
	})

	var contentLog strings.Builder
	err := prov.Stream(ctx, ports.ChatRequest{
		Messages: []ports.ChatMessage{
			{Role: "user", Content: "hi"},
		},
	}, ports.OnChunkSink(func(chunk []byte) error {
		contentLog.Write(chunk)
		return nil
	}))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if thinkingLog.String() != "deep thought" {
		t.Errorf("unexpected thinking: %q", thinkingLog.String())
	}
	if contentLog.String() != "{\"v\":1}\n" {
		t.Errorf("unexpected content: %q", contentLog.String())
	}
}

func TestAnthropic_Stream_Truncated(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		events := []string{
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"part\"}}\n\n",
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"max_tokens\"}}\n\n",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
		}
		for _, ev := range events {
			_, _ = fmt.Fprint(w, ev)
			flusher.Flush()
		}
	}))
	defer ts.Close()

	prov := New(Config{
		BaseURL:           ts.URL,
		Model:             "claude-3-7-sonnet",
		TimeoutFirstToken: 2 * time.Second,
	})

	err := prov.Stream(context.Background(), ports.ChatRequest{}, ports.OnChunkSink(func([]byte) error { return nil }))
	if err != ErrTruncated {
		t.Fatalf("expected ErrTruncated, got %v", err)
	}
}

// TestAnthropic_Stream_MergesUsageAcrossEvents 锁定 Anthropic 的分帧用量合并。
//
// Anthropic 把用量拆在两处：message_start 给输入与缓存读取量，
// message_delta 给累积输出量。适配器必须合并成一份再上报，
// 否则调用方拿到的永远是"半份"数据（这正是无法判断缓存是否命中的原因）。
//
// 有齿验证：把 absorbUsage 的"Cached 只在 >0 时覆盖"改成无条件覆盖，
// message_delta（不带 cached 字段）会把首帧读到的 96 清零，本用例失败。
func TestAnthropic_Stream_MergesUsageAcrossEvents(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		events := []string{
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":200,\"output_tokens\":1,\"cache_read_input_tokens\":96,\"cache_creation_input_tokens\":40}}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"{\\\"v\\\":1}\\n\"}}\n\n",
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":33}}\n\n",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
		}
		for _, ev := range events {
			_, _ = fmt.Fprint(w, ev)
			flusher.Flush()
		}
	}))
	defer ts.Close()

	prov := New(Config{BaseURL: ts.URL, Model: "claude-3-7-sonnet", APIKey: "k", TimeoutFirstToken: 2 * time.Second})

	var last ports.TokenUsage
	calls := 0
	err := prov.Stream(context.Background(), ports.ChatRequest{}, ports.StreamSink{
		OnChunk: func([]byte) error { return nil },
		OnUsage: func(u ports.TokenUsage) { last = u; calls++ },
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if calls == 0 {
		t.Fatal("usage 未上报")
	}
	// 最后一次回调必须同时含输入、输出与缓存命中量。
	if last.Prompt != 200 || last.Completion != 33 || last.Cached != 96 || !last.Reported {
		t.Fatalf("merged usage = %+v, want prompt=200 completion=33 cached=96", last)
	}
}

// 2xx 但零流式事件：必须归类为供应商不可用，而不是截断（同 openai 适配器）。
func TestStreamNonSSEResponseIsNotTruncation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><body>502 Bad Gateway</body></html>"))
	}))
	defer srv.Close()
	p := New(Config{BaseURL: srv.URL, Model: "m", TimeoutTotal: time.Minute})

	err := p.Stream(context.Background(), ports.ChatRequest{}, ports.OnChunkSink(func([]byte) error { return nil }))
	if !errors.Is(err, ports.ErrNonStreamingResponse) {
		t.Fatalf("want ErrNonStreamingResponse, got %v", err)
	}
}

// 思考强度映射：Anthropic 用 extended thinking 的 token 预算表达，且必须遵守两条硬约束
// （budget_tokens ≥ 1024 且严格小于 max_tokens；开启时 temperature 只能是默认的 1）。
func TestAnthropic_ThinkingBudgetMapping(t *testing.T) {
	cases := []struct {
		effort    string
		maxTokens int
		wantNone  bool
		wantMin   int
		wantMax   int
	}{
		{effort: "", maxTokens: 4096, wantNone: true}, // 默认：完全不发送
		{effort: "low", maxTokens: 4096, wantMin: 1024, wantMax: 1229},
		{effort: "medium", maxTokens: 4096, wantMin: 2048, wantMax: 2048},
		{effort: "high", maxTokens: 4096, wantMin: 3072, wantMax: 3072},
		{effort: "medium", maxTokens: 2048, wantMin: 1024, wantMax: 1024}, // 下限钳制
		{effort: "high", maxTokens: 1024, wantNone: true},                 // 放不下 → 退化为默认
		{effort: "bogus", maxTokens: 4096, wantNone: true},                // 非法值不发送也不 panic
	}
	for _, tc := range cases {
		temp := 0.5
		prov := New(Config{
			BaseURL: "https://api.anthropic.com", Model: "claude", Temperature: &temp,
			MaxTokens: tc.maxTokens, ReasoningEffort: tc.effort,
		})
		body, err := prov.buildBody(ports.ChatRequest{Messages: []ports.ChatMessage{{Role: "user", Content: "hi"}}})
		if err != nil {
			t.Fatalf("effort=%q maxTokens=%d: buildBody 失败: %v", tc.effort, tc.maxTokens, err)
		}
		var got anthropicRequest
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("unmarshal 失败: %v", err)
		}
		if tc.wantNone {
			if got.Thinking != nil {
				t.Errorf("effort=%q maxTokens=%d: 不应发送 thinking，实际 %+v", tc.effort, tc.maxTokens, got.Thinking)
			}
			if got.Temperature == nil {
				t.Errorf("effort=%q maxTokens=%d: 未开启思考时不应丢掉 temperature", tc.effort, tc.maxTokens)
			}
			continue
		}
		if got.Thinking == nil {
			t.Fatalf("effort=%q maxTokens=%d: 期望发送 thinking", tc.effort, tc.maxTokens)
		}
		if got.Thinking.Type != "enabled" {
			t.Errorf("thinking.type = %q, want enabled", got.Thinking.Type)
		}
		if budget := got.Thinking.BudgetTokens; budget < tc.wantMin || budget > tc.wantMax {
			t.Errorf("effort=%q maxTokens=%d: budget_tokens = %d, want %d..%d", tc.effort, tc.maxTokens, budget, tc.wantMin, tc.wantMax)
		}
		if got.Thinking.BudgetTokens >= got.MaxTokens {
			t.Errorf("budget_tokens %d 必须严格小于 max_tokens %d", got.Thinking.BudgetTokens, got.MaxTokens)
		}
		if got.Temperature != nil {
			t.Errorf("开启 extended thinking 时不应发送 temperature（Anthropic 只接受默认的 1）")
		}
	}
}
