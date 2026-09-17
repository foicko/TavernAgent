package application

import (
	"context"
	"encoding/json"
	"strings"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"tavernagent/internal/protocol"
	"tavernagent/internal/util/id"
)

// commitDraft 校验提议并提交（runGenerate / runContinue 共享尾部）。
// commitDraft 的 injectedMemoryIDs 是本次生成注入上下文的记忆（可为空）：
// 提交时据此判定 lastMeaningfulMentionTurn（技术契约 §8.1）。直接提交路径
// （SubmitBlocks）没有编译过程，因此传 nil。
func (s *TurnService) commitDraft(ctx context.Context, turn *domain.TurnRequest, draft protocol.TurnDraft, baseState *domain.WorldState, mode string, injectedMemoryIDs []string) (*domain.PlotNode, error) {
	parent, err := s.store.GetNode(turn.ExpectedHeadID)
	if err != nil {
		s.fail(ctx, turn.TurnID, "STATE_MISSING", "父节点缺失", false)
		return nil, err
	}
	newNode := &domain.PlotNode{
		NodeID: id.New(), SessionID: turn.SessionID, ParentID: turn.ExpectedHeadID,
		Kind: domain.NodeKindTurn, Depth: parent.Depth + 1, TurnNumber: parent.TurnNumber + 1,
		SchemaVersion: 1,
	}
	plan, err := s.buildCommitPlan(ctx, turn, draft, baseState, mode, newNode, injectedMemoryIDs)
	if err != nil {
		return nil, err
	}
	return s.settleCommit(ctx, turn, plan, newNode)
}

// buildCommitPlan 组装并校验提交计划：收据、规则、历史证据、记忆配额与状态推演。
// 这一层只读不写（除了填充 newNode 的内容），失败一律把回合落到失败终态。
func (s *TurnService) buildCommitPlan(ctx context.Context, turn *domain.TurnRequest, draft protocol.TurnDraft, baseState *domain.WorldState, mode string, newNode *domain.PlotNode, injectedMemoryIDs []string) (*ports.CommitPlan, error) {
	turnID := turn.TurnID
	// 先定节点 ID 再构建计划：记忆的 source_node_id 与 ID 都由它派生，
	// 保证记忆可回溯且 ID 全局唯一。
	checks, receiptIDs, err := s.preparedChecks(turn)
	if err != nil {
		s.fail(ctx, turnID, "STORAGE_UNAVAILABLE", err.Error(), true)
		return nil, err
	}
	rules, err := s.sessionRules(turn.SessionID)
	if err != nil {
		s.fail(ctx, turnID, "RULE_INVALID", err.Error(), false)
		return nil, err
	}
	historyExcerpts := s.historyExcerpts(turn)
	visibleMemories, memoryErr := s.store.ProjectedMemories(turn.ExpectedHeadID)
	if memoryErr != nil {
		s.fail(ctx, turnID, "STORAGE_UNAVAILABLE", memoryErr.Error(), true)
		return nil, memoryErr
	}
	pc := planContext{
		ruleset:         rules,
		RulesetVersion:  rulesetOf(turn),
		Secrets:         s.secretDefsOf(turn.SessionID),
		Checks:          checks,
		HistoryExcerpts: historyExcerpts,
		VisibleMemories: visibleMemories,
		CurrentTurn:     newNode.TurnNumber,
	}
	pd, err := buildPlan(baseState, draft, inputOf(turn), mode, newNode.NodeID, pc)
	if err != nil {
		s.fail(ctx, turnID, "OPERATION_REJECTED", err.Error(), false)
		return nil, err
	}
	newNode.ContentJSON = pd.ContentJSON
	director, err := s.store.DirectorAt(turn.ExpectedHeadID)
	if err != nil {
		s.fail(ctx, turnID, "STORAGE_UNAVAILABLE", err.Error(), true)
		return nil, err
	}
	if event := directorProgressEvent(director, draft.Director, pd.Blocks, newNode.NodeID); event != nil {
		pd.Events = append(pd.Events, event)
	}
	if err := s.applyMemoryQuota(pd, turn.ExpectedHeadID, newNode.NodeID); err != nil {
		s.fail(ctx, turnID, "STORAGE_UNAVAILABLE", err.Error(), true)
		return nil, err
	}

	for i, ev := range pd.Events {
		ev.EventID = id.New()
		ev.NodeID = newNode.NodeID
		ev.EventIndex = i
	}
	newStateJSON, _ := pd.NewState.Marshal()
	return &ports.CommitPlan{
		AttemptID: attemptFrom(ctx),
		TurnID:    turnID, ExpectedHeadID: turn.ExpectedHeadID,
		ExpectedVersion: turn.ExpectedVersion, BaseStateHash: baseState.HashID(),
		RulesetVersion: rulesetOf(turn), Node: newNode, Events: pd.Events,
		Memories:           pd.Memories,
		MentionedMemoryIDs: injectedMemoryIDs,
		NewStateHash:       pd.NewState.HashID(), NewStateJSON: newStateJSON,
		// 准备阶段掷的骰在这里生效。结算只改状态、不改结果；
		// 已经 committed 的收据不会被重复结算（契约 §5.2）。
		SettleReceipts: receiptIDs,
	}, nil
}

// historyExcerpts 收集开场与最近回合的原文片段，作为记忆/约定的引用证据来源。
func (s *TurnService) historyExcerpts(turn *domain.TurnRequest) []domain.EvidenceExcerpt {
	historyNodes, _ := s.store.RecentTurnNodes(turn.ExpectedHeadID, 20)
	var excerpts []domain.EvidenceExcerpt
	if sess, err := s.store.GetSession(turn.SessionID); err == nil && sess != nil && sess.RootNodeID != "" {
		if root, err := s.store.GetNode(sess.RootNodeID); err == nil && root != nil {
			var rc struct {
				OpeningText string `json:"openingText"`
			}
			if err := json.Unmarshal([]byte(root.ContentJSON), &rc); err == nil && strings.TrimSpace(rc.OpeningText) != "" {
				excerpts = append(excerpts, domain.EvidenceExcerpt{NodeID: root.NodeID, Text: rc.OpeningText})
			}
		}
	}
	for _, n := range historyNodes {
		var tc domain.TurnContent
		if err := json.Unmarshal([]byte(n.ContentJSON), &tc); err == nil {
			if tc.InputText != "" {
				excerpts = append(excerpts, domain.EvidenceExcerpt{NodeID: n.NodeID, Text: tc.InputText})
			}
			for _, b := range tc.Blocks {
				if b.Text != "" {
					excerpts = append(excerpts, domain.EvidenceExcerpt{NodeID: n.NodeID, Text: b.Text})
				}
			}
		}
	}
	return excerpts
}

// settleCommit 把计划推进到权威提交，并处理四种终态：
// 已提交/重复提交、被取消、冲突、未知。
func (s *TurnService) settleCommit(ctx context.Context, turn *domain.TurnRequest, plan *ports.CommitPlan, newNode *domain.PlotNode) (*domain.PlotNode, error) {
	turnID := turn.TurnID
	current, changed, err := s.store.TransitionTurn(ports.TurnTransition{TurnID: turnID, AttemptID: attemptFrom(ctx), Status: domain.TurnValidating})
	if err != nil {
		return nil, err
	}
	if !changed && (current.Status != domain.TurnValidating || (attemptFrom(ctx) != "" && current.ActiveAttemptID != attemptFrom(ctx))) {
		return nil, Err("TURN_NOT_ACTIVE", "回合已结束或生成尝试已变化", 409)
	}
	res, err := s.store.CommitTurn(plan)
	if err != nil {
		s.fail(ctx, turnID, "STORAGE_UNAVAILABLE", err.Error(), true)
		return nil, err
	}
	switch {
	case res.Committed || res.AlreadyDone:
		s.bus.BroadcastOnly(newCommittedEvent(turnID))
		if res.AlreadyDone {
			return s.store.GetNode(res.NewHeadID)
		}
		s.metrics.addCommitted()
		recovering, _ := ctx.Value(recoveryContextKey{}).(bool)
		if s.compactor != nil && !recovering {
			s.compactor.TriggerAsync(turn.SessionID, turn.BranchID, newNode.NodeID)
		}
		if s.cognitive != nil && !recovering {
			s.cognitive.TriggerAsync(turn.SessionID, turn.BranchID, newNode.NodeID, turn.TurnID)
		}
		return newNode, nil
	case res.ConflictCode == "CANCELLED":
		if s.transition(ctx, turnID, domain.TurnCancelled, "", "已取消", false) {
			s.metrics.addCancelled()
		}
		return nil, Err("TURN_CANCELLED", "回合已取消", 409)
	case res.ConflictCode != "":
		s.transition(ctx, turnID, domain.TurnConflicted, res.ConflictCode, res.ConflictCode, false)
		return nil, Err("COMMIT_CONFLICT", "提交冲突: "+res.ConflictCode, 409)
	default:
		s.fail(ctx, turnID, "UNKNOWN", "未知终态", true)
		return nil, Err("UNKNOWN", "未知终态", 500)
	}
}

// SubmitBlocksRequest 是"内容已定、直接提交"的请求（编辑助手正文走这条）。
type SubmitBlocksRequest struct {
	SessionID string
	BranchID  string
	HeadID    string
	Version   int64
	Blocks    []domain.TextBlock
	Input     domain.TurnInput
	Mode      string
}

// SubmitBlocks 走与普通回合**完全相同**的受理与提交路径，只是正文由调用方给定。
//
// 与 Accept 的唯一区别是"内容从哪来"：Accept 之后要调模型生成再解析，
// 这里直接拿给定正文构造草稿。受理、认领、构建计划、提交事务、终态事件
// 全部复用同一条实现。
//
// 早前 BranchService 自己复刻了一份精简版（少了 outbox 事件与失败终态），
// 构成"同一套不变量有两处实现"——那正是本函数要消除的。
func (s *TurnService) SubmitBlocks(ctx context.Context, req SubmitBlocksRequest) (*domain.PlotNode, *domain.TurnRequest, error) {
	if !s.beginWork() {
		return nil, nil, Err("SERVER_SHUTTING_DOWN", "服务正在退出，请稍后重试", 503)
	}
	defer s.workers.Done()
	baseSnap, err := s.store.StateAt(req.HeadID)
	if err != nil || baseSnap == nil {
		return nil, nil, Err("STATE_MISSING", "基准状态缺失", 500)
	}
	baseState, err := domain.UnmarshalWorld(baseSnap.StateJSON)
	if err != nil {
		return nil, nil, Err("STATE_CORRUPT", err.Error(), 500)
	}

	ruleset := RulesetVersion
	if sess, serr := s.store.GetSession(req.SessionID); serr == nil && sess.RulesetVersion != "" {
		ruleset = sess.RulesetVersion
	}
	turn := &domain.TurnRequest{
		TurnID: id.New(), SessionID: req.SessionID, BranchID: req.BranchID,
		IdempotencyKey: "direct_" + id.New(),
		PayloadHash:    hashString(req.SessionID + "|" + req.HeadID + "|" + req.Mode),
		ExpectedHeadID: req.HeadID, ExpectedVersion: req.Version,
		Status: domain.TurnQueued, Mode: req.Mode, RulesetVersion: ruleset,
	}
	if blob, jerr := json.Marshal(req.Input); jerr == nil {
		turn.InputJSON = string(blob)
	}
	if err := s.store.CreateTurnRequest(turn); err != nil {
		return nil, nil, Err("STORAGE_UNAVAILABLE", "受理失败: "+err.Error(), 503)
	}
	if ok, cerr := s.store.ClaimActiveTurn(req.BranchID, turn.TurnID); cerr != nil || !ok {
		_ = s.store.UpdateTurnResult(turn.TurnID, domain.TurnFailed, "", "QUEUE_FULL", "该分支已有进行中的回合")
		return nil, turn, Err("QUEUE_FULL", "该分支已有进行中的回合", 429)
	}

	node, err := s.commitDraft(ctx, turn, draftFromBlocks(req.Blocks), baseState, req.Mode, nil)
	if err != nil {
		return nil, turn, err
	}
	return node, turn, nil
}

// draftFromBlocks 把已定的正文块包装成解析器产物（提议与选项为空）。
// 走与模型输出相同的下游路径，因此提交语义完全一致。
func draftFromBlocks(blocks []domain.TextBlock) protocol.TurnDraft {
	draft := protocol.TurnDraft{
		Blocks:    make([]protocol.BlockFrame, 0, len(blocks)),
		Proposals: []protocol.Proposal{},
		Options:   []protocol.OptionFrame{},
	}
	for i, b := range blocks {
		var speaker *string
		if b.SpeakerID != "" {
			sp := b.SpeakerID
			speaker = &sp
		}
		draft.Blocks = append(draft.Blocks, protocol.BlockFrame{
			V: protocol.Version, Seq: i + 1, Type: protocol.FrameBlock,
			Kind: protocol.BlockKind(b.Kind), SpeakerID: speaker, Text: b.Text,
		})
	}
	return draft
}
