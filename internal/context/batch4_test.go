package context_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"tavernagent/internal/adapters/sqlite"
	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"tavernagent/internal/util/id"
)

// commitTurnWithEvents 在指定 headID 提交一个带事件和指定 Block 内容的回合节点。
func commitTurnWithEvents(t *testing.T, st *sqlite.Store, branchID, headID, input string, blocks []domain.TextBlock, events []*domain.DomainEvent, state *domain.WorldState) string {
	t.Helper()
	parent, err := st.GetNode(headID)
	if err != nil {
		t.Fatalf("parent %s: %v", headID, err)
	}
	nextJSON, err := state.Marshal()
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}

	commitSeq.Add(1)
	seq := strconv.FormatInt(commitSeq.Load(), 10)
	turnID := "turn_b4_" + seq
	nodeID := "node_b4_" + seq
	branch, err := st.GetBranch(branchID)
	if err != nil {
		t.Fatalf("get branch %s: %v", branchID, err)
	}
	if err := st.CreateTurnRequest(&domain.TurnRequest{
		TurnID: turnID, SessionID: testSessionID, BranchID: branchID,
		IdempotencyKey: turnID, PayloadHash: "h", ExpectedHeadID: headID,
		Status: domain.TurnQueued, Mode: "structured",
	}); err != nil {
		t.Fatalf("create turn: %v", err)
	}
	if ok, err := st.ClaimActiveTurn(branchID, turnID); err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}

	tc := domain.TurnContent{
		InputText: input,
		Blocks:    blocks,
	}
	contentJSON, err := json.Marshal(tc)
	if err != nil {
		t.Fatalf("marshal turn content: %v", err)
	}

	for i, ev := range events {
		if ev.EventID == "" {
			ev.EventID = id.New()
		}
		ev.NodeID = nodeID
		ev.EventIndex = i
	}

	res, err := st.CommitTurn(&ports.CommitPlan{
		TurnID: turnID, ExpectedHeadID: headID, ExpectedVersion: branch.Version,
		Node: &domain.PlotNode{
			NodeID: nodeID, SessionID: testSessionID, ParentID: headID, Kind: domain.NodeKindTurn,
			Depth: parent.Depth + 1, TurnNumber: parent.TurnNumber + 1, SchemaVersion: 1,
			ContentJSON: string(contentJSON),
		},
		Events:       events,
		NewStateHash: state.HashID(), NewStateJSON: nextJSON,
	})
	if err != nil || !res.Committed {
		t.Fatalf("commit: %v %+v", err, res)
	}
	return nodeID
}

// T4.1: 固化未折叠区间以稳定前缀缓存。
// 在历史回合持续增加时，未折叠区间内的较早回合（含心声）渲染文本必须 100% 字节不变。
func TestPrefixCacheStability(t *testing.T) {
	opts := ctxpkg.DefaultOptions()
	opts.CompactionPolicy.TailWindowTurns = 20
	opts.CompactionPolicy.PruneThoughtDepth = 20
	f := newFixtureFull(t, nil, "故事开场白：你来到了边境酒馆。", "", opts)

	state := domain.NewWorldState()
	head := testRootID

	// 提交 20 个回合，每个回合都带有 dialogue 和 inner_monologue
	for i := 1; i <= 20; i++ {
		blocks := []domain.TextBlock{
			{Kind: "dialogue", Text: fmt.Sprintf("这是第 %d 轮的对话内容。", i)},
			{Kind: "inner_monologue", Text: fmt.Sprintf("这是第 %d 轮的角色内心独白。", i)},
		}
		head = commitTurnWithEvents(t, f.store, testBranchID, head, fmt.Sprintf("输入 %d", i), blocks, nil, state)
	}

	// 在第 20 轮编译上下文
	req20, err := f.compiler.Compile(context.Background(), testSessionID, head, "第21轮输入", state, nil)
	if err != nil {
		t.Fatalf("compile at turn 20: %v", err)
	}

	// 收集第 20 轮编译下历史回合的消息映射 (role + content)
	var messagesAt20 []ports.ChatMessage
	for _, m := range req20.Messages {
		if m.Role == "user" || m.Role == "assistant" {
			messagesAt20 = append(messagesAt20, m)
		}
	}

	// 确保 Turn 1 assistant 消息包含心声
	turn1Assistant := ""
	for _, m := range messagesAt20 {
		if m.Role == "assistant" && strings.Contains(m.Content, "这是第 1 轮的对话内容") {
			turn1Assistant = m.Content
			if !strings.Contains(m.Content, "（这是第 1 轮的角色内心独白。）") {
				t.Fatalf("第 1 轮 assistant 消息在未折叠区间内必须保留心声:\n%s", m.Content)
			}
			break
		}
	}
	if turn1Assistant == "" {
		t.Fatalf("未找到第 1 轮 assistant 消息")
	}

	// 继续追加 5 个回合，总轮数达到 25
	for i := 21; i <= 25; i++ {
		blocks := []domain.TextBlock{
			{Kind: "dialogue", Text: fmt.Sprintf("这是第 %d 轮的对话内容。", i)},
			{Kind: "inner_monologue", Text: fmt.Sprintf("这是第 %d 轮的角色内心独白。", i)},
		}
		head = commitTurnWithEvents(t, f.store, testBranchID, head, fmt.Sprintf("输入 %d", i), blocks, nil, state)
	}

	// 在第 25 轮重新编译上下文
	req25, err := f.compiler.Compile(context.Background(), testSessionID, head, "第26轮输入", state, nil)
	if err != nil {
		t.Fatalf("compile at turn 25: %v", err)
	}

	// 收集第 20 轮编译下历史回合的消息映射 (role + content)，排除最后一条当前轮输入 prompt
	historical20 := messagesAt20[:len(messagesAt20)-1]

	// 收集第 25 轮编译下的历史消息
	var messagesAt25 []ports.ChatMessage
	for _, m := range req25.Messages {
		if m.Role == "user" || m.Role == "assistant" {
			messagesAt25 = append(messagesAt25, m)
		}
	}

	// 断言：前 20 轮的所有历史消息在第 25 轮编译中必须与第 20 轮编译时的结果 100% 字节一致！
	for idx, m20 := range historical20 {
		if idx >= len(messagesAt25) {
			t.Fatalf("第 25 轮消息丢失，在 index=%d 找不到对应消息", idx)
		}
		m25 := messagesAt25[idx]
		if m20.Role != m25.Role {
			t.Errorf("msg[%d] role mismatch: %s vs %s", idx, m20.Role, m25.Role)
		}
		if m20.Content != m25.Content {
			t.Fatalf("T4.1 破坏：历史消息在同分支未折叠区间字节突变，打穿前缀缓存！\nTurn 20 消息:\n%s\nTurn 25 消息:\n%s", m20.Content, m25.Content)
		}
	}
}

// T4.2: 账本扩展到关系轨迹与秘密揭示回合。
func TestCompileLedgerWithRelationshipAndSecretTrajectory(t *testing.T) {
	secretsJSON := `[{"secretId":"sec_crypt","title":"古老地宫","content":"在教堂后方有一处隐秘地宫入口。"}]`
	f := newFixtureFull(t, nil, "", secretsJSON, ctxpkg.DefaultOptions())

	state := domain.NewWorldState()
	state.Characters["npc_elena"] = domain.CharacterInfo{CharacterID: "npc_elena", Name: "艾莲娜", Participant: true}
	head := testRootID

	// Turn 1: 发生 relationship_delta（好感 +10）
	rel1Payload, _ := json.Marshal(domain.RelationshipDeltaPayload{
		CharacterID: "npc_elena", Field: "affection", Delta: 10, Applied: 10,
	})
	ev1 := &domain.DomainEvent{Type: domain.EventRelationshipDelta, PayloadJSON: string(rel1Payload), RulesetVersion: "v1"}
	state.Relationships["npc_elena"] = domain.RelationValue{Affection: 10}
	head = commitTurnWithEvents(t, f.store, testBranchID, head, "艾莲娜，我救了你。", []domain.TextBlock{{Kind: "dialogue", Text: "谢谢你！"}}, []*domain.DomainEvent{ev1}, state)

	// Turn 2: 发生 secret_unlock（解锁 sec_crypt）
	secPayload, _ := json.Marshal(domain.SecretUnlockPayload{SecretID: "sec_crypt", Title: "古老地宫"})
	ev2 := &domain.DomainEvent{Type: domain.EventSecretUnlock, PayloadJSON: string(secPayload), RulesetVersion: "v1"}
	state.UnlockSecret("sec_crypt")
	head = commitTurnWithEvents(t, f.store, testBranchID, head, "你在寻找什么？", []domain.TextBlock{{Kind: "dialogue", Text: "其实有一处隐秘地宫..."}}, []*domain.DomainEvent{ev2}, state)

	// Turn 3: 发生 relationship_delta（信任 +15）
	rel3Payload, _ := json.Marshal(domain.RelationshipDeltaPayload{
		CharacterID: "npc_elena", Field: "trust", Delta: 15, Applied: 15,
	})
	ev3 := &domain.DomainEvent{Type: domain.EventRelationshipDelta, PayloadJSON: string(rel3Payload), RulesetVersion: "v1"}
	state.Relationships["npc_elena"] = domain.RelationValue{Affection: 10, Trust: 15}
	head = commitTurnWithEvents(t, f.store, testBranchID, head, "我相信你。", []domain.TextBlock{{Kind: "dialogue", Text: "我们一起去吧。"}}, []*domain.DomainEvent{ev3}, state)

	// 在 Turn 3 编译上下文
	req, err := f.compiler.Compile(context.Background(), testSessionID, head, "准备出发。", state, nil)
	if err != nil {
		t.Fatalf("compile at turn 3: %v", err)
	}

	var allSystem strings.Builder
	for _, m := range req.Messages {
		if m.Role == "system" {
			allSystem.WriteString(m.Content)
			allSystem.WriteString("\n")
		}
	}
	sys := allSystem.String()

	// 1. 验证账本包含关系轨迹与转折点
	if !strings.Contains(sys, "<domain_state_ledger authoritative=\"true\">") {
		t.Fatalf("未生成权威账本:\n%s", sys)
	}
	if !strings.Contains(sys, "角色关系轨迹（权威变化记录）") {
		t.Fatalf("账本中缺失角色关系轨迹:\n%s", sys)
	}
	if !strings.Contains(sys, "艾莲娜 (npc_elena)") {
		t.Fatalf("关系轨迹缺失角色名:\n%s", sys)
	}
	if !strings.Contains(sys, "Turn#1 [好感 +10]") || !strings.Contains(sys, "Turn#3 [信任 +15]") {
		t.Fatalf("关系轨迹缺失具体转折点与回合序号:\n%s", sys)
	}
	if !strings.Contains(sys, "当前: 好感 10, 信任 15, 戒备 0") {
		t.Fatalf("关系轨迹缺失当前状态:\n%s", sys)
	}

	// 2. 验证已揭示秘密标注了揭示回合
	if !strings.Contains(sys, "【已揭示的世界观与秘密】") {
		t.Fatalf("缺失已揭示秘密段:\n%s", sys)
	}
	if !strings.Contains(sys, "[第2轮揭示]") || !strings.Contains(sys, "古老地宫") {
		t.Fatalf("已揭示秘密必须标明 [第2轮揭示]:\n%s", sys)
	}
}

// T4.3: 验证主动 Token 压缩阈值在达到预算/窗口压力时主动置位 PrepareCompression。
func TestProactiveTokenCompressionThreshold(t *testing.T) {
	opts := ctxpkg.DefaultOptions()
	opts.ContextWindow = 4000
	opts.ReservedOutput = 500
	opts.SafetyMargin = 200
	// inputBudget = 4000 - 500 - 200 = 3300
	// 75% budget = 2475

	f := newFixtureFull(t, nil, "", "", opts)
	state := domain.NewWorldState()

	// 构造恰好使 BeforeTrim 超过 75% budget 但不超过 3300 的输入
	longInput := strings.Repeat("这是一段用于测试长输入主动触发压缩的文本。", 75)
	req, err := f.compiler.Compile(context.Background(), testSessionID, testRootID, longInput, state, nil)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	if !req.NeedsCompaction {
		t.Fatalf("达到 75%% 输入预算应主动触发压缩需求 (NeedsCompaction=true), Estimated=%d, Budget=%d", req.EstimatedInputTokens, req.InputBudget)
	}
}

// 验证续写/重试路径多次 Compile 产物完全一致。
func TestContinueRequestMessageStability(t *testing.T) {
	f := newFixtureFull(t, nil, "故事开场白", "", ctxpkg.DefaultOptions())
	state := domain.NewWorldState()
	head := commitTurnWithEvents(t, f.store, testBranchID, testRootID, "第一步", []domain.TextBlock{{Kind: "narration", Text: "剧情发展。"}}, nil, state)

	// 模拟同一基准下多次重试/续写编译
	req1, err := f.compiler.Compile(context.Background(), testSessionID, head, "续写输入", state, nil)
	if err != nil {
		t.Fatalf("compile 1: %v", err)
	}
	req2, err := f.compiler.Compile(context.Background(), testSessionID, head, "续写输入", state, nil)
	if err != nil {
		t.Fatalf("compile 2: %v", err)
	}

	if len(req1.Messages) != len(req2.Messages) {
		t.Fatalf("重试编译消息数量不一致: %d vs %d", len(req1.Messages), len(req2.Messages))
	}
	for i := range req1.Messages {
		if req1.Messages[i].Role != req2.Messages[i].Role || req1.Messages[i].Content != req2.Messages[i].Content {
			t.Fatalf("重试编译消息[%d]字节不一致", i)
		}
	}
}
