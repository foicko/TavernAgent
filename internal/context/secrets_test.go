package context_test

import (
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// 卡片带两条秘密：一条有揭示条件，一条无条件。
const testSecrets = `[
  {"secretId":"sec_key","title":"钟楼的钥匙","content":"钥匙其实埋在神龛底下，被她的围巾缠着。","order":1},
  {"secretId":"sec_free","title":"她的旧名","content":"她在旧城里还有另一个名字。","order":2}
]`

// T25 核心断言：未揭示的秘密内容**绝不**进入上下文。
// 模型即使提议解锁，也改变不了这一点——它无权自证达成。
func TestSecretContentNotLeakedBeforeUnlock(t *testing.T) {
	f := newFixtureFull(t, nil, "", testSecrets, ctxpkg.DefaultOptions())
	prompt := f.systemPrompt(t, testRootID, "我们去钟楼看看。")
	if strings.Contains(prompt, "神龛底下") || strings.Contains(prompt, "钟楼的钥匙") {
		t.Fatalf("未揭示的秘密泄漏进了上下文:\n%s", prompt)
	}
	if strings.Contains(prompt, "另一个名字") {
		t.Fatalf("未揭示的秘密（无条件条目）泄漏进了上下文:\n%s", prompt)
	}
	if strings.Contains(prompt, "已揭示的世界观与秘密") {
		t.Fatalf("尚未揭示任何秘密却出现了该区块:\n%s", prompt)
	}
}

// 揭示后，内容进入**下一次**编译的上下文；未揭示的其余条目仍然被挡住。
//
// 生产路径里 Compile 收到的 state 是调用方给的基准状态投影（TurnService
// 从 StateAt 取），这里同样显式传入——揭示状态来自"这个节点的状态"。
func TestSecretContentInjectedAfterUnlock(t *testing.T) {
	f := newFixtureFull(t, nil, "", testSecrets, ctxpkg.DefaultOptions())

	state := domain.NewWorldState()
	state.UnlockSecret("sec_key")
	head := commitTurnWithState(t, f, testBranchID, testRootID, state)

	prompt := f.systemPromptWithState(t, head, "现在都清楚了。", state)
	if !strings.Contains(prompt, "神龛底下") {
		t.Fatalf("已揭示的秘密未注入:\n%s", prompt)
	}
	if strings.Contains(prompt, "另一个名字") {
		t.Fatalf("未揭示的条目被一起注入了:\n%s", prompt)
	}
}

// 揭示状态来自**查看位置**的状态投影：在解锁之前的节点上编译，看不到之后才解锁的内容。
// 这与"查看历史节点得到那一刻的状态"是同一条规则（技术契约 §7）。
func TestSecretVisibilityFollowsViewNode(t *testing.T) {
	f := newFixtureFull(t, nil, "", testSecrets, ctxpkg.DefaultOptions())

	before := commitTurnWithState(t, f, testBranchID, testRootID, domain.NewWorldState())

	unlocked := domain.NewWorldState()
	unlocked.UnlockSecret("sec_key")
	after := commitTurnWithState(t, f, testBranchID, before, unlocked)

	// 在"解锁之后"的节点上编译 → 注入。
	prompt := f.systemPromptWithState(t, after, "现在都清楚了。", unlocked)
	if !strings.Contains(prompt, "神龛底下") {
		t.Fatalf("已揭示的秘密未注入:\n%s", prompt)
	}
	// 在"解锁之前"的节点上编译 → 不注入（回看历史不应看到未来的揭示）。
	earlier := f.systemPromptWithState(t, before, "我去钟楼看看。", domain.NewWorldState())
	if strings.Contains(earlier, "神龛底下") {
		t.Fatalf("回看解锁前的节点却看到了秘密（时序泄漏）:\n%s", earlier)
	}
}

var commitSeq atomic.Int64

// commitTurnWithState 在 headID 上提交一个回合，其状态投影由调用方给定。
// 返回新节点 ID。测试夹具的根节点没有快照，因此不校验基准哈希
// （提交路径只在 BaseStateHash 非空时才校验）。
func commitTurnWithState(t *testing.T, f *fixture, branchID, headID string, state *domain.WorldState) string {
	t.Helper()
	parent, err := f.store.GetNode(headID)
	if err != nil {
		t.Fatalf("parent: %v", err)
	}
	nextJSON, err := state.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// 幂等键必须全局唯一（表约束按 session+key 唯一）：分支 B 的同序号
	// 回合会与 A 撞键，因此加一个进程内计数器。
	commitSeq.Add(1)
	seq := strconv.FormatInt(commitSeq.Load(), 10)
	turnID := "turn_sec_" + seq
	nodeID := "node_sec_" + seq
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
	res, err := f.store.CommitTurn(&ports.CommitPlan{
		TurnID: turnID, ExpectedHeadID: headID, ExpectedVersion: branch.Version,
		Node: &domain.PlotNode{
			NodeID: nodeID, SessionID: testSessionID, ParentID: headID, Kind: domain.NodeKindTurn,
			Depth: parent.Depth + 1, TurnNumber: parent.TurnNumber + 1, SchemaVersion: 1,
			ContentJSON: `{"inputText":"推进。","blocks":[{"kind":"narration","text":"推进。"}]}`,
		},
		NewStateHash: state.HashID(), NewStateJSON: nextJSON,
	})
	if err != nil || !res.Committed {
		t.Fatalf("commit: %v %+v", err, res)
	}
	return nodeID
}
