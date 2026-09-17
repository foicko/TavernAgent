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
