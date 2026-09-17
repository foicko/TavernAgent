package context_test

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// 本文件锁定"承重摘要不可被静默淘汰"这条不变量。
//
// 背景：编译管线先折叠（把摘要覆盖的原文移出历史）、后裁剪预算。裁剪循环
// 此前把摘要当作最低优先级材料，于是出现"摘要和它覆盖的原文一起消失"：
// 模型看不到中期剧情的任何载体，只能编。下面两个用例分别钉住两侧边界——
// 承重时必须显式报错，不承重时仍允许淘汰。

var budgetTurnSeq atomic.Int64

// commitBudgetTurn 提交一个正文可控的回合（预算类用例需要足够体积：
// 夹具自带的 commitTurnWithState 正文固定为"推进。"，撑不起窗口压力）。
func commitBudgetTurn(t *testing.T, f *fixture, branchID, headID, text string) string {
	t.Helper()
	parent, err := f.store.GetNode(headID)
	if err != nil {
		t.Fatalf("parent: %v", err)
	}
	state := domain.NewWorldState()
	nextJSON, err := state.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	seq := strconv.FormatInt(budgetTurnSeq.Add(1), 10)
	turnID := "turn_budget_" + seq
	nodeID := "node_budget_" + seq
	branch, err := f.store.GetBranch(branchID)
	if err != nil {
		t.Fatalf("branch: %v", err)
	}
	if err := f.store.CreateTurnRequest(&domain.TurnRequest{
		TurnID: turnID, SessionID: testSessionID, BranchID: branchID,
		IdempotencyKey: turnID, PayloadHash: "h", ExpectedHeadID: headID,
		Status: domain.TurnQueued, Mode: "structured",
	}); err != nil {
		t.Fatalf("create turn: %v", err)
	}
	if ok, err := f.store.ClaimActiveTurn(branchID, turnID); err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	body, err := json.Marshal(map[string]any{
		"inputText": "我继续往前。",
		"blocks":    []map[string]any{{"kind": "narration", "text": text}},
	})
	if err != nil {
		t.Fatalf("marshal turn content: %v", err)
	}
	if _, err := f.store.CommitTurn(&ports.CommitPlan{
		TurnID: turnID, ExpectedHeadID: headID, ExpectedVersion: branch.Version,
		Node: &domain.PlotNode{
			NodeID: nodeID, SessionID: testSessionID, ParentID: headID, Kind: domain.NodeKindTurn,
			Depth: parent.Depth + 1, TurnNumber: parent.TurnNumber + 1, SchemaVersion: 1,
			ContentJSON: string(body),
		},
		NewStateHash: state.HashID(), NewStateJSON: nextJSON,
	}); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return nodeID
}

// commitBudgetChain 提交 n 个指定体积的回合，返回头节点 ID 与每轮节点 ID。
func commitBudgetChain(t *testing.T, f *fixture, n, runes int) (string, []string) {
	t.Helper()
	head := testRootID
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		head = commitBudgetTurn(t, f, testBranchID, head, repeatRunes(i, runes))
		ids = append(ids, head)
	}
	return head, ids
}

// repeatRunes 生成体积可控且可识别的正文：内容互不相同，便于断言"哪一轮还在"。
func repeatRunes(seed, n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteRune(rune(0x4E00 + (seed*37+i)%2000))
	}
	return b.String()
}

// TestCompileErrorsWhenLoadBearingSummaryDoesNotFit 是本批次的核心用例：
// 摘要覆盖的原文已被折叠移除（承重），预算又装不下摘要时，必须显式报错，
// 不允许把摘要悄悄丢掉让模型在失忆状态下继续写。
//
// 有齿验证：把 budget.go 淘汰循环里的承重判定去掉（恢复 `summaries[1:]` 无条件丢弃），
// 本用例会在第一个断言处失败——当前实现正是如此（用例先红后绿）。
func TestCompileErrorsWhenLoadBearingSummaryDoesNotFit(t *testing.T) {
	opts := ctxpkg.DefaultOptions()
	// 尾部窗口压到 2 轮：早段必然进入折叠区，摘要成为那一段历史的唯一载体。
	opts.CompactionPolicy = ctxpkg.CompactionPolicy{TailWindowTurns: 2, MinUncompactedTurns: 1}
	f := newFixtureFull(t, nil, "", "", opts)

	head, ids := commitBudgetChain(t, f, 12, 300)
	// 摘要覆盖前 3 轮；to_node 落在尾部窗口之外，保证被折叠。
	saveSummary(t, f, testRootID, ids[2], repeatRunes(99, 1500))

	compiler := f.compiler.WithBudget(4096, 1024) // 输入预算 = 4096-1024-max(512,204) = 2560
	req, err := compiler.Compile(context.Background(), testSessionID, head, "我继续往前。", domain.NewWorldState(), nil)
	if err == nil {
		t.Fatalf("承重摘要装不下时必须显式报错，实际编译成功（messages=%d，tokens≈%d）；"+
			"这正是「摘要与原文一起消失」的旧行为", len(req.Messages), ctxpkg.MessageTokens(req.Messages))
	}
	ce, ok := err.(*ctxpkg.Error)
	if !ok || ce.Code != "CONTEXT_OVER_BUDGET" {
		t.Fatalf("应返回 CONTEXT_OVER_BUDGET，得到 %T: %v", err, err)
	}
	if !strings.Contains(ce.Message, "摘要") || !strings.Contains(ce.Message, "窗口") {
		t.Fatalf("错误信息应说明摘要装不下并给出可操作建议，实际：%s", ce.Message)
	}
}

// TestCompileDropsRedundantSummaryBeforeHistory 钉住不变量的另一侧：
// 摘要覆盖的区间仍完整在历史里（冗余副本）时，淘汰摘要不丢任何原文，
// 而且必须**先于正文历史**被淘汰——否则会为了腾空间去删真正的历史。
func TestCompileDropsRedundantSummaryBeforeHistory(t *testing.T) {
	opts := ctxpkg.DefaultOptions()
	opts.CompactionPolicy = ctxpkg.CompactionPolicy{TailWindowTurns: 20, MinUncompactedTurns: 8}
	f := newFixtureFull(t, nil, "", "", opts)

	head, ids := commitBudgetChain(t, f, 4, 300)
	// 摘要覆盖最后两轮：折叠只移除尾部保护之外的最旧回合，摘要区间仍完整
	// 在历史里——它是冗余副本，可以且应当先于正文历史被淘汰。
	summaryText := repeatRunes(99, 3000)
	saveSummary(t, f, ids[2], ids[3], summaryText)

	compiler := f.compiler.WithBudget(4608, 1024)
	req, err := compiler.Compile(context.Background(), testSessionID, head, "我继续往前。", domain.NewWorldState(), nil)
	if err != nil {
		t.Fatalf("冗余摘要应被优先淘汰而不是整轮失败：%v", err)
	}
	prompt := ""
	for _, m := range req.Messages {
		prompt += m.Content + "\n"
	}
	if strings.Contains(prompt, summaryText[:12]) {
		t.Fatalf("预算不足时未淘汰冗余摘要")
	}
	for i, id := range ids {
		_ = id
		marker := repeatRunes(i, 300)[:12]
		if !strings.Contains(prompt, marker) {
			t.Fatalf("淘汰冗余摘要不该影响正文历史：第 %d 轮已不在提示词里", i+1)
		}
	}
}

// TestCompileErrorsWhenSummaryRangeIsOlderThanLoadedHistory 覆盖第三种情况：
// 摘要区间比加载到的历史更早（从未进入窗口）。它同样不是冗余副本——
// 丢掉它，那段剧情就彻底不在提示词里了，因此必须报错而不是静默丢弃。
//
// 只按"是否折叠过"判断承重的实现会在这里漏判（区间没被折叠，却也不在历史里）。
func TestCompileErrorsWhenSummaryRangeIsOlderThanLoadedHistory(t *testing.T) {
	opts := ctxpkg.DefaultOptions()
	opts.CompactionPolicy = ctxpkg.CompactionPolicy{TailWindowTurns: 20, MinUncompactedTurns: 8}
	opts.MaxHistoryTurns = 4 // 只加载最近 4 轮，早段永远不进提示词
	f := newFixtureFull(t, nil, "", "", opts)

	head, ids := commitBudgetChain(t, f, 12, 300)
	saveSummary(t, f, testRootID, ids[2], repeatRunes(99, 3000))

	compiler := f.compiler.WithBudget(4608, 1024)
	_, err := compiler.Compile(context.Background(), testSessionID, head, "我继续往前。", domain.NewWorldState(), nil)
	if err == nil {
		t.Fatalf("区间早于加载窗口的摘要装不下时必须显式报错，不能静默丢弃")
	}
	ce, ok := err.(*ctxpkg.Error)
	if !ok || ce.Code != "CONTEXT_OVER_BUDGET" {
		t.Fatalf("应返回 CONTEXT_OVER_BUDGET，得到 %T: %v", err, err)
	}
}

// 预算裁剪必须可观测：OnBudget 此前全仓没有生产消费者，"这一轮悄悄丢了什么"
// 在运行期不可见。这个用例钉住"回调拿到了真实裁剪数字"。
func TestBudgetReportReachesObserver(t *testing.T) {
	opts := ctxpkg.DefaultOptions()
	opts.CompactionPolicy = ctxpkg.CompactionPolicy{TailWindowTurns: 3, MinUncompactedTurns: 1}
	f := newFixtureFull(t, nil, "", "", opts)
	head, _ := commitBudgetChain(t, f, 12, 300)

	var got ctxpkg.BudgetReport
	seen := 0
	compiler := f.compiler.WithBudget(4096, 1024).WithBudgetObserver(func(report ctxpkg.BudgetReport) {
		got = report
		seen++
	})
	_, err := compiler.Compile(context.Background(), testSessionID, head, "我继续往前。", domain.NewWorldState(), nil)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if seen != 1 {
		t.Fatalf("OnBudget 应恰好回调一次，实际 %d 次", seen)
	}
	if got.DroppedHistory == 0 {
		t.Fatalf("12 个大回合 + 4096 窗口应触发历史裁剪，报告：%+v", got)
	}
	if got.ProtectedTurns == 0 || got.Budget == 0 {
		t.Fatalf("报告应带保护轮数与预算：%+v", got)
	}
}

// 校准系数必须真的进入预算判定，否则"用真实用量校准"只是装饰。
//
// 注意观察量：编译器会**填满**预算，所以裁剪后的 Total 不会随系数变小；
// 随系数成比例变化的是裁前的 BeforeTrim（同一份材料的估算总量）。
func TestTokenCalibrationScalesBudgetDecisions(t *testing.T) {
	opts := ctxpkg.DefaultOptions()
	opts.CompactionPolicy = ctxpkg.CompactionPolicy{TailWindowTurns: 3, MinUncompactedTurns: 1}
	f := newFixtureFull(t, nil, "", "", opts)
	head, _ := commitBudgetChain(t, f, 12, 300)
	state := domain.NewWorldState()

	var rawReport, calReport ctxpkg.BudgetReport
	raw := f.compiler.WithBudget(4096, 1024).WithBudgetObserver(func(r ctxpkg.BudgetReport) { rawReport = r })
	if _, err := raw.Compile(context.Background(), testSessionID, head, "我继续往前。", state, nil); err != nil {
		t.Fatalf("raw compile: %v", err)
	}
	calibrated := f.compiler.WithBudget(4096, 1024).WithTokenCalibration(0.5).
		WithBudgetObserver(func(r ctxpkg.BudgetReport) { calReport = r })
	if _, err := calibrated.Compile(context.Background(), testSessionID, head, "我继续往前。", state, nil); err != nil {
		t.Fatalf("calibrated compile: %v", err)
	}
	if rawReport.BeforeTrim == 0 || calReport.BeforeTrim == 0 {
		t.Fatalf("报告缺少裁前读数：raw=%+v cal=%+v", rawReport, calReport)
	}
	ratio := float64(calReport.BeforeTrim) / float64(rawReport.BeforeTrim)
	if ratio > 0.55 || ratio < 0.45 {
		t.Fatalf("系数 0.5 应让裁前估算减半，实际比值 %.2f（raw=%d cal=%d）",
			ratio, rawReport.BeforeTrim, calReport.BeforeTrim)
	}
	if calReport.Budget != rawReport.Budget {
		t.Fatalf("校准不应改变输入预算本身：raw=%d cal=%d", rawReport.Budget, calReport.Budget)
	}
	// 系数减半后同样的窗口能装下更多材料：被裁掉的历史不应多于未校准版本。
	if calReport.DroppedHistory > rawReport.DroppedHistory {
		t.Fatalf("校准后反而丢了更多历史：raw=%d cal=%d", rawReport.DroppedHistory, calReport.DroppedHistory)
	}
}
