package http

// 在途正文快照的端点用例（服务端部分）。
import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"tavernagent/internal/ports"
)

// httpPartialProvider 写出半行块帧后阻塞：服务端此刻正持有"当前块的实时文本"。
type httpPartialProvider struct {
	started chan struct{}
}

func (p *httpPartialProvider) Capabilities(context.Context) (ports.ProviderCapabilities, error) {
	return ports.ProviderCapabilities{ID: "partial", Streaming: true}, nil
}

func (p *httpPartialProvider) Stream(ctx context.Context, _ ports.ChatRequest, sink ports.StreamSink) error {
	if err := sink.Chunk([]byte(`{"v":1,"seq":1,"type":"block","kind":"narration","text":"潮水漫过甲板，远处`)); err != nil {
		return err
	}
	close(p.started)
	<-ctx.Done()
	return ctx.Err()
}

// 生成中刷新/断线重连的客户端拿不到当前块的行内增量（它们是 Sequence 0 的临时事件，
// 永不重放）。GET /turns/{id} 必须把服务端权威的在途文本给出来，否则这一段会缺字。
func TestTurnViewExposesInFlightDraft(t *testing.T) {
	provider := &httpPartialProvider{started: make(chan struct{})}
	_, base := newTestServerWithProvider(t, provider)

	body := `{"title":"潮汐","playerName":"旅人","openingText":"你推开门。","characterJson":` + str(testCard) + `}`
	req, _ := http.NewRequest("POST", base+"/api/v1/sessions", strings.NewReader(body))
	req.Header.Set("Origin", base)
	resp := post(t, req)
	if resp.status != 201 {
		t.Fatalf("create status = %d body=%s", resp.status, resp.body)
	}
	var created struct {
		SessionID string `json:"sessionId"`
		BranchID  string `json:"branchId"`
		RootNode  string `json:"rootNodeId"`
	}
	json.Unmarshal([]byte(resp.body), &created)

	turnReq := `{"idempotencyKey":"k1","expectedHeadId":"` + created.RootNode + `","expectedVersion":0,"input":{"kind":"text","text":"继续"}}`
	req2, _ := http.NewRequest("POST", base+"/api/v1/sessions/"+created.SessionID+"/branches/"+created.BranchID+"/turns", strings.NewReader(turnReq))
	req2.Header.Set("Origin", base)
	resp2 := post(t, req2)
	if resp2.status != 202 {
		t.Fatalf("accept status = %d body=%s", resp2.status, resp2.body)
	}
	var accepted struct {
		TurnID string `json:"turnId"`
	}
	json.Unmarshal([]byte(resp2.body), &accepted)

	deadline := time.Now().Add(asyncWaitBudget)
	var draft struct {
		AttemptID string `json:"attemptId"`
		InFlight  *struct {
			Seq  int    `json:"seq"`
			Kind string `json:"kind"`
			Text string `json:"text"`
		} `json:"inFlight"`
	}
	for time.Now().Before(deadline) {
		req3, _ := http.NewRequest("GET", base+"/api/v1/turns/"+accepted.TurnID, nil)
		resp3 := post(t, req3)
		var view map[string]any
		json.Unmarshal([]byte(resp3.body), &view)
		raw, _ := json.Marshal(view["draft"])
		if json.Unmarshal(raw, &draft) == nil && draft.InFlight != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if draft.InFlight == nil {
		t.Fatal("生成中的回合必须给出在途正文")
	}
	if draft.AttemptID == "" || draft.InFlight.Seq != 1 || draft.InFlight.Kind != "narration" {
		t.Fatalf("draft = %+v", draft)
	}
	if !strings.HasPrefix("潮水漫过甲板，远处", draft.InFlight.Text) || len([]rune(draft.InFlight.Text)) < 4 {
		t.Fatalf("在途正文 = %q", draft.InFlight.Text)
	}
}
