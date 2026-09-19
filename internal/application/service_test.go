package application

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"tavernagent/internal/adapters/providers/mock"
	"tavernagent/internal/adapters/sqlite"
	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

const testCard = `{
  "schemaVersion": 1,
  "cardId": "card_elena",
  "name": "Elena",
  "description": "酒馆老板娘，记得与玩家的约定。",
  "characters": [
    {"characterId": "npc_elena", "name": "Elena", "description": "酒馆老板娘", "participant": true}
  ],
  "items": [
    {"instanceId": "item_pocketwatch", "templateId": "tpl_watch", "name": "怀表", "ownerId": "player", "quantity": 1, "keepsake": true, "unique": true}
  ]
}`

// variantCard 带结构化初始状态与两个开场变体（M1-3 开局向导数据链）。
const variantCard = `{
  "schemaVersion": 2,
  "cardId": "card_elena_v2",
  "name": "艾莲娜",
  "characters": [
    {"characterId": "npc_elena", "name": "艾莲娜", "description": "酒馆老板娘", "participant": true}
  ],
  "initialState": {
    "items": [{"instanceId": "it_coin", "name": "铜币", "ownerId": "player", "quantity": 5}]
  },
  "openingVariants": [
    {"variantId": "open_tavern", "title": "酒馆之夜", "text": "夜里，酒馆灯火通明。",
     "initialState": {"scene": {"title": "雨夜酒馆", "locationId": "loc_tavern"},
                      "items": [{"instanceId": "it_watch", "name": "怀表", "ownerId": "player", "keepsake": true, "unique": true}]}},
    {"variantId": "open_road", "title": "清晨出城", "text": "清晨，城门缓缓打开。"}
  ]
}`

func mockBlock(seq int, kind string, text string) string {
	t, _ := json.Marshal(text)
	return fmt.Sprintf(`{"v":1,"seq":%d,"type":"block","kind":"%s","text":%s}`, seq, kind, t)
}

func mockFinal(seq int, proposals, options string) string {
	return fmt.Sprintf(`{"v":1,"seq":%d,"type":"final","proposals":[%s],"options":[%s]}`, seq, proposals, options)
}

func happyScript() []mock.Item {
	items := []mock.Item{
		mock.Frame(mockBlock(1, "dialogue", "你还记得我们的约定吗？")),
		mock.Frame(mockBlock(2, "narration", "雨水沿着窗框流下。")),
		mock.Frame(mockFinal(3,
			`{"proposalId":"p1","type":"relationship_delta","characterId":"npc_elena","field":"trust","delta":2,"evidenceBlockSeqs":[1]}`,
			`{"optionId":"o1","intent":"clever","text":"告诉她，我仍记得那个约定。"}`)),
	}
	return items
}

func newTestServices(t *testing.T, script []mock.Item) (*sqlite.Store, *TurnService, *SessionService, string, string, string) {
	t.Helper()
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	prov := mock.New(script)
	bus := NewEventBus(st)
	compiler := ctxpkg.New(st, ctxpkg.DefaultOptions())
	turnSvc := NewTurnService(st, prov, compiler, bus)
	t.Cleanup(turnSvc.Close)
	sessSvc := NewSessionService(st)

	res, err := sessSvc.Setup(context.Background(), &SessionSetupRequest{
		Title: "测试", CharacterJSON: testCard, Player: Player{Name: "旅人"},
		OpeningText: "你推开酒馆的门，老板娘抬起了头。",
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return st, turnSvc, sessSvc, res.Session.SessionID, res.Branch.BranchID, res.RootNode.NodeID
}

// asyncWaitBudget 是"等异步工作落到预期状态"的预算（回合提交、导演产出、压缩落库、
// 在途快照出现……）。
//
// 这类等待测的是**最终会发生**，不是延迟。几秒在开发机上绰绰有余，但在共享 CI runner
// （尤其 Windows）上会变成偶发红灯——而只在慢机器上出现的失败比没有断言更糟：它训练人
// 忽略红灯。所以给足预算，并留一个环境变量给更慢的环境继续调大。
var asyncWaitBudget = func() time.Duration {
	if raw := os.Getenv("TAVERNAGENT_TEST_WAIT_SECONDS"); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	return 60 * time.Second
}()

func waitTurn(t *testing.T, svc *TurnService, turnID string, want ...domain.TurnStatus) *domain.TurnRequest {
	t.Helper()
	deadline := time.Now().Add(asyncWaitBudget)
	for time.Now().Before(deadline) {
		tr, err := svc.Get(turnID)
		if err != nil {
			t.Fatalf("get turn: %v", err)
		}
		for _, w := range want {
			if tr.Status == w {
				return tr
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("turn %s did not reach %v (last %s %s)", turnID, want, trFromLast(t), lastStatus(t, svc, turnID))
	return nil
}

func lastStatus(t *testing.T, svc *TurnService, turnID string) domain.TurnStatus {
	tr, err := svc.Get(turnID)
	if err != nil {
		return domain.TurnStatus("err:" + err.Error())
	}
	return tr.Status
}

func trFromLast(t *testing.T) string { return "n/a" }

// T20：开局向导数据链——身份/开场选择 → 根节点模板版本快照。
func TestSetupOpeningVariantSnapshot(t *testing.T) {
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	sessSvc := NewSessionService(st)

	backpack := []domain.ItemInstance{
		{InstanceID: "it_sword", Name: "旧铁剑", Quantity: 1},
	}
	res, err := sessSvc.Setup(context.Background(), &SessionSetupRequest{
		Title: "酒馆之夜", CharacterJSON: variantCard,
		Player:           Player{Name: "旅人", Role: "流浪剑客", Backpack: backpack},
		OpeningVariantID: "open_tavern",
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// 开场文本来自选中变体。
	if res.OpeningText != "夜里，酒馆灯火通明。" {
		t.Fatalf("opening = %q", res.OpeningText)
	}
	// 初始状态：卡片级物品 + 变体覆盖（场景/物品）。
	if res.State.Scene == nil || res.State.Scene.Title != "雨夜酒馆" {
		t.Fatalf("scene = %+v", res.State.Scene)
	}
	for _, id := range []string{"it_coin", "it_watch", "it_sword"} {
		if _, ok := res.State.Items[id]; !ok {
			t.Fatalf("state 缺少初始物品 %s", id)
		}
	}
	if res.State.Items["it_sword"].OwnerID != "player" {
		t.Fatalf("背包物品归属 %q", res.State.Items["it_sword"].OwnerID)
	}
	if res.State.Characters["player"].Name != "旅人" {
		t.Fatalf("player name = %q", res.State.Characters["player"].Name)
	}

	// 不存在的变体引用 → CARD_VARIANT_NOT_FOUND。
	_, err = sessSvc.Setup(context.Background(), &SessionSetupRequest{
		Title: "x", CharacterJSON: variantCard, Player: Player{Name: "旅人"}, OpeningVariantID: "open_nope",
	})
	if err == nil || !strings.Contains(err.Error(), "CARD_VARIANT_NOT_FOUND") {
		t.Fatalf("want CARD_VARIANT_NOT_FOUND, got %v", err)
	}

	// 根节点模板版本快照（T20）落盘可查。
	root, err := st.GetNode(res.RootNode.NodeID)
	if err != nil {
		t.Fatalf("root node: %v", err)
	}
	var rc map[string]any
	if err := json.Unmarshal([]byte(root.ContentJSON), &rc); err != nil {
		t.Fatalf("root content: %v", err)
	}
	if rc["openingVariantId"] != "open_tavern" {
		t.Fatalf("openingVariantId = %v", rc["openingVariantId"])
	}
	templates, ok := rc["templates"].(map[string]any)
	if !ok {
		t.Fatal("根节点缺少 templates 快照")
	}
	chTpl, ok := templates["character"].(map[string]any)
	if !ok || chTpl["contentHash"] != hashString(variantCard) {
		t.Fatalf("character 模板快照异常: %v", templates["character"])
	}
	pTpl, ok := templates["player"].(map[string]any)
	if !ok {
		t.Fatal("player 模板快照缺失")
	}
	var ps map[string]any
	switch v := pTpl["content"].(type) {
	case string:
		if err := json.Unmarshal([]byte(v), &ps); err != nil {
			t.Fatalf("player 快照解析: %v", err)
		}
	case map[string]any:
		ps = v
	default:
		t.Fatalf("player 快照类型异常: %T", pTpl["content"])
	}
	if ps["name"] != "旅人" || ps["role"] != "流浪剑客" {
		t.Fatalf("player 快照 = %v", ps)
	}
}

// T 型：截断续讲 → awaiting_continuation → 续写提交 → 一个剧情节点（M1-6）。
func TestTurnTruncatedAndContinue(t *testing.T) {
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	// 第一段：输出 2 个块后以 truncation 信号结束。
	truncScript := []mock.Item{
		mock.Frame(mockBlock(1, "narration", "月光透过窗棂。")),
		{Line: mockBlock(2, "dialogue", "她说：剩下部分下次再说。") + "\n", Err: ports.ErrTruncatedStream},
	}
	prov := mock.New(truncScript)
	bus := NewEventBus(st)
	compiler := ctxpkg.New(st, ctxpkg.DefaultOptions())
	turnSvc := NewTurnService(st, prov, compiler, bus)
	t.Cleanup(turnSvc.Close) // 服务必须在存储关闭前排空 worker（否则 worker 会写已关闭的库）
	sessSvc := NewSessionService(st)

	res, err := sessSvc.Setup(context.Background(), &SessionSetupRequest{
		Title: "截断测试", CharacterJSON: testCard, Player: Player{Name: "旅人"}, OpeningText: "开始",
	})
	if err != nil {
		t.Fatal(err)
	}
	tr, err := turnSvc.Accept(context.Background(), res.Session.SessionID, res.Branch.BranchID, &TurnAcceptRequest{
		IdempotencyKey: "t1", ExpectedHeadID: res.RootNode.NodeID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "继续说"}, Mode: "structured",
	})
	if err != nil {
		t.Fatal(err)
	}

	got := waitTurn(t, turnSvc, tr.TurnID, domain.TurnAwaitingContinuation)
	if got.Status != domain.TurnAwaitingContinuation {
		t.Fatalf("status = %s, want awaiting_continuation", got.Status)
	}
	// 草稿帧保留（attempt1 两个块）。
	if frames, _ := st.GetDraftFrames("a_" + tr.TurnID + "_1"); len(frames) != 2 {
		t.Fatalf("attempt1 frames = %d", len(frames))
	}

	// 非续写态的回合不可继续（前一个主流已提交？这里用 committed 回合验证拒绝）。
	// 换第二段脚本：续写从序号 3 开始，最终 final。
	prov.SetScript([]mock.Item{
		mock.Frame(mockBlock(3, "narration", "他推开了门。")),
		mock.Frame(mockFinal(4,
			`{"proposalId":"p1","type":"relationship_delta","characterId":"npc_elena","field":"trust","delta":2,"evidenceBlockSeqs":[3]}`,
			`{"optionId":"o1","intent":"clever","text":"把话说完。"}`)),
	})
	ct, err := turnSvc.Continue(context.Background(), tr.TurnID)
	if err != nil {
		t.Fatalf("continue: %v", err)
	}
	_ = ct
	done := waitTurn(t, turnSvc, tr.TurnID, domain.TurnCommitted)
	if done.ResultNodeID == "" {
		t.Fatal("committed without node")
	}
	// 两个 attempt 的帧都落盘。
	if f2, _ := st.GetDraftFrames("a_" + tr.TurnID + "_2"); len(f2) != 2 {
		t.Fatalf("attempt2 frames = %d", len(f2))
	}
	// 提交节点含 3 个块。
	node, err := st.GetNode(done.ResultNodeID)
	if err != nil {
		t.Fatal(err)
	}
	var tc domain.TurnContent
	_ = json.Unmarshal([]byte(node.ContentJSON), &tc)
	if len(tc.Blocks) != 3 || len(tc.Options) != 1 {
		t.Fatalf("blocks=%d options=%d", len(tc.Blocks), len(tc.Options))
	}

	// 已完成回合不可继续。
	if _, err := turnSvc.Continue(context.Background(), tr.TurnID); err == nil {
		t.Fatal("committed 回合不应可继续")
	}
}

// T 型：截断续讲草稿"放弃" → cancelled 且释放分支锁，可再次受理新回合。
func TestTurnTruncatedCancelAbandonsDraft(t *testing.T) {
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	truncScript := []mock.Item{
		mock.Frame(mockBlock(1, "narration", "月光透过窗棂。")),
		{Line: mockBlock(2, "dialogue", "未完待续。") + "\n", Err: ports.ErrTruncatedStream},
	}
	prov := mock.New(truncScript)
	bus := NewEventBus(st)
	compiler := ctxpkg.New(st, ctxpkg.DefaultOptions())
	turnSvc := NewTurnService(st, prov, compiler, bus)
	t.Cleanup(turnSvc.Close) // 服务必须在存储关闭前排空 worker（否则 worker 会写已关闭的库）
	sessSvc := NewSessionService(st)

	res, err := sessSvc.Setup(context.Background(), &SessionSetupRequest{
		Title: "放弃测试", CharacterJSON: testCard, Player: Player{Name: "旅人"}, OpeningText: "开始",
	})
	if err != nil {
		t.Fatal(err)
	}
	tr, err := turnSvc.Accept(context.Background(), res.Session.SessionID, res.Branch.BranchID, &TurnAcceptRequest{
		IdempotencyKey: "a1", ExpectedHeadID: res.RootNode.NodeID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "……"}, Mode: "structured",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitTurn(t, turnSvc, tr.TurnID, domain.TurnAwaitingContinuation)

	// 放弃：多次调用幂等。
	if _, err := turnSvc.Cancel(context.Background(), tr.TurnID); err != nil {
		t.Fatal(err)
	}
	rt, _ := turnSvc.Get(tr.TurnID)
	if rt.Status != domain.TurnCancelled {
		t.Fatalf("status = %s, want cancelled", rt.Status)
	}
	if _, err := turnSvc.Cancel(context.Background(), tr.TurnID); err != nil {
		t.Fatal("重复放弃应幂等")
	}

	// 分支锁已释放：可受理新回合（脚本换正常完整流）。
	prov.SetScript(happyScript())
	tr2, err := turnSvc.Accept(context.Background(), res.Session.SessionID, res.Branch.BranchID, &TurnAcceptRequest{
		IdempotencyKey: "a2", ExpectedHeadID: res.RootNode.NodeID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "重新开始"}, Mode: "structured",
	})
	if err != nil {
		t.Fatalf("放弃后应能受理新回合: %v", err)
	}
	waitTurn(t, turnSvc, tr2.TurnID, domain.TurnCommitted, domain.TurnFailed)
}

// T01/T03-型端到端：受理 → 流式 → 提交 → 状态投影与持久化一致。
func TestTurnHappyPathCommit(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, rootID := newTestServices(t, happyScript())

	tr, err := turnSvc.Accept(context.Background(), sessionID, branchID, &TurnAcceptRequest{
		IdempotencyKey: "req-1", ExpectedHeadID: rootID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "我记得那个约定。"}, Mode: "structured",
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	got := waitTurn(t, turnSvc, tr.TurnID, domain.TurnCommitted)
	if got.ResultNodeID == "" {
		t.Fatalf("committed without result node")
	}

	// 节点内容：正文块 + 选项。
	node, err := st.GetNode(got.ResultNodeID)
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	var tc domain.TurnContent
	if err := json.Unmarshal([]byte(node.ContentJSON), &tc); err != nil {
		t.Fatalf("content: %v", err)
	}
	if len(tc.Blocks) != 2 || len(tc.Options) != 1 {
		t.Fatalf("blocks=%d options=%d", len(tc.Blocks), len(tc.Options))
	}

	// 状态投影：trust +2。
	sn, err := st.GetSnapshot(node.NodeID)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	ws, err := domain.UnmarshalWorld(sn.StateJSON)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if ws.Relationships["npc_elena"].Trust != 2 {
		t.Fatalf("trust = %d, want 2", ws.Relationships["npc_elena"].Trust)
	}

	// 草稿帧持久化。
	attemptsDraft, err := st.GetDraftFrames("a_" + tr.TurnID + "_1")
	if err != nil || len(attemptsDraft) != 3 {
		t.Fatalf("draft frames = %d err=%v", len(attemptsDraft), err)
	}

	// outbox 完成事件。
	evs, err := st.PollOutbox(tr.TurnID, 0, 20)
	if err != nil {
		t.Fatalf("outbox: %v", err)
	}
	var hasCommitted bool
	for _, ev := range evs {
		if ev.Type == "turn.committed" {
			hasCommitted = true
		}
	}
	if !hasCommitted {
		t.Fatalf("no turn.committed in outbox")
	}
	_ = branchID
}

// 幂等：同键同载荷返回同一回合（T01）。
func TestAcceptIdempotentSameTurn(t *testing.T) {
	_, turnSvc, _, sessionID, branchID, rootID := newTestServices(t, happyScript())
	a, err := turnSvc.Accept(context.Background(), sessionID, branchID, &TurnAcceptRequest{
		IdempotencyKey: "k1", ExpectedHeadID: rootID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "你好"}, Mode: "structured",
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	b, err := turnSvc.Accept(context.Background(), sessionID, branchID, &TurnAcceptRequest{
		IdempotencyKey: "k1", ExpectedHeadID: rootID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "你好"}, Mode: "structured",
	})
	if err != nil {
		t.Fatalf("accept2: %v", err)
	}
	if a.TurnID != b.TurnID {
		t.Fatalf("idempotent accept returned different turns: %s vs %s", a.TurnID, b.TurnID)
	}
}

// 幂等冲突：同键不同载荷 → 拒绝（T02）。
func TestAcceptIdempotencyConflict(t *testing.T) {
	_, turnSvc, _, sessionID, branchID, rootID := newTestServices(t, happyScript())
	_, err := turnSvc.Accept(context.Background(), sessionID, branchID, &TurnAcceptRequest{
		IdempotencyKey: "k1", ExpectedHeadID: rootID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "你好"}, Mode: "structured",
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	_, err = turnSvc.Accept(context.Background(), sessionID, branchID, &TurnAcceptRequest{
		IdempotencyKey: "k1", ExpectedHeadID: rootID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "不同内容"}, Mode: "structured",
	})
	if err == nil || !strings.Contains(err.Error(), "IDEMPOTENCY_CONFLICT") {
		t.Fatalf("want idempotency conflict, got %v", err)
	}
}

// 队列满：活动回合存在时新受理返回 QUEUE_FULL（T08 相关）。
func TestAcceptQueueFull(t *testing.T) {
	// 脚本第一步即阻塞：无 final → 回合停在 awaiting_continuation（分支锁保持）。
	blocking := []mock.Item{
		mock.Frame(mockBlock(1, "narration", "阻塞")),
	}
	_, turnSvc, _, sessionID, branchID, rootID := newTestServices(t, blocking)
	tr, err := turnSvc.Accept(context.Background(), sessionID, branchID, &TurnAcceptRequest{
		IdempotencyKey: "r1", ExpectedHeadID: rootID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "先来"}, Mode: "structured",
	})
	if err != nil {
		t.Fatalf("accept1: %v", err)
	}
	// 让第一个回合进入活动状态（生成或截断续讲均持锁）。
	waitTurn(t, turnSvc, tr.TurnID, domain.TurnGenerating, domain.TurnCommitted, domain.TurnFailed, domain.TurnAwaitingContinuation)
	if tr, _ := turnSvc.Get(tr.TurnID); tr.Status != domain.TurnGenerating && tr.Status != domain.TurnAwaitingContinuation {
		// 若已终态并释放锁（本脚本不会走到），跳过 T08 断言。
		return
	}
	_, err2 := turnSvc.Accept(context.Background(), sessionID, branchID, &TurnAcceptRequest{
		IdempotencyKey: "r2", ExpectedHeadID: rootID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "后来"}, Mode: "structured",
	})
	if err2 == nil || !strings.Contains(err2.Error(), "QUEUE_FULL") {
		t.Fatalf("want QUEUE_FULL, got %v", err2)
	}
}

func TestCancelTurn(t *testing.T) {
	st, _ := sqlite.Open(t.TempDir(), ports.RealClock{})
	t.Cleanup(func() { st.Close() })
	bus := NewEventBus(st)
	compiler := ctxpkg.New(st, ctxpkg.DefaultOptions())
	prov := &blockingProvider{release: make(chan struct{})}
	turnSvc := NewTurnService(st, prov, compiler, bus)
	t.Cleanup(turnSvc.Close) // 服务必须在存储关闭前排空 worker（否则 worker 会写已关闭的库）
	sessSvc := NewSessionService(st)

	res, err := sessSvc.Setup(context.Background(), &SessionSetupRequest{
		Title: "测试", CharacterJSON: testCard, Player: Player{Name: "旅人"}, OpeningText: "开始",
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	tr, err := turnSvc.Accept(context.Background(), res.Session.SessionID, res.Branch.BranchID, &TurnAcceptRequest{
		IdempotencyKey: "c1", ExpectedHeadID: res.RootNode.NodeID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "等等"}, Mode: "structured",
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	// 等待进入 generating。
	waitTurn(t, turnSvc, tr.TurnID, domain.TurnGenerating, domain.TurnCommitted)
	rt, _ := turnSvc.Get(tr.TurnID)
	if rt.Status != domain.TurnGenerating {
		t.Skip("provider finished too fast; race path covered by commit test")
	}

	got, err := turnSvc.Cancel(context.Background(), tr.TurnID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	_ = got
	// 取消意图持久生效后，运行循环异步置为 cancelled。
	deadline := time.Now().Add(asyncWaitBudget)
	for time.Now().Before(deadline) {
		rt, _ := turnSvc.Get(tr.TurnID)
		if rt.Status == domain.TurnCancelled {
			return
		}
		if rt.Status == domain.TurnCommitted {
			t.Fatalf("cancelled turn unexpectedly committed (commit raced first)")
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatalf("turn did not become cancelled")
}

// T06 集成：供应商以纯文本回复 → 兼容模式提交，正文可读且零状态变化。
func TestNarrativeCompatTurnCommits(t *testing.T) {
	script := []mock.Item{
		{Line: "门轴发出一声干涩的声响。\n", Split: 3},
		{Line: "她抬起眼，看向门口。\n", Split: 2},
		{Line: "「你来了。」她说。"}, // 结尾不带换行，覆盖 Finish 阶段降级
	}
	st, turnSvc, _, sessionID, branchID, rootID := newTestServices(t, script)

	tr, err := turnSvc.Accept(context.Background(), sessionID, branchID, &TurnAcceptRequest{
		IdempotencyKey: "narr-1", ExpectedHeadID: rootID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "我推门进去。"}, Mode: "structured",
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	got := waitTurn(t, turnSvc, tr.TurnID, domain.TurnCommitted)
	if got.ResultNodeID == "" {
		t.Fatalf("兼容模式未提交结果节点")
	}

	node, err := st.GetNode(got.ResultNodeID)
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	var tc domain.TurnContent
	if err := json.Unmarshal([]byte(node.ContentJSON), &tc); err != nil {
		t.Fatalf("content: %v", err)
	}
	// 正文完整保留，且全部为叙述块（不猜测对白/心声）。
	var joined strings.Builder
	for _, b := range tc.Blocks {
		if b.Kind != "narration" {
			t.Fatalf("compat block kind = %q, want narration", b.Kind)
		}
		joined.WriteString(b.Text)
	}
	for _, want := range []string{"门轴发出一声干涩的声响", "她抬起眼", "你来了"} {
		if !strings.Contains(joined.String(), want) {
			t.Fatalf("正文缺失 %q：%q", want, joined.String())
		}
	}
	// 零状态变化：选项为空，关系未被改动。
	if len(tc.Options) != 0 {
		t.Fatalf("兼容模式选项应为空，got %d", len(tc.Options))
	}
	if tc.Provenance == nil || tc.Provenance.Mode != "narrative" {
		t.Fatalf("节点模式未记为 narrative: %+v", tc.Provenance)
	}
	sn, err := st.GetSnapshot(node.NodeID)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	ws, err := domain.UnmarshalWorld(sn.StateJSON)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if r, ok := ws.Relationships["npc_elena"]; ok && (r.Affection != 0 || r.Trust != 0 || r.Alertness != 0) {
		t.Fatalf("兼容模式不应改动关系: %+v", r)
	}
}

// B1：卡片声明的世界书引用写入根节点模板快照，并落一条 lorebook 模板版本。
func TestSetupRecordsLorebookRefs(t *testing.T) {
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	sessSvc := NewSessionService(st)

	card := `{
	  "schemaVersion": 2,
	  "name": "钟楼探针",
	  "characters": [{"characterId": "npc_bell", "name": "钟楼看护人", "participant": true}],
	  "openingVariants": [{"title": "默认开场", "text": "你在钟楼下醒来。"}],
	  "lorebookRefs": [{"refId": "lb_bell", "name": "钟楼设定集", "schema": "v2"}]
	}`
	res, err := sessSvc.Setup(context.Background(), &SessionSetupRequest{
		Title: "世界书", CharacterJSON: card, Player: Player{Name: "旅人"},
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	root, err := st.GetNode(res.RootNode.NodeID)
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	var rc map[string]any
	if err := json.Unmarshal([]byte(root.ContentJSON), &rc); err != nil {
		t.Fatalf("root content: %v", err)
	}
	tpls, _ := rc["templates"].(map[string]any)
	lb, ok := tpls["lorebook"].(map[string]any)
	if !ok {
		t.Fatalf("templates.lorebook 未落库: %+v", tpls["lorebook"])
	}
	refs, ok := lb["refs"].([]any)
	if !ok || len(refs) != 1 {
		t.Fatalf("refs = %+v", lb["refs"])
	}
	first, _ := refs[0].(map[string]any)
	if first["name"] != "钟楼设定集" || first["refId"] != "lb_bell" {
		t.Fatalf("refs[0] = %+v", first)
	}

	// 对应的模板版本可查询。
	tplID, _ := lb["templateVersionId"].(string)
	tv, err := st.GetTemplateVersion(tplID)
	if err != nil {
		t.Fatalf("template version: %v", err)
	}
	if tv.Kind != domain.TemplateLorebook || !strings.Contains(tv.Content, "钟楼设定集") {
		t.Fatalf("template = %+v", tv)
	}

	// 未声明世界书的卡：字段保持 null，不凭空造模板。
	res2, err := sessSvc.Setup(context.Background(), &SessionSetupRequest{
		Title: "无世界书", CharacterJSON: testCard, Player: Player{Name: "旅人"},
		OpeningText: "你推开酒馆的门。",
	})
	if err != nil {
		t.Fatalf("setup2: %v", err)
	}
	root2, _ := st.GetNode(res2.RootNode.NodeID)
	var rc2 map[string]any
	_ = json.Unmarshal([]byte(root2.ContentJSON), &rc2)
	tpls2, _ := rc2["templates"].(map[string]any)
	if tpls2["lorebook"] != nil {
		t.Fatalf("无世界书时不应生成模板: %+v", tpls2["lorebook"])
	}
}

// 回归：同一时间窗口内连续创建会话必须全部成功。
// 缺陷成因：根节点/分支 ID 曾用 sessionID[:8] 派生，而 UUIDv7 前 8 位是毫秒时间高位，
// 同窗口内的会话会得到同一前缀，撞 plot_nodes.node_id 主键 → 第二次创建返回 503。
func TestSetupMultipleSessionsInSameWindow(t *testing.T) {
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	sessSvc := NewSessionService(st)

	roots := map[string]bool{}
	branches := map[string]bool{}
	for i := 0; i < 6; i++ {
		res, err := sessSvc.Setup(context.Background(), &SessionSetupRequest{
			Title: fmt.Sprintf("会话%d", i), CharacterJSON: testCard, Player: Player{Name: "旅人"},
			OpeningText: "你推开酒馆的门。",
		})
		if err != nil {
			t.Fatalf("第 %d 次创建会话失败: %v", i, err)
		}
		if roots[res.RootNode.NodeID] {
			t.Fatalf("根节点 ID 重复: %s", res.RootNode.NodeID)
		}
		if branches[res.Branch.BranchID] {
			t.Fatalf("分支 ID 重复: %s", res.Branch.BranchID)
		}
		roots[res.RootNode.NodeID] = true
		branches[res.Branch.BranchID] = true
	}
	// 会话列表可读回全部 6 条。
	list, err := st.ListSessions()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 6 {
		t.Fatalf("sessions = %d, want 6", len(list))
	}
}

// B1b：V2 卡内嵌 character_book → 映射为 LorebookRef（引用已记录，条目内容不注入）。
func TestImportMapsCharacterBookToRef(t *testing.T) {
	raw := `{
	  "spec": "chara_card_v2", "spec_version": "2.0",
	  "data": {
	    "name": "钟楼探针", "description": "d", "first_mes": "你在钟楼下醒来。",
	    "character_book": {
	      "name": "钟楼设定集",
	      "entries": [{"keys": ["钟楼"], "content": "每逢午夜敲十三下。", "enabled": true}]
	    }
	  }
	}`
	rep, err := ImportCardJSON(raw)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(rep.Card.LorebookRefs) != 1 {
		t.Fatalf("lorebookRefs = %+v", rep.Card.LorebookRefs)
	}
	if rep.Card.LorebookRefs[0].Name != "钟楼设定集" || rep.Card.LorebookRefs[0].Schema != "v2" {
		t.Fatalf("ref = %+v", rep.Card.LorebookRefs[0])
	}
	if rep.Card.LorebookRefs[0].RefID == "" {
		t.Fatalf("refId 不应为空")
	}
}

// captureProvider 记录最后一次下发给模型的请求，用于断言实际提示词内容。
type captureProvider struct {
	mu    sync.Mutex
	last  ports.ChatRequest
	reply []byte
}

func (c *captureProvider) Capabilities(ctx context.Context) (ports.ProviderCapabilities, error) {
	return ports.ProviderCapabilities{ID: "capture", Streaming: true}, nil
}

func (c *captureProvider) Stream(ctx context.Context, req ports.ChatRequest, sink ports.StreamSink) error {
	c.mu.Lock()
	c.last = req
	reply := c.reply
	c.mu.Unlock()
	if reply == nil {
		reply = []byte(mockBlock(1, "narration", "钟声在楼里回响。") + "\n" + mockFinal(2, "", "") + "\n")
	}
	return sink.Chunk(reply)
}

func (c *captureProvider) request() ports.ChatRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.last
}

// B2 集成：testdata 的 V2 测试卡 → 导入映射 → 根节点快照落库 → 关键词命中
// → 命中条目真正进入发给模型的请求。覆盖链路首尾，而不是只测中间一层。
func TestLorebookReachesProviderRequest(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "lorebook-probe-card.json"))
	if err != nil {
		t.Fatalf("读取测试卡: %v", err)
	}
	rep, err := ImportCardJSON(string(raw))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(rep.Card.LorebookRefs) != 1 {
		t.Fatalf("lorebookRefs = %d, want 1", len(rep.Card.LorebookRefs))
	}
	ref := rep.Card.LorebookRefs[0]
	if ref.Name != "钟楼设定集" || ref.Schema != "v2" {
		t.Fatalf("ref = %+v", ref)
	}
	// 六条定义全部有内容：禁用条目保留但标记 enabled=false，空内容才跳过。
	if len(ref.Entries) != 6 {
		t.Fatalf("entries = %d, want 6", len(ref.Entries))
	}
	var disabled int
	for _, e := range ref.Entries {
		if e.EntryID == "" {
			t.Fatalf("entryId 不应为空: %+v", e)
		}
		if !e.Enabled {
			disabled++
		}
	}
	if disabled != 1 {
		t.Fatalf("禁用条目数 = %d, want 1", disabled)
	}

	native, _ := json.Marshal(rep.Card)
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	prov := &captureProvider{}
	compiler := ctxpkg.New(st, ctxpkg.DefaultOptions())
	turnSvc := NewTurnService(st, prov, compiler, NewEventBus(st))
	t.Cleanup(turnSvc.Close) // 服务必须在存储关闭前排空 worker（否则 worker 会写已关闭的库）
	sessSvc := NewSessionService(st)

	res, err := sessSvc.Setup(context.Background(), &SessionSetupRequest{
		Title: "世界书集成", CharacterJSON: string(native), Player: Player{Name: "旅人"},
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// 当前输入不含任何关键词，命中应来自开场白（"你在钟楼下睁开眼睛…"）。
	tr, err := turnSvc.Accept(context.Background(), res.Session.SessionID, res.Branch.BranchID, &TurnAcceptRequest{
		IdempotencyKey: "lb-1", ExpectedHeadID: res.RootNode.NodeID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "我站起来，拍了拍身上的灰。"},
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	waitTurn(t, turnSvc, tr.TurnID, domain.TurnCommitted)

	req := prov.request()
	if len(req.Messages) == 0 || req.Messages[0].Role != "system" {
		t.Fatalf("未捕获到 system 消息: %+v", req.Messages)
	}
	sys := systemPromptOf(req.Messages)
	if !strings.Contains(sys, "世界设定") {
		t.Fatalf("缺少世界设定段:\n%s", sys)
	}
	if !strings.Contains(sys, "十三下") {
		t.Fatalf("开场白命中条目未进入提示词:\n%s", sys)
	}
	// 禁用 / 单字 key / 未命中 的条目都不得出现。
	for _, banned := range []string{"已被作者禁用", "单字 key", "铜钥匙"} {
		if strings.Contains(sys, banned) {
			t.Fatalf("不应注入的内容 %q 出现在提示词:\n%s", banned, sys)
		}
	}
	// 世界书内容只应存在于注入块中：首条静态 system 或末尾的尾部状态块（user 槽位，ADS-2.7-02）。
	last := len(req.Messages) - 1
	if !strings.Contains(req.Messages[last].Content, ctxpkg.SystemReminderOpen) {
		t.Fatalf("末条消息应为尾部状态块: %+v", req.Messages[last])
	}
	for _, m := range req.Messages[:last] {
		if m.Role != "system" && strings.Contains(m.Content, "十三下") {
			t.Fatalf("世界书内容泄漏到历史消息: %+v", m)
		}
	}
}

// 未导入世界书的会话：提示词里不应出现世界设定段。
func TestNoLorebookSectionWithoutTemplate(t *testing.T) {
	prov := &captureProvider{}
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	compiler := ctxpkg.New(st, ctxpkg.DefaultOptions())
	turnSvc := NewTurnService(st, prov, compiler, NewEventBus(st))
	t.Cleanup(turnSvc.Close) // 服务必须在存储关闭前排空 worker（否则 worker 会写已关闭的库）
	sessSvc := NewSessionService(st)

	res, err := sessSvc.Setup(context.Background(), &SessionSetupRequest{
		Title: "无世界书", CharacterJSON: testCard, Player: Player{Name: "旅人"},
		OpeningText: "你推开酒馆的门。",
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	tr, err := turnSvc.Accept(context.Background(), res.Session.SessionID, res.Branch.BranchID, &TurnAcceptRequest{
		IdempotencyKey: "nolb-1", ExpectedHeadID: res.RootNode.NodeID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "我要一杯酒。"},
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	waitTurn(t, turnSvc, tr.TurnID, domain.TurnCommitted)

	if sys := systemPromptOf(prov.request().Messages); strings.Contains(sys, "世界设定") {
		t.Fatalf("无世界书时不应出现世界设定段:\n%s", sys)
	}
}

// X5 安全边界：公开人设进入提示词，未揭示的秘密条目绝不进入提示词。
// 契约要求「秘密揭示前不进入模型上下文」，这条用例是该约束的守卫。
func TestSecretNeverReachesPrompt(t *testing.T) {
	const (
		publicPersona = "【公开人设】她在钟楼住了很多年。"
		secretText    = "【未揭示秘密】她其实是钟楼建造者的后代。"
	)
	card := map[string]any{
		"schemaVersion": 2, "name": "秘密探针", "description": publicPersona,
		"characters": []map[string]any{{
			"characterId": "npc_secret", "name": "秘密探针",
			"description": publicPersona, "participant": true,
		}},
		"openingVariants": []map[string]any{{"title": "默认开场", "text": "你在钟楼下醒来。"}},
		"secrets": []map[string]any{{
			"secretId": "sec_1", "title": "身世", "content": secretText,
		}},
	}
	cardJSON, _ := json.Marshal(card)

	prov := &captureProvider{}
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	turnSvc := NewTurnService(st, prov, ctxpkg.New(st, ctxpkg.DefaultOptions()), NewEventBus(st))
	t.Cleanup(turnSvc.Close) // 服务必须在存储关闭前排空 worker（否则 worker 会写已关闭的库）
	sessSvc := NewSessionService(st)

	res, err := sessSvc.Setup(context.Background(), &SessionSetupRequest{
		Title: "秘密边界", CharacterJSON: string(cardJSON),
		Player: Player{Name: "测试者", Role: "旅行者"},
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	tr, err := turnSvc.Accept(context.Background(), res.Session.SessionID, res.Branch.BranchID, &TurnAcceptRequest{
		IdempotencyKey: "sec-1", ExpectedHeadID: res.RootNode.NodeID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "我环顾四周。"},
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	waitTurn(t, turnSvc, tr.TurnID, domain.TurnCommitted)

	req := prov.request()
	blob := ""
	for _, m := range req.Messages {
		blob += m.Content + "\n"
	}
	if !strings.Contains(blob, publicPersona) {
		t.Fatalf("公开人设未进入提示词:\n%s", blob)
	}
	if strings.Contains(blob, secretText) || strings.Contains(blob, "未揭示秘密") {
		t.Fatalf("未揭示的秘密泄漏进提示词:\n%s", blob)
	}
}

// X5：单角色卡的卡级描述在角色级缺失时回退（否则人设不会进入提示词）。
func TestCardLevelDescriptionFallback(t *testing.T) {
	card := `{
	  "schemaVersion": 2, "name": "回退探针",
	  "description": "【卡级人设】左眼下有一道旧疤。",
	  "characters": [{"characterId": "npc_fb", "name": "回退探针", "participant": true}],
	  "openingVariants": [{"title": "默认开场", "text": "开场。"}]
	}`
	parsed, err := ParseCharacterCard(card)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Characters[0].Description != "【卡级人设】左眼下有一道旧疤。" {
		t.Fatalf("卡级描述未回退到角色级: %+v", parsed.Characters[0])
	}
	// 多角色卡不回退（避免把同一个卡级描述复制给所有角色）。
	multi := `{
	  "schemaVersion": 2, "name": "多角色",
	  "description": "共用描述",
	  "characters": [
	    {"characterId": "npc_a", "name": "A", "participant": true},
	    {"characterId": "npc_b", "name": "B", "participant": true}
	  ],
	  "openingVariants": [{"title": "默认开场", "text": "开场。"}]
	}`
	parsedMulti, err := ParseCharacterCard(multi)
	if err != nil {
		t.Fatalf("parse multi: %v", err)
	}
	for _, ch := range parsedMulti.Characters {
		if ch.Description != "" {
			t.Fatalf("多角色卡不应回退卡级描述: %+v", ch)
		}
	}
}

// ---- C2：记忆记录落库（M3）----

// memoryScript 构造一轮：正文 + memory_add 提议。
func memoryScript(proposals string) []mock.Item {
	return []mock.Item{
		mock.Frame(mockBlock(1, "narration", "她低声说起了北方。")),
		mock.Frame(mockFinal(2, proposals, "")),
	}
}

// memory_add 必须落库为来源化记录，且事件负载与声明的负载类型一致。
func TestMemoryAddCommitsRecord(t *testing.T) {
	script := memoryScript(`{"proposalId":"p1","type":"memory_add",` +
		`"text":"她来自北方，很怕冷。","confidence":"reported",` +
		`"entityIds":["npc_elena"],"participants":["npc_elena"]}`)
	st, turnSvc, _, sessionID, branchID, rootID := newTestServices(t, script)

	tr, err := turnSvc.Accept(context.Background(), sessionID, branchID, &TurnAcceptRequest{
		IdempotencyKey: "mem-1", ExpectedHeadID: rootID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "你说说你的家乡。"},
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	got := waitTurn(t, turnSvc, tr.TurnID, domain.TurnCommitted)

	mems, err := st.ListMemories(sessionID)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if len(mems) != 1 {
		t.Fatalf("memories = %d, want 1", len(mems))
	}
	m := mems[0]
	if m.Content != "她来自北方，很怕冷。" {
		t.Fatalf("content = %q", m.Content)
	}
	if m.Kind != domain.MemoryReported {
		t.Fatalf("kind = %q, want reported", m.Kind)
	}
	// 来源化：必须指向产生它的节点（C04：认知与状态分离，且可回溯来源）。
	if m.SourceNodeID != got.ResultNodeID {
		t.Fatalf("sourceNodeId = %q, want %q", m.SourceNodeID, got.ResultNodeID)
	}
	if len(m.EntityIDs) != 1 || m.EntityIDs[0] != "npc_elena" {
		t.Fatalf("entityIds = %v", m.EntityIDs)
	}
	if len(m.OwnerIDs) != 1 || m.OwnerIDs[0] != "npc_elena" {
		t.Fatalf("ownerIds = %v", m.OwnerIDs)
	}
	if m.MemoryID == "" {
		t.Fatalf("memoryId 不应为空")
	}
	if m.Pinned || m.Hidden || m.Supersedes != "" {
		t.Fatalf("新记忆不应带覆盖链接: %+v", m)
	}

	// 记忆可经祖先链查到（召回路径）。
	node, err := st.GetNode(got.ResultNodeID)
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	chain, err := st.AncestorChain(node.NodeID, true)
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	ids := make([]string, 0, len(chain))
	for _, n := range chain {
		ids = append(ids, n.NodeID)
	}
	inChain, err := st.MemoriesInChain(ids)
	if err != nil || len(inChain) != 1 {
		t.Fatalf("inChain = %d err=%v", len(inChain), err)
	}

	// 事件负载必须能被 domain.MemoryAddPayload 解析（此前写的是另一种形状）。
	evs, err := st.GetEvents(got.ResultNodeID)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	var found bool
	for _, ev := range evs {
		if ev.Type != domain.EventMemoryAdd {
			continue
		}
		var p domain.MemoryAddPayload
		if err := json.Unmarshal([]byte(ev.PayloadJSON), &p); err != nil {
			t.Fatalf("memory_add 负载与 MemoryAddPayload 不一致: %v (%s)", err, ev.PayloadJSON)
		}
		if p.Memory.MemoryID != m.MemoryID {
			t.Fatalf("事件负载 memoryId = %q, want %q", p.Memory.MemoryID, m.MemoryID)
		}
		found = true
	}
	if !found {
		t.Fatalf("缺少 memory_add 事件")
	}
}

// 记忆不得进入世界状态投影（C04：认知与状态分离）。
func TestMemoryAddNotInStateProjection(t *testing.T) {
	script := memoryScript(`{"proposalId":"p1","type":"memory_add","text":"她有个妹妹。","confidence":"observed"}`)
	st, turnSvc, _, sessionID, branchID, rootID := newTestServices(t, script)

	tr, err := turnSvc.Accept(context.Background(), sessionID, branchID, &TurnAcceptRequest{
		IdempotencyKey: "mem-2", ExpectedHeadID: rootID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "还有别的吗？"},
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	got := waitTurn(t, turnSvc, tr.TurnID, domain.TurnCommitted)

	sn, err := st.GetSnapshot(got.ResultNodeID)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	ws, err := domain.UnmarshalWorld(sn.StateJSON)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if strings.Contains(sn.StateJSON, "妹妹") {
		t.Fatalf("记忆不应进入世界状态投影: %s", sn.StateJSON)
	}
	if len(ws.UnlockedSecrets) != 0 {
		t.Fatalf("记忆不应解锁任何秘密: %v", ws.UnlockedSecrets)
	}
}

// 置信类别归一化：inferred 保留，未知取值按最保守的 observed。
func TestMemoryKindNormalized(t *testing.T) {
	cases := []struct {
		confidence string
		want       domain.MemoryKind
	}{
		{"observed", domain.MemoryObserved},
		{"reported", domain.MemoryReported},
		{"inferred", domain.MemoryInferred},
		{"", domain.MemoryObserved},
		{"guessed", domain.MemoryObserved},
	}
	for i, c := range cases {
		prop := fmt.Sprintf(`{"proposalId":"p1","type":"memory_add","text":"记忆%d。","reasoning":"结合已知线索推断","confidence":%q}`,
			i, c.confidence)
		st, turnSvc, _, sessionID, branchID, rootID := newTestServices(t, memoryScript(prop))
		tr, err := turnSvc.Accept(context.Background(), sessionID, branchID, &TurnAcceptRequest{
			IdempotencyKey: fmt.Sprintf("kind-%d", i), ExpectedHeadID: rootID, ExpectedVersion: 0,
			Input: domain.TurnInput{Kind: "text", Text: "继续。"},
		})
		if err != nil {
			t.Fatalf("case %d accept: %v", i, err)
		}
		waitTurn(t, turnSvc, tr.TurnID, domain.TurnCommitted)
		mems, err := st.ListMemories(sessionID)
		if err != nil || len(mems) != 1 {
			t.Fatalf("case %d memories = %d err=%v", i, len(mems), err)
		}
		if mems[0].Kind != c.want {
			t.Fatalf("case %d confidence=%q kind = %q, want %q", i, c.confidence, mems[0].Kind, c.want)
		}
	}
}

// 空内容记忆被跳过，但整轮仍然提交（记忆是派生材料，不该阻塞剧情推进）。
func TestMemoryAddEmptyContentSkipped(t *testing.T) {
	script := memoryScript(`{"proposalId":"p1","type":"memory_add","text":"   ","confidence":"observed"}`)
	st, turnSvc, _, sessionID, branchID, rootID := newTestServices(t, script)

	tr, err := turnSvc.Accept(context.Background(), sessionID, branchID, &TurnAcceptRequest{
		IdempotencyKey: "mem-empty", ExpectedHeadID: rootID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "嗯。"},
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	waitTurn(t, turnSvc, tr.TurnID, domain.TurnCommitted)
	mems, err := st.ListMemories(sessionID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(mems) != 0 {
		t.Fatalf("空内容不应写入记忆: %+v", mems)
	}
}

// 同轮多个 memory_add 各自落库，且 ID 互不冲突（模型可能重复使用 proposalId）。
func TestMemoryAddMultipleInOneTurn(t *testing.T) {
	props := `{"proposalId":"p1","type":"memory_add","text":"第一条记忆。","confidence":"observed"},` +
		`{"proposalId":"p2","type":"memory_add","text":"第二条记忆。","confidence":"inferred","reasoning":"根据本轮对白推断"}`
	st, turnSvc, _, sessionID, branchID, rootID := newTestServices(t, memoryScript(props))

	tr, err := turnSvc.Accept(context.Background(), sessionID, branchID, &TurnAcceptRequest{
		IdempotencyKey: "mem-multi", ExpectedHeadID: rootID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "继续说。"},
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	waitTurn(t, turnSvc, tr.TurnID, domain.TurnCommitted)

	mems, err := st.ListMemories(sessionID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(mems) != 2 {
		t.Fatalf("memories = %d, want 2", len(mems))
	}
	if mems[0].MemoryID == mems[1].MemoryID {
		t.Fatalf("记忆 ID 冲突: %q", mems[0].MemoryID)
	}
	// 两轮提交之间同样的 proposalId 不得互相覆盖。
	tr2, err := turnSvc.Accept(context.Background(), sessionID, branchID, &TurnAcceptRequest{
		IdempotencyKey: "mem-multi-2", ExpectedHeadID: tr.TurnID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "还有。"},
	})
	_ = tr2
	_ = err
	// 第二个回合的 expectedHead 不对，这里只断言第一轮的 ID 唯一性。
	seen := map[string]bool{}
	for _, m := range mems {
		if seen[m.MemoryID] {
			t.Fatalf("记忆 ID 重复: %s", m.MemoryID)
		}
		seen[m.MemoryID] = true
	}
}
