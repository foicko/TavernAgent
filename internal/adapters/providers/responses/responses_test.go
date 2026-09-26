package responses

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tavernagent/internal/ports"
)

func TestResponses_Stream_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/responses") {
			t.Errorf("expected URL ending in /responses, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}

		events := []string{
			"event: response.reasoning_text.delta\ndata: {\"type\":\"response.reasoning_text.delta\",\"delta\":\"thinking...\"}\n\n",
			"event: response.text.delta\ndata: {\"type\":\"response.text.delta\",\"delta\":\"{\\\"v\\\":1,\\\"seq\\\":1}\\n\"}\n\n",
			"event: response.done\ndata: {\"type\":\"response.done\",\"response\":{\"status\":\"completed\"}}\n\n",
			"data: [DONE]\n\n",
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
		Model:             "gpt-4o",
		APIKey:            "test-key",
		TimeoutFirstToken: 2 * time.Second,
		TimeoutIdle:       2 * time.Second,
		TimeoutTotal:      5 * time.Second,
	})

	var contentLog strings.Builder
	err := prov.Stream(ctx, ports.ChatRequest{
		Model: "gpt-4o",
		Messages: []ports.ChatMessage{
			{Role: "user", Content: "hello"},
		},
	}, ports.OnChunkSink(func(chunk []byte) error {
		contentLog.Write(chunk)
		return nil
	}))

	if err != nil {
		t.Fatalf("unexpected stream error: %v", err)
	}

	if thinkingLog.String() != "thinking..." {
		t.Errorf("unexpected thinking: %q", thinkingLog.String())
	}
	if contentLog.String() != "{\"v\":1,\"seq\":1}\n" {
		t.Errorf("unexpected content: %q", contentLog.String())
	}
}

// 真实 Responses 事件名：思考摘要走 thinking、正文走 content，二者不得混淆。
// 此前适配器只显式识别不存在的 response.text.delta，思考摘要会被当作正文，
// 污染帧解析（用户库里 deepseek-flash 的 PROTOCOL_INVALID 失败即与此相关）。
func TestResponses_RealEventNames_SplitReasoningAndContent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		events := []string{
			"event: response.reasoning_summary_text.delta\ndata: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"让我想想\"}\n\n",
			"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"{\\\"v\\\":1,\\\"seq\\\":1}\"}\n\n",
			"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"\\n\"}\n\n",
			"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n",
			"data: [DONE]\n\n",
		}
		for _, ev := range events {
			_, _ = fmt.Fprint(w, ev)
			flusher.Flush()
		}
	}))
	defer ts.Close()

	var thinking, content strings.Builder
	ctx := ports.WithThinkingCallback(context.Background(), func(d string) { thinking.WriteString(d) })

	prov := New(Config{
		BaseURL:           ts.URL,
		Model:             "m",
		TimeoutFirstToken: 2 * time.Second,
		TimeoutIdle:       2 * time.Second,
		TimeoutTotal:      5 * time.Second,
	})
	err := prov.Stream(ctx, ports.ChatRequest{
		Messages: []ports.ChatMessage{{Role: "user", Content: "hi"}},
	}, ports.OnChunkSink(func(chunk []byte) error {
		content.Write(chunk)
		return nil
	}))
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if thinking.String() != "让我想想" {
		t.Errorf("thinking = %q, want 让我想想", thinking.String())
	}
	if content.String() != "{\"v\":1,\"seq\":1}\n" {
		t.Errorf("content = %q", content.String())
	}
}

func TestResponses_Stream_Truncated(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		events := []string{
			"event: response.text.delta\ndata: {\"type\":\"response.text.delta\",\"delta\":\"partial\"}\n\n",
			"event: response.done\ndata: {\"type\":\"response.done\",\"response\":{\"status\":\"incomplete\",\"status_details\":{\"reason\":\"max_output_tokens\"}}}\n\n",
			"data: [DONE]\n\n",
		}
		for _, ev := range events {
			_, _ = fmt.Fprint(w, ev)
			flusher.Flush()
		}
	}))
	defer ts.Close()

	prov := New(Config{
		BaseURL:           ts.URL,
		Model:             "gpt-4o",
		TimeoutFirstToken: 2 * time.Second,
	})

	err := prov.Stream(context.Background(), ports.ChatRequest{}, ports.OnChunkSink(func([]byte) error { return nil }))
	if err != ErrTruncated {
		t.Fatalf("expected ErrTruncated, got %v", err)
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

// 思考强度：只在显式设置时发送 reasoning.effort，默认档整个对象缺席。
func TestResponses_ReasoningEffort(t *testing.T) {
	bodyOf := func(effort string) string {
		prov := New(Config{BaseURL: "https://api.example.com", Model: "m", ReasoningEffort: effort})
		body, err := prov.buildBody(ports.ChatRequest{Messages: []ports.ChatMessage{{Role: "user", Content: "hi"}}})
		if err != nil {
			t.Fatalf("buildBody 失败: %v", err)
		}
		return string(body)
	}
	if body := bodyOf(""); strings.Contains(body, "reasoning") {
		t.Errorf("默认档不应发送 reasoning，实际 %s", body)
	}
	if body := bodyOf("medium"); !strings.Contains(body, `"reasoning":{"effort":"medium"}`) {
		t.Errorf("期望 reasoning.effort=medium，实际 %s", body)
	}
}

// 流中途的失败必须以错误返回：真实 error 事件的 message 在顶层，response.failed
// 带 status=failed；content_filter 截断不能进入续写（续写只会被再次过滤）。
func TestResponses_StreamFailureEvents(t *testing.T) {
	cases := []struct {
		name  string
		event string
		want  string
	}{
		{"error-event", `{"type":"error","code":"server_error","message":"upstream exploded"}`, "upstream exploded"},
		{"response-failed", `{"type":"response.failed","response":{"status":"failed","error":{"code":"rate_limit_exceeded","message":"slow down"}}}`, "slow down"},
		{"content-filter", `{"type":"response.incomplete","response":{"status":"incomplete","incomplete_details":{"reason":"content_filter"}}}`, "content_filter"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")
				_, _ = fmt.Fprintf(w, "data: %s\n\n", tc.event)
			}))
			defer ts.Close()
			prov := New(Config{BaseURL: ts.URL, Model: "gpt-4o", TimeoutFirstToken: 2 * time.Second})
			err := prov.Stream(context.Background(), ports.ChatRequest{}, ports.OnChunkSink(func([]byte) error { return nil }))
			if err == nil || errors.Is(err, ErrTruncated) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q (not truncation), got %v", tc.want, err)
			}
		})
	}
}

// 真实字段 incomplete_details.reason=max_output_tokens 才是可续写的截断。
func TestResponses_IncompleteMaxOutputTokensIsTruncation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.incomplete\",\"response\":{\"status\":\"incomplete\",\"incomplete_details\":{\"reason\":\"max_output_tokens\"}}}\n\n")
	}))
	defer ts.Close()
	prov := New(Config{BaseURL: ts.URL, Model: "gpt-4o", TimeoutFirstToken: 2 * time.Second})
	err := prov.Stream(context.Background(), ports.ChatRequest{}, ports.OnChunkSink(func([]byte) error { return nil }))
	if err != ErrTruncated {
		t.Fatalf("expected ErrTruncated, got %v", err)
	}
}

// 网关不补结尾换行：最后一个 data 行随 EOF 一起返回，也必须被处理（否则截断状态丢失）。
func TestResponses_FinalEventWithoutTrailingNewline(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.incomplete\",\"response\":{\"status\":\"incomplete\",\"incomplete_details\":{\"reason\":\"max_output_tokens\"}}}")
	}))
	defer ts.Close()
	prov := New(Config{BaseURL: ts.URL, Model: "gpt-4o", TimeoutFirstToken: 2 * time.Second})
	err := prov.Stream(context.Background(), ports.ChatRequest{}, ports.OnChunkSink(func([]byte) error { return nil }))
	if err != ErrTruncated {
		t.Fatalf("expected ErrTruncated from final unterminated event, got %v", err)
	}
}
