package application

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"tavernagent/internal/adapters/sqlite"
	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// scriptedSummaryProvider 按调用次序返回脚本输出，并记录每次请求，
// 用于验证"修复重试"把哪些内容回喂给了模型。
type scriptedSummaryProvider struct {
	mu       sync.Mutex
	outputs  []string
	calls    int
	requests []ports.ChatRequest
}

func (p *scriptedSummaryProvider) Capabilities(context.Context) (ports.ProviderCapabilities, error) {
	return ports.ProviderCapabilities{ID: "scripted", Streaming: true, StructuredOutput: true, ContextWindow: 128000}, nil
}

func (p *scriptedSummaryProvider) Stream(ctx context.Context, req ports.ChatRequest, sink ports.StreamSink) error {
	p.mu.Lock()
	index := p.calls
	p.calls++
	p.requests = append(p.requests, req)
	output := ""
	if index < len(p.outputs) {
		output = p.outputs[index]
	} else if len(p.outputs) > 0 {
		output = p.outputs[len(p.outputs)-1]
	}
	p.mu.Unlock()
	if output == "" {
		return fmt.Errorf("scripted provider exhausted")
	}
	return sink.Chunk([]byte(output))
}

func (p *scriptedSummaryProvider) requestCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func (p *scriptedSummaryProvider) lastRequest(t *testing.T) ports.ChatRequest {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.requests) == 0 {
		t.Fatalf("provider 未被调用")
	}
	return p.requests[len(p.requests)-1]
}

const invalidSummaryXML = `<story_checkpoint>
<narrative_arc>
玩家与老板娘在酒馆达成初步约定，但格式缺了伏笔章节。
</narrative_arc>
</story_checkpoint>`

const validSummaryXML = `<story_checkpoint>
<narrative_arc>
玩家与老板娘在酒馆达成初步约定；修复后的交接快照。
</narrative_arc>
<character_dynamics>
<mindset character="Elena">对玩家心存戒备但逐渐信任</mindset>
</character_dynamics>
<open_loops>
- [承诺] 明早把怀表归还
</open_loops>
</story_checkpoint>`

// buildSummarySession 造一个"历史足够长、已能触发压缩"的会话，返回 store 与服务。
func buildSummarySession(t *testing.T) (*sqlite.Store, *SessionService, string, string, string) {
	t.Helper()
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	bus := NewEventBus(st)
	sessionSvc := NewSessionService(st)
	setup, err := sessionSvc.Setup(context.Background(), &SessionSetupRequest{
		Title:         "摘要维护测试",
		CharacterJSON: testCard,
		Player:        Player{Name: "玩家"},
		OpeningText:   "你推开酒馆的门，老板娘抬起了头。",
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	turnSvc := NewTurnService(st, nil, ctxpkg.New(st, ctxpkg.DefaultOptions()), bus)
	headID := setup.Branch.HeadNodeID
	for i := 1; i <= 15; i++ {
		node, _, err := turnSvc.SubmitBlocks(context.Background(), SubmitBlocksRequest{
			SessionID: setup.Session.SessionID,
			BranchID:  setup.Branch.BranchID,
			HeadID:    headID,
			Version:   int64(i - 1),
			Input:     domain.TurnInput{Text: fmt.Sprintf("这是第 %d 轮输入", i)},
			Blocks: []domain.TextBlock{
				{Kind: "inner_monologue", Text: fmt.Sprintf("心声_%d", i)},
				{Kind: "dialogue", Text: fmt.Sprintf("对白_%d", i)},
			},
			Mode: "structured",
		})
		if err != nil {
			t.Fatalf("submit turn %d: %v", i, err)
		}
		headID = node.NodeID
	}
	return st, sessionSvc, setup.Session.SessionID, setup.Branch.BranchID, headID
}

func summaryCompactor(t *testing.T, st *sqlite.Store, prov ports.ModelProvider) *CompactorService {
	t.Helper()
	mgr := NewProviderManager(fakeStoreWithSlot("reflection", ports.ProviderConfig{Slot: "reflection", Enabled: true, Kind: "mock"}), func(cfg ports.ProviderConfig) (ports.ModelProvider, error) { return prov, nil }, prov)
	if err := mgr.Reload(); err != nil {
		t.Fatal(err)
	}
	compactor := NewCompactorService(st, mgr, NewEventBus(st), ctxpkg.CompactionPolicy{
		TailWindowTurns: 6, MinUncompactedTurns: 8, PruneThoughtDepth: 6,
	})
	compactor.SetMetrics(&RuntimeMetrics{})
	return compactor
}

// 结构不合法的摘要会被"归一 + 一次修复重试"救回来：第二次调用必须带上
// 校验器的具体错误与首次输出（结构化错误回喂，而不是自动改写模型输出）。
func TestCompactorRepairsInvalidSummaryStructure(t *testing.T) {
	st, _, sessionID, branchID, headID := buildSummarySession(t)
	prov := &scriptedSummaryProvider{outputs: []string{invalidSummaryXML, validSummaryXML}}
	compactor := summaryCompactor(t, st, prov)

	art, err := compactor.RunOnce(context.Background(), sessionID, branchID, headID)
	if err != nil {
		t.Fatalf("修复重试后应成功：%v", err)
	}
	if art == nil || !strings.Contains(art.Text, "修复后的交接快照") {
		t.Fatalf("应采用修复后的摘要：%+v", art)
	}
	if prov.requestCount() != 2 {
		t.Fatalf("应恰好重试一次（含首次共 2 次调用），实际 %d 次", prov.requestCount())
	}
	repair := prov.lastRequest(t)
	last := repair.Messages[len(repair.Messages)-1]
	if !strings.Contains(last.Content, "不符合格式要求") || !strings.Contains(last.Content, "open_loops") {
		t.Fatalf("修复重试必须回喂具体错误，实际末条消息：%s", last.Content)
	}
	prev := repair.Messages[len(repair.Messages)-2]
	if prev.Role != "assistant" || !strings.Contains(prev.Content, "narrative_arc") {
		t.Fatalf("修复重试必须附带首次输出，实际上一条消息：%s/%s", prev.Role, prev.Content)
	}
}

// 同一错误连续失败达到阈值后进入冷却：不再每回合重付一次完整生成；
// 冷却期内返回 (nil, nil)，调用方看到的是"这一轮没做维护"而不是失败。
func TestCompactorBlocksAfterRepeatedSummaryFailures(t *testing.T) {
	st, _, sessionID, branchID, headID := buildSummarySession(t)
	prov := &scriptedSummaryProvider{outputs: []string{invalidSummaryXML}}
	compactor := summaryCompactor(t, st, prov)

	for attempt := 1; attempt <= summaryStormThreshold; attempt++ {
		art, err := compactor.RunOnce(context.Background(), sessionID, branchID, headID)
		if err == nil {
			t.Fatalf("第 %d 次应报错（摘要始终不合法）", attempt)
		}
		if art != nil {
			t.Fatalf("失败不应产出摘要工件")
		}
	}
	callsBefore := prov.requestCount()
	art, err := compactor.RunOnce(context.Background(), sessionID, branchID, headID)
	if err != nil || art != nil {
		t.Fatalf("冷却期内应静默跳过，得到 art=%v err=%v", art, err)
	}
	if prov.requestCount() != callsBefore {
		t.Fatalf("冷却期内不得再调用模型（调用数 %d → %d）", callsBefore, prov.requestCount())
	}
}

// 采纳侧要求"摘要必须严格小于它替代的原文"：比原文还长的摘要没有压缩价值，
// 落库只会占着预算挤掉别的材料。
func TestCompactorRejectsSummaryNotSmallerThanSource(t *testing.T) {
	st, _, sessionID, branchID, headID := buildSummarySession(t)
	// 合法结构但内容极长：≈9000 token，远大于原文（15 轮小回合），低于 32 KiB 流上限。
	bloated := "<story_checkpoint><narrative_arc>" + strings.Repeat("啰嗦的复述。", 1400) +
		"</narrative_arc><open_loops>- [悬念] 无</open_loops></story_checkpoint>"
	prov := &scriptedSummaryProvider{outputs: []string{bloated, bloated}}
	compactor := summaryCompactor(t, st, prov)

	art, err := compactor.RunOnce(context.Background(), sessionID, branchID, headID)
	if err == nil || art != nil {
		t.Fatalf("比原文更长的摘要不该被采纳：art=%v err=%v", art, err)
	}
	if !strings.Contains(err.Error(), "not smaller") {
		t.Fatalf("错误信息应说明原因，实际：%v", err)
	}
	sums, err := st.SummariesOnPath(headID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sums) != 0 {
		t.Fatalf("超长摘要不应落库，实际 %d 条", len(sums))
	}
}

// 同类失败的错误文本里数字会变（token 计数不同），逐字比较会让退避永远触发不了。
func TestSummaryErrorSignatureIgnoresNumbers(t *testing.T) {
	first := normalizeSummaryErrorSignature("summary not smaller than its source range: summary=528 source=517")
	second := normalizeSummaryErrorSignature("summary not smaller than its source range: summary=692 source=513")
	if first != second {
		t.Fatalf("同类失败应得到同一签名：\n%s\n%s", first, second)
	}
	other := normalizeSummaryErrorSignature("摘要 XML 含未授权层级")
	if other == first {
		t.Fatalf("不同类失败不应同签名")
	}
	if !strings.Contains(first, "#") {
		t.Fatalf("数字应被抹平为占位符：%s", first)
	}
}
