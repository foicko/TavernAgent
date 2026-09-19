package application

import (
	"context"
	"strconv"
	"strings"

	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"tavernagent/internal/protocol"
)

// enterContinuation 进入截断续讲态：草稿保留，分支锁定不释放。
func (s *TurnService) enterContinuation(ctx context.Context, turnID, reason string) {
	if s.transition(ctx, turnID, domain.TurnAwaitingContinuation, "", reason, true) {
		s.metrics.addContinuation()
	}
}

// Continue 续写同一个未提交草稿：恢复已持久化帧 → 再次生成补全 → 提交。
func (s *TurnService) Continue(ctx context.Context, turnID string) (*domain.TurnRequest, error) {
	if !s.beginWork() {
		return nil, Err("SERVER_SHUTTING_DOWN", "服务正在退出，请稍后重试", 503)
	}
	defer s.workers.Done()
	turn, err := s.store.GetTurn(turnID)
	if err != nil {
		return nil, Err("NOT_FOUND", "回合不存在", 404)
	}
	if turn.Status != domain.TurnAwaitingContinuation {
		return nil, Err("TURN_NOT_CONTINUABLE", "只有截断续讲状态的回合可以继续", 409)
	}
	ok, err := s.store.ResumeTurn(turnID)
	if err != nil {
		return nil, Err("STORAGE_UNAVAILABLE", "认领分支失败: "+err.Error(), 503)
	}
	if !ok {
		return nil, Err("TURN_NOT_CONTINUABLE", "回合已被继续、取消或分支已变化", 409)
	}
	turn.Status = domain.TurnPreparing
	s.workers.Add(1)
	go func() { defer s.workers.Done(); s.runContinue(s.workCtx, turn) }()
	return turn, nil
}

func (s *TurnService) runContinue(ctx context.Context, turn *domain.TurnRequest) {
	turnID := turn.TurnID
	ctx = withAttempt(ctx, turn.ActiveAttemptID)
	gctx, cleanup := s.registerCancel(turnID)
	defer cleanup()

	// 1. 从已持久化帧重建解析器（静默，不重复触发事件）。
	parser := protocol.NewStreamParser()
	attempts, err := s.store.ListAttempts(turnID)
	if err != nil {
		s.fail(ctx, turnID, "STORAGE_UNAVAILABLE", err.Error(), true)
		return
	}
	var previous strings.Builder
	lastSeq, attemptNo := 0, 1
	for _, a := range attempts {
		attemptNo = max(attemptNo, a.AttemptNo+1)
		frames, err := s.store.GetDraftFrames(a.AttemptID)
		if err != nil {
			s.fail(ctx, turnID, "STORAGE_UNAVAILABLE", err.Error(), true)
			return
		}
		for _, f := range frames {
			if err := parser.Feed(append([]byte(f.Payload), '\n')); err != nil {
				s.fail(ctx, turnID, "PROTOCOL_INVALID", "草稿重建失败: "+err.Error(), false)
				return
			}
			if f.FrameSeq > lastSeq {
				previous.WriteString(f.Payload + "\n")
				lastSeq = f.FrameSeq
			}
		}
	}

	// 2. 基准状态与上下文（稀疏检查点下由 StateAt 重放得到）。
	baseSnap, err := s.store.StateAt(turn.ExpectedHeadID)
	if err != nil || baseSnap == nil {
		s.fail(ctx, turnID, "STATE_MISSING", "基准状态缺失", false)
		return
	}
	baseState, err := domain.UnmarshalWorld(baseSnap.StateJSON)
	if err != nil {
		s.fail(ctx, turnID, "STATE_CORRUPT", err.Error(), false)
		return
	}
	prov, compiler, provCfg, err := s.attemptProvider(turn.Mode)
	if err != nil {
		s.fail(ctx, turnID, "PROVIDER_UNAVAILABLE", err.Error(), true)
		return
	}
	extra := []ports.ChatMessage{
		{Role: "assistant", Content: previous.String()},
		{Role: "user", Content: "（续写）上面的 assistant 消息是本次已持久化草稿。保持人物与情节连续，不重复已有内容。请从序号 " + strconv.Itoa(lastSeq+1) + " 继续输出后续帧，最后以 final 帧收尾。"},
	}
	extraTokens := ctxpkg.MessageTokens(extra)
	checks, _, err := s.preparedChecks(turn)
	if err != nil {
		s.fail(ctx, turnID, "STORAGE_UNAVAILABLE", err.Error(), true)
		return
	}
	req, err := compiler.ReserveInput(extraTokens).Compile(ctx, turn.SessionID, turn.ExpectedHeadID, inputTextOf(turn), directivesOf(turn), baseState, checks)
	if err != nil {
		s.fail(ctx, turnID, contextCodeOf(err), err.Error(), true)
		return
	}
	req.Messages = append(req.Messages, extra...)
	req.InputBudget += extraTokens
	req.EstimatedInputTokens += extraTokens

	// 3. 新尝试继续追加帧。
	attempt2 := "a_" + turnID + "_" + strconv.Itoa(attemptNo)
	attempt := &domain.TurnAttempt{AttemptID: attempt2, TurnID: turnID, AttemptNo: attemptNo, BaseHeadID: turn.ExpectedHeadID, BaseVersion: turn.ExpectedVersion}
	if err := s.store.CreateAttempt(attempt); err != nil {
		s.fail(ctx, turnID, "STORAGE_UNAVAILABLE", err.Error(), true)
		return
	}
	s.attachFrameSink(parser, attempt2, turnID)
	ctx = withAttempt(ctx, attempt2)
	if !s.transition(ctx, turnID, domain.TurnGenerating, "", "", false) {
		return
	}
	var usage ports.TokenUsage
	req.Task, req.TurnID, req.AttemptID = "continuation", turnID, attempt2
	streamErr := prov.Stream(gctx, req, ports.StreamSink{
		OnChunk: func(chunk []byte) error { return parser.Feed(chunk) },
		OnUsage: func(u ports.TokenUsage) { usage = u },
	})
	s.afterStream(ctx, turn, attempt2, parser, baseState, streamErr, req.InjectedMemories, usage, req.EstimatedInputTokens, provCfg)
}
