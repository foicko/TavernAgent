package openai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"tavernagent/internal/ports"
)

// sseServer 模拟 OpenAI 兼容 /chat/completions 流式端点。
type sseServer struct {
	mu      sync.Mutex
	chunks  []string // 逐条 data: 载荷
	status  int      // 默认 200
	body    string   // 非流式响应体（HTTP 错误场景）
	sawAuth string
	gotBody string
}

func (s *sseServer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.sawAuth = r.Header.Get("Authorization")
		b := make([]byte, 0)
		if r.ContentLength >= 0 {
			b = make([]byte, r.ContentLength)
		}
		_, _ = r.Body.Read(b)
		s.gotBody = string(b)
		chunks := s.chunks
		status := s.status
		body := s.body
		s.mu.Unlock()

		if status == 0 {
			status = 200
		}
		if status != 200 {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, c := range chunks {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", c)
		}
		w.(http.Flusher).Flush()
	})
}

func TestStreamDeliversDeltaContent(t *testing.T) {
	s := &sseServer{chunks: []string{
		`{"choices":[{"delta":{"content":"{\"v\":"}}]}`,
		`{"choices":[{"delta":{"content":"1,\"type\":\"block\"}"}}]}`,
		`{"choices":[{"delta":{"content":""},"finish_reason":"stop"}]}`,
		`[DONE]`,
	}}
	srv := httptest.NewServer(s.handler())
	defer srv.Close()
	p := New(Config{BaseURL: srv.URL, Model: "m", APIKey: "k"})

	var got []byte
	err := p.Stream(context.Background(), ports.ChatRequest{
		Messages: []ports.ChatMessage{{Role: "user", Content: "hi"}},
	}, ports.OnChunkSink(func(c []byte) error {
		got = append(got, c...)
		return nil
	}))
	if err != nil {
		t.Fatalf("stream err: %v", err)
	}
	want := "{\"v\":1,\"type\":\"block\"}"
	if string(got) != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
	if s.sawAuth != "Bearer k" {
		t.Fatalf("auth header = %q", s.sawAuth)
	}
	if !strings.Contains(s.gotBody, `"model":"m"`) || !strings.Contains(s.gotBody, `"stream":true`) {
		t.Fatalf("request body = %s", s.gotBody)
	}
}

func TestStreamTruncated(t *testing.T) {
	s := &sseServer{chunks: []string{
		`{"choices":[{"delta":{"content":"partial"}}]}`,
		`{"choices":[{"delta":{"content":""},"finish_reason":"length"}]}`,
		`[DONE]`,
	}}
	srv := httptest.NewServer(s.handler())
	defer srv.Close()
	p := New(Config{BaseURL: srv.URL, Model: "m", APIKey: "k"})

	err := p.Stream(context.Background(), ports.ChatRequest{}, ports.OnChunkSink(func([]byte) error { return nil }))
	if !errors.Is(err, ErrTruncated) {
		t.Fatalf("want ErrTruncated, got %v", err)
	}
}

func TestStreamHTTPError(t *testing.T) {
	s := &sseServer{status: 401, body: `{"error":{"message":"invalid api key","code":"invalid_api_key"}}`}
	srv := httptest.NewServer(s.handler())
	defer srv.Close()
	p := New(Config{BaseURL: srv.URL, Model: "m", APIKey: "bad"})

	err := p.Stream(context.Background(), ports.ChatRequest{}, ports.OnChunkSink(func([]byte) error { return nil }))
	var he *HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("want HTTPError, got %T %v", err, err)
	}
	if he.StatusCode != 401 || he.Code != "invalid_api_key" {
		t.Fatalf("HTTPError = %+v", he)
	}
}

func TestStreamFirstTokenTimeout(t *testing.T) {
	// 服务器不回任何数据，等待首字超时。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()
	p := New(Config{BaseURL: srv.URL, Model: "m", TimeoutFirstToken: 100 * time.Millisecond})

	err := p.Stream(context.Background(), ports.ChatRequest{}, ports.OnChunkSink(func([]byte) error { return nil }))
	if !errors.Is(err, ErrTimeoutFirstToken) {
		t.Fatalf("want ErrTimeoutFirstToken, got %v", err)
	}
}

func TestStreamReasoningContentPreventsTimeout(t *testing.T) {
	// 模拟 DeepSeek-R1：先发 reasoning_content（思考过程），延后输出正式 content
	s := &sseServer{chunks: []string{
		`{"choices":[{"delta":{"reasoning_content":"Let me think about how to narrate this story..."}}]}`,
		`{"choices":[{"delta":{"content":"{\"v\":1,\"type\":\"block\"}"}}]}`,
		`{"choices":[{"delta":{"content":""},"finish_reason":"stop"}]}`,
		`[DONE]`,
	}}
	srv := httptest.NewServer(s.handler())
	defer srv.Close()

	p := New(Config{
		BaseURL:           srv.URL,
		Model:             "deepseek-reasoner",
		TimeoutFirstToken: 200 * time.Millisecond,
		TimeoutIdle:       200 * time.Millisecond,
	})

	var got []byte
	err := p.Stream(context.Background(), ports.ChatRequest{}, ports.OnChunkSink(func(c []byte) error {
		got = append(got, c...)
		return nil
	}))
	if err != nil {
		t.Fatalf("stream failed unexpectedly: %v", err)
	}

	// 验证：reasoning_content 成功保活，且不会污染推向帧解析器的正文内容
	want := `{"v":1,"type":"block"}`
	if string(got) != want {
		t.Fatalf("got = %q, want %q", got, want)
	}
}

func TestNormalizeMessages(t *testing.T) {
	tests := []struct {
		name     string
		input    []ports.ChatMessage
		wantLast chatMessage
		wantLen  int
	}{
		{
			name: "trailing system converted to user",
			input: []ports.ChatMessage{
				{Role: "system", Content: "sys"},
				{Role: "user", Content: "u1"},
				{Role: "assistant", Content: "a1"},
				{Role: "system", Content: "continue prompt"},
			},
			wantLast: chatMessage{Role: "user", Content: "continue prompt"},
			wantLen:  4,
		},
		{
			name: "trailing assistant appends user continue",
			input: []ports.ChatMessage{
				{Role: "system", Content: "sys"},
				{Role: "user", Content: "u1"},
				{Role: "assistant", Content: "a1"},
			},
			wantLast: chatMessage{Role: "user", Content: "请继续"},
			wantLen:  4,
		},
		{
			name:     "empty input gets default user",
			input:    []ports.ChatMessage{},
			wantLast: chatMessage{Role: "user", Content: "..."},
			wantLen:  1,
		},
		{
			name: "empty user at end gets fallback content",
			input: []ports.ChatMessage{
				{Role: "user", Content: "hello"},
				{Role: "assistant", Content: "hi"},
				{Role: "user", Content: "   \n"},
			},
			wantLast: chatMessage{Role: "user", Content: "..."},
			wantLen:  3,
		},
		{
			name: "empty assistant in middle is pruned",
			input: []ports.ChatMessage{
				{Role: "user", Content: "hello"},
				{Role: "assistant", Content: "   "},
				{Role: "user", Content: "world"},
			},
			wantLast: chatMessage{Role: "user", Content: "world"},
			wantLen:  2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeMessages(tc.input)
			if len(got) != tc.wantLen {
				t.Fatalf("len = %d, want %d; got = %+v", len(got), tc.wantLen, got)
			}
			last := got[len(got)-1]
			if last.Role != tc.wantLast.Role || last.Content != tc.wantLast.Content {
				t.Fatalf("last = %+v, want %+v", last, tc.wantLast)
			}
		})
	}
}

// TestStreamReportsUsageIncludingCachedTokens 锁定用量采集链路。
//
// 有齿验证：删掉 buildBody 里的 stream_options，供应商就不会回传 usage，
// 本用例在 "callbacks=1" 处失败；删掉 sseChunk.Usage 解析则在数值断言处失败。
func TestStreamReportsUsageIncludingCachedTokens(t *testing.T) {
	s := &sseServer{chunks: []string{
		`{"choices":[{"delta":{"content":"hello"}}]}`,
		`{"choices":[],"usage":{"prompt_tokens":120,"completion_tokens":30,"prompt_tokens_details":{"cached_tokens":96}}}`,
	}}
	srv := httptest.NewServer(s.handler())
	defer srv.Close()
	p := New(Config{BaseURL: srv.URL, Model: "m", APIKey: "k"})

	var got ports.TokenUsage
	calls := 0
	err := p.Stream(context.Background(), ports.ChatRequest{}, ports.StreamSink{
		OnChunk: func([]byte) error { return nil },
		OnUsage: func(u ports.TokenUsage) { got = u; calls++ },
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if calls != 1 {
		t.Fatalf("usage callbacks = %d, want 1", calls)
	}
	if got.Prompt != 120 || got.Completion != 30 || got.Cached != 96 || !got.Reported {
		t.Fatalf("usage = %+v, want prompt=120 completion=30 cached=96 reported=true", got)
	}
	// 请求体必须带 stream_options，否则供应商不回传 usage（用量会永远测不到）。
	if !strings.Contains(s.gotBody, `"stream_options":{"include_usage":true}`) {
		t.Fatalf("request body missing stream_options: %s", s.gotBody)
	}
}

// TestStreamWithoutUsageReportsNothing 确认"没回传"与"消耗为 0"被区分开：
// 没有 usage 帧时不应回调，Reported 不会被误置为 true。
func TestStreamWithoutUsageReportsNothing(t *testing.T) {
	s := &sseServer{chunks: []string{
		`{"choices":[{"delta":{"content":"hello"},"finish_reason":"stop"}]}`,
	}}
	srv := httptest.NewServer(s.handler())
	defer srv.Close()
	p := New(Config{BaseURL: srv.URL, Model: "m", APIKey: "k"})

	calls := 0
	err := p.Stream(context.Background(), ports.ChatRequest{}, ports.StreamSink{
		OnChunk: func([]byte) error { return nil },
		OnUsage: func(ports.TokenUsage) { calls++ },
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if calls != 0 {
		t.Fatalf("usage callbacks = %d, want 0 (usage absent)", calls)
	}
}

func TestStreamParentCancellationIsNotReportedAsTimeout(t *testing.T) {
	// 服务器保持连接不返回数据；调用方取消后必须报「已取消」而不是总时限超时，
	// 否则用量账本与排障会把后台任务的正常取消记成供应商故障。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()
	p := New(Config{BaseURL: srv.URL, Model: "m", TimeoutTotal: time.Minute})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()

	err := p.Stream(ctx, ports.ChatRequest{}, ports.OnChunkSink(func([]byte) error { return nil }))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if errors.Is(err, ErrTimeoutTotal) {
		t.Fatal("取消被误报为总时限超时")
	}
}

func TestStreamOwnTotalTimeoutStillReported(t *testing.T) {
	// 调用方没取消、只有总时限到期时，仍然要报 ErrTimeoutTotal（区分不能过度）。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()
	p := New(Config{BaseURL: srv.URL, Model: "m", TimeoutTotal: 80 * time.Millisecond})

	err := p.Stream(context.Background(), ports.ChatRequest{}, ports.OnChunkSink(func([]byte) error { return nil }))
	if !errors.Is(err, ErrTimeoutTotal) {
		t.Fatalf("want ErrTimeoutTotal, got %v", err)
	}
}

// 2xx 但不是 SSE（网关返回 HTML 错误页）：必须归类为"供应商不可用"，
// 而不是"截断"——后者会让应用层拿着空草稿进入续写态。
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
	if errors.Is(err, ports.ErrTruncatedStream) {
		t.Fatal("非流式响应不得归类为截断")
	}
}

// 流内错误事件（data: {"error":...}）此前被当成"空 choices"静默忽略，
// 用户只能看到一个无从解释的截断。必须显式报错。
func TestStreamSurfacesInStreamErrorEvent(t *testing.T) {
	sse := &sseServer{chunks: []string{`{"error":{"message":"rate limit exceeded","type":"rate_limit","code":"429"}}`}}
	srv := httptest.NewServer(sse.handler())
	defer srv.Close()
	p := New(Config{BaseURL: srv.URL, Model: "m", TimeoutTotal: time.Minute})

	err := p.Stream(context.Background(), ports.ChatRequest{}, ports.OnChunkSink(func([]byte) error { return nil }))
	if err == nil || !strings.Contains(err.Error(), "rate limit exceeded") {
		t.Fatalf("流内错误必须浮出，实际 %v", err)
	}
}

// 思考强度：只在显式设置时发送 reasoning_effort，默认档整个字段缺席
// （不支持的供应商不会因为多出的字段直接 400）。
func TestOpenAI_ReasoningEffort(t *testing.T) {
	bodyOf := func(effort string) string {
		prov := New(Config{BaseURL: "https://api.example.com", Model: "m", ReasoningEffort: effort})
		body, err := prov.buildBody(ports.ChatRequest{Messages: []ports.ChatMessage{{Role: "user", Content: "hi"}}})
		if err != nil {
			t.Fatalf("buildBody 失败: %v", err)
		}
		return string(body)
	}
	if body := bodyOf(""); strings.Contains(body, "reasoning_effort") {
		t.Errorf("默认档不应发送 reasoning_effort，实际 %s", body)
	}
	if body := bodyOf("high"); !strings.Contains(body, `"reasoning_effort":"high"`) {
		t.Errorf("期望 reasoning_effort=high，实际 %s", body)
	}
}
