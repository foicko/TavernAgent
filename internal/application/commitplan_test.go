package application

import (
	"encoding/json"
	"testing"

	"tavernagent/internal/domain"
	"tavernagent/internal/protocol"
)

// ---- H2a：状态单一实现 + 规则版本会话化 ----

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// buildPlan 产出的新状态必须与「用同一批事件重放」得到的状态逐字节一致。
//
// 这条用例是 H2a 的核心守卫：此前提交路径自己写一遍状态转移（改 Moods、
// 调 ApplyRelationshipDelta），与 domain.ApplyEvent 构成两份必须永远一致的逻辑，
// 只有 T15 间接守着。现在提交路径改为调 ApplyEvents，这条断言把"应该一致"
// 变成"结构上只有一处实现，且结果逐字节相同"。
func TestBuildPlanStateMatchesReplay(t *testing.T) {
	base := domain.NewWorldState()
	base.Relationships["npc_x"] = domain.RelationValue{Affection: 5, Trust: 1}
	base.Characters["npc_x"] = domain.CharacterInfo{CharacterID: "npc_x", Name: "X", Participant: true}

	draft := protocol.TurnDraft{
		Blocks: []protocol.BlockFrame{{
			V: protocol.Version, Seq: 1, Type: protocol.FrameBlock,
			Kind: protocol.BlockNarration, Text: "她点了点头。",
		}},
		Proposals: []protocol.Proposal{
			{ProposalID: "p1", Type: "relationship_delta", CharacterID: "npc_x", Field: "affection", Delta: 3},
			{ProposalID: "p2", Type: "relationship_delta", CharacterID: "npc_x", Field: "affection", Delta: -1},
			{ProposalID: "p3", Type: "mood_set", CharacterID: "npc_x", MoodCode: "calm", Text: "平静"},
			{ProposalID: "p4", Type: "memory_add", Text: "她点了点头。", Confidence: "observed"},
		},
		Options: []protocol.OptionFrame{},
	}

	pd, err := buildPlan(base, draft, domain.TurnInput{Kind: "text", Text: "你好。"},
		protocol.ModeStructured, "node_test", planContext{ruleset: domain.DefaultRuleset(), RulesetVersion: "ruleset.test"})
	if err != nil {
		t.Fatalf("buildPlan: %v", err)
	}

	// 用同一批事件在基线上重放，得到"重放视角"的状态。
	replay := base.Clone()
	if _, err := domain.ApplyEvents(replay, pd.Events); err != nil {
		t.Fatalf("replay: %v", err)
	}

	if pd.NewState.HashID() != replay.HashID() {
		t.Fatalf("提交状态与重放状态不一致（两处实现已分叉）\n  buildPlan=%s\n  replay   =%s\n  buildPlan 内容=%s\n  replay 内容   =%s",
			pd.NewState.HashID(), replay.HashID(), mustJSON(pd.NewState), mustJSON(replay))
	}

	// 状态确实变了（否则上面的相等是平凡的）。
	if pd.NewState.HashID() == base.HashID() {
		t.Fatalf("状态未发生变化，用例失去意义")
	}
	// 同轮同维度的增量被聚合：+3 与 -1 合成 +2，最终 5+2=7。
	if got := pd.NewState.Relationships["npc_x"].Affection; got != 7 {
		t.Fatalf("好感 = %d, want 7（同轮聚合）", got)
	}
	// 情绪由事件承载并被应用。
	if pd.NewState.Moods["npc_x"].MoodCode != "calm" {
		t.Fatalf("情绪未应用: %+v", pd.NewState.Moods)
	}
	// 记忆事件不改变世界状态（C04：认知与状态分离）。
	if _, ok := pd.NewState.Moods["nonexistent"]; ok {
		t.Fatalf("记忆不应产生状态副作用")
	}
}

// 事件负载携带的是 clamp 之后的 applied，而状态由该事件重放得到——
// 因此越过上限的增量在快照与重放两侧必须一致。
func TestBuildPlanClampConsistency(t *testing.T) {
	base := domain.NewWorldState()
	base.Relationships["npc_x"] = domain.RelationValue{Affection: domain.AffectionMax - 1}
	base.Characters["npc_x"] = domain.CharacterInfo{CharacterID: "npc_x", Name: "X", Participant: true}

	draft := protocol.TurnDraft{
		Blocks: []protocol.BlockFrame{{
			V: protocol.Version, Seq: 1, Type: protocol.FrameBlock,
			Kind: protocol.BlockNarration, Text: "x",
		}},
		Proposals: []protocol.Proposal{
			{ProposalID: "p1", Type: "relationship_delta", CharacterID: "npc_x", Field: "affection", Delta: 50},
		},
		Options: []protocol.OptionFrame{},
	}
	pd, err := buildPlan(base, draft, domain.TurnInput{Kind: "text", Text: "x"},
		protocol.ModeStructured, "node_clamp", planContext{ruleset: domain.DefaultRuleset(), RulesetVersion: "ruleset.test"})
	if err != nil {
		t.Fatalf("buildPlan: %v", err)
	}

	replay := base.Clone()
	if _, err := domain.ApplyEvents(replay, pd.Events); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if pd.NewState.HashID() != replay.HashID() {
		t.Fatalf("clamp 场景下提交与重放不一致")
	}
	if got := pd.NewState.Relationships["npc_x"].Affection; got != domain.AffectionMax {
		t.Fatalf("好感 = %d, want 上限 %d", got, domain.AffectionMax)
	}
}

// 会话在建立时记录规则版本，回合受理时沿用它并固定。
func TestRulesetRecordedOnSessionAndTurn(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, affectionScript(1))

	sess, err := st.GetSession(sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if sess.RulesetVersion != RulesetVersion {
		t.Fatalf("会话规则版本 = %q, want %q", sess.RulesetVersion, RulesetVersion)
	}

	turn := acceptAndWait(t, turnSvc, st, sessionID, branchID, "rs-1", "你好。")
	got, err := st.GetTurn(turn.TurnID)
	if err != nil {
		t.Fatalf("get turn: %v", err)
	}
	if got.RulesetVersion != RulesetVersion {
		t.Fatalf("回合规则版本 = %q, want %q", got.RulesetVersion, RulesetVersion)
	}

	// 事件也带上同一版本（导出与重放要能判断按哪套规则结算）。
	evs, err := st.GetEvents(turn.ResultNodeID)
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	if len(evs) == 0 {
		t.Fatalf("应有事件")
	}
	for _, ev := range evs {
		if ev.RulesetVersion != RulesetVersion {
			t.Fatalf("事件规则版本 = %q, want %q", ev.RulesetVersion, RulesetVersion)
		}
	}

	// 快照同样记录规则版本。
	snap, err := st.GetSnapshot(turn.ResultNodeID)
	if err == nil && snap != nil && snap.RulesetVersion != RulesetVersion {
		t.Fatalf("快照规则版本 = %q, want %q", snap.RulesetVersion, RulesetVersion)
	}
}

// 迁移前的旧数据（未记录规则版本）回退到当前常量，而不是报错或留空。
func TestRulesetFallsBackForLegacyData(t *testing.T) {
	if got := rulesetOf(&domain.TurnRequest{}); got != RulesetVersion {
		t.Fatalf("空规则版本应回退到当前常量，实际 %q", got)
	}
	custom := &domain.TurnRequest{RulesetVersion: "ruleset.legacy.v0"}
	if got := rulesetOf(custom); got != "ruleset.legacy.v0" {
		t.Fatalf("已记录的规则版本应原样保留，实际 %q", got)
	}
}

// 会话升级规则后，旧回合重放仍按当时记录的版本，不被新值覆盖。
func TestTurnKeepsItsOwnRuleset(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, affectionScript(1))
	turn := acceptAndWait(t, turnSvc, st, sessionID, branchID, "rs-2", "你好。")

	// 通过显式入口升级会话规则（模拟未来的规则迁移）。
	if err := st.SetSessionRuleset(sessionID, "ruleset.upgraded.v2"); err != nil {
		t.Fatalf("升级会话规则失败: %v", err)
	}

	// 已受理的回合保留自己当时固定的版本。
	got, err := st.GetTurn(turn.TurnID)
	if err != nil {
		t.Fatalf("get turn: %v", err)
	}
	if got.RulesetVersion != RulesetVersion {
		t.Fatalf("已受理回合的规则版本不应被会话升级改动: %q", got.RulesetVersion)
	}

	// 升级后新受理的回合采用新版本。
	next := acceptAndWait(t, turnSvc, st, sessionID, branchID, "rs-3", "继续。")
	nv, err := st.GetTurn(next.TurnID)
	if err != nil {
		t.Fatalf("get next turn: %v", err)
	}
	if nv.RulesetVersion != "ruleset.upgraded.v2" {
		t.Fatalf("升级后的新回合应采用新版本，实际 %q", nv.RulesetVersion)
	}
}

func TestBuildPlanRuleConsequencesAndUnauthorizedItems(t *testing.T) {
	base := domain.NewWorldState()
	base.Characters["npc_x"] = domain.CharacterInfo{CharacterID: "npc_x", Name: "X", Participant: true}
	base.Items["watch"] = domain.ItemInstance{InstanceID: "watch", Name: "旧怀表", OwnerID: "player", Quantity: 1}
	payload, _ := json.Marshal(domain.ItemTransferPayload{ItemID: "watch", From: "player", To: "npc_x", Quantity: 1})
	check := domain.CheckResult{RollID: "roll1", ActionID: "gift", Effects: []domain.RuleEffect{{Type: domain.EventItemTransfer, Payload: payload}}}
	pc := planContext{ruleset: domain.DefaultRuleset(), RulesetVersion: RulesetVersion, Checks: []domain.CheckResult{check}}
	draft := protocol.TurnDraft{Blocks: []protocol.BlockFrame{{Seq: 1, Kind: protocol.BlockNarration, Text: "他接过怀表。"}},
		Proposals: []protocol.Proposal{{ProposalID: "p1", Type: "promise_propose", CharacterID: "npc_x", Text: "明天见面"}}}
	pd, err := buildPlan(base, draft, domain.TurnInput{Text: "送给你。"}, "structured", "node_rule", pc)
	if err != nil {
		t.Fatal(err)
	}
	if pd.NewState.Items["watch"].OwnerID != "npc_x" || base.Items["watch"].OwnerID != "player" || len(pd.NewState.Promises) != 1 {
		t.Fatal("rule effect or immutability failed")
	}
	replay := base.Clone()
	if _, err := domain.ApplyEvents(replay, pd.Events); err != nil || replay.HashID() != pd.NewState.HashID() {
		t.Fatal("replay mismatch", err)
	}
	for _, proposal := range []protocol.Proposal{
		{ProposalID: "bad", Type: "item_grant", Text: "凭空的钥匙", To: "player", Quantity: 1},
		{ProposalID: "bad", Type: "item_transfer", ItemID: "watch", From: "player", To: "npc_x", Quantity: 1},
		{ProposalID: "bad", Type: "item_transfer", ActionRef: "gift", ReceiptRef: "roll1", ItemID: "watch", From: "player", To: "npc_x", Quantity: 2},
	} {
		draft.Proposals = []protocol.Proposal{proposal}
		if _, err := buildPlan(base, draft, domain.TurnInput{}, "structured", "bad", pc); err == nil {
			t.Fatalf("unauthorized proposal accepted: %+v", proposal)
		}
	}
	draft.Proposals = []protocol.Proposal{{ProposalID: "ok", Type: "item_transfer", ActionRef: "gift", ReceiptRef: "roll1", ItemID: "watch", From: "player", To: "npc_x", Quantity: 1}}
	if _, err := buildPlan(base, draft, domain.TurnInput{}, "structured", "ok", pc); err != nil {
		t.Fatal(err)
	}
}

func TestBuildPlanMemoryFromHistoryExcerpts(t *testing.T) {
	base := domain.NewWorldState()
	base.Characters["npc_liel"] = domain.CharacterInfo{CharacterID: "npc_liel", Name: "莉尔", Participant: true}

	draft := protocol.TurnDraft{
		Blocks: []protocol.BlockFrame{{
			Seq: 1, Kind: protocol.BlockNarration, Text: "旅人收回目光。",
		}},
		Proposals: []protocol.Proposal{
			{
				ProposalID: "p1", Type: "memory_add", Text: "莉尔答应尝试挑选便服。",
				CharacterID: "npc_liel", EntityIDs: []string{"npc_liel"},
				SourceQuote: "我会试着去准备的", EvidenceConfidence: "high",
			},
		},
	}

	// 1. 没有历史摘要时，来自上一轮的引文无法验证，应当失败
	pcNoHistory := planContext{ruleset: domain.DefaultRuleset(), RulesetVersion: RulesetVersion}
	if _, err := buildPlan(base, draft, domain.TurnInput{Text: "早点休息。"}, "structured", "node_curr", pcNoHistory); err == nil {
		t.Fatal("expected failure when quote is in prior turn but history excerpts missing")
	}

	// 2. 携带历史摘要（前几轮的内容），应当顺利通过且 SourceNodeID 指向历史节点
	pcWithHistory := planContext{
		ruleset:        domain.DefaultRuleset(),
		RulesetVersion: RulesetVersion,
		HistoryExcerpts: []domain.EvidenceExcerpt{
			{NodeID: "node_prior_5", Text: "莉尔轻声说：我会试着去准备的，大人。"},
		},
	}
	pd, err := buildPlan(base, draft, domain.TurnInput{Text: "早点休息。"}, "structured", "node_curr", pcWithHistory)
	if err != nil {
		t.Fatalf("buildPlan with history excerpts failed: %v", err)
	}
	if len(pd.Memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(pd.Memories))
	}
	if pd.Memories[0].Evidence == nil || pd.Memories[0].Evidence.SourceNodeID != "node_prior_5" {
		t.Fatalf("expected sourceNodeId node_prior_5, got %+v", pd.Memories[0].Evidence)
	}
}

// T3.1: 路径上存在同 SubjectKey 的活跃记忆时，自动将其 Supersedes，并设置时间轴
func TestSubjectKeySupersedesActiveMemoryOnPath(t *testing.T) {
	base := domain.NewWorldState()
	base.Characters["npc_liel"] = domain.CharacterInfo{CharacterID: "npc_liel", Name: "莉尔", Participant: true}

	existingMem := &domain.MemoryRecord{
		MemoryID:      "mem_old_1",
		SourceNodeID:  "node_0",
		Kind:          domain.MemoryObserved,
		Content:       "莉尔习惯穿着女仆装。",
		SubjectKey:    "character.liel.clothing",
		CreatedTurn:   1,
		ValidFromTurn: 1,
	}

	draft := protocol.TurnDraft{
		Blocks: []protocol.BlockFrame{
			{V: protocol.Version, Seq: 1, Type: protocol.FrameBlock, Kind: protocol.BlockNarration, Text: "莉尔换上了一身朴素的便服。"},
		},
		Proposals: []protocol.Proposal{
			{
				ProposalID: "p1", Type: "memory_add", Text: "莉尔换上了朴素的便服。",
				CharacterID: "npc_liel", EntityIDs: []string{"npc_liel"},
				SourceQuote: "莉尔换上了一身朴素的便服", EvidenceConfidence: "high",
				SubjectKey: "character.liel.clothing",
			},
		},
	}

	pc := planContext{
		ruleset:         domain.DefaultRuleset(),
		RulesetVersion:  RulesetVersion,
		VisibleMemories: []*domain.MemoryRecord{existingMem},
		CurrentTurn:     10,
	}

	pd, err := buildPlan(base, draft, domain.TurnInput{Text: "去集市吧。"}, "structured", "node_10", pc)
	if err != nil {
		t.Fatalf("buildPlan failed: %v", err)
	}
	if len(pd.Memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(pd.Memories))
	}
	newMem := pd.Memories[0]
	if newMem.Supersedes != existingMem.MemoryID {
		t.Fatalf("new memory should supersede old memory on path: got %q, want %q", newMem.Supersedes, existingMem.MemoryID)
	}
	if newMem.CreatedTurn != 10 || newMem.ValidFromTurn != 10 {
		t.Fatalf("new memory turns incorrect: created=%d, validFrom=%d", newMem.CreatedTurn, newMem.ValidFromTurn)
	}

	// 验证生效视图中仅保留最新一条
	active := domain.ApplyMemoryOverlays([]*domain.MemoryRecord{existingMem, newMem})
	if len(active) != 1 || active[0].MemoryID != newMem.MemoryID {
		t.Fatalf("expected only newMem to be active, got %+v", active)
	}
}

// T3.1: 同一回合中提议了多次相同 SubjectKey，后续提议自动 Supersedes 本轮前面的提议
func TestSubjectKeySameTurnSupersession(t *testing.T) {
	base := domain.NewWorldState()
	base.Characters["npc_liel"] = domain.CharacterInfo{CharacterID: "npc_liel", Name: "莉尔", Participant: true}

	draft := protocol.TurnDraft{
		Blocks: []protocol.BlockFrame{
			{V: protocol.Version, Seq: 1, Type: protocol.FrameBlock, Kind: protocol.BlockNarration, Text: "莉尔先穿上了便服，随后又换成了礼服。"},
		},
		Proposals: []protocol.Proposal{
			{
				ProposalID: "p1", Type: "memory_add", Text: "莉尔穿上了便服。",
				CharacterID: "npc_liel", EntityIDs: []string{"npc_liel"},
				SourceQuote: "莉尔先穿上了便服", EvidenceConfidence: "high",
				SubjectKey: "character.liel.clothing",
			},
			{
				ProposalID: "p2", Type: "memory_add", Text: "莉尔换成了礼服。",
				CharacterID: "npc_liel", EntityIDs: []string{"npc_liel"},
				SourceQuote: "随后又换成了礼服", EvidenceConfidence: "high",
				SubjectKey: "character.liel.clothing",
			},
		},
	}

	pc := planContext{
		ruleset:        domain.DefaultRuleset(),
		RulesetVersion: RulesetVersion,
		CurrentTurn:    5,
	}

	pd, err := buildPlan(base, draft, domain.TurnInput{Text: "准备出发。"}, "structured", "node_5", pc)
	if err != nil {
		t.Fatalf("buildPlan failed: %v", err)
	}
	if len(pd.Memories) != 2 {
		t.Fatalf("expected 2 memories, got %d", len(pd.Memories))
	}
	firstMem := pd.Memories[0]
	secondMem := pd.Memories[1]

	if secondMem.Supersedes != firstMem.MemoryID {
		t.Fatalf("second memory should supersede first memory in same turn: got %q, want %q", secondMem.Supersedes, firstMem.MemoryID)
	}
	if firstMem.ValidUntilTurn != 5 {
		t.Fatalf("first memory validUntilTurn should be set to 5, got %d", firstMem.ValidUntilTurn)
	}

	// 验证生效视图仅保留第二条
	active := domain.ApplyMemoryOverlays([]*domain.MemoryRecord{firstMem, secondMem})
	if len(active) != 1 || active[0].MemoryID != secondMem.MemoryID {
		t.Fatalf("expected only secondMem to be active, got %+v", active)
	}
}
