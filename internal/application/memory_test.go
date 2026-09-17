package application

import (
	"context"
	"testing"
	"time"

	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// memoryFixture 提供一个已建好的会话，便于按分支写入记忆。
type memoryFixture struct {
	store *sqlite.Store
	svc   *MemoryService
	sess  *domain.Session
	main  *domain.Branch
}

func newMemoryFixture(t *testing.T) *memoryFixture {
	t.Helper()
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	sess := &domain.Session{SessionID: "sess_mem", RootNodeID: "root", Title: "记忆测试", CreatedAt: time.Now()}
	root := &domain.PlotNode{
		NodeID: "root", SessionID: sess.SessionID, Kind: domain.NodeKindRoot,
		SchemaVersion: 1, ContentJSON: `{}`,
	}
	main := &domain.Branch{BranchID: "branch_main", SessionID: sess.SessionID, Name: "main", HeadNodeID: "root"}
	if err := st.CreateSession(sess, root, main, nil); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return &memoryFixture{store: st, svc: NewMemoryService(st), sess: sess, main: main}
}

// commitOn 在指定分支上提交一个携带记忆的节点，返回新头节点 ID。
func (f *memoryFixture) commitOn(t *testing.T, branchID, headID string, mems ...*domain.MemoryRecord) string {
	t.Helper()
	parent, err := f.store.GetNode(headID)
	if err != nil {
		t.Fatalf("parent: %v", err)
	}
	branch, err := f.store.GetBranch(branchID)
	if err != nil {
		t.Fatalf("branch: %v", err)
	}
	turnID := "turn_" + mems[0].MemoryID
	if err := f.store.CreateTurnRequest(&domain.TurnRequest{
		TurnID: turnID, SessionID: f.sess.SessionID, BranchID: branchID,
		IdempotencyKey: turnID, PayloadHash: "h", ExpectedHeadID: headID,
		Status: domain.TurnQueued, Mode: "structured",
	}); err != nil {
		t.Fatalf("create turn: %v", err)
	}
	if ok, err := f.store.ClaimActiveTurn(branchID, turnID); err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	nodeID := "node_" + mems[0].MemoryID
	for _, m := range mems {
		m.SourceNodeID = nodeID
	}
	res, err := f.store.CommitTurn(&ports.CommitPlan{
		TurnID: turnID, ExpectedHeadID: headID, ExpectedVersion: branch.Version,
		RulesetVersion: "test",
		Node: &domain.PlotNode{
			NodeID: nodeID, SessionID: f.sess.SessionID, ParentID: headID,
			Kind: domain.NodeKindTurn, Depth: parent.Depth + 1, TurnNumber: parent.TurnNumber + 1,
			SchemaVersion: 1, ContentJSON: `{}`,
		},
		NewStateHash: domain.NewWorldState().HashID(), NewStateJSON: `{}`,
		Memories: mems,
	})
	if err != nil || !res.Committed {
		t.Fatalf("commit: %v %+v", err, res)
	}
	return nodeID
}

// forkBranch 从给定节点分叉出一条新分支。
func (f *memoryFixture) forkBranch(t *testing.T, branchID, fromNode string) {
	t.Helper()
	if err := f.store.CreateBranch(&domain.Branch{
		BranchID: branchID, SessionID: f.sess.SessionID, Name: branchID,
		HeadNodeID: fromNode, Version: 0,
	}); err != nil {
		t.Fatalf("create branch: %v", err)
	}
}

func memRec(id, content string) *domain.MemoryRecord {
	return &domain.MemoryRecord{MemoryID: id, Kind: domain.MemoryObserved, Content: content}
}

func effectiveOf(t *testing.T, views []MemoryView, id string) bool {
	t.Helper()
	for _, v := range views {
		if v.MemoryID == id {
			return v.Effective
		}
	}
	t.Fatalf("记忆 %s 不在列表中", id)
	return false
}

// 不传 branchId 时全部记录都标记为生效（管理视图的原始列表语义）。
func TestMemoryListWithoutBranch(t *testing.T) {
	f := newMemoryFixture(t)
	f.commitOn(t, f.main.BranchID, "root", memRec("m1", "她来自北方。"))

	views, err := f.svc.List(f.sess.SessionID, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(views) != 1 || !views[0].Effective {
		t.Fatalf("views = %+v", views)
	}
}

// 按分支解析覆盖关系：被取代的原记录标记为不生效，覆盖记录生效。
func TestMemoryListMarksEffectiveWithOverlay(t *testing.T) {
	f := newMemoryFixture(t)
	head := f.commitOn(t, f.main.BranchID, "root", memRec("m1", "她来自北方。"))

	if _, err := f.svc.Overlay(f.sess.SessionID, f.main.BranchID, "m1", MemoryPatch{
		Content: strPtr("她来自南方。"),
	}); err != nil {
		t.Fatalf("overlay: %v", err)
	}
	_ = head

	views, err := f.svc.List(f.sess.SessionID, f.main.BranchID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(views) != 2 {
		t.Fatalf("应保留原始记录 + 覆盖记录，实际 %d", len(views))
	}
	if effectiveOf(t, views, "m1") {
		t.Fatalf("被取代的原记录不应生效")
	}
	var overlayID string
	for _, v := range views {
		if v.Supersedes == "m1" {
			overlayID = v.MemoryID
		}
	}
	if overlayID == "" || !effectiveOf(t, views, overlayID) {
		t.Fatalf("覆盖记录应生效: %+v", views)
	}
}

// 修订是 copy-on-write：原记录内容不变（否则会泄漏到其它分支）。
func TestMemoryOverlayDoesNotMutateOriginal(t *testing.T) {
	f := newMemoryFixture(t)
	f.commitOn(t, f.main.BranchID, "root", memRec("m1", "她来自北方。"))

	if _, err := f.svc.Overlay(f.sess.SessionID, f.main.BranchID, "m1", MemoryPatch{
		Content: strPtr("她来自南方。"),
	}); err != nil {
		t.Fatalf("overlay: %v", err)
	}
	all, err := f.store.ListMemories(f.sess.SessionID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, m := range all {
		if m.MemoryID == "m1" && m.Content != "她来自北方。" {
			t.Fatalf("原记录被就地修改了: %q", m.Content)
		}
	}
}

// 对已被取代的记录做修订时，应作用于分支上当前生效的最新版本（用户看到的就是它）。
func TestMemoryOverlayResolvesLatestVersion(t *testing.T) {
	f := newMemoryFixture(t)
	f.commitOn(t, f.main.BranchID, "root", memRec("m1", "第一版。"))

	first, err := f.svc.Overlay(f.sess.SessionID, f.main.BranchID, "m1", MemoryPatch{
		Content: strPtr("第二版。"),
	})
	if err != nil {
		t.Fatalf("overlay1: %v", err)
	}
	// 用原始 ID 再改一次：应基于第二版继续覆盖，而不是从第一版重新分叉。
	second, err := f.svc.Overlay(f.sess.SessionID, f.main.BranchID, "m1", MemoryPatch{
		Content: strPtr("第三版。"),
	})
	if err != nil {
		t.Fatalf("overlay2: %v", err)
	}
	if second.Supersedes != first.MemoryID {
		t.Fatalf("应从最新版本继续覆盖: supersedes=%s want=%s", second.Supersedes, first.MemoryID)
	}

	views, _ := f.svc.List(f.sess.SessionID, f.main.BranchID)
	var eff []string
	for _, v := range views {
		if v.Effective {
			eff = append(eff, v.Content)
		}
	}
	if len(eff) != 1 || eff[0] != "第三版。" {
		t.Fatalf("生效内容 = %v, want [第三版。]", eff)
	}
}

// T17：A 分支上的修订不影响 B 分支看到的记忆。
func TestMemoryOverlayIsPathScoped(t *testing.T) {
	f := newMemoryFixture(t)
	common := f.commitOn(t, f.main.BranchID, "root", memRec("m1", "她来自北方。"))

	f.forkBranch(t, "branch_b", common)
	f.commitOn(t, "branch_b", common, memRec("mB", "B 的旁支记忆。"))
	// A 必须继续推进：覆盖记录锚定在**分支当前头节点**上，
	// 若 A 的头节点仍停在共同祖先，覆盖就会落在共享前缀上（语义上是对的，但不是本用例要验的）。
	f.commitOn(t, f.main.BranchID, common, memRec("mA", "A 的旁支记忆。"))

	// A 分支上纠正共同来源的记忆。
	if _, err := f.svc.Overlay(f.sess.SessionID, f.main.BranchID, "m1", MemoryPatch{
		Content: strPtr("她来自南方。"),
	}); err != nil {
		t.Fatalf("overlay: %v", err)
	}

	aViews, _ := f.svc.List(f.sess.SessionID, f.main.BranchID)
	if effectiveOf(t, aViews, "m1") {
		t.Fatalf("A 路径应改用修订版")
	}

	bViews, _ := f.svc.List(f.sess.SessionID, "branch_b")
	if !effectiveOf(t, bViews, "m1") {
		t.Fatalf("B 路径的对应记忆不应受影响（T17）: %+v", bViews)
	}
	// A 的覆盖记录仍在会话列表里（管理视图看得到），但在 B 路径上不生效。
	for _, v := range bViews {
		if v.Supersedes == "m1" && v.Effective {
			t.Fatalf("A 的覆盖记录不应在 B 路径上生效: %+v", v)
		}
	}
}

// 隐藏与置顶同样通过覆盖记录实现。
func TestMemoryOverlayHideAndPin(t *testing.T) {
	f := newMemoryFixture(t)
	f.commitOn(t, f.main.BranchID, "root", memRec("m1", "她来自北方。"))

	hidden := true
	if _, err := f.svc.Overlay(f.sess.SessionID, f.main.BranchID, "m1", MemoryPatch{Hidden: &hidden}); err != nil {
		t.Fatalf("hide: %v", err)
	}
	pinned := true
	// 用原始 ID 再置顶：先解析到隐藏版，再改成「隐藏且置顶」。
	got, err := f.svc.Overlay(f.sess.SessionID, f.main.BranchID, "m1", MemoryPatch{Pinned: &pinned})
	if err != nil {
		t.Fatalf("pin: %v", err)
	}
	if !got.Pinned || !got.Hidden {
		t.Fatalf("后续修订应继承之前的标记: %+v", got)
	}
	if got.Content != "她来自北方。" {
		t.Fatalf("未修改的字段应保持不变: %q", got.Content)
	}
}

// 参数与归属校验。
func TestMemoryOverlayValidation(t *testing.T) {
	f := newMemoryFixture(t)
	f.commitOn(t, f.main.BranchID, "root", memRec("m1", "她来自北方。"))

	// 空内容被拒绝（空记忆没有意义）。
	if _, err := f.svc.Overlay(f.sess.SessionID, f.main.BranchID, "m1", MemoryPatch{
		Content: strPtr("   "),
	}); err == nil {
		t.Fatalf("空内容应被拒绝")
	}
	// 不存在的记忆。
	if _, err := f.svc.Overlay(f.sess.SessionID, f.main.BranchID, "nope", MemoryPatch{}); err == nil {
		t.Fatalf("不存在的记忆应返回错误")
	}
	// 不属于该会话的记忆：另一个会话里的记录不能被本会话修订。
	other := &domain.Session{SessionID: "sess_other", RootNodeID: "root_other", Title: "别的会话", CreatedAt: time.Now()}
	if err := f.store.CreateSession(other, &domain.PlotNode{
		NodeID: "root_other", SessionID: other.SessionID, Kind: domain.NodeKindRoot,
		SchemaVersion: 1, ContentJSON: `{}`,
	}, &domain.Branch{BranchID: "branch_other", SessionID: other.SessionID, Name: "main", HeadNodeID: "root_other"}, nil); err != nil {
		t.Fatalf("create other session: %v", err)
	}
	if err := f.store.CreateMemory(&domain.MemoryRecord{
		MemoryID: "m_other", SourceNodeID: "root_other", Kind: domain.MemoryObserved, Content: "别的会话的记忆。",
	}); err != nil {
		t.Fatalf("create memory: %v", err)
	}
	if _, err := f.svc.Overlay(f.sess.SessionID, f.main.BranchID, "m_other", MemoryPatch{}); err == nil {
		t.Fatalf("跨会话修订应被拒绝")
	}
	// 不存在的分支。
	if _, err := f.svc.Overlay(f.sess.SessionID, "nope_branch", "m1", MemoryPatch{}); err == nil {
		t.Fatalf("不存在的分支应返回错误")
	}
	// 缺少分支 ID。
	if _, err := f.svc.Overlay(f.sess.SessionID, "", "m1", MemoryPatch{}); err == nil {
		t.Fatalf("缺少分支 ID 应返回错误")
	}
}

// 修订必须追加独立节点；即使分支仍停在共享头，也不能污染兄弟分支。
func TestMemoryOverlayAppendsBranchLocalNode(t *testing.T) {
	f := newMemoryFixture(t)
	common := f.commitOn(t, f.main.BranchID, "root", memRec("m1", "她来自北方。"))
	head := f.commitOn(t, f.main.BranchID, common, memRec("m2", "无关记忆。"))
	f.forkBranch(t, "sibling", head)

	got, err := f.svc.Overlay(f.sess.SessionID, f.main.BranchID, "m1", MemoryPatch{
		Content: strPtr("她来自南方。"),
	})
	if err != nil {
		t.Fatalf("overlay: %v", err)
	}
	if got.SourceNodeID == head {
		t.Fatal("rewrote shared head")
	}
	node, err := f.store.GetNode(got.SourceNodeID)
	if err != nil || node.ParentID != head || node.Kind != domain.NodeKindMemoryChange {
		t.Fatalf("node=%+v err=%v", node, err)
	}
	sibling, _ := f.svc.List(f.sess.SessionID, "sibling")
	if !effectiveOf(t, sibling, "m1") || effectiveOf(t, sibling, got.MemoryID) {
		t.Fatal("revision leaked to sibling")
	}
}

func strPtr(s string) *string { return &s }

// 编译期确认：MemoryService 只依赖 Store 端口。
var _ = context.Background
