package sqlite

// 精简测试后保留的共享夹具：迁移/原子性/状态哈希等安全与不可逆路径的用例依赖它。
import (
	"testing"
	"time"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"tavernagent/internal/util/id"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir, ports.RealClock{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func seedSession(t *testing.T, s *Store) (*domain.Session, *domain.PlotNode, *domain.Branch) {
	t.Helper()
	sess := &domain.Session{SessionID: id.New(), RootNodeID: "root", Title: "测试会话", CreatedAt: time.Now()}
	root := &domain.PlotNode{
		NodeID: "root", SessionID: sess.SessionID, Kind: domain.NodeKindRoot,
		ContentJSON: `{"initial":true}`, SchemaVersion: 1,
	}
	branch := &domain.Branch{BranchID: "branch_main", SessionID: sess.SessionID, Name: "main", HeadNodeID: "root", Version: 0}
	if err := s.CreateSession(sess, root, branch, nil); err != nil {
		t.Fatalf("create session: %v", err)
	}
	sess.RootNodeID = "root"
	return sess, root, branch
}

// seedTurn 在分支上准备一个生成中的回合（等价于应用层受理状态）。
func seedTurn(t *testing.T, s *Store, sessionID, branchID, turnID, head string, version int64) {
	t.Helper()
	req := &domain.TurnRequest{
		TurnID: turnID, SessionID: sessionID, BranchID: branchID, IdempotencyKey: "k-" + turnID,
		PayloadHash: "h", ExpectedHeadID: head, ExpectedVersion: version,
		Status: domain.TurnGenerating, Mode: "structured",
	}
	if err := s.CreateTurnRequest(req); err != nil {
		t.Fatalf("create turn: %v", err)
	}
	if ok, err := s.ClaimActiveTurn(branchID, turnID); err != nil || !ok {
		t.Fatalf("claim active: ok=%v err=%v", ok, err)
	}
	if err := s.CreateAttempt(&domain.TurnAttempt{AttemptID: "a-" + turnID, TurnID: turnID, AttemptNo: 1, BaseHeadID: head, BaseVersion: version}); err != nil {
		t.Fatalf("attempt: %v", err)
	}
}

func makeCommitPlan(sessionID, turnID, branchID, newHead, expectedHead string, expectedVersion int64, baseHash string, evs []*domain.DomainEvent, state *domain.WorldState) *ports.CommitPlan {
	plan := &ports.CommitPlan{
		TurnID:          turnID,
		ExpectedHeadID:  expectedHead,
		ExpectedVersion: expectedVersion,
		BaseStateHash:   baseHash,
		Node: &domain.PlotNode{
			NodeID: newHead, SessionID: sessionID, ParentID: expectedHead, Kind: domain.NodeKindTurn,
			TurnNumber: 1, SchemaVersion: 1, ContentJSON: `{"input":"hi","blocks":[]}`,
		},
		NewStateHash: HashState(state),
		NewStateJSON: MarshalState(state),
		Events:       evs,
	}
	for _, ev := range evs {
		if ev.NodeID == "" {
			ev.NodeID = plan.Node.NodeID
		}
	}
	return plan
}
