package application

import (
	"context"
	"encoding/json"
	"errors"

	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"tavernagent/internal/protocol"
	"tavernagent/internal/util/id"
)

// run 是回合处理流水线。
func (s *TurnService) run(ctx context.Context, turn *domain.TurnRequest) {
	turnID := turn.TurnID
	// 取消意图在流水线入口就检查：已被取消的回合不再做状态重放、规则检查与
	// 上下文编译——它们都不产生正文，只会让分支锁多占一段编译时间。
	// runGenerate 里还有一次检查，那是流式输出前的最后一道闸。
	if cancelled, err := s.store.HasCancelIntent(turnID); err != nil || cancelled {
		return
	}
	attemptID := "a_" + turnID + "_1"
	if !s.transition(ctx, turnID, domain.TurnPreparing, "", "", false) {
		return
	}
	_, _ = s.bus.Publish(turnID, "turn.started", map[string]any{"turnId": turnID, "branchId": turn.BranchID})

	// 基准状态取自身节点或其最近检查点 + 事件重放（稀疏检查点，技术契约 §7）。
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

	attempt := &domain.TurnAttempt{
		AttemptID: attemptID, TurnID: turnID, AttemptNo: 1,
		BaseHeadID: turn.ExpectedHeadID, BaseVersion: turn.ExpectedVersion,
	}
	if err := s.store.CreateAttempt(attempt); err != nil {
		s.fail(ctx, turnID, "STORAGE_UNAVAILABLE", err.Error(), true)
		return
	}
	ctx = withAttempt(ctx, attemptID)

	prov, compiler, provCfg, err := s.attemptProvider(turn.Mode)
	if err != nil {
		s.fail(ctx, turnID, "PROVIDER_UNAVAILABLE", err.Error(), true)
		return
	}
	checks, _, err := s.preparedChecks(turn)
	if err != nil {
		s.fail(ctx, turnID, "STORAGE_UNAVAILABLE", err.Error(), true)
		return
	}
	req, err := compiler.Compile(ctx, turn.SessionID, turn.ExpectedHeadID, inputTextOf(turn), directivesOf(turn), baseState, checks)
	if err != nil {
		s.fail(ctx, turnID, contextCodeOf(err), err.Error(), true)
		return
	}

	if !s.transition(ctx, turnID, domain.TurnGenerating, "", "", false) {
		return
	}
	if req.NeedsCompaction && s.compactor != nil {
		s.compactor.TriggerPressure(turn.SessionID, turn.BranchID, turn.ExpectedHeadID, req.ProtectedTurns)
	}
	s.runGenerate(ctx, turn, attemptID, req, baseState, prov, provCfg)
}

// calibratedCompiler 构造本次尝试的编译器：预算 + 真实用量校准 + 预算观测。
// 三者都作用在 attempt 副本上，共享的 compiler 不被修改。
func (s *TurnService) calibratedCompiler(cfg ports.ProviderConfig) *ctxpkg.Compiler {
	compiler := s.compiler.WithBudget(cfg.ContextWindow, cfg.MaxTokens).
		WithBudgetObserver(s.metrics.AddBudgetReport).
		WithPhaseObserver(s.metrics.AddPhase)
	if factor := s.tokenCalibrationFactor(); factor > 0 {
		compiler = compiler.WithTokenCalibration(factor)
	}
	return compiler
}

// attemptProvider 解析供应商与其配置。返回 cfg 供用量台账记录来源
// （slot / kind / model），否则台账无法区分成本来自哪个模型。
func (s *TurnService) attemptProvider(mode string) (ports.ModelProvider, *ctxpkg.Compiler, ports.ProviderConfig, error) {
	if s.manager != nil {
		p, cfg, err := s.manager.Resolve(mode, false)
		return p, s.calibratedCompiler(cfg), cfg, err
	}
	p, err := s.resolveProvider(mode)
	return p, s.calibratedCompiler(ports.ProviderConfig{Slot: mode}), ports.ProviderConfig{Slot: mode}, err
}

func (s *TurnService) runGenerate(ctx context.Context, turn *domain.TurnRequest, attemptID string, req ports.ChatRequest, baseState *domain.WorldState, prov ports.ModelProvider, provCfg ports.ProviderConfig) {
	turnID := turn.TurnID
	gctx, cleanup := s.registerCancel(turnID)
	defer cleanup()
	if cancelled, err := s.store.HasCancelIntent(turnID); err != nil || cancelled {
		return
	}

	parser := protocol.NewStreamParser()
	s.attachFrameSink(parser, attemptID, turnID)
	// 注册在途解析器：刷新或重连的客户端靠它取回当前块的实时文本。
	unregisterLive := s.registerLive(turnID, attemptID, parser)
	defer unregisterLive()

	streamCtx := ports.WithThinkingCallback(gctx, func(thinkingChunk string) {
		pb, err := json.Marshal(map[string]any{
			"attemptId": attemptID,
			"thinking":  thinkingChunk,
		})
		if err != nil {
			return
		}
		s.bus.BroadcastOnly(&domain.OutboxEvent{
			EventID:     id.New(),
			AggregateID: turnID,
			Type:        "turn.thinking",
			PayloadJSON: string(pb),
		})
	})

	// 用量采集（T0.1）：真实值来自供应商 usage，估算值来自编译器。
	// 两者都存，用于校准 token 估算；采集失败绝不影响回合提交。
	var usage ports.TokenUsage
	req.Task, req.TurnID, req.AttemptID = "generation", turnID, attemptID
	streamErr := prov.Stream(streamCtx, req, ports.StreamSink{
		OnChunk: func(chunk []byte) error { return parser.Feed(chunk) },
		OnUsage: func(u ports.TokenUsage) { usage = u },
	})
	s.afterStream(ctx, turn, attemptID, parser, baseState, streamErr, req.InjectedMemories, usage, req.EstimatedInputTokens, provCfg)
}

// attachFrameSink 注册帧落盘 + block 事件推送。
func (s *TurnService) attachFrameSink(parser *protocol.StreamParser, attemptID, turnID string) {
	parser.OnDelta(func(seq int, kind protocol.BlockKind, speakerID *string, delta string) error {
		pb, err := json.Marshal(map[string]any{
			"attemptId": attemptID,
			"seq":       seq,
			"kind":      string(kind),
			"speakerId": speakerID,
			"delta":     delta,
		})
		if err != nil {
			return nil
		}
		s.bus.BroadcastOnly(&domain.OutboxEvent{
			EventID:     id.New(),
			AggregateID: turnID,
			Type:        "block.delta",
			PayloadJSON: string(pb),
		})
		return nil
	})

	parser.OnFrame(func(frame any) error {
		seq := frameSeq(frame)
		payload := frameJSON(frame)
		if err := s.store.AppendDraftFrame(attemptID, seq, payload, hashString(payload)); err != nil {
			return err
		}
		if b, ok := frame.(protocol.BlockFrame); ok {
			_, _ = s.bus.Publish(turnID, "block.appended", map[string]any{
				"attemptId": attemptID, "frameSeq": seq, "frame": b,
			})
		}
		return nil
	})
}

// recordUsage 把一次模型调用的用量写入台账。
//
// 这是纯观测行为：写入失败只会被忽略，绝不改变回合的提交/失败语义。
// 真实值（prompt/completion/cached）与估算值（estimated）同时落库，
// 以便用真实读数校准编译侧的 token 估算。
func (s *TurnService) recordUsage(turnID, attemptID string, usage ports.TokenUsage, estimatedTokens int, provCfg ports.ProviderConfig) {
	if s.usage == nil || s.manager != nil {
		return
	}
	_ = s.usage.RecordTurnUsage(ports.TurnUsageRecord{
		TurnID:     turnID,
		AttemptID:  attemptID,
		Model:      provCfg.Model,
		Provider:   provCfg.Kind,
		Prompt:     usage.Prompt,
		Completion: usage.Completion,
		Cached:     usage.Cached,
		Estimated:  estimatedTokens,
		Reported:   usage.Reported,
	})
}

// afterStream 结算一次流式生成后的统一终止路径：
// 取消 > 协议错误 > 供应商截断/缺 final → 续写态 > 供应商错误 > 正常提交。
func (s *TurnService) afterStream(ctx context.Context, turn *domain.TurnRequest, attemptID string, parser *protocol.StreamParser, baseState *domain.WorldState, streamErr error, injected []domain.MemoryRef, usage ports.TokenUsage, estimatedTokens int, provCfg ports.ProviderConfig) {
	turnID := turn.TurnID

	// 用量台账：观测数据，先落地且**失败不影响回合提交**。
	s.recordUsage(turnID, attemptID, usage, estimatedTokens, provCfg)
	s.metrics.addMemories(len(injected))

	if isCancel, _ := s.store.HasCancelIntent(turnID); isCancel {
		rt, _ := s.store.GetTurn(turnID)
		if rt != nil && rt.Status == domain.TurnCommitted {
			s.bus.BroadcastOnly(newCommittedEvent(turnID))
			return
		}
		if s.transition(ctx, turnID, domain.TurnCancelled, "", "已取消", false) {
			s.metrics.addCancelled()
		}
		return
	}

	if streamErr != nil {
		if s.workCtx.Err() != nil {
			if _, err := parser.Finish(); err != nil {
				if len(parser.Blocks()) > 0 && parser.Mode() != protocol.ModeNarrative {
					s.enterContinuation(ctx, turnID, "服务已退出，草稿已保存；重启后可继续")
				} else {
					s.fail(ctx, turnID, "SERVER_SHUTTING_DOWN", "服务退出中断了生成，请重试", true)
				}
				return
			}
			// A complete final frame can still be committed before Close returns.
			streamErr = nil
		}
	}
	if streamErr != nil {
		var pval *protocol.ValidationError
		if errors.As(streamErr, &pval) || errors.Is(streamErr, protocol.ErrTooLarge) || errors.Is(streamErr, protocol.ErrAfterFinal) {
			s.fail(ctx, turnID, "PROTOCOL_INVALID", streamErr.Error(), false)
			return
		}
		// 供应商没按流式协议说话（2xx 但零事件）：没有草稿可续写，必须报
		// 供应商不可用，而不是让用户看到一个空草稿的续写态。
		if errors.Is(streamErr, ports.ErrNonStreamingResponse) {
			s.fail(ctx, turnID, "PROVIDER_UNAVAILABLE", streamErr.Error(), true)
			return
		}
		if errors.Is(streamErr, ports.ErrTruncatedStream) {
			// 兼容模式没有可重建的帧草稿，续写无从补齐：整轮可重试重来。
			if parser.Mode() == protocol.ModeNarrative {
				s.fail(ctx, turnID, "PROVIDER_TRUNCATED", "兼容模式下生成被截断，请重试本轮", true)
				return
			}
			s.enterContinuation(ctx, turnID, "供应商返回 length 截断")
			return
		}
		s.fail(ctx, turnID, "PROVIDER_UNAVAILABLE", streamErr.Error(), true)
		return
	}

	draft, finishErr := parser.Finish()
	if finishErr != nil {
		if errors.Is(finishErr, protocol.ErrMissingFinal) && len(parser.Blocks()) > 0 {
			reason := "已输出正文但未收到 final"
			if te := parser.TailError(); te != nil {
				// 尾行存在但被判定非法：把真实原因带出来，避免「未收到 final」误导排查。
				reason = "final 帧非法或缺失: " + te.Error()
			}
			s.enterContinuation(ctx, turnID, reason)
			return
		}
		s.fail(ctx, turnID, "PROTOCOL_INVALID", "结构化回合缺少 final 帧", false)
		return
	}
	_, _ = s.commitDraft(ctx, turn, draft, baseState, parser.Mode(), injected)
}

func (s *TurnService) fail(ctx context.Context, turnID, code, message string, retryable bool) {
	if !s.transition(ctx, turnID, domain.TurnFailed, code, message, retryable) {
		return
	}
	s.metrics.addFailed()
	if code == "CONTEXT_OVER_BUDGET" {
		s.metrics.addBudgetExceeded()
		// 承重摘要装不下是最伤体验的一类预算失败（模型会彻底失去一段历史），
		// 单独计数以便排障时与"输入太长/窗口太小"区分开。
		s.metrics.addBudgetUnfit()
	}
}
