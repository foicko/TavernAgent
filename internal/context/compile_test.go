package context_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"tavernagent/internal/adapters/sqlite"
	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

const (
	testSessionID = "sess_test"
	testRootID    = "root_test"
	testBranchID  = "branch_test"

	testPlayerName = "测试者"
	testPlayerRole = "旅行者"
)

type fixture struct {
	store    *sqlite.Store
	compiler *ctxpkg.Compiler
}

// newFixture 构造一个会话：根节点带 lorebook 模板快照 + 对应模板版本。
// books 为空时模板快照的 lorebook 记为 null（模拟未导入世界书的会话）。
func newFixture(t *testing.T, books []domain.Lorebook, opts ctxpkg.CompilerOptions) *fixture {
	return newFixtureWithOpening(t, books, "", opts)
}

func newFixtureWithOpening(t *testing.T, books []domain.Lorebook, opening string, opts ctxpkg.CompilerOptions) *fixture {
	return newFixtureFull(t, books, opening, "", opts)
}

// newFixtureFull 额外支持带秘密定义的角色模板（M4d：卡片 JSON 的 secrets 数组）。
// secretsJSON 为空时角色模板仍会创建（内容为最小卡片），保证秘密读取路径被真实覆盖。
func newFixtureFull(t *testing.T, books []domain.Lorebook, opening, secretsJSON string, opts ctxpkg.CompilerOptions) *fixture {
	t.Helper()
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	snapshot := map[string]any{"lorebook": nil, "rules": nil}
	cardJSON := `{"schemaVersion":2,"cardId":"card_t","name":"测试卡","characters":[],"secrets":` + secretsJSON + `}`
	charTpl := &domain.TemplateVersion{
		TemplateVersionID: "tpl_card_t", Kind: domain.TemplateCharacter, SchemaVersion: 1,
		Content: cardJSON, ContentHash: "h_card",
	}
	var templates []*domain.TemplateVersion
	templates = append(templates, charTpl)
	snapshot["character"] = map[string]any{
		"templateVersionId": charTpl.TemplateVersionID, "schemaVersion": 1, "contentHash": "h_card",
	}
	if len(books) > 0 {
		content, _ := json.Marshal(books)
		tpl := &domain.TemplateVersion{
			TemplateVersionID: "tpl_lb", Kind: domain.TemplateLorebook, SchemaVersion: 1,
			Content: string(content),
		}
		templates = append(templates, tpl)
		snapshot["lorebook"] = map[string]any{
			"templateVersionId": tpl.TemplateVersionID, "schemaVersion": 1,
		}
	}
	// 玩家身份快照：Compile 从 templates.player.content 解析玩家名与身份，
	// 不再由调用方传字面量（历史缺陷 X6）。
	snapshot["player"] = map[string]any{
		"schemaVersion": 1,
		"content":       map[string]any{"name": testPlayerName, "role": testPlayerRole},
	}
	rootContent, _ := json.Marshal(map[string]any{"templates": snapshot, "openingText": opening})
	root := &domain.PlotNode{
		NodeID: testRootID, SessionID: testSessionID, Kind: domain.NodeKindRoot,
		SchemaVersion: 1, ContentJSON: string(rootContent),
	}
	sess := &domain.Session{SessionID: testSessionID, RootNodeID: testRootID, Title: "测试"}
	branch := &domain.Branch{BranchID: testBranchID, SessionID: testSessionID, Name: "main", HeadNodeID: testRootID}
	if err := st.CreateSession(sess, root, branch, templates); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return &fixture{store: st, compiler: ctxpkg.New(st, opts)}
}

func (f *fixture) systemPrompt(t *testing.T, nodeID, input string) string {
	t.Helper()
	return f.systemPromptWithState(t, nodeID, input, nil)
}

func (f *fixture) compileMessages(t *testing.T, nodeID, input string, state *domain.WorldState) []ports.ChatMessage {
	t.Helper()
	return f.compileMessagesWithChecks(t, nodeID, input, state, nil)
}

// compileMessagesWithChecks 允许带上准备阶段已定的检定结果（契约 §5.2）。
func (f *fixture) compileMessagesWithChecks(t *testing.T, nodeID, input string, state *domain.WorldState, checks []domain.CheckResult) []ports.ChatMessage {
	t.Helper()
	req, err := f.compiler.Compile(context.Background(), testSessionID, nodeID, input, state, checks)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(req.Messages) == 0 || req.Messages[0].Role != "system" {
		t.Fatalf("首条消息应为 system，实际 %+v", req.Messages)
	}
	return req.Messages
}

// systemPromptWithState 返回"系统提示"全文。
//
// 启用 SplitDynamicContext 后系统提示被拆成静态与动态两条 system 消息，
// 因此这里拼接**所有** system 消息而不是取 Messages[0]——测试用例的意图是
// "这些材料是否进入请求"，而不是"它们排在第几条消息"。
func (f *fixture) systemPromptWithState(t *testing.T, nodeID, input string, state *domain.WorldState) string {
	t.Helper()
	var b strings.Builder
	for _, m := range f.compileMessages(t, nodeID, input, state) {
		if m.Role == "system" {
			b.WriteString(m.Content)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// systemPromptWithChecks 允许带上准备阶段已定的检定结果（契约 §5.2）。
func (f *fixture) systemPromptWithChecks(t *testing.T, nodeID, input string, checks []domain.CheckResult) string {
	t.Helper()
	var b strings.Builder
	for _, m := range f.compileMessagesWithChecks(t, nodeID, input, nil, checks) {
		if m.Role == "system" {
			b.WriteString(m.Content)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func book(entries ...domain.LorebookEntry) []domain.Lorebook {
	return []domain.Lorebook{{RefID: "lb1", Name: "钟楼设定集", Schema: "v2", Entries: entries}}
}

func entry(id string, enabled bool, content string, keys ...string) domain.LorebookEntry {
	return domain.LorebookEntry{EntryID: id, Keys: keys, Content: content, Enabled: enabled}
}

const loreSectionHeader = "世界设定"

// 关键词命中 → 条目注入 system prompt。
func TestLorebookInjectsMatchedEntry(t *testing.T) {
	f := newFixture(t, book(
		entry("e1", true, "钟楼每逢午夜会敲响十三下。", "钟楼", "塔楼"),
		entry("e2", true, "看护人从不离开围墙。", "看护人"),
	), ctxpkg.DefaultOptions())

	got := f.systemPrompt(t, testRootID, "我推开了钟楼的门。")
	if !strings.Contains(got, loreSectionHeader) || !strings.Contains(got, "十三下") {
		t.Fatalf("命中条目未注入:\n%s", got)
	}
	// 未命中的条目不得出现（避免无差别塞入全部设定）。
	if strings.Contains(got, "从不离开围墙") {
		t.Fatalf("未命中条目被注入:\n%s", got)
	}
}

// 无关键词命中 → 不出现世界设定段。
func TestLorebookIDsAreScopedToBookAndOptional(t *testing.T) {
	books := []domain.Lorebook{
		{Name: "第一册", Entries: []domain.LorebookEntry{entry("", true, "甲册事实", "灯塔"), entry("", true, "第二个无 ID 条目", "灯塔"), entry("shared", true, "甲册编号事实", "灯塔")}},
		{Name: "第二册", Entries: []domain.LorebookEntry{entry("shared", true, "乙册编号事实", "灯塔")}},
	}
	f := newFixture(t, books, ctxpkg.DefaultOptions())
	got := f.systemPrompt(t, testRootID, "我走向灯塔。")
	for _, expected := range []string{"甲册事实", "第二个无 ID 条目", "甲册编号事实", "乙册编号事实"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("distinct entry omitted: %s", expected)
		}
	}
}

func TestLorebookSkipsWhenNothingMatches(t *testing.T) {
	f := newFixture(t, book(entry("e1", true, "钟楼每逢午夜会敲响十三下。", "钟楼")), ctxpkg.DefaultOptions())
	got := f.systemPrompt(t, testRootID, "我在路边买了两个苹果。")
	if strings.Contains(got, loreSectionHeader) {
		t.Fatalf("无命中时不应出现世界设定段:\n%s", got)
	}
}

// 禁用条目不参与命中。
func TestLorebookSkipsDisabledEntry(t *testing.T) {
	f := newFixture(t, book(
		entry("e1", false, "这条被作者禁用，不应出现。", "废弃的传闻"),
	), ctxpkg.DefaultOptions())
	got := f.systemPrompt(t, testRootID, "我听说过那个废弃的传闻。")
	if strings.Contains(got, "这条被作者禁用") {
		t.Fatalf("禁用条目被注入:\n%s", got)
	}
}

// 少于最小长度的 key（单字）不参与命中。
func TestLorebookIgnoresShortKey(t *testing.T) {
	f := newFixture(t, book(entry("e1", true, "单字 key 的内容。", "钟")), ctxpkg.DefaultOptions())
	got := f.systemPrompt(t, testRootID, "钟声响了。")
	if strings.Contains(got, "单字 key 的内容") {
		t.Fatalf("单字 key 不应命中:\n%s", got)
	}
}

// 条数预算：超出上限的条目被跳过。
func TestLorebookEntryBudget(t *testing.T) {
	opts := ctxpkg.DefaultOptions()
	opts.MaxLorebookEntries = 1
	f := newFixture(t, book(
		entry("e1", true, "条目一的内容。", "钟楼"),
		entry("e2", true, "条目二的内容。", "看护人"),
	), opts)

	got := f.systemPrompt(t, testRootID, "钟楼的看护人站在门口。")
	if !strings.Contains(got, "条目一的内容") {
		t.Fatalf("预算内首条未注入:\n%s", got)
	}
	if strings.Contains(got, "条目二的内容") {
		t.Fatalf("超出条数预算仍被注入:\n%s", got)
	}
}

// 字符预算：长条目被跳过，短条目仍可注入（不是直接截断前几条）。
func TestLorebookRuneBudget(t *testing.T) {
	long := strings.Repeat("很长的设定内容。", 80) // 约 800 字符
	opts := ctxpkg.DefaultOptions()
	opts.MaxLorebookRunes = 200
	f := newFixture(t, book(
		entry("e1", true, long, "长条目"),
		entry("e2", true, "短条目内容。", "钟楼"),
	), opts)

	got := f.systemPrompt(t, testRootID, "长条目与钟楼同时出现。")
	if strings.Contains(got, long[:40]) {
		t.Fatalf("超字符预算的长条目仍被注入:\n%s", got)
	}
	if !strings.Contains(got, "短条目内容。") {
		t.Fatalf("预算内短条目应被注入:\n%s", got)
	}
}

// 未导入世界书的会话：不出现世界设定段。
func TestLorebookAbsentSession(t *testing.T) {
	f := newFixture(t, nil, ctxpkg.DefaultOptions())
	got := f.systemPrompt(t, testRootID, "钟楼的门开着。")
	if strings.Contains(got, loreSectionHeader) {
		t.Fatalf("未导入世界书却出现世界设定段:\n%s", got)
	}
}

// 命中范围覆盖最近正文，而不只是当前输入。
func TestLorebookHitFromRecentHistory(t *testing.T) {
	f := newFixture(t, book(entry("e1", true, "钟楼每逢午夜会敲响十三下。", "钟楼")), ctxpkg.DefaultOptions())

	tc, _ := json.Marshal(domain.TurnContent{
		InputKind: "text", InputText: "我走向钟楼。",
		Blocks: []domain.TextBlock{{Kind: "narration", Text: "钟楼的影子被月光拉得很长。"}},
	})
	turn := &domain.PlotNode{
		NodeID: "node_turn1", SessionID: testSessionID, ParentID: testRootID,
		Kind: domain.NodeKindTurn, Depth: 1, TurnNumber: 1, SchemaVersion: 1,
		ContentJSON: string(tc),
	}
	if err := f.store.InsertNode(turn); err != nil {
		t.Fatalf("insert turn node: %v", err)
	}

	// 当前输入不含关键词，但历史正文含 "钟楼"。
	got := f.systemPrompt(t, turn.NodeID, "我停下来，什么也没做。")
	if !strings.Contains(got, "十三下") {
		t.Fatalf("历史正文中的关键词未触发注入:\n%s", got)
	}
	// 同一轮编译器仍应把历史作为消息带入。
	req, err := f.compiler.Compile(context.Background(), testSessionID, turn.NodeID, "我停下来。", nil, nil)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	var historySeen bool
	for _, m := range req.Messages {
		if m.Role == "assistant" && strings.Contains(m.Content, "钟楼的影子") {
			historySeen = true
		}
	}
	if !historySeen {
		t.Fatalf("历史消息缺失: %+v", req.Messages)
	}
}

// 世界书内容只进 system prompt，不进入历史消息（不污染正文）。
func TestLorebookNotInjectedIntoHistoryMessages(t *testing.T) {
	f := newFixture(t, book(entry("e1", true, "钟楼每逢午夜会敲响十三下。", "钟楼")), ctxpkg.DefaultOptions())
	req, err := f.compiler.Compile(context.Background(), testSessionID, testRootID, "钟楼前。", nil, nil)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	// 意图是"世界书不得混进历史对话"，因此只检查非 system 消息：
	// 启用前缀缓存分离后，动态 system 消息合法地出现在历史之后。
	for _, m := range req.Messages {
		if m.Role == "system" {
			continue
		}
		if strings.Contains(m.Content, "十三下") {
			t.Fatalf("世界书内容出现在非 system 消息中: %+v", m)
		}
	}
}

// 开场白属于故事上下文，其关键词同样触发注入（首轮对话无需重复关键词）。
func TestLorebookHitFromOpeningText(t *testing.T) {
	f := newFixtureWithOpening(t, book(
		entry("e1", true, "钟楼每逢午夜会敲响十三下。", "钟楼"),
	), "你在钟楼下睁开眼睛，头顶的铜钟静止不动。", ctxpkg.DefaultOptions())

	got := f.systemPrompt(t, testRootID, "我站起来，拍了拍身上的灰。")
	if !strings.Contains(got, "十三下") {
		t.Fatalf("开场白关键词未触发注入:\n%s", got)
	}
}

// ---- X5 / X6：提示词组装 ----

// X5：角色静态信息（人设）必须进入 system prompt。
// 来源限定为世界状态里的 CharacterInfo，秘密条目不在该结构中，
// 因此该注入路径在结构上无法泄漏未揭示的秘密。
func TestPersonaInjectedIntoSystemPrompt(t *testing.T) {
	f := newFixture(t, nil, ctxpkg.DefaultOptions())
	state := domain.NewWorldState()
	state.Characters["npc_bell"] = domain.CharacterInfo{
		CharacterID: "npc_bell", Name: "看护人",
		Description: "沉默寡言的守钟人，左手缺了一根小指。", Participant: true,
	}
	state.Characters["player"] = domain.CharacterInfo{
		CharacterID: "player", Name: testPlayerName, Participant: true,
	}

	sys := f.systemPromptWithState(t, testRootID, "我看着他。", state)
	if !strings.Contains(sys, "【角色设定】") {
		t.Fatalf("缺少角色设定段:\n%s", sys)
	}
	if !strings.Contains(sys, "左手缺了一根小指") {
		t.Fatalf("人设未注入:\n%s", sys)
	}
	if !strings.Contains(sys, "npc_bell") {
		t.Fatalf("角色 ID 未注入（模型需要它填 speakerId）:\n%s", sys)
	}
	// 玩家不是 NPC，其（空的）描述不应被当作角色设定。
	if strings.Contains(sys, "【角色设定】\n- 测试者") {
		t.Fatalf("玩家被误当作 NPC 注入:\n%s", sys)
	}
}

// X5：无描述的角色不应产生空条目。
func TestPersonaSkipsEmptyDescription(t *testing.T) {
	f := newFixture(t, nil, ctxpkg.DefaultOptions())
	state := domain.NewWorldState()
	state.Characters["npc_x"] = domain.CharacterInfo{CharacterID: "npc_x", Name: "X", Participant: true}
	if sys := f.systemPromptWithState(t, testRootID, "喂。", state); strings.Contains(sys, "【角色设定】") {
		t.Fatalf("空描述不应产生角色设定段:\n%s", sys)
	}
}

// X5：开场白以 assistant 消息进入上下文，作为故事的第一段正文。
func TestOpeningInjectedAsAssistantMessage(t *testing.T) {
	opening := "你推开门，冷风灌进衣领。"
	f := newFixtureWithOpening(t, nil, opening, ctxpkg.DefaultOptions())

	msgs := f.compileMessages(t, testRootID, "我走进去。", nil)
	if len(msgs) < 3 {
		t.Fatalf("消息过少: %+v", msgs)
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != opening {
		t.Fatalf("开场白未作为首条 assistant 消息注入: %+v", msgs[1])
	}
	if msgs[2].Role != "user" || !strings.HasSuffix(msgs[2].Content, "我走进去。") {
		t.Fatalf("当前输入位置异常: %+v", msgs[2])
	}
}

// X5：开场白排在历史之前，不能被截断掉。
func TestOpeningPrecedesHistory(t *testing.T) {
	opening := "故事的开头。"
	f := newFixtureWithOpening(t, nil, opening, ctxpkg.DefaultOptions())

	tc, _ := json.Marshal(domain.TurnContent{
		InputKind: "text", InputText: "第一轮输入",
		Blocks: []domain.TextBlock{{Kind: "narration", Text: "第一轮正文"}},
	})
	turn := &domain.PlotNode{
		NodeID: "node_t1", SessionID: testSessionID, ParentID: testRootID,
		Kind: domain.NodeKindTurn, Depth: 1, TurnNumber: 1, SchemaVersion: 1, ContentJSON: string(tc),
	}
	if err := f.store.InsertNode(turn); err != nil {
		t.Fatalf("insert: %v", err)
	}
	msgs := f.compileMessages(t, turn.NodeID, "第二轮输入", nil)
	if msgs[1].Content != opening {
		t.Fatalf("开场白应紧随 system: %+v", msgs[1])
	}
	if msgs[2].Content != "第一轮输入" || !strings.Contains(msgs[3].Content, "第一轮正文") {
		t.Fatalf("历史顺序异常: %+v", msgs[2:4])
	}
}

// X6：玩家名与身份来自根节点快照，不再是被硬编码的 "player"。
func TestPlayerIdentityFromSnapshot(t *testing.T) {
	f := newFixture(t, nil, ctxpkg.DefaultOptions())
	msgs := f.compileMessages(t, testRootID, "我走进去。", nil)

	sys := msgs[0].Content
	if !strings.Contains(sys, "【玩家】") || !strings.Contains(sys, testPlayerName) {
		t.Fatalf("玩家身份未注入 system prompt:/n%s", sys)
	}
	if !strings.Contains(sys, testPlayerRole) {
		t.Fatalf("玩家身份倾向未注入:\n%s", sys)
	}
	last := msgs[len(msgs)-1]
	if last.Role != "user" {
		t.Fatalf("末条消息应为 user: %+v", last)
	}
	if !strings.HasPrefix(last.Content, testPlayerName+"：") {
		t.Fatalf("玩家名未用于输入前缀（曾被硬编码为 player）: %q", last.Content)
	}
	if strings.Contains(last.Content, "player：") {
		t.Fatalf("仍在使用硬编码的 player 前缀: %q", last.Content)
	}
}

// X6：根节点缺少玩家快照时不注入玩家段，也不加前缀（不猜默认名）。
func TestPlayerIdentityAbsent(t *testing.T) {
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	rootContent, _ := json.Marshal(map[string]any{"templates": map[string]any{"lorebook": nil}})
	root := &domain.PlotNode{
		NodeID: testRootID, SessionID: testSessionID, Kind: domain.NodeKindRoot,
		SchemaVersion: 1, ContentJSON: string(rootContent),
	}
	sess := &domain.Session{SessionID: testSessionID, RootNodeID: testRootID, Title: "t"}
	branch := &domain.Branch{BranchID: testBranchID, SessionID: testSessionID, Name: "main", HeadNodeID: testRootID}
	if err := st.CreateSession(sess, root, branch, nil); err != nil {
		t.Fatalf("create session: %v", err)
	}
	f := &fixture{store: st, compiler: ctxpkg.New(st, ctxpkg.DefaultOptions())}
	msgs := f.compileMessages(t, testRootID, "我走进去。", nil)

	if strings.Contains(msgs[0].Content, "【玩家】") {
		t.Fatalf("无玩家快照时不应注入玩家段:\n%s", msgs[0].Content)
	}
	if msgs[len(msgs)-1].Content != "我走进去。" {
		t.Fatalf("无玩家名时不应加前缀: %q", msgs[len(msgs)-1].Content)
	}
}

// ---- C3：记忆召回 ----

// seedMemory 通过真实提交事务写入一条记忆并推进分支头。
// 记忆没有独立写入口（必须与产生它的节点同事务落库），所以测试也走 CommitTurn。
func (f *fixture) seedMemory(t *testing.T, headID string, m *domain.MemoryRecord) string {
	t.Helper()
	return f.seedMemoryOn(t, testBranchID, headID, m)
}

// seedMemoryOn 在指定分支上提交一条记忆（T17 用例需要在另一条分支上写入）。
func (f *fixture) seedMemoryOn(t *testing.T, branchID, headID string, m *domain.MemoryRecord) string {
	t.Helper()
	parent, err := f.store.GetNode(headID)
	if err != nil {
		t.Fatalf("parent: %v", err)
	}
	turnID := "turn_" + m.MemoryID
	if err := f.store.CreateTurnRequest(&domain.TurnRequest{
		TurnID: turnID, SessionID: testSessionID, BranchID: branchID,
		IdempotencyKey: turnID, PayloadHash: "h", ExpectedHeadID: headID,
		Status: domain.TurnQueued, Mode: "structured",
	}); err != nil {
		t.Fatalf("create turn: %v", err)
	}
	if ok, err := f.store.ClaimActiveTurn(branchID, turnID); err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	// 分支版本每次提交自增，提交前按当前值取 CAS 期望版本。
	branch, err := f.store.GetBranch(branchID)
	if err != nil {
		t.Fatalf("branch: %v", err)
	}
	nodeID := "node_" + m.MemoryID
	m.SourceNodeID = nodeID
	res, err := f.store.CommitTurn(&ports.CommitPlan{
		TurnID: turnID, ExpectedHeadID: headID, ExpectedVersion: branch.Version,
		RulesetVersion: "test",
		Node: &domain.PlotNode{
			NodeID: nodeID, SessionID: testSessionID, ParentID: headID,
			Kind: domain.NodeKindTurn, Depth: parent.Depth + 1, TurnNumber: parent.TurnNumber + 1,
			SchemaVersion: 1, ContentJSON: `{"blocks":[{"kind":"narration","text":"推进。"}]}`,
		},
		NewStateHash: domain.NewWorldState().HashID(), NewStateJSON: `{}`,
		Memories: []*domain.MemoryRecord{m},
	})
	if err != nil {
		t.Fatalf("commit memory: %v", err)
	}
	if !res.Committed {
		t.Fatalf("memory commit not applied: %+v", res)
	}
	return nodeID
}

func remember(id, content string, entityIDs ...string) *domain.MemoryRecord {
	return &domain.MemoryRecord{
		MemoryID: id, Kind: domain.MemoryObserved, Content: content, EntityIDs: entityIDs,
	}
}

// 内容相关 → 召回并注入「已知记忆」段。
func TestMemoryRecalledWhenRelevant(t *testing.T) {
	f := newFixture(t, nil, ctxpkg.DefaultOptions())
	head := f.seedMemory(t, testRootID, remember("m1", "她来自北方，很怕冷。"))

	got := f.systemPrompt(t, head, "你来自北方吗？")
	if !strings.Contains(got, "已知记忆") || !strings.Contains(got, "很怕冷") {
		t.Fatalf("相关记忆未注入:\n%s", got)
	}
}

// 编译结果必须带上本次注入的记忆清单：提交时据此判定 lastMeaningfulMentionTurn
// （契约 §8.1）。清单是"注入过"而不是"提到过"，因此没有被召回时必须是空的——
// 否则一次召回就会把 recency 永久抬高。
func TestCompileReportsInjectedMemoryIDs(t *testing.T) {
	f := newFixture(t, nil, ctxpkg.DefaultOptions())
	head := f.seedMemory(t, testRootID, remember("m1", "她来自北方，很怕冷。"))

	req, err := f.compiler.Compile(context.Background(), testSessionID, head, "你来自北方吗？", nil, nil)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(req.InjectedMemoryIDs) != 1 || req.InjectedMemoryIDs[0] != "m1" {
		t.Fatalf("注入清单 = %v, want [m1]", req.InjectedMemoryIDs)
	}

	req2, err := f.compiler.Compile(context.Background(), testSessionID, head, "我在路边买了些水果。", nil, nil)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(req2.InjectedMemoryIDs) != 0 {
		t.Fatalf("未召回却报告了注入清单: %v", req2.InjectedMemoryIDs)
	}
}

// 内容不相关 → 不注入（不把全部记忆无差别塞进提示词）。
func TestMemoryNotRecalledWhenIrrelevant(t *testing.T) {
	f := newFixture(t, nil, ctxpkg.DefaultOptions())
	head := f.seedMemory(t, testRootID, remember("m1", "她来自北方，很怕冷。"))

	got := f.systemPrompt(t, head, "今天天气不错，我去买了些水果。")
	if strings.Contains(got, "已知记忆") {
		t.Fatalf("不相关记忆不应注入:\n%s", got)
	}
}

// 置顶记忆无条件保留（用户显式要求）。
func TestPinnedMemoryAlwaysRecalled(t *testing.T) {
	f := newFixture(t, nil, ctxpkg.DefaultOptions())
	m := remember("m1", "她左手缺了一根小指。")
	m.Pinned = true
	head := f.seedMemory(t, testRootID, m)

	got := f.systemPrompt(t, head, "今天天气不错。")
	if !strings.Contains(got, "小指") {
		t.Fatalf("置顶记忆应无条件注入:\n%s", got)
	}
}

// 隐藏的记忆不进入上下文；置顶也不能绕过隐藏语义。
func TestHiddenMemoriesExcluded(t *testing.T) {
	f := newFixture(t, nil, ctxpkg.DefaultOptions())

	hidden := remember("m1", "她来自北方，很怕冷。")
	hidden.Hidden = true
	head := f.seedMemory(t, testRootID, hidden)

	pinnedHidden := remember("m4", "这条被隐藏但置顶了。")
	pinnedHidden.Hidden = true
	pinnedHidden.Pinned = true
	head = f.seedMemory(t, head, pinnedHidden)

	got := f.systemPrompt(t, head, "你来自北方吗？这条被隐藏但置顶了。")
	if strings.Contains(got, "已知记忆") {
		t.Fatalf("隐藏的记忆不应注入:\n%s", got)
	}
}

// 实体命中：记忆声明的实体的显示名出现在上下文中即召回，哪怕内容用词不同。
func TestMemoryEntityHit(t *testing.T) {
	f := newFixture(t, nil, ctxpkg.DefaultOptions())
	head := f.seedMemory(t, testRootID, remember("m1", "她说自己很少离开这里。", "npc_bell"))

	state := domain.NewWorldState()
	state.Characters["npc_bell"] = domain.CharacterInfo{
		CharacterID: "npc_bell", Name: "看护人", Participant: true,
	}
	got := f.systemPromptWithState(t, head, "我问看护人在不在。", state)
	if !strings.Contains(got, "很少离开这里") {
		t.Fatalf("实体命中未召回:\n%s", got)
	}

	// 实体没出现时，同样的记忆不应因实体命中被召回。
	irrelevant := f.systemPromptWithState(t, head, "我在路边买了些水果。", state)
	if strings.Contains(irrelevant, "很少离开这里") {
		t.Fatalf("实体未出现却被召回:\n%s", irrelevant)
	}
}

// 条数预算：超出上限的记忆被跳过。
func TestMemoryEntryBudget(t *testing.T) {
	opts := ctxpkg.DefaultOptions()
	opts.MaxMemories = 1
	f := newFixture(t, nil, opts)

	// 两条都与输入相关，按得分/ID 稳定取一条。
	head := f.seedMemory(t, testRootID, remember("m1", "北方的冬天很长。"))
	head = f.seedMemory(t, head, remember("m2", "北方的雪很大。"))

	got := f.systemPrompt(t, head, "北方的冬天和雪。")
	if !strings.Contains(got, "已知记忆") {
		t.Fatalf("至少应注入一条:\n%s", got)
	}
	count := 0
	for _, content := range []string{"北方的冬天很长。", "北方的雪很大。"} {
		if strings.Contains(got, content) {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("条数预算未生效:\n%s", got)
	}
}

// 字符预算：超预算的长记忆被跳过，短记忆仍可注入。
func TestMemoryRuneBudget(t *testing.T) {
	opts := ctxpkg.DefaultOptions()
	opts.MaxMemoryRunes = 60
	f := newFixture(t, nil, opts)

	long := "北方的冬天很长。" + strings.Repeat("补充说明北方的冬天。", 20)
	head := f.seedMemory(t, testRootID, remember("m1", long))
	head = f.seedMemory(t, head, remember("m2", "北方有极光。"))

	got := f.systemPrompt(t, head, "北方的冬天有极光吗？")
	if strings.Contains(got, "补充说明") {
		t.Fatalf("超字符预算的长记忆被注入:\n%s", got)
	}
	if !strings.Contains(got, "北方有极光") {
		t.Fatalf("预算内的短记忆应注入:\n%s", got)
	}
}

// 召回是只读的：同一输入重复编译得到同一结果，且不改变任何记忆状态（T18）。
func TestRecallIsReadOnlyAndDeterministic(t *testing.T) {
	f := newFixture(t, nil, ctxpkg.DefaultOptions())
	head := f.seedMemory(t, testRootID, remember("m1", "她来自北方，很怕冷。"))

	first := f.systemPrompt(t, head, "你来自北方吗？")
	second := f.systemPrompt(t, head, "你来自北方吗？")
	if first != second {
		t.Fatalf("重复编译结果不一致（应为确定性）")
	}
	mems, err := f.store.ListMemories(testSessionID)
	if err != nil || len(mems) != 1 {
		t.Fatalf("list: %d err=%v", len(mems), err)
	}
	m := mems[0]
	if m.Pinned || m.Hidden || m.Supersedes != "" {
		t.Fatalf("召回不应改变记忆状态: %+v", m)
	}
	if m.Confidence != 0 {
		t.Fatalf("召回不应提升置信度/权重: %v", m.Confidence)
	}
}

// T16/T17：分叉出去的分支看不到另一条路径上的记忆（编译层可见性）。
func TestMemoryBranchIsolationAtCompile(t *testing.T) {
	f := newFixture(t, nil, ctxpkg.DefaultOptions())
	aHead := f.seedMemory(t, testRootID, remember("mA", "她来自北方，很怕冷。"))

	// B 分支：从共同祖先 root 分出，提交自己的节点。
	if err := f.store.CreateBranch(&domain.Branch{
		BranchID: "branch_b", SessionID: testSessionID, Name: "b",
		HeadNodeID: testRootID, Version: 0,
	}); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	turnID := "turn_b"
	if err := f.store.CreateTurnRequest(&domain.TurnRequest{
		TurnID: turnID, SessionID: testSessionID, BranchID: "branch_b",
		IdempotencyKey: turnID, PayloadHash: "h", ExpectedHeadID: testRootID,
		Status: domain.TurnQueued, Mode: "structured",
	}); err != nil {
		t.Fatalf("create turn b: %v", err)
	}
	if ok, err := f.store.ClaimActiveTurn("branch_b", turnID); err != nil || !ok {
		t.Fatalf("claim b: %v %v", ok, err)
	}
	bMem := remember("mB", "她其实来自南方。")
	bMem.SourceNodeID = "node_b"
	res, err := f.store.CommitTurn(&ports.CommitPlan{
		TurnID: turnID, ExpectedHeadID: testRootID, ExpectedVersion: 0, RulesetVersion: "test",
		Node: &domain.PlotNode{
			NodeID: "node_b", SessionID: testSessionID, ParentID: testRootID,
			Kind: domain.NodeKindTurn, Depth: 1, TurnNumber: 1, SchemaVersion: 1, ContentJSON: `{}`,
		},
		NewStateHash: domain.NewWorldState().HashID(), NewStateJSON: `{}`,
		Memories: []*domain.MemoryRecord{bMem},
	})
	if err != nil || !res.Committed {
		t.Fatalf("commit b: %v %+v", err, res)
	}

	// A 路径：看得到 A 的记忆，看不到 B 的。
	aSys := f.systemPrompt(t, aHead, "她来自哪里？北方的冬天很冷。")
	if !strings.Contains(aSys, "很怕冷") {
		t.Fatalf("A 路径应看到自己的记忆:\n%s", aSys)
	}
	if strings.Contains(aSys, "其实来自南方") {
		t.Fatalf("A 路径不应看到 B 分支的记忆:\n%s", aSys)
	}

	// B 路径：看到 B 的记忆，看不到 A 的。
	bSys := f.systemPrompt(t, "node_b", "她来自哪里？南方还是北方？")
	if !strings.Contains(bSys, "其实来自南方") {
		t.Fatalf("B 路径应看到自己的记忆:\n%s", bSys)
	}
	if strings.Contains(bSys, "很怕冷") {
		t.Fatalf("B 路径不应看到 A 分支的记忆:\n%s", bSys)
	}
}

// 提示词必须列出当前可用且会被接受的提议类型。
// 此前只举了 relationship_delta 一例，真实模型因此从不产出 memory_add——
// 记忆闭环虽有召回能力，却永远没有内容可召回。
// 只列已授权的三类：列出未授权类型会诱导模型提出它们，而 buildPlan 会整轮拒绝。
func TestProtocolInstructionListsAuthorizedProposals(t *testing.T) {
	sys := newFixture(t, nil, ctxpkg.DefaultOptions()).systemPrompt(t, testRootID, "随便说点什么。")
	for _, want := range []string{
		"relationship_delta", "mood_set", "memory_add",
		"goal_set", "promise_propose",
	} {
		if !strings.Contains(sys, want) {
			t.Fatalf("协议说明缺少已授权的提议类型 %q:\n%s", want, sys)
		}
	}
	// 未授权的硬操作不得出现在提示词里（否则诱导模型产出会被整轮拒绝的提议）。
	for _, banned := range []string{"item_grant", "item_transfer", "item_consume", "secret_propose", "promise_settle", "scene_propose", "milestone_propose"} {
		if strings.Contains(sys, banned) {
			t.Fatalf("协议说明不应列出未授权类型 %q:\n%s", banned, sys)
		}
	}
	// 置信类别枚举必须与 domain.MemoryKind 一致，否则模型只能瞎猜。
	for _, kind := range []string{"observed", "reported", "inferred"} {
		if !strings.Contains(sys, kind) {
			t.Fatalf("协议说明缺少置信类别 %q:/n%s", kind, sys)
		}
	}
}

// copy-on-write：覆盖记录生效，被取代的原记录在本路径上不再召回。
func TestMemoryOverlayReplacesOriginal(t *testing.T) {
	f := newFixture(t, nil, ctxpkg.DefaultOptions())
	head := f.seedMemory(t, testRootID, remember("m1", "她来自北方，很怕冷。"))

	ov := remember("m2", "她来自南方，很怕冷。")
	ov.Supersedes = "m1"
	head = f.seedMemory(t, head, ov)

	got := f.systemPrompt(t, head, "她来自北方还是南方？她很怕冷吗？")
	if !strings.Contains(got, "来自南方") {
		t.Fatalf("覆盖版本未注入:\n%s", got)
	}
	if strings.Contains(got, "来自北方") {
		t.Fatalf("被取代的原记录仍被召回:\n%s", got)
	}
}

// T17：在 A 分支上纠正/隐藏共同来源的记忆，B 分支的对应记忆不受影响。
// 这是 copy-on-write 相对原地更新的核心价值——原地更新会同时改变 B 看到的记忆。
func TestMemoryOverlayIsPathScoped(t *testing.T) {
	f := newFixture(t, nil, ctxpkg.DefaultOptions())

	// 共同祖先上产生一条记忆（挂在 node_common 上，两条分支共享）。
	common := f.seedMemory(t, testRootID, remember("m1", "她来自北方，很怕冷。"))

	// B 分支从共同祖先分叉出去。
	if err := f.store.CreateBranch(&domain.Branch{
		BranchID: "branch_b", SessionID: testSessionID, Name: "b",
		HeadNodeID: common, Version: 0,
	}); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	bHead := f.seedMemoryOn(t, "branch_b", common, remember("mB", "B 分支的旁支记忆。"))

	// A 分支（branch_main）上纠正那条共同记忆：新建覆盖记录，不改原记录。
	ov := remember("m2", "她来自南方，很怕冷。")
	ov.Supersedes = "m1"
	aHead := f.seedMemory(t, common, ov)

	// A 路径：看到修订版，看不到原版。
	aSys := f.systemPrompt(t, aHead, "她来自北方还是南方？")
	if !strings.Contains(aSys, "来自南方") {
		t.Fatalf("A 路径应采用修订版:\n%s", aSys)
	}
	if strings.Contains(aSys, "来自北方") {
		t.Fatalf("A 路径不应再看到被取代的原记录:\n%s", aSys)
	}

	// B 路径：原记录不受 A 的修订影响（T17）。
	bSys := f.systemPrompt(t, bHead, "她来自北方还是南方？")
	if !strings.Contains(bSys, "来自北方") {
		t.Fatalf("B 路径的对应记忆不应受 A 的修订影响:\n%s", bSys)
	}
	if strings.Contains(bSys, "来自南方") {
		t.Fatalf("A 的覆盖记录不应泄漏到 B 路径:\n%s", bSys)
	}
}

// T17（隐藏）：A 隐藏共同来源的记忆，B 仍能召回。
func TestMemoryHideIsPathScoped(t *testing.T) {
	f := newFixture(t, nil, ctxpkg.DefaultOptions())
	common := f.seedMemory(t, testRootID, remember("m1", "她来自北方，很怕冷。"))

	if err := f.store.CreateBranch(&domain.Branch{
		BranchID: "branch_b", SessionID: testSessionID, Name: "b",
		HeadNodeID: common, Version: 0,
	}); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	bHead := f.seedMemoryOn(t, "branch_b", common, remember("mB", "B 的旁支记忆。"))

	// A 上隐藏：覆盖记录内容相同但带 hidden，原记录被它取代。
	hide := remember("m2", "她来自北方，很怕冷。")
	hide.Supersedes = "m1"
	hide.Hidden = true
	aHead := f.seedMemory(t, common, hide)

	aSys := f.systemPrompt(t, aHead, "她来自北方吗？很怕冷吗？")
	if strings.Contains(aSys, "很怕冷") {
		t.Fatalf("A 路径应已隐藏该记忆:\n%s", aSys)
	}
	bSys := f.systemPrompt(t, bHead, "她来自北方吗？很怕冷吗？")
	if !strings.Contains(bSys, "很怕冷") {
		t.Fatalf("B 路径不应受 A 的隐藏影响:\n%s", bSys)
	}
}

// 置顶同样按路径隔离：A 置顶不会让 B 无条件带上该记忆。
func TestMemoryPinIsPathScoped(t *testing.T) {
	f := newFixture(t, nil, ctxpkg.DefaultOptions())
	common := f.seedMemory(t, testRootID, remember("m1", "她来自北方，很怕冷。"))

	if err := f.store.CreateBranch(&domain.Branch{
		BranchID: "branch_b", SessionID: testSessionID, Name: "b",
		HeadNodeID: common, Version: 0,
	}); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	bHead := f.seedMemoryOn(t, "branch_b", common, remember("mB", "B 的旁支记忆。"))

	pin := remember("m2", "她来自北方，很怕冷。")
	pin.Supersedes = "m1"
	pin.Pinned = true
	aHead := f.seedMemory(t, common, pin)

	// A：与内容无关的输入也会带上（置顶）。
	aSys := f.systemPrompt(t, aHead, "今天天气不错，我去买了些水果。")
	if !strings.Contains(aSys, "很怕冷") {
		t.Fatalf("A 路径的置顶记忆应无条件注入:\n%s", aSys)
	}
	// B：同样的无关输入下不应带上（未置顶，且相关度为 0）。
	bSys := f.systemPrompt(t, bHead, "今天天气不错，我去买了些水果。")
	if strings.Contains(bSys, "很怕冷") {
		t.Fatalf("A 的置顶不应污染 B 路径:\n%s", bSys)
	}
}

// 检定结果是后端已掷骰的权威事实，必须原文注入——
// 只给结果模型描写不出过程，只给骰值等于把判定权又交回给模型。
func TestCheckResultsInjectedIntoPrompt(t *testing.T) {
	f := newFixture(t, nil, ctxpkg.DefaultOptions())

	prompt := f.systemPromptWithChecks(t, testRootID, "我去推那扇门。",
		[]domain.CheckResult{{
			RollID: "roll_x", ActionID: "check.strength", Attribute: "strength",
			AttributeVal: 18, AttributeMod: 4, Natural: 9, Total: 13, DC: 12,
			Outcome: domain.OutcomeSuccess, RulesetVer: "ruleset.simplified.v1",
		}})
	if !strings.Contains(prompt, "【检定结果】") {
		t.Fatalf("检定结果未注入:\n%s", prompt)
	}
	for _, want := range []string{"check.strength", "属性 18", "修正 +4", "骰值 9", "总值 13", "难度 12", "成功"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("提示词缺少 %q:/n%s", want, prompt)
		}
	}
	// 没有检定时不得出现该区块（避免暗示模型"这里该有个结果"）。
	plain := f.systemPrompt(t, testRootID, "我去推那扇门。")
	if strings.Contains(plain, "【检定结果】") {
		t.Fatalf("无检定却注入了检定区块:\n%s", plain)
	}
}

func TestSplitDynamicContext(t *testing.T) {
	opts := ctxpkg.DefaultOptions()
	opts.SplitDynamicContext = true
	books := book(domain.LorebookEntry{
		EntryID: "lb_clock", Enabled: true,
		Keys: []string{"钟楼"}, SecondaryKeys: []string{"老铜钟"},
		Content: "钟楼建于三百年以前。",
	})
	f := newFixtureWithOpening(t, books, "故事开始了，风雨交加。", opts)

	req, err := f.compiler.Compile(context.Background(), testSessionID, testRootID, "我看见了钟楼上的那座老铜钟。", nil, []domain.CheckResult{{
		RollID: "roll_1", ActionID: "check.perception", Attribute: "perception",
		AttributeVal: 15, AttributeMod: 2, Natural: 14, Total: 16, DC: 10,
		Outcome: domain.OutcomeSuccess, RulesetVer: "ruleset.simplified.v1",
	}})
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	// 消息顺序应为：
	// 0: system (静态设定)
	// 1: assistant (开场白)
	// 2: system (动态情境提示：钟楼设定、检定结果)
	// 3: user (最新玩家输入)
	if len(req.Messages) != 4 {
		t.Fatalf("期望 4 条消息，实际 %d 条: %+v", len(req.Messages), req.Messages)
	}

	staticSys := req.Messages[0].Content
	if !strings.Contains(staticSys, "你是本故事的叙述者") || !strings.Contains(staticSys, testPlayerName) {
		t.Fatalf("静态首条消息缺少基础设定:\n%s", staticSys)
	}
	if strings.Contains(staticSys, "钟楼建于三百年以前") || strings.Contains(staticSys, "【检定结果】") {
		t.Fatalf("静态首条消息不应包含动态世界书或检定结果:\n%s", staticSys)
	}

	dynSys := req.Messages[2]
	if dynSys.Role != "system" {
		t.Fatalf("第 3 条消息角色应为 system，实际: %s", dynSys.Role)
	}
	if !strings.Contains(dynSys.Content, "【当前情境与状态提示】") ||
		!strings.Contains(dynSys.Content, "钟楼建于三百年以前") ||
		!strings.Contains(dynSys.Content, "【检定结果】") {
		t.Fatalf("动态提示词内容不完整:\n%s", dynSys.Content)
	}
}

func TestLorebookSecondaryKeys(t *testing.T) {
	books := book(domain.LorebookEntry{
		EntryID: "lb_relic", Enabled: true,
		Keys: []string{"密宝"}, SecondaryKeys: []string{"月光石", "深渊之核"},
		Content: "月光石在夜间会发出幽蓝色的微光。",
	})
	f := newFixture(t, books, ctxpkg.DefaultOptions())

	// 1) 仅有次要关键词 -> 不应命中（SillyTavern selective 必须主次同时满足）
	p1 := f.systemPrompt(t, testRootID, "我从怀里掏出了月光石。")
	if strings.Contains(p1, "月光石在夜间会发出幽蓝色的微光") {
		t.Fatalf("仅有次要关键词不应命中世界书条目:\n%s", p1)
	}

	// 2) 仅有主要关键词 -> 不应命中
	p2 := f.systemPrompt(t, testRootID, "我找到了一件密宝。")
	if strings.Contains(p2, "月光石在夜间会发出幽蓝色的微光") {
		t.Fatalf("未出现次要关键词时不应命中世界书条目:\n%s", p2)
	}

	// 3) 主次关键词均出现 -> 命中
	p3 := f.systemPrompt(t, testRootID, "我找到了密宝月光石。")
	if !strings.Contains(p3, "月光石在夜间会发出幽蓝色的微光") {
		t.Fatalf("主次关键词同时出现时应命中世界书条目:\n%s", p3)
	}
}

// TestDefaultOptionsKeepCacheFriendlySplit 锁定"生产默认启用前缀缓存分离"。
//
// 这个开关此前默认 false，导致 KV Cache 优化在生产从未生效——代码注释写明
// 它是缓存优化，但只有 3 个测试文件把它置为 true。默认值一旦被改回 false，
// 缓存收益会静默消失（功能完全正常，只是变慢变贵），因此必须有断言守住。
//
// 有齿验证：把 DefaultOptions 里的 SplitDynamicContext: true 删掉即失败。
func TestDefaultOptionsKeepCacheFriendlySplit(t *testing.T) {
	if !ctxpkg.DefaultOptions().SplitDynamicContext {
		t.Fatal("DefaultOptions().SplitDynamicContext = false；生产默认必须启用前缀缓存分离，" +
			"否则合并模式下 system 内一旦出现差异，其后的开场白与全量历史都无法命中缓存")
	}
}

// TestDefaultOptionsSplitPlacesVolatileStateAfterHistory 验证默认配置下
// 易变内容确实排在历史之后：缓存前缀才能覆盖"静态 system + 开场白 + 历史"。
func TestDefaultOptionsSplitPlacesVolatileStateAfterHistory(t *testing.T) {
	f := newFixtureWithOpening(t, nil, "开场。", ctxpkg.DefaultOptions())
	req, err := f.compiler.Compile(context.Background(), testSessionID, testRootID, "继续。", nil, nil)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(req.Messages) < 2 {
		t.Fatalf("messages = %d, want >= 2", len(req.Messages))
	}
	// 首条必须是 system（静态），且不含动态段落标题。
	if req.Messages[0].Role != "system" {
		t.Fatalf("messages[0].Role = %q, want system", req.Messages[0].Role)
	}
	if strings.Contains(req.Messages[0].Content, "【当前情境与状态提示】") {
		t.Fatal("静态 system 不应包含动态情境段落（会破坏前缀稳定性）")
	}
	// 动态段落（若存在）必须落在历史之后，而不是被并进首条 system。
	// 空上下文下动态段可能为空（无世界书/记忆/检定/状态），此时跳过位置断言。
	for i, m := range req.Messages {
		if strings.Contains(m.Content, "【当前情境与状态提示】") {
			if i == 0 {
				t.Fatal("动态情境提示被并入首条 system，前缀缓存将失效")
			}
			return
		}
	}
}
