package application

import (
	"context"
	"strings"
	"testing"

	"tavernagent/internal/adapters/providers/mock"
	"tavernagent/internal/domain"
)

// 回归：模型把整段帧输出挤在一行（无换行）时，回合必须正常提交，而不是被误判为
// 「结构化回合缺少 final 帧」。用户库里的失败回合一（provider=openai-responses、
// model=deepseek-flash）正是这个形态：completion_tokens 719，但 draft_frames 为 0。
func TestWholeTurnOnSingleLineCommits(t *testing.T) {
	oneLine := mockBlock(1, "narration", "门轴发出一声干涩的声响。") + mockFinal(2, "", "")
	_, turnSvc, _, sessionID, branchID, rootID := newTestServices(t, []mock.Item{{Line: oneLine}})

	tr, err := turnSvc.Accept(context.Background(), sessionID, branchID, &TurnAcceptRequest{
		IdempotencyKey: "single-line", ExpectedHeadID: rootID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "推门"}, Mode: "structured",
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	got := waitTurn(t, turnSvc, tr.TurnID, domain.TurnCommitted, domain.TurnFailed)
	if got.Status != domain.TurnCommitted {
		t.Fatalf("status = %s, failure = %s %s", got.Status, got.FailureCode, got.FailureMessage)
	}
}

// 空输出（模型只跑了思考、没吐正文）：归为可重试的供应商空输出，而不是不可重试的
// 协议错误——后者会让用户反复重试仍卡在同一处。
func TestEmptyModelOutputIsRetryableProviderFailure(t *testing.T) {
	_, turnSvc, _, sessionID, branchID, rootID := newTestServices(t, nil)

	tr, err := turnSvc.Accept(context.Background(), sessionID, branchID, &TurnAcceptRequest{
		IdempotencyKey: "empty", ExpectedHeadID: rootID, ExpectedVersion: 0,
		Input: domain.TurnInput{Kind: "text", Text: "在吗"}, Mode: "structured",
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	got := waitTurn(t, turnSvc, tr.TurnID, domain.TurnFailed)
	if got.FailureCode != "PROVIDER_EMPTY_OUTPUT" {
		t.Fatalf("failureCode = %q, want PROVIDER_EMPTY_OUTPUT", got.FailureCode)
	}
	if !strings.Contains(got.FailureMessage, "没有返回任何正文") {
		t.Fatalf("failureMessage = %q", got.FailureMessage)
	}
}
