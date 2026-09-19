package application

import (
	"context"
	"testing"

	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// fakeNodes 用内存图充当父链读取面（headstate 只用到 GetNode）。
type fakeNodes map[string]*domain.PlotNode

func (f fakeNodes) GetNode(nodeID string) (*domain.PlotNode, error) {
	if n, ok := f[nodeID]; ok {
		return n, nil
	}
	return nil, ports.ErrNotFound
}

func memNode(id, parent string) *domain.PlotNode {
	return &domain.PlotNode{NodeID: id, ParentID: parent, Kind: domain.NodeKindMemoryChange}
}

func turnNode(id, parent string) *domain.PlotNode {
	return &domain.PlotNode{NodeID: id, ParentID: parent, Kind: domain.NodeKindTurn}
}

// headMatchesClient 是"客户端快照还算不算数"的唯一判据，这里把它能容忍与
// 不能容忍的形态逐一钉死。容忍范围必须窄：维护节点可以，对话推进不可以。
func TestHeadMatchesClientToleratesOnlyMaintenanceAdvance(t *testing.T) {
	// 图：root → turn1 → cog1 → cog2      root → turn1 → turn2
	nodes := fakeNodes{}
	nodes["root"] = &domain.PlotNode{NodeID: "root", Kind: domain.NodeKindRoot}
	nodes["turn1"] = turnNode("turn1", "root")
	nodes["cog1"] = memNode("cog1", "turn1")
	nodes["cog2"] = memNode("cog2", "cog1")
	nodes["turn2"] = turnNode("turn2", "turn1")
	nodes["dir1"] = &domain.PlotNode{NodeID: "dir1", ParentID: "root", Kind: domain.NodeKindDirectorEvent}

	cases := []struct {
		name            string
		head            string
		version         int64
		expectedHead    string
		expectedVersion int64
		want            bool
	}{
		{"完全相同", "turn1", 1, "turn1", 1, true},
		{"头相同但版本不同", "turn1", 2, "turn1", 1, false},
		{"只多了一个认知节点", "cog1", 2, "turn1", 1, true},
		{"只多了两个认知节点", "cog2", 3, "turn1", 1, true},
		{"只多了一个导演事件", "dir1", 1, "root", 0, true},
		{"对话推进不算同位置", "turn2", 2, "turn1", 1, false},
		{"维护节点叠在对话推进之上仍算推进", "cog2", 3, "turn1", 1, true},
		{"客户端快照比分支还新", "root", 0, "cog2", 5, false},
		{"客户端指向不存在的节点", "cog2", 3, "ghost", 1, false},
		{"空期望头退化为纯版本比较", "cog1", 2, "", 2, true},
		{"空期望头版本不符", "cog1", 2, "", 1, false},
	}
	for _, tc := range cases {
		branch := &domain.Branch{BranchID: "b", HeadNodeID: tc.head, Version: tc.version}
		if got := headMatchesClient(nodes, branch, tc.expectedHead, tc.expectedVersion); got != tc.want {
			t.Fatalf("%s: headMatchesClient(%s@%d, 期望 %s@%d) = %v, want %v",
				tc.name, tc.head, tc.version, tc.expectedHead, tc.expectedVersion, got, tc.want)
		}
	}
}

// 回归：后台认知在回合提交后落地（head 与 version 都前进），读者手里还是
// 提交前那份快照，此时继续剧情必须被接受。
//
// 这正是桌面壳上发生过的故障形态：认知节点比客户端刷新晚约 6 秒落地，
// 下一次提交就撞上 HEAD_CONFLICT "分支已变化，请刷新当前节点"，而作者的选择
// 其实完全有效。维护节点不推进叙事位置，因此不该产生冲突。
func TestMaintenanceNodeAdvanceDoesNotConflict(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, affectionScript(2))
	first := acceptAndWait(t, turnSvc, st, sessionID, branchID, "h-1", "你好。")

	before, err := st.GetBranch(branchID)
	if err != nil {
		t.Fatalf("branch: %v", err)
	}
	staleHead, staleVersion := before.HeadNodeID, before.Version
	if staleHead != first.ResultNodeID {
		t.Fatalf("回合结果应是分支头：%s vs %s", staleHead, first.ResultNodeID)
	}

	commitMaintenanceNode(t, st, sessionID, branchID, "cog_test_batch", "cog_test_node")

	after, err := st.GetBranch(branchID)
	if err != nil {
		t.Fatalf("branch: %v", err)
	}
	if after.HeadNodeID == staleHead || after.Version <= staleVersion {
		t.Fatalf("维护节点应当推进 head/version：%s@%d → %s@%d",
			staleHead, staleVersion, after.HeadNodeID, after.Version)
	}

	turn, err := turnSvc.Accept(context.Background(), sessionID, branchID, &TurnAcceptRequest{
		IdempotencyKey: "h-2", ExpectedHeadID: staleHead, ExpectedVersion: staleVersion,
		Input: domain.TurnInput{Kind: "text", Text: "我接着说。"},
	})
	if err != nil {
		t.Fatalf("维护节点落地不得让提交变成冲突: %v", err)
	}
	if got := waitTurn(t, turnSvc, turn.TurnID, domain.TurnCommitted); got.Status != domain.TurnCommitted {
		t.Fatalf("回合未提交: %s", got.Status)
	}
}

// 提交仍然必须在**对话推进**面前失败：放宽的只是维护节点，不是并发写。
func TestDialogueAdvanceStillConflicts(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, rootID := newTestServices(t, affectionScript(2))
	acceptAndWait(t, turnSvc, st, sessionID, branchID, "d-1", "我先来。")

	_, err := turnSvc.Accept(context.Background(), sessionID, branchID, &TurnAcceptRequest{
		IdempotencyKey: "d-2", ExpectedHeadID: rootID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "我也来。"},
	})
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Code != "HEAD_CONFLICT" {
		t.Fatalf("基于过期头的写入应被拒绝，实际 = %v", err)
	}
}

// commitMaintenanceNode 直接走存储层的维护写入（与后台认知同一条路径）。
func commitMaintenanceNode(t *testing.T, st *sqlite.Store, sessionID, branchID, batchID, nodeID string) {
	t.Helper()
	br, err := st.GetBranch(branchID)
	if err != nil {
		t.Fatalf("branch: %v", err)
	}
	res, err := st.CommitMemoryBatch(context.Background(), &ports.MemoryBatch{
		BatchID: batchID, SessionID: sessionID, BranchID: branchID,
		ExpectedHeadID: br.HeadNodeID, ExpectedVersion: br.Version,
		NodeID: nodeID, PayloadHash: "hash_" + batchID, Reason: "cognition.updated",
		Memories: []*domain.MemoryRecord{{
			MemoryID: nodeID + "_m", SourceNodeID: nodeID,
			Kind: domain.MemoryObserved, Content: "她提到北方的雪。",
		}},
	})
	if err != nil {
		t.Fatalf("commit memory batch: %v", err)
	}
	if res.ConflictCode != "" {
		t.Fatalf("维护写入冲突: %s", res.ConflictCode)
	}
	node, err := st.GetNode(res.NewHeadID)
	if err != nil || node.Kind != domain.NodeKindMemoryChange {
		t.Fatalf("维护写入应产生 memory_change 新头：node=%+v err=%v", node, err)
	}
}
