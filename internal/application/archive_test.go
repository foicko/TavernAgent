package application

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"tavernagent/internal/adapters/providers/mock"
	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/domain"
	"tavernagent/internal/pack"
)

// packScript 返回一轮：正文 + 关系增量 + 一个选项。
// 往返测试需要选项，才能在导入后验证 optionRef.nodeId 被重映射（否则
// 用旧节点 ID 提交会因为引用不存在而失败）。
func packScript() []mock.Item {
	return []mock.Item{
		mock.Frame(mockBlock(1, "narration", "她的神情缓和了一些。")),
		mock.Frame(mockFinal(2,
			`{"proposalId":"p1","type":"relationship_delta","characterId":"npc_elena","field":"affection","delta":2}`,
			`{"optionId":"o1","intent":"clever","text":"我走过去。"}`)),
	}
}

// ---- F6：剧情包往返与安全（T21 / T22 / T28）----

// chainShape 返回某节点祖先链的形状序列（kind/depth/turnNumber）。
// 用它比较"拓扑等价"：ID 会被重映射，形状不会。
func chainShape(t *testing.T, st *sqlite.Store, nodeID string) []string {
	t.Helper()
	chain, err := st.AncestorChain(nodeID, true)
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	out := make([]string, 0, len(chain))
	for _, n := range chain {
		out = append(out, string(n.Kind)+":"+strconv.Itoa(n.Depth)+":"+strconv.Itoa(n.TurnNumber))
	}
	return out
}

func exportPack(t *testing.T, svc *ArchiveService, sessionID, branchID string) *ExportResult {
	t.Helper()
	res, err := svc.Export(context.Background(), sessionID, branchID)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(res.Data) == 0 {
		t.Fatalf("导出结果为空")
	}
	return res
}

// T21：导出再导入后，语义状态与拓扑等价，且重映射 ID 后仍可继续。
func TestRoundTripEquivalence(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, packScript())
	svc := NewArchiveService(st, "test")

	acceptAndWait(t, turnSvc, st, sessionID, branchID, "rt-1", "你好。")
	second := acceptAndWait(t, turnSvc, st, sessionID, branchID, "rt-2", "继续。")

	exported := exportPack(t, svc, sessionID, "")
	if exported.Manifest.Counts["nodes"] == 0 {
		t.Fatalf("清单统计异常: %+v", exported.Manifest.Counts)
	}

	imp, err := svc.Import(context.Background(), exported.Data)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	newSessionID := imp.Session.SessionID
	if newSessionID == sessionID {
		t.Fatalf("导入必须分配新的会话 ID")
	}

	// 拓扑等价：同一深度的形状序列一致。
	origShape := chainShape(t, st, second.ResultNodeID)
	newBranch, err := st.GetBranch(firstBranchOf(t, st, newSessionID))
	if err != nil {
		t.Fatalf("new branch: %v", err)
	}
	newShape := chainShape(t, st, newBranch.HeadNodeID)
	if strings.Join(origShape, "|") != strings.Join(newShape, "|") {
		t.Fatalf("拓扑不等价\n orig=%v\n new =%v", origShape, newShape)
	}

	// 语义等价：状态投影的哈希一致（状态里不含节点 ID）。
	oldState, err := st.StateAt(second.ResultNodeID)
	if err != nil {
		t.Fatalf("old state: %v", err)
	}
	newState, err := st.StateAt(newBranch.HeadNodeID)
	if err != nil {
		t.Fatalf("new state: %v", err)
	}
	if oldState.StateHash != newState.StateHash {
		t.Fatalf("状态不等价\n orig=%s\n new =%s", oldState.StateJSON, newState.StateJSON)
	}

	// 记忆随包携带（含 copy-on-write 的覆盖关系）。
	oldMems, _ := st.ListMemories(sessionID)
	newMems, _ := st.ListMemories(newSessionID)
	if len(oldMems) != len(newMems) {
		t.Fatalf("记忆数量不等: %d vs %d", len(oldMems), len(newMems))
	}

	// 重映射后可继续：用**导入后**的头节点选项提交下一轮。
	// 若 optionRef.nodeId 没有被重映射，这里会因引用旧节点而失败。
	opts := optionsOf(t, st, newBranch.HeadNodeID)
	if len(opts) == 0 {
		t.Fatalf("导入后的回合应带选项")
	}
	tr, err := turnSvc.Accept(context.Background(), newSessionID, newBranch.BranchID, &TurnAcceptRequest{
		IdempotencyKey: "rt-3", ExpectedHeadID: newBranch.HeadNodeID, ExpectedVersion: newBranch.Version,
		Input: domain.TurnInput{
			Kind:      "option",
			Text:      opts[0].Text,
			OptionRef: &domain.OptionRef{NodeID: newBranch.HeadNodeID, OptionID: opts[0].OptionID},
		},
	})
	if err != nil {
		t.Fatalf("导入后继续失败（可能 ID 重映射不一致）: %v", err)
	}
	got := waitTurn(t, turnSvc, tr.TurnID, domain.TurnCommitted)
	if got.ResultNodeID == "" {
		t.Fatalf("继续后没有结果节点")
	}
}

// T21（单分支导出）：只导出一条分支时，仍包含它的全部必要祖先与模板。
func TestExportSingleBranchKeepsAncestors(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, affectionScript(2))
	svc := NewArchiveService(st, "test")

	first := acceptAndWait(t, turnSvc, st, sessionID, branchID, "sb-1", "你好。")
	second := acceptAndWait(t, turnSvc, st, sessionID, branchID, "sb-2", "继续。")

	// 从第 2 轮派生一条候选分支，让会话里有两个分支。
	derived, err := NewBranchService(st, turnSvc).DeriveTurn(context.Background(), sessionID, DeriveRequest{
		NodeID: second.ResultNodeID,
	})
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	waitTurn(t, turnSvc, derived.Turn.TurnID, domain.TurnCommitted)

	// 只导出 main 分支。
	exported := exportPack(t, svc, sessionID, branchID)
	if exported.Manifest.Scope.Full {
		t.Fatalf("单分支导出不应标记为全量")
	}
	if exported.Manifest.Scope.BranchID != branchID {
		t.Fatalf("范围分支不符: %s", exported.Manifest.Scope.BranchID)
	}

	imp, err := svc.Import(context.Background(), exported.Data)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	branches, err := st.ListBranches(imp.Session.SessionID)
	if err != nil {
		t.Fatalf("list branches: %v", err)
	}
	if len(branches) != 1 {
		t.Fatalf("单分支导出导入后应只有 1 个分支，实际 %d", len(branches))
	}
	// 祖先链完整（含根与两轮正文）。
	shape := chainShape(t, st, branches[0].HeadNodeID)
	if len(shape) != len(chainShape(t, st, second.ResultNodeID)) {
		t.Fatalf("祖先链不完整: %v", shape)
	}
	_ = first
}

// T22：结构非法的包被拒绝，且不会污染既有数据。
func TestImportRejectsInvalidBundles(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(b *domain.SessionBundle)
	}{
		{"无节点", func(b *domain.SessionBundle) { b.Nodes = nil }},
		{"两个根", func(b *domain.SessionBundle) {
			b.Nodes = append(b.Nodes, &domain.PlotNode{
				NodeID: "root_extra", SessionID: b.Session.SessionID, Kind: domain.NodeKindRoot,
				Depth: 0, SchemaVersion: 1, ContentJSON: "{}",
			})
		}},
		{"根引用不一致", func(b *domain.SessionBundle) { b.RootNodeID = "node_nope" }},
		{"悬空父引用", func(b *domain.SessionBundle) { b.Nodes[1].ParentID = "node_missing" }},
		{"成环", func(b *domain.SessionBundle) {
			b.Nodes[0].ParentID = b.Nodes[1].NodeID // 根指向子节点 → 环
		}},
		{"分支头不存在", func(b *domain.SessionBundle) { b.Branches[0].HeadNodeID = "node_nope" }},
		{"无分支", func(b *domain.SessionBundle) { b.Branches = nil }},
		{"事件指向不存在节点", func(b *domain.SessionBundle) {
			if len(b.Events) == 0 {
				b.Events = []*domain.DomainEvent{{EventID: "e1", NodeID: "nope", Type: domain.EventMemoryAdd, PayloadJSON: "{}"}}
				return
			}
			b.Events[0].NodeID = "node_nope"
		}},
		{"记忆来源不存在", func(b *domain.SessionBundle) {
			b.Memories = append(b.Memories, &domain.MemoryRecord{MemoryID: "m_x", SourceNodeID: "node_nope", Content: "x"})
		}},
		{"缺少被引用的模板", func(b *domain.SessionBundle) { b.Templates = nil }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, affectionScript(1))
			svc := NewArchiveService(st, "test")
			acceptAndWait(t, turnSvc, st, sessionID, branchID, "inv-1", "你好。")

			before, err := st.ListSessions()
			if err != nil {
				t.Fatalf("list sessions: %v", err)
			}

			exported := exportPack(t, svc, sessionID, "")
			rr, err := pack.Read(strings.NewReader(string(exported.Data)), int64(len(exported.Data)))
			if err != nil {
				t.Fatalf("read back: %v", err)
			}
			c.mutate(rr.Bundle)

			if err := validateBundle(rr.Bundle); err == nil {
				t.Fatalf("非法包应被拒绝")
			}

			after, _ := st.ListSessions()
			if len(after) != len(before) {
				t.Fatalf("校验失败不应写入数据: %d → %d", len(before), len(after))
			}
		})
	}
}

// T22：规则版本不兼容时明确拒绝（不做"导入后仍能正确运行"的假设）。
func TestImportRejectsUnsupportedRuleset(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, affectionScript(1))
	svc := NewArchiveService(st, "test")
	acceptAndWait(t, turnSvc, st, sessionID, branchID, "rs-1", "你好。")

	exported := exportPack(t, svc, sessionID, "")
	rr, err := pack.Read(strings.NewReader(string(exported.Data)), int64(len(exported.Data)))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	rr.Bundle.RulesetVersion = "ruleset.future.v9"

	err = validateBundle(rr.Bundle)
	if err == nil {
		t.Fatalf("不兼容规则版本应被拒绝")
	}
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Code != "UNSUPPORTED_RULESET" {
		t.Fatalf("错误码 = %v, want UNSUPPORTED_RULESET", err)
	}
}

// T28：剧情包内不含 API Key 之类的秘密。
func TestPackContainsNoSecrets(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, affectionScript(1))
	svc := NewArchiveService(st, "test")
	acceptAndWait(t, turnSvc, st, sessionID, branchID, "sec-1", "你好。")

	exported := exportPack(t, svc, sessionID, "")
	blob := string(exported.Data)
	for _, needle := range []string{"sk-", "apiKey", "api_key", "accessToken", "pin"} {
		if strings.Contains(blob, needle) {
			t.Fatalf("剧情包不应包含 %q", needle)
		}
	}
}

// 导出→导入→再导出：文件清单与统计保持一致。
//
// 注意不能比较文件摘要：nodes/memories/branches 里都带 ID，重映射后字节必然不同，
// 摘要相等只说明"ID 没被改"——那恰恰是错的。语义等价由状态哈希与拓扑断言保证。
func TestRoundTripIsStable(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, affectionScript(1))
	svc := NewArchiveService(st, "test")
	acceptAndWait(t, turnSvc, st, sessionID, branchID, "idem-1", "你好。")

	first := exportPack(t, svc, sessionID, "")
	imp, err := svc.Import(context.Background(), first.Data)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	second := exportPack(t, svc, imp.Session.SessionID, "")

	// 文件清单一致。
	if len(first.Manifest.Files) != len(second.Manifest.Files) {
		t.Fatalf("文件数不一致: %d vs %d", len(first.Manifest.Files), len(second.Manifest.Files))
	}
	for i := range first.Manifest.Files {
		if first.Manifest.Files[i].Path != second.Manifest.Files[i].Path {
			t.Fatalf("文件顺序不一致: %s vs %s", first.Manifest.Files[i].Path, second.Manifest.Files[i].Path)
		}
	}
	// 统计一致（数量是语义，不受 ID 重映射影响）。
	for _, k := range []string{"templates", "nodes", "events", "branches", "memories", "bookmarks", "snapshots"} {
		if first.Manifest.Counts[k] != second.Manifest.Counts[k] {
			t.Fatalf("统计 %s 不一致: %d vs %d", k, first.Manifest.Counts[k], second.Manifest.Counts[k])
		}
	}
	// ID 确实被重映射（否则导入会与既有数据冲突）。
	origNode := first.Manifest.Session.RootNodeID
	newNode := second.Manifest.Session.RootNodeID
	if origNode == newNode {
		t.Fatalf("导入后根节点 ID 应被重映射")
	}
}

// firstBranchOf 返回会话的第一个分支 ID（导出/导入的默认分支）。
func firstBranchOf(t *testing.T, st *sqlite.Store, sessionID string) string {
	t.Helper()
	branches, err := st.ListBranches(sessionID)
	if err != nil || len(branches) == 0 {
		t.Fatalf("list branches: %v", err)
	}
	return branches[0].BranchID
}

// 导出时清单里的时间与统计必须自洽。
func TestManifestSelfConsistent(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, affectionScript(1))
	svc := NewArchiveService(st, "test")
	acceptAndWait(t, turnSvc, st, sessionID, branchID, "mf-1", "你好。")

	res := exportPack(t, svc, sessionID, "")
	m := res.Manifest
	if m.Format != pack.Format || m.FormatVersion != domain.PackFormatVersion {
		t.Fatalf("格式标识异常: %+v", m)
	}
	if _, err := time.Parse(time.RFC3339, m.ExportedAt); err != nil {
		t.Fatalf("导出时间不是 RFC3339: %q", m.ExportedAt)
	}
	if m.Session.RootNodeID == "" {
		t.Fatalf("清单缺少根节点")
	}
	var total int64
	for _, f := range m.Files {
		total += f.Size
		if f.SHA256 == "" {
			t.Fatalf("文件 %s 缺少摘要", f.Path)
		}
	}
	if total == 0 {
		t.Fatalf("清单内文件总长为 0")
	}
	raw, _ := json.Marshal(m)
	if !strings.Contains(string(raw), "nodes.jsonl") {
		t.Fatalf("清单应列出节点文件")
	}
}
