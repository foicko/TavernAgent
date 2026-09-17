package application

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"tavernagent/internal/adapters/providers/mock"
	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/domain"
)

// ---- E4：选项过期、改写与动作引用（T19）----

// optionScript 返回一轮脚本：带一个选项，选项可携带动作引用。
func optionScript(actionRef string) []mock.Item {
	opt := fmt.Sprintf(`{"optionId":"o1","intent":"clever","text":"我走过去。","actionRef":%q}`, actionRef)
	return []mock.Item{
		mock.Frame(mockBlock(1, "narration", "她看着你。")),
		mock.Frame(mockFinal(2, "", opt)),
	}
}

// optionsOf 读取某节点已提交的选项。
func optionsOf(t *testing.T, st *sqlite.Store, nodeID string) []domain.Option {
	t.Helper()
	node, err := st.GetNode(nodeID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	var tc domain.TurnContent
	if err := json.Unmarshal([]byte(node.ContentJSON), &tc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return tc.Options
}

func acceptOption(t *testing.T, svc *TurnService, sessionID, branchID string, headID string, version int64, key string, in domain.TurnInput) (*domain.TurnRequest, error) {
	t.Helper()
	return svc.Accept(context.Background(), sessionID, branchID, &TurnAcceptRequest{
		IdempotencyKey: key, ExpectedHeadID: headID, ExpectedVersion: version, Input: in,
	})
}

// 过期选项被拒绝：分支头推进后，上一轮留下的选项不得执行。
func TestStaleOptionRejectedAfterHeadAdvanced(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, optionScript(""))
	first := acceptAndWait(t, turnSvc, st, sessionID, branchID, "o-1", "你好。")
	opts := optionsOf(t, st, first.ResultNodeID)
	if len(opts) == 0 {
		t.Fatalf("首轮应产生选项")
	}

	// 用首轮选项正常提交第二轮。
	second, err := acceptOption(t, turnSvc, sessionID, branchID, first.ResultNodeID, 1, "o-2", domain.TurnInput{
		Kind: "option", Text: opts[0].Text,
		OptionRef: &domain.OptionRef{NodeID: first.ResultNodeID, OptionID: opts[0].OptionID},
	})
	if err != nil {
		t.Fatalf("合法选项提交失败: %v", err)
	}
	waitTurn(t, turnSvc, second.TurnID, domain.TurnCommitted)

	// 分支头已推进；再用首轮选项 → 过期。
	_, err = acceptOption(t, turnSvc, sessionID, branchID, second.ResultNodeID, 2, "o-3", domain.TurnInput{
		Kind: "option", Text: opts[0].Text,
		OptionRef: &domain.OptionRef{NodeID: first.ResultNodeID, OptionID: opts[0].OptionID},
	})
	if err == nil {
		t.Fatalf("过期选项应被拒绝")
	}
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Code != "STALE_OPTION" {
		t.Fatalf("错误码 = %v, want STALE_OPTION", err)
	}
}

// 不存在的 optionId 被拒绝（不能凭空构造选项引用）。
func TestUnknownOptionIDRejected(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, optionScript(""))
	first := acceptAndWait(t, turnSvc, st, sessionID, branchID, "o-4", "你好。")

	_, err := acceptOption(t, turnSvc, sessionID, branchID, first.ResultNodeID, 1, "o-5", domain.TurnInput{
		Kind: "option", Text: "随便写点什么。",
		OptionRef: &domain.OptionRef{NodeID: first.ResultNodeID, OptionID: "o_nope"},
	})
	if err == nil {
		t.Fatalf("不存在的选项应被拒绝")
	}
	if apiErr, ok := err.(*APIError); !ok || apiErr.Code != "STALE_OPTION" {
		t.Fatalf("错误码 = %v, want STALE_OPTION", err)
	}
}

// 改写后的选项按自由输入处理：不沿用原选项引用。
func TestRewrittenOptionBecomesFreeInput(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, optionScript(""))
	first := acceptAndWait(t, turnSvc, st, sessionID, branchID, "o-6", "你好。")
	opts := optionsOf(t, st, first.ResultNodeID)

	tr, err := acceptOption(t, turnSvc, sessionID, branchID, first.ResultNodeID, 1, "o-7", domain.TurnInput{
		Kind: "option", Text: "我转身走出门去。",
		OptionRef: &domain.OptionRef{NodeID: first.ResultNodeID, OptionID: opts[0].OptionID},
	})
	if err != nil {
		t.Fatalf("改写选项应被受理为自由输入: %v", err)
	}
	got, err := st.GetTurn(tr.TurnID)
	if err != nil {
		t.Fatalf("get turn: %v", err)
	}
	in := inputOf(got)
	if in.Kind != "text" {
		t.Fatalf("改写应降级为 text，实际 %q", in.Kind)
	}
	if in.OptionRef != nil {
		t.Fatalf("改写后不应保留选项引用: %+v", in.OptionRef)
	}
	if in.Text != "我转身走出门去。" {
		t.Fatalf("输入文本 = %q", in.Text)
	}
}

// 选项文本以服务端保存的原文为准，不信任客户端回传。
func TestOptionTextTakenFromServer(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, optionScript(""))
	first := acceptAndWait(t, turnSvc, st, sessionID, branchID, "o-8", "你好。")
	opts := optionsOf(t, st, first.ResultNodeID)

	// 文本与服务端一致（通过校验）时，落库文本仍是服务端原文。
	tr, err := acceptOption(t, turnSvc, sessionID, branchID, first.ResultNodeID, 1, "o-9", domain.TurnInput{
		Kind: "option", Text: opts[0].Text,
		OptionRef: &domain.OptionRef{NodeID: first.ResultNodeID, OptionID: opts[0].OptionID},
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	got, _ := st.GetTurn(tr.TurnID)
	in := inputOf(got)
	if in.Text != opts[0].Text {
		t.Fatalf("落库文本 = %q, want %q", in.Text, opts[0].Text)
	}
	if in.OptionRef == nil || in.OptionRef.OptionID != opts[0].OptionID {
		t.Fatalf("选项引用丢失: %+v", in.OptionRef)
	}
}

// 未授权的 actionRef 不写入选项：模型自造的动作不能进入授权面（技术契约 §4）。
func TestUnauthorizedActionRefStripped(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, optionScript("exec_arbitrary_code"))
	first := acceptAndWait(t, turnSvc, st, sessionID, branchID, "o-10", "你好。")

	opts := optionsOf(t, st, first.ResultNodeID)
	if len(opts) == 0 {
		t.Fatalf("首轮应产生选项")
	}
	if opts[0].ActionRef != "" {
		t.Fatalf("未授权的 actionRef 应被清除，实际保留 %q", opts[0].ActionRef)
	}
	if opts[0].Text != "我走过去。" {
		t.Fatalf("清除动作引用不应影响选项文本: %q", opts[0].Text)
	}
}

// 缺少引用或引用缺失节点的选项输入被拒绝。
func TestOptionWithoutRefRejected(t *testing.T) {
	st, turnSvc, _, sessionID, branchID, _ := newTestServices(t, optionScript(""))
	first := acceptAndWait(t, turnSvc, st, sessionID, branchID, "o-11", "你好。")

	if _, err := acceptOption(t, turnSvc, sessionID, branchID, first.ResultNodeID, 1, "o-12", domain.TurnInput{
		Kind: "option", Text: "x",
	}); err == nil {
		t.Fatalf("缺少选项引用应被拒绝")
	}
	if _, err := acceptOption(t, turnSvc, sessionID, branchID, first.ResultNodeID, 1, "o-13", domain.TurnInput{
		Kind: "option", Text: "x",
		OptionRef: &domain.OptionRef{NodeID: "node_missing", OptionID: "o1"},
	}); err == nil {
		t.Fatalf("引用不存在节点应被拒绝")
	}
}
