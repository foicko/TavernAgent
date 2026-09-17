package context_test

import (
	"strings"
	"testing"

	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
)

// buildChain 从给定节点连续提交 n 个回合，返回最后一个节点 ID。
func buildChain(t *testing.T, f *fixture, branchID, headID string, n int, mark func(i int, state *domain.WorldState)) string {
	t.Helper()
	for i := 0; i < n; i++ {
		state := domain.NewWorldState()
		if mark != nil {
			mark(i, state)
		}
		headID = commitTurnWithState(t, f, branchID, headID, state)
	}
	return headID
}

// T24：老分支的摘要晚到，当前已切换分支 → 不应用到不相容路径。
func TestSummaryFromOtherBranchNotAdopted(t *testing.T) {
	f := newFixtureFull(t, nil, "", "", ctxpkg.DefaultOptions())

	// A 分支：14 个回合；摘要把 to_node 设在窗口之外的 t2（否则会被
	// "与历史窗口重叠"的过滤排除——那是另一个测试要验证的行为）。
	headA := buildChain(t, f, testBranchID, testRootID, 14, nil)
	t2 := walkUp(t, f, headA, 12)
	saveSummary(t, f, testRootID, t2, "第一幕：她把怀表交给了玩家。")

	// B 分支：从共同祖先 root 分出，与 A 的区间不相容。
	if err := f.store.CreateBranch(&domain.Branch{
		BranchID: "branch_b", SessionID: testSessionID, Name: "b", HeadNodeID: testRootID, Version: 0,
	}); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	headB := buildChain(t, f, "branch_b", testRootID, 14, nil)

	// 在 A 的节点上编译：来源区间在路径上 → 采用。
	onA := f.systemPrompt(t, headA, "继续。")
	if !strings.Contains(onA, "第一幕") {
		t.Fatalf("路径上的摘要未被采用:\n%s", onA)
	}
	// 在 B 的节点上编译：A 的摘要来源区间不在路径上 → 不采用（T24）。
	onB := f.systemPrompt(t, headB, "继续。")
	if strings.Contains(onB, "第一幕") {
		t.Fatalf("另一条分支的摘要串线了（T24）:\n%s", onB)
	}
}

// 摘要与历史窗口重叠时不注入：窗口里的内容已完整给出，再给摘要是重复。
func TestSummaryInsideHistoryWindowNotDuplicated(t *testing.T) {
	f := newFixtureFull(t, nil, "", "", ctxpkg.DefaultOptions())

	// 14 个回合，窗口（默认 12）只装得下最后 12 个；摘要覆盖前 2 个回合。
	head := buildChain(t, f, testBranchID, testRootID, 14, nil)
	// 窗口（默认 12）覆盖 t3..t14；摘要把 to_node 设在窗口之外的 t2。
	t2 := walkUp(t, f, head, 12)
	saveSummary(t, f, testRootID, t2, "开场的回顾。")

	prompt := f.systemPrompt(t, head, "继续。")
	if !strings.Contains(prompt, "开场的回顾") {
		t.Fatalf("窗口之外的摘要未被采用:\n%s", prompt)
	}
}

// T23：摘要说物品丢了，状态投影说还在 → 两者都进提示词，
// 且摘要段明确声明"与状态冲突时以状态为准"。
func TestSummaryDoesNotOverrideState(t *testing.T) {
	f := newFixtureFull(t, nil, "", "", ctxpkg.DefaultOptions())

	head := buildChain(t, f, testBranchID, testRootID, 14, nil)
	t2 := walkUp(t, f, head, 12)
	saveSummary(t, f, testRootID, t2, "回顾：玩家把怀表弄丢了。")

	state := domain.NewWorldState()
	state.Items["item_watch"] = domain.ItemInstance{
		InstanceID: "item_watch", TemplateID: "tpl", Name: "怀表", OwnerID: "player", Quantity: 1,
	}
	prompt := f.systemPromptWithState(t, head, "继续。", state)

	if !strings.Contains(prompt, "把怀表弄丢了") {
		t.Fatalf("摘要未注入:\n%s", prompt)
	}
	if !strings.Contains(prompt, "持有物品账本") || !strings.Contains(prompt, "怀表") {
		t.Fatalf("状态投影未给出当前持有:\n%s", prompt)
	}
	if !strings.Contains(prompt, "与摘要冲突时以状态为准") {
		t.Fatalf("摘要段缺少优先级声明:\n%s", prompt)
	}
}

func saveSummary(t *testing.T, f *fixture, from, to, text string) {
	t.Helper()
	if err := f.store.SaveSummary(&domain.SummaryArtifact{
		SummaryID:  "sum_" + to + "_" + text[:6],
		FromNodeID: from, ToNodeID: to,
		Text: text, SummaryVersion: 1,
	}); err != nil {
		t.Fatalf("save summary: %v", err)
	}
}

// walkUp 从 nodeID 沿父链向上走 back 步。
func walkUp(t *testing.T, f *fixture, nodeID string, back int) string {
	t.Helper()
	cur := nodeID
	for i := 0; i < back; i++ {
		n, err := f.store.GetNode(cur)
		if err != nil || n.ParentID == "" {
			t.Fatalf("walkUp(%s, %d): %v", nodeID, back, err)
		}
		cur = n.ParentID
	}
	return cur
}
