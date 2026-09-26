package application

import (
	"testing"

	"tavernagent/internal/domain"
)

func mkTurnNode(id string) *domain.PlotNode {
	return &domain.PlotNode{NodeID: id, Kind: domain.NodeKindTurn}
}

// 能力投影把"能做什么"从客户端收回服务端：这些用例锁住语义，
// 让客户端不必再自己实现一遍（此前 web 端用 storyHelpers 推导）。
func TestSessionCapabilities(t *testing.T) {
	head := mkTurnNode("n_head")
	old := mkTurnNode("n_old")

	t.Run("空闲且在分支头", func(t *testing.T) {
		branch := &domain.Branch{BranchID: "b1", HeadNodeID: "n_head"}
		caps := sessionCapabilities(branch, head)
		if !caps.CanSend || caps.Busy || caps.ViewingHistory || caps.CanFork || caps.Block != nil {
			t.Fatalf("空闲态能力异常: %+v", caps)
		}
		if !caps.AtTurnNode {
			t.Error("回合节点应标记 AtTurnNode")
		}
	})

	t.Run("生成中", func(t *testing.T) {
		branch := &domain.Branch{BranchID: "b1", HeadNodeID: "n_head", ActiveTurnID: "t9"}
		caps := sessionCapabilities(branch, head)
		if !caps.Busy || caps.ActiveTurnID != "t9" {
			t.Fatalf("在途回合未反映: %+v", caps)
		}
		if caps.CanSend {
			t.Error("生成中不应允许提交")
		}
		if caps.Block == nil || caps.Block.Code != "TURN_IN_FLIGHT" {
			t.Fatalf("缺少阻塞原因: %+v", caps.Block)
		}
	})

	t.Run("查看历史节点", func(t *testing.T) {
		branch := &domain.Branch{BranchID: "b1", HeadNodeID: "n_head"}
		caps := sessionCapabilities(branch, old)
		if !caps.ViewingHistory || !caps.CanFork {
			t.Fatalf("历史浏览能力异常: %+v", caps)
		}
		if caps.Busy {
			t.Error("历史浏览不是忙碌")
		}
	})

	t.Run("根节点不是回合节点", func(t *testing.T) {
		branch := &domain.Branch{BranchID: "b1", HeadNodeID: "root"}
		root := &domain.PlotNode{NodeID: "root", Kind: domain.NodeKindRoot}
		caps := sessionCapabilities(branch, root)
		if caps.AtTurnNode {
			t.Error("根节点不应标记 AtTurnNode")
		}
		if caps.ViewingHistory || caps.CanFork {
			t.Errorf("分支头即根节点时不应算历史浏览: %+v", caps)
		}
	})
}

func TestCapabilitiesOfTurn(t *testing.T) {
	cases := []struct {
		status       domain.TurnStatus
		canCancel    bool
		canContinue  bool
		canRetry     bool
		wantTerminal bool
	}{
		{domain.TurnGenerating, true, false, false, false},
		{domain.TurnAwaitingContinuation, true, true, false, false},
		{domain.TurnAwaitingApproval, true, false, false, false},
		{domain.TurnCommitted, false, false, false, true},
		{domain.TurnCancelled, false, false, false, true},
		{domain.TurnFailed, false, false, true, true},
		{domain.TurnConflicted, false, false, true, true},
	}
	for _, tc := range cases {
		got := turnCapabilities(tc.status)
		if got.CanCancel != tc.canCancel || got.CanContinue != tc.canContinue ||
			got.CanRetry != tc.canRetry || got.Terminal != tc.wantTerminal {
			t.Errorf("状态 %s → %+v，期望 cancel=%v continue=%v retry=%v terminal=%v",
				tc.status, got, tc.canCancel, tc.canContinue, tc.canRetry, tc.wantTerminal)
		}
	}
	if got := CapabilitiesOfTurn(nil); got != (TurnCapabilities{}) {
		t.Errorf("空回合应返回零值能力: %+v", got)
	}
}
