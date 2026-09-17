package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/application"
	"tavernagent/internal/ports"
)

// TestRuntimeStatusEndpoint 锁定运行读数端点（T0.2）。
//
// 有齿验证：把 Handler() 里的 "/api/status" 路由删掉，用例在 404 处失败；
// 把 runtimeStatus 的空值降级改成直接报错，用例在 "空端口仍返回 200" 处失败。
func TestRuntimeStatusEndpoint(t *testing.T) {
	dir := t.TempDir()
	st, err := sqlite.Open(dir, ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	metrics := &application.RuntimeMetrics{}
	_ = st.RecordTurnUsage(ports.TurnUsageRecord{
		TurnID: "t1", AttemptID: "a1", Model: "m", Provider: "openai-chat",
		Prompt: 100, Completion: 20, Cached: 80, Estimated: 130, Reported: true,
		CreatedAt: "2026-09-13T00:00:00Z",
	})

	s := mustNew(t, Deps{Addr: "127.0.0.1:8890", Metrics: metrics, Usage: st})
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8890/api/status", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var got struct {
		Counters application.MetricsSnapshot `json:"counters"`
		Usage    ports.UsageTotals           `json:"usage"`
		Recent   []ports.TurnUsageRecord     `json:"recent"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (%s)", err, w.Body.String())
	}
	if got.Usage.Calls != 1 || got.Usage.Prompt != 100 || got.Usage.Cached != 80 {
		t.Fatalf("usage = %+v, want calls=1 prompt=100 cached=80", got.Usage)
	}
	if len(got.Recent) != 1 || got.Recent[0].AttemptID != "a1" {
		t.Fatalf("recent = %+v, want one record with attempt a1", got.Recent)
	}
	// 真实值与估算值必须同时可见，才能校准估算系数。
	if got.Recent[0].Estimated != 130 {
		t.Fatalf("estimated = %d, want 130", got.Recent[0].Estimated)
	}
}

// TestRuntimeStatusDegradesWithoutPorts 确认观测缺失时端点降级而非报错：
// 观测不得影响服务可用性。
func TestRuntimeStatusDegradesWithoutPorts(t *testing.T) {
	s := mustNew(t, Deps{Addr: "127.0.0.1:8890"}) // Metrics 与 Usage 均为 nil
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8890/api/status", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200 (观测缺失应降级)", w.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := got["counters"]; !ok {
		t.Fatalf("counters 缺失: %v", got)
	}
}
