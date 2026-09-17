package application

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"tavernagent/internal/adapters/sqlite"
	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"tavernagent/internal/protocol"
	"tavernagent/internal/util/id"
)

// 条件满足时产生解锁事件，且事件应用后状态确实标记为已解锁。
func TestBuildPlanUnlocksSecretWhenConditionMet(t *testing.T) {
	base := domain.NewWorldState()
	base.Characters["npc_a"] = domain.CharacterInfo{CharacterID: "npc_a", Name: "甲", Participant: true}
	base.Relationships["npc_a"] = domain.RelationValue{Affection: 5}

	pc := planContext{
		ruleset:        domain.DefaultRuleset(),
		RulesetVersion: "ruleset.simplified.v1",
		Secrets: []domain.SecretDef{{
			SecretID: "sec_1", Title: "地窖的秘密", Content: "账本埋在神龛下。",
			RevealWhen: mustRule(t, `{"op":"cmp","field":"affection:npc_a","cmp":"gte","value":"5"}`),
		}},
	}
	pd, err := buildPlan(base, protocol.TurnDraft{Blocks: []protocol.BlockFrame{{Kind: "narration", Text: "她开口了。"}}},
		domain.TurnInput{Kind: "text", Text: "我信任你了。"}, protocol.ModeStructured, "n1", pc)
	if err != nil {
		t.Fatalf("buildPlan: %v", err)
	}
	if len(pd.Events) != 1 || pd.Events[0].Type != domain.EventSecretUnlock {
		t.Fatalf("应产生恰好一个解锁事件，得到 %+v", pd.Events)
	}
	// 事件应用后状态必须真的解锁（提交与重放共用这条路径）。
	if _, err := domain.ApplyEvents(pd.NewState, pd.Events); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !pd.NewState.IsSecretUnlocked("sec_1") {
		t.Fatalf("事件应用后仍未解锁: %+v", pd.NewState.UnlockedSecrets)
	}
}

// 条件不满足 → 无事件。模型就算提议了也不解锁（无权自证，T25）。
func TestBuildPlanDoesNotUnlockWhenConditionUnmet(t *testing.T) {
	base := domain.NewWorldState()
	base.Characters["npc_a"] = domain.CharacterInfo{CharacterID: "npc_a", Name: "甲", Participant: true}
	base.Relationships["npc_a"] = domain.RelationValue{Affection: 1}

	pc := planContext{
		ruleset:        domain.DefaultRuleset(),
		RulesetVersion: "ruleset.simplified.v1",
		Secrets: []domain.SecretDef{{
			SecretID: "sec_1", Content: "账本埋在神龛下。",
			RevealWhen: mustRule(t, `{"op":"cmp","field":"affection:npc_a","cmp":"gte","value":"5"}`),
		}},
	}
	// 模型同时提议了关系增量与解锁：只有关系增量生效。
	draft := protocol.TurnDraft{
		Blocks: []protocol.BlockFrame{{Kind: "narration", Text: "她有点动摇。"}},
		Proposals: []protocol.Proposal{
			{ProposalID: "p1", Type: "relationship_delta", CharacterID: "npc_a", Field: "affection", Delta: 2},
			{ProposalID: "p2", Type: "secret_propose", RuleID: "sec_1"},
		},
	}
	pd, err := buildPlan(base, draft, domain.TurnInput{Kind: "text", Text: "我们聊聊。"}, protocol.ModeStructured, "n1", pc)
	if err != nil {
		t.Fatalf("buildPlan: %v", err)
	}
	for _, ev := range pd.Events {
		if ev.Type == domain.EventSecretUnlock {
			t.Fatalf("条件未满足却产生了解锁事件")
		}
	}
	// 正文保留：提议被忽略不能惩罚整轮。
	if len(pd.Blocks) != 1 {
		t.Fatalf("正文被丢弃: %+v", pd.Blocks)
	}
}

// 已解锁的秘密不再重复产生解锁事件（重放/重复来源都安全）。
func TestBuildPlanDoesNotReunlockAlreadyUnlocked(t *testing.T) {
	base := domain.NewWorldState()
	base.Characters["npc_a"] = domain.CharacterInfo{CharacterID: "npc_a", Name: "甲", Participant: true}
	base.UnlockSecret("sec_1")

	pc := planContext{
		ruleset:        domain.DefaultRuleset(),
		RulesetVersion: "ruleset.simplified.v1",
		Secrets: []domain.SecretDef{{
			SecretID: "sec_1", Content: "账本埋在神龛下。",
			RevealWhen: mustRule(t, `{"op":"cmp","field":"scene.location","cmp":"eq","value":"cellar"}`),
		}},
	}
	base.Scene = &domain.Scene{LocationID: "cellar"}
	pd, err := buildPlan(base, protocol.TurnDraft{Blocks: []protocol.BlockFrame{{Kind: "narration", Text: "。"}}},
		domain.TurnInput{Kind: "text", Text: "。"}, protocol.ModeStructured, "n1", pc)
	if err != nil {
		t.Fatalf("buildPlan: %v", err)
	}
	for _, ev := range pd.Events {
		if ev.Type == domain.EventSecretUnlock {
			t.Fatalf("已解锁的秘密再次产生了解锁事件")
		}
	}
}

// 非法规则必须让整轮失败：静默跳过会让秘密永远无法揭示且无人知晓。
func TestBuildPlanFailsOnBrokenSecretRule(t *testing.T) {
	base := domain.NewWorldState()
	pc := planContext{
		ruleset:        domain.DefaultRuleset(),
		RulesetVersion: "ruleset.simplified.v1",
		Secrets: []domain.SecretDef{{
			SecretID: "sec_bad", Content: "x",
			// 直接构造 AST（不经过 ParseRuleExpr）：ParseRuleExpr 在导入入口
			// 就会拦住它；这里要测的是 buildPlan 自己的守卫（数据可能在
			// 会话建立后损坏）。
			RevealWhen: &domain.RuleExpr{Op: domain.OpCmp, Field: "unknown.field", Cmp: domain.CmpEq, Value: "1"},
		}},
	}
	if _, err := buildPlan(base, protocol.TurnDraft{Blocks: []protocol.BlockFrame{{Kind: "narration", Text: "。"}}},
		domain.TurnInput{Kind: "text", Text: "。"}, protocol.ModeStructured, "n1", pc); err == nil {
		t.Fatalf("非法揭示条件应让整轮失败")
	}
}

// 端到端（提交路径）：条件达成的那一轮产生解锁事件并推进状态，
// 下一轮才把内容注入上下文——不是当轮（当轮编译发生在解锁判定之前）。
func TestSecretUnlockFlowThroughCommitAndView(t *testing.T) {
	card := `{
	  "schemaVersion": 2, "cardId": "card_sec", "name": "卡",
	  "characters": [{"characterId":"npc_a","name":"甲","participant":true}],
	  "secrets": [{"secretId":"sec_1","title":"地窖的秘密","content":"账本埋在神龛下。",
	               "revealWhen":"{\"op\":\"cmp\",\"field\":\"affection:npc_a\",\"cmp\":\"gte\",\"value\":\"3\"}"}],
	  "openingVariants": [{"title":"默认","text":"开场。"}]
	}`
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	sessSvc := NewSessionService(st)
	res, err := sessSvc.Setup(context.Background(), &SessionSetupRequest{
		Title: "秘密测试", CharacterJSON: card, Player: Player{Name: "旅人"},
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	sessID, branchID, root := res.Session.SessionID, res.Branch.BranchID, res.RootNode.NodeID

	// 初始视图：未揭示，不回传内容。
	view, err := sessSvc.View(sessID, ViewQuery{})
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if len(view.Secrets) != 1 || view.Secrets[0].Revealed || view.Secrets[0].Content != "" {
		t.Fatalf("未揭示却回传了内容: %+v", view.Secrets)
	}

	// 提交一轮：走真实 buildPlan → CommitTurn 链路（与 commitDraft 相同的步骤），
	// 模型提议把好感推到阈值之上。解锁判定就在 buildPlan 里，绕过它测不到。
	newHead := commitViaBuildPlan(t, st, sessID, branchID, root, pcFor(t, st, sessID))

	// 提交之后的视图：已揭示，内容可见。
	view2, err := sessSvc.View(sessID, ViewQuery{})
	if err != nil {
		t.Fatalf("view2: %v", err)
	}
	if len(view2.Secrets) != 1 || !view2.Secrets[0].Revealed || !strings.Contains(view2.Secrets[0].Content, "账本") {
		t.Fatalf("揭示后视图异常: %+v", view2.Secrets)
	}

	// 下一轮编译注入内容；当轮（解锁前）不注入。
	compiler := ctxpkg.New(st, ctxpkg.DefaultOptions())
	baseSnap, err := st.StateAt(newHead)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	state, err := domain.UnmarshalWorld(baseSnap.StateJSON)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	prompt, err := compiler.Compile(context.Background(), sessID, newHead, "现在说说看。", state, nil)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	// 启用前缀缓存分离后系统提示拆成静态/动态两条 system 消息，
	// 因此拼接全部 system 消息再断言（意图是"是否进入请求"，而非"排第几条"）。
	if !strings.Contains(systemPromptOf(prompt.Messages), "账本") {
		t.Fatalf("已揭示的秘密未注入下一轮上下文:\n%s", systemPromptOf(prompt.Messages))
	}
}

func mustRule(t *testing.T, s string) *domain.RuleExpr {
	t.Helper()
	expr, err := domain.ParseRuleExpr(s)
	if err != nil || expr == nil {
		t.Fatalf("ParseRuleExpr(%s): %v", s, err)
	}
	return expr
}

// pcFor 构造与 commitDraft 相同的计划上下文（秘密定义从会话读取）。
func pcFor(t *testing.T, st *sqlite.Store, sessionID string) planContext {
	t.Helper()
	return planContext{
		ruleset:        domain.DefaultRuleset(),
		RulesetVersion: "ruleset.simplified.v1",
		Secrets:        sessionSecretDefs(st, sessionID),
	}
}

// commitViaBuildPlan 复刻 commitDraft 的核心步骤：buildPlan → 补事件 ID →
// CommitTurn。解锁判定在 buildPlan 里，因此必须走这条链路而不是手工拼事件。
func commitViaBuildPlan(t *testing.T, st *sqlite.Store, sessionID, branchID, parentID string, pc planContext) string {
	t.Helper()
	parent, err := st.GetNode(parentID)
	if err != nil {
		t.Fatalf("parent: %v", err)
	}
	branch, err := st.GetBranch(branchID)
	if err != nil {
		t.Fatalf("branch: %v", err)
	}
	bs, err := st.StateAt(parentID)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	base, err := domain.UnmarshalWorld(bs.StateJSON)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	nodeID := id.New()
	draft := protocol.TurnDraft{
		Blocks: []protocol.BlockFrame{{V: 1, Seq: 1, Type: protocol.FrameBlock, Kind: "narration", Text: "她笑了。"}},
		Proposals: []protocol.Proposal{
			{ProposalID: "p1", Type: "relationship_delta", CharacterID: "npc_a", Field: "affection", Delta: 5},
		},
	}
	input := domain.TurnInput{Kind: "text", Text: "聊了很久。"}
	pd, err := buildPlan(base, draft, input, protocol.ModeStructured, nodeID, pc)
	if err != nil {
		t.Fatalf("buildPlan: %v", err)
	}
	for i, ev := range pd.Events {
		ev.EventID = id.New()
		ev.NodeID = nodeID
		ev.EventIndex = i
	}
	stateJSON, err := pd.NewState.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	turnID := id.New()
	if err := st.CreateTurnRequest(&domain.TurnRequest{
		TurnID: turnID, SessionID: sessionID, BranchID: branchID,
		IdempotencyKey: turnID, PayloadHash: "h", ExpectedHeadID: parentID,
		Status: domain.TurnQueued, Mode: "structured",
	}); err != nil {
		t.Fatalf("create turn: %v", err)
	}
	if ok, err := st.ClaimActiveTurn(branchID, turnID); err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	content, _ := json.Marshal(domain.TurnContent{InputText: input.Text, Blocks: pd.Blocks})
	res, err := st.CommitTurn(&ports.CommitPlan{
		TurnID: turnID, ExpectedHeadID: parentID, ExpectedVersion: branch.Version,
		BaseStateHash: base.HashID(), RulesetVersion: pc.RulesetVersion,
		Node: &domain.PlotNode{
			NodeID: nodeID, SessionID: sessionID, ParentID: parentID, Kind: domain.NodeKindTurn,
			Depth: parent.Depth + 1, TurnNumber: parent.TurnNumber + 1, SchemaVersion: 1,
			ContentJSON: string(content),
		},
		Events: pd.Events, Memories: pd.Memories,
		NewStateHash: pd.NewState.HashID(), NewStateJSON: stateJSON,
	})
	if err != nil || !res.Committed {
		t.Fatalf("commit: %v %+v", err, res)
	}
	return nodeID
}

// systemPromptOf 拼接请求中全部 system 消息。
// 测试意图是"材料是否进入请求"，不应绑定它在第几条消息。
func systemPromptOf(msgs []ports.ChatMessage) string {
	var b strings.Builder
	for _, m := range msgs {
		if m.Role == "system" {
			b.WriteString(m.Content)
			b.WriteString("\n")
		}
	}
	return b.String()
}
