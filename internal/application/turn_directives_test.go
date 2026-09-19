package application

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"tavernagent/internal/adapters/providers/mock"
	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/domain"
)

// noteScript 同时给出一个选项与一条关系变化：用来验证「选项被清空、变化被记录」
// 这两件互相独立的事。
func noteScript() []mock.Item {
	return []mock.Item{
		mock.Frame(mockBlock(1, "narration", "她抬眼看了看你。")),
		mock.Frame(mockFinal(2,
			`{"proposalId":"p1","type":"relationship_delta","characterId":"npc_elena","field":"affection","delta":2}`,
			`{"optionId":"o1","intent":"clever","text":"我坐下。"}`)),
	}
}

func turnContentOf(t *testing.T, st *sqlite.Store, nodeID string) domain.TurnContent {
	t.Helper()
	node, err := st.GetNode(nodeID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	var tc domain.TurnContent
	if err := json.Unmarshal([]byte(node.ContentJSON), &tc); err != nil {
		t.Fatalf("unmarshal turn content: %v", err)
	}
	return tc
}

func acceptWithInput(t *testing.T, svc *TurnService, st *sqlite.Store, sessionID, branchID, key string, in domain.TurnInput) *domain.TurnContent {
	t.Helper()
	br, err := st.GetBranch(branchID)
	if err != nil {
		t.Fatalf("branch: %v", err)
	}
	tr, err := svc.Accept(context.Background(), sessionID, branchID, &TurnAcceptRequest{
		IdempotencyKey: key, ExpectedHeadID: br.HeadNodeID, ExpectedVersion: br.Version, Input: in,
	})
	if err != nil {
		t.Fatalf("accept %s: %v", key, err)
	}
	done := waitTurn(t, svc, tr.TurnID, domain.TurnCommitted)
	tc := turnContentOf(t, st, done.ResultNodeID)
	return &tc
}

// 玩家注记与选项模式必须随回合落库：回看时要能分清"当时发生了什么"与"当时要求
// 怎么演"，而不是只剩正文。同时 Changes 要由**已生效的事件**推导出来。
func TestPlayerNoteAndChangesArePersisted(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, noteScript())

	tc := acceptWithInput(t, turnSvc, st, sessionID, branchID, "note-1", domain.TurnInput{
		Kind: "text", Text: "我推门进去。", Note: "这次让 NPC 先开口，少写我的动作。",
	})

	if tc.InputNote != "这次让 NPC 先开口，少写我的动作。" {
		t.Fatalf("注记未落库: %q", tc.InputNote)
	}
	if tc.InputText != "我推门进去。" {
		t.Fatalf("输入被污染: %q", tc.InputText)
	}
	if tc.OptionsMode != domain.OptionsAuto {
		t.Fatalf("缺省选项模式应为 auto，实际 %q", tc.OptionsMode)
	}
	if len(tc.Changes) != 1 || tc.Changes[0].Kind != "relationship" || tc.Changes[0].Delta != 2 {
		t.Fatalf("变化摘要与事件不一致: %+v", tc.Changes)
	}
	if !strings.Contains(tc.Changes[0].Text, "+2") {
		t.Fatalf("变化摘要应显示实际生效的量: %+v", tc.Changes[0])
	}
}

// never 是硬保证：模型仍然写了选项也必须被清空。只靠提示词约定的话，
// 模型不听话时玩家看到的就是"我明明关了还在弹"。
func TestOptionsNeverStripsModelOptions(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, noteScript())

	off := acceptWithInput(t, turnSvc, st, sessionID, branchID, "opt-off", domain.TurnInput{
		Kind: "text", Text: "我站着不动。", Options: domain.OptionsNever,
	})
	if len(off.Options) != 0 {
		t.Fatalf("never 模式应清空选项，实际 %+v", off.Options)
	}
	if off.OptionsMode != domain.OptionsNever {
		t.Fatalf("模式未落库: %q", off.OptionsMode)
	}

	// 对照组：默认模式保留模型的选项，证明上一段不是"选项本来就没生成"。
	on := acceptWithInput(t, turnSvc, st, sessionID, branchID, "opt-auto", domain.TurnInput{
		Kind: "text", Text: "我站着不动。",
	})
	if len(on.Options) != 1 {
		t.Fatalf("auto 模式应保留模型给出的选项，实际 %+v", on.Options)
	}
}

// auto 模式不连续两轮都给选项：提示词里的判据在真实模型上不够硬，这条是确定性
// 兑现“只在关键节点弹出”的那一半——抉择之后必然要承接后果，那本身就不是新抉择点。
// always 明确要求每轮都给，应该不受这条限制。
func TestOptionsAutoSkipsConsecutiveTurns(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, noteScript())

	first := acceptWithInput(t, turnSvc, st, sessionID, branchID, "cool-1", domain.TurnInput{Kind: "text", Text: "我推门进去。"})
	if len(first.Options) != 1 {
		t.Fatalf("第一轮应呈现选项，实际 %+v", first.Options)
	}

	second := acceptWithInput(t, turnSvc, st, sessionID, branchID, "cool-2", domain.TurnInput{Kind: "text", Text: "我说了句话。"})
	if len(second.Options) != 0 {
		t.Fatalf("紧接着的一轮不应再摆选项，实际 %+v", second.Options)
	}
	if second.SuppressedOptions != 1 {
		t.Fatalf("收起台阶要留痕（读者分得出“本无抉择”与“系统没摆”），实际 %d", second.SuppressedOptions)
	}

	third := acceptWithInput(t, turnSvc, st, sessionID, branchID, "cool-3", domain.TurnInput{Kind: "text", Text: "我等着。"})
	if len(third.Options) != 1 {
		t.Fatalf("间隔一轮之后应恢复呈现，实际 %+v", third.Options)
	}

	fourth := acceptWithInput(t, turnSvc, st, sessionID, branchID, "cool-4", domain.TurnInput{Kind: "text", Text: "我抬起头。", Options: domain.OptionsAlways})
	if len(fourth.Options) != 1 {
		t.Fatalf("always 应绕过冷却，实际 %+v", fourth.Options)
	}
	if fourth.SuppressedOptions != 0 {
		t.Fatalf("always 不应记录被收起的选项，实际 %d", fourth.SuppressedOptions)
	}
}

// 演绎指令的取值错误必须明确报错：模式决定"要不要给玩家选择"，猜错了会直接
// 改变玩法体验，而客户端却以为自己设置生效了。
func TestTurnInputDirectiveValidation(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, affectionScript(1))
	br, _ := st.GetBranch(branchID)

	cases := []struct {
		name     string
		in       domain.TurnInput
		wantCode string
	}{
		{"注记过长", domain.TurnInput{Kind: "text", Text: "继续。", Note: strings.Repeat("字", 2001)}, "NOTE_TOO_LONG"},
		{"选项模式非法", domain.TurnInput{Kind: "text", Text: "继续。", Options: "sometimes"}, "BAD_REQUEST"},
	}
	for _, tc := range cases {
		_, err := turnSvc.Accept(context.Background(), sessionID, branchID, &TurnAcceptRequest{
			IdempotencyKey: "v-" + tc.name, ExpectedHeadID: br.HeadNodeID, ExpectedVersion: br.Version, Input: tc.in,
		})
		apiErr, ok := err.(*APIError)
		if !ok || apiErr.Code != tc.wantCode {
			t.Fatalf("%s: err = %v, want %s", tc.name, err, tc.wantCode)
		}
	}

	// 边界之内要放行：2000 个字符正是上限。
	ok := acceptWithInput(t, turnSvc, st, sessionID, branchID, "v-ok", domain.TurnInput{
		Kind: "text", Text: "继续。", Note: strings.Repeat("字", 2000),
	})
	if len([]rune(ok.InputNote)) != 2000 {
		t.Fatalf("上限内的注记应原样保留，实际 %d 字", len([]rune(ok.InputNote)))
	}
}
