package sqlite

import (
	"errors"
	"testing"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// M23：提交方声明了 BaseStateHash，但基准节点的祖先链上没有任何状态投影
// （导入不完整 / 写入中断 / 数据损坏）时，必须 fail-closed——否则基于陈旧 head
// 的提交会在没有 STATE_MISMATCH 防护的情况下被接受。
func TestCommitTurnFailsClosedWhenBaseStateMissing(t *testing.T) {
	s := newTestStore(t)
	sess, _, branch := seedSession(t, s)
	seedTurn(t, s, sess.SessionID, branch.BranchID, "turn1", "root", 0)

	base := domain.NewWorldState()
	// 故意不落根节点状态快照。
	plan := makeCommitPlan(sess.SessionID, "turn1", branch.BranchID, "node1", "root", 0, HashState(base), nil, base.Clone())

	if _, err := s.CommitTurn(plan); !errors.Is(err, ports.ErrNoState) {
		t.Fatalf("无基准状态投影时必须以 %v 拒绝提交，得到 err=%v", ports.ErrNoState, err)
	}
	// 失败提交不得推进分支头，也不得落节点。
	b, err := s.GetBranch(branch.BranchID)
	if err != nil {
		t.Fatalf("get branch: %v", err)
	}
	if b.HeadNodeID != "root" || b.Version != 0 {
		t.Fatalf("失败提交不得推进分支头: %+v", b)
	}
	if _, err := s.GetNode("node1"); err == nil {
		t.Fatal("失败提交不得写入结果节点")
	}
}
