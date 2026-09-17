package sqlite

import (
	"database/sql"
	"encoding/json"
	"testing"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"tavernagent/internal/util/id"
)

// commitNode 在指定分支上提交一个带正文与记忆的回合，返回新节点 ID。
func commitNode(t *testing.T, s *Store, sess *domain.Session, branchID, head, turnID, inputText string, memories []*domain.MemoryRecord, mentioned []string) string {
	t.Helper()
	parent, err := s.GetNode(head)
	if err != nil {
		t.Fatalf("parent %s: %v", head, err)
	}
	branch, err := s.GetBranch(branchID)
	if err != nil {
		t.Fatalf("branch %s: %v", branchID, err)
	}
	bs, err := s.StateAt(head)
	if err != nil {
		t.Fatalf("state at %s: %v", head, err)
	}
	state, err := domain.UnmarshalWorld(bs.StateJSON)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	nodeID := id.New()
	content, _ := json.Marshal(domain.TurnContent{
		InputText: inputText,
		Blocks:    []domain.TextBlock{{Kind: "narration", Text: "（正文）"}},
	})
	for _, m := range memories {
		if m.MemoryID == "" {
			m.MemoryID = id.New()
		}
		m.SourceNodeID = nodeID
	}
	seedTurn(t, s, sess.SessionID, branch.BranchID, turnID, head, branch.Version)
	plan := &ports.CommitPlan{
		TurnID: turnID, ExpectedHeadID: head, ExpectedVersion: branch.Version,
		BaseStateHash: state.HashID(),
		Node: &domain.PlotNode{
			NodeID: nodeID, SessionID: sess.SessionID, ParentID: head, Kind: domain.NodeKindTurn,
			Depth: parent.Depth + 1, TurnNumber: parent.TurnNumber + 1,
			SchemaVersion: 1, ContentJSON: string(content),
		},
		Memories:           memories,
		MentionedMemoryIDs: mentioned,
		NewStateHash:       state.HashID(), NewStateJSON: MarshalState(state),
	}
	res, err := s.CommitTurn(plan)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if !res.Committed {
		t.Fatalf("commit not applied: %+v", res)
	}
	return nodeID
}

// lastMentionsOnPath 直接读 memory_mentions，断言"库里到底存了什么"。
// 走 SQL 而不是复用检索路径：提及写入的正确性不该由读取方的实现来证明。
func lastMentionsOnPath(t *testing.T, s *Store, nodeID string, ids []string) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, id := range ids {
		var turn sql.NullInt64
		err := s.db.QueryRow(`
WITH RECURSIVE up(node_key) AS (
  SELECT node_id FROM plot_nodes WHERE node_id = ?
  UNION ALL
  SELECT p.parent_id FROM plot_nodes p JOIN up u ON p.node_id = u.node_key WHERE p.parent_id IS NOT NULL
)
SELECT MAX(mm.turn_number) FROM memory_mentions mm JOIN up u ON u.node_key = mm.node_id
 WHERE mm.memory_id = ?`, nodeID, id).Scan(&turn)
		if err != nil {
			t.Fatalf("query mentions: %v", err)
		}
		if turn.Valid && turn.Int64 > 0 {
			out[id] = int(turn.Int64)
		}
	}
	return out
}

func newMemory(id, content string, entities ...string) *domain.MemoryRecord {
	return &domain.MemoryRecord{MemoryID: id, Kind: domain.MemoryObserved, Content: content, EntityIDs: entities}
}

// 词法索引可用时，召回必须给出排名；不能悄悄走兜底路径。
func TestRecallUsesLexicalIndex(t *testing.T) {
	s := newTestStore(t)
	if !s.LexicalIndexAvailable() {
		t.Fatalf("FTS5 索引不可用：新检索路径会静默退回兜底")
	}
	sess, root, _ := seedSession(t, s)
	seedRootState(t, s, root.NodeID)

	head := commitNode(t, s, sess, "branch_main", root.NodeID, "turn1", "开场",
		[]*domain.MemoryRecord{
			newMemory("m1", "铜钥匙一直挂在门后的钉子上。"),
			newMemory("m2", "石板路在雨后湿滑，走夜路要提防。"),
			newMemory("m3", "她习惯在清晨磨刀，声音很有规律。"),
		}, nil)

	cands, err := s.RecallMemories(head, ports.MemoryRecallQuery{MatchExpr: `"钥匙"`})
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if len(cands) != 1 {
		t.Fatalf("候选数 = %d, want 1（只有铜钥匙那条命中）", len(cands))
	}
	c := cands[0]
	if c.Memory.MemoryID != "m1" || !c.HasRank {
		t.Fatalf("候选 = %s hasRank=%v", c.Memory.MemoryID, c.HasRank)
	}
	if c.SourceTurn != 1 {
		t.Fatalf("来源回合 = %d, want 1", c.SourceTurn)
	}
}

// SQL resolves overlays before ranking: hidden and irrelevant corrected records
// must not exhaust a bounded pool. Pinned records remain explicit candidates.
func TestRecallPoolFiltersHiddenAndUnrelatedCover(t *testing.T) {
	s := newTestStore(t)
	sess, root, _ := seedSession(t, s)
	seedRootState(t, s, root.NodeID)

	pinned := newMemory("m_pin", "她左手缺了一根小指。")
	pinned.Pinned = true
	hidden := newMemory("m_hid", "这条被隐藏了。")
	hidden.Hidden = true
	cover := newMemory("m_cov", "她其实来自南方。")
	cover.Supersedes = "m_old"

	head := commitNode(t, s, sess, "branch_main", root.NodeID, "turn1", "无关输入",
		[]*domain.MemoryRecord{pinned, hidden, cover}, nil)

	cands, err := s.RecallMemories(head, ports.MemoryRecallQuery{MatchExpr: `"完全不相干"`})
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	got := map[string]bool{}
	for _, c := range cands {
		got[c.Memory.MemoryID] = true
	}
	if len(got) != 1 || !got["m_pin"] {
		t.Fatalf("unexpected candidates: %v", got)
	}
}

// 记忆正文不写名字时，实体显示名仍要能命中——否则按角色名的查询会
// 整条漏掉最重要的关系性认知。
func TestRecallIndexesEntityDisplayName(t *testing.T) {
	s := newTestStore(t)
	sess, root, _ := seedSession(t, s)
	seedRootState(t, s, root.NodeID)

	// 基准状态取库里的真实快照哈希；新状态是在它之上加入角色信息。
	bs, err := s.StateAt(root.NodeID)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	base, err := domain.UnmarshalWorld(bs.StateJSON)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	state := base.Clone()
	state.Characters["npc_bell"] = domain.CharacterInfo{CharacterID: "npc_bell", Name: "看护人"}
	stateJSON := MarshalState(state)

	parent, _ := s.GetNode(root.NodeID)
	nodeID := id.New()
	content, _ := json.Marshal(domain.TurnContent{InputText: "她在灯下坐着。"})
	seedTurn(t, s, sess.SessionID, "branch_main", "turn1", root.NodeID, 0)
	res, err := s.CommitTurn(&ports.CommitPlan{
		TurnID: "turn1", ExpectedHeadID: root.NodeID, ExpectedVersion: 0,
		BaseStateHash: base.HashID(),
		Node: &domain.PlotNode{
			NodeID: nodeID, SessionID: sess.SessionID, ParentID: root.NodeID, Kind: domain.NodeKindTurn,
			Depth: parent.Depth + 1, TurnNumber: 1, SchemaVersion: 1, ContentJSON: string(content),
		},
		Memories: []*domain.MemoryRecord{
			// SourceNodeID 指向本次提交的节点：记忆必须可回溯到来源（否则外键挡住）。
			func() *domain.MemoryRecord {
				m := newMemory("m1", "她说自己很少离开这里。", "npc_bell")
				m.SourceNodeID = nodeID
				return m
			}(),
		},
		NewStateHash: state.HashID(), NewStateJSON: stateJSON,
	})
	if err != nil || !res.Committed {
		t.Fatalf("commit: %v %+v", err, res)
	}

	// 正文里没有「看护人」，只有显示名来自索引侧的实体解析。
	cands, err := s.RecallMemories(nodeID, ports.MemoryRecallQuery{MatchExpr: `"看护" OR "护人"`})
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if len(cands) != 1 || cands[0].Memory.MemoryID != "m1" {
		t.Fatalf("实体显示名未进入索引，候选 = %+v", cands)
	}
}

// 提及只在「提交正文确实提到」时落库：注入本身不算。
func TestCommitWritesMentionsOnlyWithEvidence(t *testing.T) {
	s := newTestStore(t)
	sess, root, _ := seedSession(t, s)
	seedRootState(t, s, root.NodeID)

	mentioned := newMemory("m_yes", "她来自北方，很怕冷。")
	silent := newMemory("m_no", "她左手缺了一根小指。")
	head := commitNode(t, s, sess, "branch_main", root.NodeID, "turn1", "她来自北方，很怕冷吗？",
		[]*domain.MemoryRecord{mentioned, silent}, []string{"m_yes", "m_no"})

	got := lastMentionsOnPath(t, s, head, []string{"m_yes", "m_no"})
	if got["m_yes"] != 1 {
		t.Fatalf("正文提到的记忆应记提及，得到 %v", got)
	}
	if _, ok := got["m_no"]; ok {
		t.Fatalf("正文没提到却被记了提及：%v", got)
	}
}

// T16/T17：提及是**路径**属性——A 分支的提及不能抬高 B 分支的 recency。
func TestMentionsArePathScoped(t *testing.T) {
	s := newTestStore(t)
	sess, root, _ := seedSession(t, s)
	seedRootState(t, s, root.NodeID)

	m := newMemory("m1", "她来自北方，很怕冷。")
	aHead := commitNode(t, s, sess, "branch_main", root.NodeID, "turnA", "她来自北方，很怕冷吗？",
		[]*domain.MemoryRecord{m}, []string{"m1"})

	if err := s.CreateBranch(&domain.Branch{
		BranchID: "branch_b", SessionID: sess.SessionID, Name: "b", HeadNodeID: root.NodeID, Version: 0,
	}); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	bHead := commitNode(t, s, sess, "branch_b", root.NodeID, "turnB", "无关输入", nil, nil)

	aGot := lastMentionsOnPath(t, s, aHead, []string{"m1"})
	if aGot["m1"] != 1 {
		t.Fatalf("A 分支应看到提及：%v", aGot)
	}
	bGot := lastMentionsOnPath(t, s, bHead, []string{"m1"})
	if _, ok := bGot["m1"]; ok {
		t.Fatalf("B 分支看到了 A 分支的提及（分支隔离泄漏）：%v", bGot)
	}
}

// 索引回填：模拟迁移后索引为空但记忆已存在的情况，必须能重建。
func TestMemoryIndexBackfill(t *testing.T) {
	s := newTestStore(t)
	sess, root, _ := seedSession(t, s)
	seedRootState(t, s, root.NodeID)
	head := commitNode(t, s, sess, "branch_main", root.NodeID, "turn1", "开场",
		[]*domain.MemoryRecord{newMemory("m1", "铜钥匙一直挂在门后的钉子上。")}, nil)

	// 清空索引，等价于"索引表刚建好、记忆还在"的迁移场景。
	if _, err := s.db.Exec(`DELETE FROM ` + memoryFTSTable); err != nil {
		t.Fatalf("clear index: %v", err)
	}
	if err := s.ensureMemoryIndex(); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	cands, err := s.RecallMemories(head, ports.MemoryRecallQuery{MatchExpr: `"钥匙"`})
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if len(cands) != 1 || cands[0].Memory.MemoryID != "m1" {
		t.Fatalf("回填后仍未召回，候选 = %+v", cands)
	}
}

// 更新记忆后索引必须跟着变：否则检索会命中已经不存在的内容。
func TestUpdateMemoryReindexes(t *testing.T) {
	s := newTestStore(t)
	sess, root, _ := seedSession(t, s)
	seedRootState(t, s, root.NodeID)
	head := commitNode(t, s, sess, "branch_main", root.NodeID, "turn1", "开场",
		[]*domain.MemoryRecord{newMemory("m1", "铜钥匙一直挂在门后的钉子上。")}, nil)

	m := newMemory("m1", "原来那把铜钥匙早就丢了。")
	if err := s.UpdateMemory(m); err != nil {
		t.Fatalf("update: %v", err)
	}
	// 旧内容里的独有词元必须失效。
	stale, err := s.RecallMemories(head, ports.MemoryRecallQuery{MatchExpr: `"钉子"`})
	if err != nil {
		t.Fatalf("recall stale: %v", err)
	}
	if len(stale) != 0 {
		t.Fatalf("旧内容的词元仍留在索引里：%+v", stale)
	}
	fresh, err := s.RecallMemories(head, ports.MemoryRecallQuery{MatchExpr: `"那把" OR "把铜"`})
	if err != nil {
		t.Fatalf("recall fresh: %v", err)
	}
	if len(fresh) != 1 {
		t.Fatalf("更新后的内容未进索引：%+v", fresh)
	}
}

// 重要度的默认值与区间收敛。
func TestImportancePersistedAndClamped(t *testing.T) {
	s := newTestStore(t)
	sess, root, _ := seedSession(t, s)
	seedRootState(t, s, root.NodeID)

	m := newMemory("m1", "她来自北方。")
	m.Importance = 9
	commitNode(t, s, sess, "branch_main", root.NodeID, "turn1", "开场",
		[]*domain.MemoryRecord{m}, nil)

	mems, err := s.ListMemories(sess.SessionID)
	if err != nil || len(mems) != 1 {
		t.Fatalf("list: %d err=%v", len(mems), err)
	}
	if mems[0].Importance != 9 {
		t.Fatalf("重要度未持久化：%d", mems[0].Importance)
	}
	if domain.ClampMemoryImportance(0) != domain.DefaultMemoryImportance {
		t.Fatalf("0 应收敛到默认值")
	}
	if domain.ClampMemoryImportance(99) != domain.MaxMemoryImportance {
		t.Fatalf("超上限应收敛到最大值")
	}
}

// 索引行与记忆行必须 rowid 对齐。
//
// 这不是洁癖：召回查询要「先按 BM25 取前 N，再回表验可见性」，两个方向的连接
// 都走 rowid；错位不会报任何错，只会让 memory_id（FTS 里的 UNINDEXED 列）
// 变成连接键，把整段从 35ms 推到 120ms 以上——一条"性能静默劣化"的不变量。
func TestMemoryIndexRowidsAligned(t *testing.T) {
	s := newTestStore(t)
	sess, root, _ := seedSession(t, s)
	seedRootState(t, s, root.NodeID)
	commitNode(t, s, sess, "branch_main", root.NodeID, "turn1", "开场",
		[]*domain.MemoryRecord{
			newMemory("m1", "铜钥匙一直挂在门后的钉子上。"),
			newMemory("m2", "她习惯在清晨磨刀。"),
		}, nil)

	var indexed, aligned int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + memoryFTSTable).Scan(&indexed); err != nil {
		t.Fatalf("count index: %v", err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + memoryFTSTable +
		` f JOIN memory_records r ON r.rowid = f.rowid`).Scan(&aligned); err != nil {
		t.Fatalf("count aligned: %v", err)
	}
	if indexed == 0 || aligned != indexed {
		t.Fatalf("索引行未与记忆行对齐：索引 %d 行，对齐 %d 行", indexed, aligned)
	}
}

// 错位的索引必须被启动检查发现并重建（否则性能静默劣化且无人察觉）。
func TestMemoryIndexRebuildsWhenMisaligned(t *testing.T) {
	s := newTestStore(t)
	sess, root, _ := seedSession(t, s)
	seedRootState(t, s, root.NodeID)
	head := commitNode(t, s, sess, "branch_main", root.NodeID, "turn1", "开场",
		[]*domain.MemoryRecord{newMemory("m1", "铜钥匙一直挂在门后的钉子上。")}, nil)

	// 人为制造错位：把索引行整体平移一个偏移量（等价于旧布局）。
	if _, err := s.db.Exec(`DELETE FROM ` + memoryFTSTable); err != nil {
		t.Fatalf("clear: %v", err)
	}
	rows, err := s.db.Query(`SELECT rowid, content FROM memory_records`)
	if err != nil {
		t.Fatalf("query memories: %v", err)
	}
	type mis struct {
		rowid   int64
		content string
	}
	var misaligned []mis
	for rows.Next() {
		var m mis
		if err := rows.Scan(&m.rowid, &m.content); err != nil {
			rows.Close()
			t.Fatalf("scan: %v", err)
		}
		misaligned = append(misaligned, m)
	}
	rows.Close()
	for _, m := range misaligned {
		tokens := memoryIndexTokens(&domain.MemoryRecord{Content: m.content}, nil)
		if _, err := s.db.Exec(`INSERT INTO `+memoryFTSTable+`(rowid, memory_id, tokens) VALUES(?,?,?)`,
			m.rowid+100000, "m1", tokens); err != nil {
			t.Fatalf("insert misaligned: %v", err)
		}
	}

	if err := s.ensureMemoryIndex(); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	var aligned int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + memoryFTSTable +
		` f JOIN memory_records r ON r.rowid = f.rowid`).Scan(&aligned); err != nil {
		t.Fatalf("count aligned: %v", err)
	}
	if aligned != len(misaligned) {
		t.Fatalf("错位未被重建：对齐 %d 行，应为 %d 行", aligned, len(misaligned))
	}
	cands, err := s.RecallMemories(head, ports.MemoryRecallQuery{MatchExpr: `"钥匙"`})
	if err != nil || len(cands) != 1 {
		t.Fatalf("重建后召回异常：%+v err=%v", cands, err)
	}
}

// TestHiddenMemoriesExcludedFromCandidatePool 固化候选池排除隐藏记录的契约（T2.3）。
// 隐藏记录（hidden=1）不论重要度或是否匹配，绝不进入候选池。
func TestHiddenMemoriesExcludedFromCandidatePool(t *testing.T) {
	s := newTestStore(t)
	sess, root, _ := seedSession(t, s)
	seedRootState(t, s, root.NodeID)

	normal := newMemory("m_norm", "钥匙在桌子上。")
	hidden := newMemory("m_hid", "钥匙在保险柜里。")
	hidden.Hidden = true
	hidden.Importance = 10

	head := commitNode(t, s, sess, "branch_main", root.NodeID, "turn1", "测试隐藏",
		[]*domain.MemoryRecord{normal, hidden}, nil)

	// 无论是词法匹配还是保底路，隐藏记忆都绝对不能入池
	cands, err := s.RecallMemories(head, ports.MemoryRecallQuery{MatchExpr: `"钥匙"`, FallbackLimit: 10})
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	for _, c := range cands {
		if c.Memory.MemoryID == "m_hid" || c.Memory.Hidden {
			t.Fatalf("隐藏记忆出现在候选池中: %+v", c.Memory)
		}
	}
}

// TestImportanceFallbackPresentInCandidatePool 验证无词法匹配时重要性保底路生效（T2.2）。
func TestImportanceFallbackPresentInCandidatePool(t *testing.T) {
	s := newTestStore(t)
	sess, root, _ := seedSession(t, s)
	seedRootState(t, s, root.NodeID)

	mLow := newMemory("m_low", "普通的日常流水账。")
	mLow.Importance = 1
	mHigh := newMemory("m_high", "至关重要的古老秘密没有任何共同词元。")
	mHigh.Importance = 9

	head := commitNode(t, s, sess, "branch_main", root.NodeID, "turn1", "测试保底",
		[]*domain.MemoryRecord{mLow, mHigh}, nil)

	// 不开保底路时，无词法匹配则候选池为空
	candsNoFallback, err := s.RecallMemories(head, ports.MemoryRecallQuery{MatchExpr: `"完全不匹配的词元"`})
	if err != nil {
		t.Fatalf("recall without fallback: %v", err)
	}
	if len(candsNoFallback) != 0 {
		t.Fatalf("未开启保底时候选池应为空，得到 %d 条", len(candsNoFallback))
	}

	// 开启保底路时，高重要度的记忆成功进入候选池
	candsWithFallback, err := s.RecallMemories(head, ports.MemoryRecallQuery{
		MatchExpr:     `"完全不匹配的词元"`,
		FallbackLimit: 5,
	})
	if err != nil {
		t.Fatalf("recall with fallback: %v", err)
	}
	foundHigh := false
	for _, c := range candsWithFallback {
		if c.Memory.MemoryID == "m_high" {
			foundHigh = true
			if c.HasRank {
				t.Fatalf("保底候选不应具有词法 rank: %+v", c)
			}
		}
	}
	if !foundHigh {
		t.Fatalf("保底路未将高重要度记忆引入候选池: %+v", candsWithFallback)
	}
}
