package context

import (
	"strings"
	"testing"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

func TestEstimateTokens(t *testing.T) {
	if got := EstimateTokens(""); got != 0 {
		t.Fatalf("空文本应为 0，得到 %d", got)
	}
	// CJK 每字符 1 token（保守上限）。
	if got := EstimateTokens("铜钥匙"); got != 3 {
		t.Fatalf("EstimateTokens(铜钥匙) = %d, want 3", got)
	}
	// 拉丁按 3 字符 1 token（向上取整）。
	if got := EstimateTokens("abcdef"); got != 2 {
		t.Fatalf("EstimateTokens(abcdef) = %d, want 2", got)
	}
	// 空白不计。
	if got := EstimateTokens("a b"); got != 1 {
		t.Fatalf("EstimateTokens(a b) = %d, want 1", got)
	}
}

func TestInputBudget(t *testing.T) {
	// 窗口未知 → 0（调用方据此跳过裁剪，而不是猜上限）。
	if got := (CompilerOptions{}).InputBudget(); got != 0 {
		t.Fatalf("未配置窗口应返回 0，得到 %d", got)
	}
	if got := (CompilerOptions{ContextWindow: 8000, ReservedOutput: 1000, SafetyMargin: 500}).InputBudget(); got != 6500 {
		t.Fatalf("预算 = %d, want 6500", got)
	}
	// Invalid configurations remain invalid; never invent a larger context.
	if got := (CompilerOptions{ContextWindow: 100, ReservedOutput: 5000}).InputBudget(); got != -4900 {
		t.Fatalf("overcommitted output budget = %d", got)
	}
}

// 展示用的 InputBudget(window, reserved) 不应为负：校验放行的配置
// （窗口 2048、输出预留留空→默认 2048）按公式会得到 -512。
func TestPackageInputBudgetNeverNegative(t *testing.T) {
	if got := InputBudget(2048, 0); got != 0 {
		t.Fatalf("InputBudget(2048, 0) = %d, want 0", got)
	}
	if got := InputBudget(8192, 1024); got != 8192-1024-512 {
		t.Fatalf("InputBudget(8192, 1024) = %d", got)
	}
}

func mustTurn(content string) *domain.PlotNode {
	return mustTurnID("n_"+content, content)
}

// mustTurnID 生成带独立 NodeID 的回合，便于断言"丢的是最旧的那个"。
func mustTurnID(id, content string) *domain.PlotNode {
	return &domain.PlotNode{
		NodeID: id, Kind: domain.NodeKindTurn, Depth: 1, TurnNumber: 1,
		ContentJSON: `{"inputText":"` + content + `","blocks":[{"kind":"narration","text":"` + content + `"}]}`,
	}
}

// repeatCJK 生成 n 个互不相同的中文字符（n 字 ≈ n token）。
func repeatCJK(t *testing.T, n int) string {
	t.Helper()
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteRune(rune(0x4E00 + i))
	}
	return b.String()
}

// M5：历史裁剪改为"阶段内二分最小丢弃数"后，结果必须仍然最小——
// 少丢一个节点就装不下，且丢的是最旧的节点、最近的历史必须保留。
func TestFitToBudgetHistoryDropsMinimalOldestTurns(t *testing.T) {
	c := New(nil, CompilerOptions{ContextWindow: 6000, ReservedOutput: 200, SafetyMargin: 100})
	c.options.CompactionPolicy.TailWindowTurns = 5
	sc := sessionContext{OpeningText: "开场。"}
	state := domain.NewWorldState()
	history := make([]*domain.PlotNode, 0, 40)
	for i := 0; i < 40; i++ {
		content := string(rune(0x4E00+i)) + repeatCJK(t, 99)
		history = append(history, mustTurn(content))
	}

	_, _, fitHistory, report, err := c.fitToBudget(nil, nil, history, nil, sc, state, "输入。", "")
	if err != nil {
		t.Fatalf("fit 应成功: %v", err)
	}
	if report.DroppedHistory == 0 {
		t.Fatalf("应触发历史裁剪: %+v", report)
	}
	if report.Total > report.Budget {
		t.Fatalf("裁剪后仍超预算: %d > %d", report.Total, report.Budget)
	}
	if len(fitHistory) != len(history)-report.DroppedHistory {
		t.Fatalf("保留数 %d != 总数 %d - 丢弃 %d", len(fitHistory), len(history), report.DroppedHistory)
	}
	// 最小性：少丢一个节点就应装不下（否则二分丢多了）。
	k := report.DroppedHistory
	if got := c.EstimateMessages(c.buildMessages(nil, nil, history[k-1:], nil, nil, sc, state, "输入。", "", k-1)); got <= report.Budget {
		t.Fatalf("丢弃数不是最小：少丢一个也装得下（%d <= %d）", got, report.Budget)
	}
	// 丢的是最旧的：最近一轮必须保留。
	if fitHistory[len(fitHistory)-1].NodeID != history[len(history)-1].NodeID {
		t.Fatalf("最近的历史被裁掉了")
	}
}

// 预算裁剪的顺序：世界书先丢，其次记忆，最后才是历史。
// 三个压力逐级上升，保证"只丢了该丢的那一层"——顺序写错时这里会立刻暴露。
func TestFitToBudgetDropsLowestPriorityFirst(t *testing.T) {
	c := New(nil, CompilerOptions{ContextWindow: 4000, ReservedOutput: 200, SafetyMargin: 100})
	budget := c.options.InputBudget()
	sc := sessionContext{OpeningText: "开场。"}
	state := domain.NewWorldState()
	state.Characters["npc_a"] = domain.CharacterInfo{CharacterID: "npc_a", Name: "甲", Participant: true}
	big := repeatCJK(t, 300)

	// 压力 1：只有世界书超预算 → 只丢世界书。
	lore := make([]lorebookHit, 0, 10)
	for i := 0; i < 10; i++ {
		lore = append(lore, lorebookHit{Entry: domain.LorebookEntry{EntryID: "e", Content: big}})
	}
	fitLore, _, _, report, err := c.fitToBudget(lore, nil, nil, nil, sc, state, "输入。", "")
	if err != nil {
		t.Fatalf("fit: %v", err)
	}
	if report.DroppedLorebook == 0 || report.DroppedMemories != 0 || report.DroppedHistory != 0 {
		t.Fatalf("应只丢世界书: %+v", report)
	}
	if len(fitLore) == len(lore) {
		t.Fatalf("世界书条数未减少")
	}
	if over := EstimateTokens(renderLoreSection(fitLore)); over > budget-report.Mandatory {
		t.Fatalf("裁剪后世界书仍超预算: %d > %d", over, budget-report.Mandatory)
	}

	// 压力 2：世界书很小、记忆超预算 → 只丢记忆。
	bigMem := &domain.MemoryRecord{MemoryID: "m", Content: big}
	_, fitMems, _, report2, err := c.fitToBudget(lore[:1],
		[]*domain.MemoryRecord{bigMem, bigMem, bigMem, bigMem, bigMem, bigMem, bigMem, bigMem, bigMem, bigMem, bigMem, bigMem}, nil, nil, sc, state, "输入。", "")
	if err != nil {
		t.Fatalf("fit2: %v", err)
	}
	if report2.DroppedMemories == 0 || report2.DroppedHistory != 0 || report2.DroppedLorebook != 1 {
		t.Fatalf("应只丢记忆: %+v", report2)
	}
	if len(fitMems) == 12 {
		t.Fatalf("记忆条数未减少")
	}

	// 压力 3：只有历史超预算 → 丢最旧的回合，且停在"最近 N 轮"保护下限。
	// 构造要点：回合数需多于保护下限（默认 20），且单回合体积要落在
	// "30 轮超出预算、裁到 20 轮能装下"的区间，否则会走 422 分支。
	// 窗口取值与必修提示词（扮演准则 + 输出协议 + 人设）的体量绑定：必修部分增删
	// 约 300 token 就要同步挪动窗口，否则本用例会滑出上述区间而误报。
	// v2 重写扮演准则（+347）、v3 加选项判定与 <player_note> 声明（+268）后由 8500 上调。
	c3 := New(nil, CompilerOptions{ContextWindow: 9150, ReservedOutput: 200, SafetyMargin: 100})
	mid := repeatCJK(t, 150)
	history := make([]*domain.PlotNode, 0, 30)
	for i := 0; i < 30; i++ {
		history = append(history, mustTurnID("turn_"+itoa(i), mid))
	}
	_, _, fitHistory, report3, err := c3.fitToBudget(nil, nil, history, nil, sc, state, "输入。", "")
	if err != nil {
		t.Fatalf("fit3: %v", err)
	}
	if report3.DroppedHistory == 0 {
		t.Fatalf("应丢历史: %+v", report3)
	}
	// 最旧的先丢：剩下的第一个应是原本靠后的回合。
	if len(fitHistory) > 0 && fitHistory[0].NodeID != history[report3.DroppedHistory].NodeID {
		t.Fatalf("丢的不是最旧的回合：剩首 = %s, want %s",
			fitHistory[0].NodeID, history[report3.DroppedHistory].NodeID)
	}
	// 保护下限：不得把回合数裁到 20 以下。
	if got := countTurns(fitHistory); got < 20 {
		t.Fatalf("裁剪后回合数 = %d，突破了最近 20 轮的保护下限", got)
	}
}

// TestFitToBudgetProtectsRecentTurns 锁定"最近回合不被静默删除"。
//
// 契约要求摘要只替代其覆盖区间、保留最近对话，但预算裁剪此前没有下限，
// 会把最近几轮原文一路删到空——与契约冲突且用户无感知。
//
// 有齿验证：把 budget.go 里 `countTurns(ancestors) > protected` 的保护条件去掉，
// 本用例会在 "DroppedHistory != 0" 处失败。
func TestFitToBudgetProtectsRecentTurns(t *testing.T) {
	c := New(nil, CompilerOptions{ContextWindow: 4000, ReservedOutput: 200, SafetyMargin: 100})
	sc := sessionContext{OpeningText: "开场。"}
	state := domain.NewWorldState()
	state.Characters["npc_a"] = domain.CharacterInfo{CharacterID: "npc_a", Name: "甲", Participant: true}
	big := repeatCJK(t, 300)

	// 10 个回合：低于保护下限 20，因此一个都不该被裁。
	history := make([]*domain.PlotNode, 0, 10)
	for i := 0; i < 10; i++ {
		history = append(history, mustTurnID("t_"+itoa(i), big))
	}
	_, _, fitHistory, report, err := c.fitToBudget(nil, nil, history, nil, sc, state, "输入。", "")
	if err == nil {
		t.Fatalf("无可裁剪材料时应显式报错（不静默删最近回合）: %+v", report)
	}
	if report.DroppedHistory != 0 {
		t.Fatalf("DroppedHistory = %d，最近回合被裁剪了（应受保护）", report.DroppedHistory)
	}
	if len(fitHistory) != len(history) {
		t.Fatalf("历史被改动：%d -> %d", len(history), len(fitHistory))
	}
	ce, ok := err.(*Error)
	if !ok || ce.Code != "CONTEXT_OVER_BUDGET" {
		t.Fatalf("应返回 CONTEXT_OVER_BUDGET，得到 %v", err)
	}
	if !strings.Contains(ce.Message, "已保护最近") {
		t.Fatalf("错误信息应说明保护下限: %s", ce.Message)
	}
}

// 不可省略部分超预算必须报错，不能静默截断核心约束（契约 §9.1）。
func TestFitToBudgetErrorsWhenMandatoryExceeds(t *testing.T) {
	c := New(nil, CompilerOptions{ContextWindow: MinInputBudget})
	sc := sessionContext{OpeningText: strings.Repeat("开场白。", 2000)}
	_, _, _, _, err := c.fitToBudget(nil, nil, nil, nil, sc, domain.NewWorldState(), strings.Repeat("输入。", 2000), "")
	if err == nil {
		t.Fatalf("不可省略部分超预算应报错")
	}
	ce, ok := err.(*Error)
	if !ok || ce.Code != "CONTEXT_OVER_BUDGET" {
		t.Fatalf("应返回 CONTEXT_OVER_BUDGET，得到 %v", err)
	}
}

// 预算充裕时不裁剪。
func TestFitToBudgetKeepsEverythingWithinBudget(t *testing.T) {
	c := New(nil, CompilerOptions{ContextWindow: 200000, ReservedOutput: 1000, SafetyMargin: 500})
	lore := []lorebookHit{{Entry: domain.LorebookEntry{EntryID: "e", Content: "短条目。"}}}
	mems := []*domain.MemoryRecord{{MemoryID: "m", Content: "短记忆。"}}
	history := []*domain.PlotNode{mustTurn("短回合。")}
	_, _, _, report, err := c.fitToBudget(lore, mems, history, nil, sessionContext{OpeningText: "开场。"},
		domain.NewWorldState(), "输入。", "")
	if err != nil {
		t.Fatalf("fit: %v", err)
	}
	if report.DroppedLorebook != 0 || report.DroppedMemories != 0 || report.DroppedHistory != 0 {
		t.Fatalf("预算充裕却裁剪了: %+v", report)
	}
}

// nodeOnlyStore 只实现摘要冗余判定需要的 GetNode；其余方法未实现（调用即 panic）。
type nodeOnlyStore struct {
	ports.CompileDeps
	nodes map[string]*domain.PlotNode
}

func (s nodeOnlyStore) GetNode(id string) (*domain.PlotNode, error) {
	return s.nodes[id], nil
}

// SummariesOnPath 按端点深度升序返回，[0] 往往是承重的最旧摘要。
// 淘汰循环此前只看 summaries[0] 是否冗余，排在后面的冗余摘要永远丢不掉，
// 于是本可装下的回合以 CONTEXT_OVER_BUDGET 失败。
func TestFitBundleBudgetDropsRedundantSummaryAtAnyIndex(t *testing.T) {
	nodes := map[string]*domain.PlotNode{}
	for d := 1; d <= 7; d++ {
		id := "d" + itoa(d)
		nodes[id] = &domain.PlotNode{NodeID: id, Kind: domain.NodeKindTurn, Depth: d, TurnNumber: d,
			ContentJSON: `{"inputText":"前进。","blocks":[{"kind":"narration","text":"第` + itoa(d) + `轮。"}]}`}
	}
	// 历史只剩 5..7 轮（1..2 轮已折叠），低于保护下限，不可再裁。
	ancestors := []*domain.PlotNode{nodes["d5"], nodes["d6"], nodes["d7"]}
	loadBearing := &domain.SummaryArtifact{SummaryID: "s_old", FromNodeID: "d1", ToNodeID: "d2", Text: "早段剧情摘要。"}
	redundantSummary := &domain.SummaryArtifact{SummaryID: "s_new", FromNodeID: "d6", ToNodeID: "d7", Text: repeatCJK(t, 800)}
	summaries := []*domain.SummaryArtifact{loadBearing, redundantSummary}

	sc := sessionContext{OpeningText: "开场。"}
	state := domain.NewWorldState()
	c := New(nil, CompilerOptions{})
	c.store = nodeOnlyStore{nodes: nodes}
	// 预算恰好装下"必修 + 历史 + 承重摘要"，装不下那条 800 字的冗余摘要。
	fits := c.EstimateMessages(c.buildMessages(nil, nil, ancestors, []*domain.SummaryArtifact{loadBearing}, nil, sc, state, "输入。", "", 0))
	c.options.ContextWindow = fits + 50

	_, _, fitHistory, fitSummaries, report, err := c.fitBundleBudget(nil, nil, ancestors, summaries, nil, sc, state, "输入。", "", c.EstimateMessages(c.buildMessages(nil, nil, nil, nil, nil, sc, state, "输入。", "", 0)))
	if err != nil {
		t.Fatalf("丢掉排在后面的冗余摘要即可装下，却整轮失败：%v", err)
	}
	if report.DroppedSummaries != 1 || len(fitSummaries) != 1 || fitSummaries[0].SummaryID != "s_old" {
		t.Fatalf("应只丢冗余摘要、保留承重摘要：report=%+v summaries=%v", report, fitSummaries)
	}
	if len(fitHistory) != len(ancestors) {
		t.Fatalf("不该裁历史：%d -> %d", len(ancestors), len(fitHistory))
	}
	if len(summaries) != 2 || summaries[1] != redundantSummary {
		t.Fatalf("调用方的摘要切片被改写了")
	}
}
