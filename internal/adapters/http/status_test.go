package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/application"
	ctxpkg "tavernagent/internal/context"
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
		Prompt   string                      `json:"prompt"`
		Ablation string                      `json:"ablation"`
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
	// 提示词版本指纹必须随观测端点暴露：评估报告要能说明"跑的是哪份提示词"
	// （ADS-7.8-04），否则成功率变化无法归因。
	if got.Prompt != ctxpkg.PromptManifest() {
		t.Fatalf("prompt = %q, want %q", got.Prompt, ctxpkg.PromptManifest())
	}
	// 未启用消融时必须明确回报 none，而不是留空——留空无法区分
	// "生产形态"与"字段忘了填"。
	if got.Ablation != "none" {
		t.Fatalf("ablation = %q, want none", got.Ablation)
	}
}

// TestRuntimeStatusReportsAblation 锁定消融状态的自证（ADS-7.8-01）。
//
// 基线评估的产物必须自带"这一轮关了哪些特性"，否则结论只能依赖运行者的记忆。
func TestRuntimeStatusReportsAblation(t *testing.T) {
	ablation, err := ctxpkg.ParseAblation("all")
	if err != nil {
		t.Fatal(err)
	}
	s := mustNew(t, Deps{Addr: "127.0.0.1:8890", Metrics: &application.RuntimeMetrics{}, Ablation: ablation})
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8890/api/status", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var got struct {
		Ablation string `json:"ablation"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (%s)", err, w.Body.String())
	}
	if got.Ablation != ablation.String() {
		t.Fatalf("ablation = %q, want %q", got.Ablation, ablation.String())
	}
}

// TestCompilePhasesAreAggregated 锁定编译阶段耗时的汇总口径（ADS-7.7）。
//
// 阶段耗时是"准备一条请求慢在哪一段"的最低成本现场：Compile 内部本来就按阶段计时，
// 但没有消费者时它就等于不存在。
func TestCompilePhasesAreAggregated(t *testing.T) {
	m := &application.RuntimeMetrics{}
	m.AddPhase("lorebook", 3*time.Millisecond)
	m.AddPhase("lorebook", 5*time.Millisecond)
	m.AddPhase("budget", time.Millisecond)
	// 未登记的阶段必须计入 other，而不是被静默丢弃。
	m.AddPhase("brand_new_phase", 2*time.Millisecond)
	// 负数（时钟回拨等异常）不得污染读数。
	m.AddPhase("budget", -1)

	snap := m.Snapshot()
	if got := snap.CompilePhases["lorebook"]; got.Calls != 2 || got.TotalMs != 8 {
		t.Fatalf("lorebook = %+v, want calls=2 totalMs=8", got)
	}
	if got := snap.CompilePhases["budget"]; got.Calls != 1 || got.TotalMs != 1 {
		t.Fatalf("budget = %+v, want calls=1 totalMs=1", got)
	}
	if got := snap.CompilePhases["other"]; got.Calls != 1 {
		t.Fatalf("other = %+v, want calls=1（未知阶段不得丢失）", got)
	}
	if _, ok := snap.CompilePhases["session_context"]; ok {
		t.Fatal("未发生的阶段不应出现在快照里")
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
