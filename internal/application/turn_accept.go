package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"tavernagent/internal/domain"
	"tavernagent/internal/util/id"
)

// TurnAcceptRequest 是回合受理输入。
type TurnAcceptRequest struct {
	IdempotencyKey  string
	ExpectedHeadID  string
	ExpectedVersion int64
	Input           domain.TurnInput
	AfterTurnID     string
	Mode            string
	Recheck         bool
	ReuseRollID     string // set only by a validated derivation
	DeriveNodeID    string
	// ExpectedCharacterID 是客户端声明的角色卡 ID（M4l 双向校验，契约 §11.2）。
	// 空表示不校验（旧客户端兼容）。
	ExpectedCharacterID string
}

// acceptPrepared 是一次受理通过校验后的产物。
type acceptPrepared struct {
	input       domain.TurnInput
	matched     *domain.Option
	existing    *domain.TurnRequest // 非 nil = 幂等重放，直接返回该回合
	payloadHash string
	inputJSON   string
	sess        *domain.Session
	// baseHeadID/baseVersion 是本次回合真正追加到的位置（受理时解析出的分支头）。
	// 它与客户端声明的 base 可能差几个维护节点，见 headMatchesClient。
	baseHeadID  string
	baseVersion int64
}

// Accept 受理回合并调度执行。幂等键相同且载荷一致时返回既有回合（T01）。
func (s *TurnService) Accept(ctx context.Context, sessionID, branchID string, req *TurnAcceptRequest) (*domain.TurnRequest, error) {
	if !s.beginWork() {
		return nil, Err("SERVER_SHUTTING_DOWN", "服务正在退出，请稍后重试", 503)
	}
	defer s.workers.Done()
	s.acceptMu.Lock()
	defer s.acceptMu.Unlock()
	if s.workCtx.Err() != nil {
		return nil, Err("SERVER_SHUTTING_DOWN", "服务正在退出，请稍后重试", 503)
	}
	prep, err := s.acceptPreflight(sessionID, branchID, req)
	if err != nil {
		return nil, err
	}
	if prep.existing != nil {
		return prep.existing, nil
	}
	turn, err := s.persistAcceptedTurn(ctx, sessionID, branchID, req, prep)
	if err != nil {
		return nil, err
	}
	s.dispatchTurn(branchID, turn)
	return turn, nil
}

// acceptPreflight 完成受理前的全部校验：分支归属、角色绑定、幂等重放、
// 输入归一化与分支头/版本冲突。返回的 existing 非 nil 表示这是一次重放。
func (s *TurnService) acceptPreflight(sessionID, branchID string, req *TurnAcceptRequest) (*acceptPrepared, error) {
	if req == nil || req.IdempotencyKey == "" {
		return nil, Err("BAD_REQUEST", "缺少回合请求或幂等键", 400)
	}
	branch, err := s.store.GetBranch(branchID)
	if err != nil {
		return nil, Err("NOT_FOUND", "分支不存在", 404)
	}
	if branch.SessionID != sessionID {
		return nil, Err("BRANCH_SESSION_MISMATCH", "分支不属于该会话", 400)
	}
	sess, err := requireCharacter(s.store, sessionID, req.ExpectedCharacterID)
	if err != nil {
		return nil, err
	}
	existing, err := s.store.FindTurnByIdempotency(sessionID, req.IdempotencyKey)
	if err != nil {
		return nil, Err("STORAGE_UNAVAILABLE", "读取回合幂等记录失败", 503)
	}
	// Validate a replay against its immutable original base, even when the
	// branch has already advanced beyond the chosen option.
	validationHead := branch.HeadNodeID
	if existing != nil {
		validationHead = existing.ExpectedHeadID
	}
	input, matched, err := s.validateTurnInput(validationHead, req.Input)
	if err != nil {
		return nil, err
	}
	req.Input = input
	inputJSON, _ := json.Marshal(req.Input)
	payloadHash := acceptPayloadHash(req)
	if existing != nil {
		if err := s.checkReplayMatches(existing, branchID, req, payloadHash, string(inputJSON)); err != nil {
			return nil, err
		}
		return &acceptPrepared{existing: existing}, nil
	}
	if !headMatchesClient(s.store, branch, req.ExpectedHeadID, req.ExpectedVersion) {
		return nil, Err("HEAD_CONFLICT", "分支已变化，请刷新当前节点", 409)
	}
	if req.Recheck && (matched == nil || matched.ActionRef == "") {
		return nil, Err("CHECK_REQUIRED", "该回合没有可重新掷骰的规则动作", 400)
	}
	// 基准取**当前分支头**而不是客户端声明值：客户端读到视图之后、提交之前，
	// 后台维护写入（认知/记忆）完全可能已经落地并抬高 head 与 version。
	// 回合的父节点、状态推演与提交 CAS 都必须基于真实的分支头，否则受理能过、
	// 提交却会撞车——读者白花一次生成，最后只拿到"分支已变化"。
	return &acceptPrepared{input: input, matched: matched, payloadHash: payloadHash, inputJSON: string(inputJSON), sess: sess,
		baseHeadID: branch.HeadNodeID, baseVersion: branch.Version}, nil
}

// checkReplayMatches 校验重放请求与既有回合一致（同分支、同基准、同载荷），
// 不一致即 409——幂等键不能复用到别的内容上。
//
// 基准比较沿用受理时的口径：回合记录里存的是**受理时解析出的位置**，而重放请求
// 带的是客户端原本声明的头——两者相差几个维护节点是正常的（认知落地在前、
// 客户端刷新在后），不能因此把客户端的原样重试判成"换了内容"。
func (s *TurnService) checkReplayMatches(existing *domain.TurnRequest, branchID string, req *TurnAcceptRequest, payloadHash, inputJSON string) error {
	legacy := req.DeriveNodeID == "" && !req.Recheck && req.ReuseRollID == "" && existing.PayloadHash == hashString(inputJSON) && existing.Mode == acceptMode(req.Mode)
	baseMatches := existing.ExpectedHeadID == req.ExpectedHeadID && existing.ExpectedVersion == req.ExpectedVersion
	if !baseMatches {
		// 两个方向都看：重放可能落后于既有回合的基准（最常见的原样重试），
		// 也可能因为客户端刷新过而走到前面去。
		baseMatches = headMatchesClient(s.store, &domain.Branch{HeadNodeID: existing.ExpectedHeadID, Version: existing.ExpectedVersion}, req.ExpectedHeadID, req.ExpectedVersion) ||
			headMatchesClient(s.store, &domain.Branch{HeadNodeID: req.ExpectedHeadID, Version: req.ExpectedVersion}, existing.ExpectedHeadID, existing.ExpectedVersion)
	}
	if existing.BranchID != branchID || !baseMatches || (existing.PayloadHash != payloadHash && !legacy) {
		return Err("IDEMPOTENCY_CONFLICT", "相同幂等键不能用于不同内容", 409)
	}
	return nil
}

// persistAcceptedTurn 落库回合并认领分支；选项带动作引用时在准备阶段掷骰
// （契约 §5.2：模型只能演绎已经定下的结果）。任一步失败都要把回合落到
// 失败终态并释放分支锁，不能留一个"占着锁的空回合"。
func (s *TurnService) persistAcceptedTurn(ctx context.Context, sessionID, branchID string, req *TurnAcceptRequest, prep *acceptPrepared) (*domain.TurnRequest, error) {
	mode := req.Mode
	if mode == "" {
		mode = "structured"
	}
	// 规则版本在受理时从会话取定并固定到本次尝试：之后即使会话升级规则，
	// 这个回合仍按当时那套规则结算与重放（契约 §7）。
	ruleset := RulesetVersion
	if prep.sess.RulesetVersion != "" {
		ruleset = prep.sess.RulesetVersion
	}
	turn := &domain.TurnRequest{
		TurnID: id.New(), SessionID: sessionID, BranchID: branchID,
		IdempotencyKey: req.IdempotencyKey, PayloadHash: prep.payloadHash,
		ExpectedHeadID: prep.baseHeadID, ExpectedVersion: prep.baseVersion,
		AfterTurnID: req.AfterTurnID, Status: domain.TurnQueued, Mode: mode,
		RulesetVersion: ruleset,
		InputJSON:      prep.inputJSON,
	}
	if err := s.store.CreateTurnRequest(turn); err != nil {
		return nil, Err("STORAGE_UNAVAILABLE", "受理失败: "+err.Error(), 503)
	}
	ok, err := s.store.ClaimActiveTurn(branchID, turn.TurnID)
	if err != nil {
		return nil, Err("STORAGE_UNAVAILABLE", "受理失败: "+err.Error(), 503)
	}
	if !ok {
		// 队列满：删除孤儿请求，避免占用幂等键。
		_ = s.store.SetCancelIntent(turn.TurnID, true)
		_ = s.store.UpdateTurnResult(turn.TurnID, domain.TurnFailed, "", "QUEUE_FULL", "该分支已有进行中的回合")
		return nil, Err("QUEUE_FULL", "该分支已有进行中的回合", 429)
	}

	// 准备阶段：选项带着动作引用时，先过授权面再掷骰（契约 §5.2）。
	// 必须在生成开始之前完成——模型只能演绎已经定下的结果，不能自己编。
	if prep.matched != nil && prep.matched.ActionRef != "" {
		if _, perr := s.PrepareCheck(ctx, sessionID, branchID, turn.ExpectedHeadID,
			PrepareCheckRequest{ActionID: prep.matched.ActionRef, TurnID: turn.TurnID, Recheck: req.Recheck, ReuseRollID: req.ReuseRollID}); perr != nil {
			_ = s.store.ReleaseActiveTurn(branchID, turn.TurnID)
			_ = s.store.UpdateTurnResult(turn.TurnID, domain.TurnFailed, "", "CHECK_FAILED", perr.Error())
			return nil, perr
		}
	}
	return turn, nil
}

// dispatchTurn 取得分支串行化信号并在独立 goroutine 中按分支顺序执行生成。
func (s *TurnService) dispatchTurn(branchID string, turn *domain.TurnRequest) {
	s.semMu.Lock()
	ch, exists := s.sem[branchID]
	if !exists {
		ch = make(chan struct{}, 1)
		s.sem[branchID] = ch
	}
	s.semMu.Unlock()
	s.workers.Add(1)
	s.metrics.addAccepted()
	go func() {
		defer s.workers.Done()
		ch <- struct{}{}
		defer func() { <-ch }()
		s.run(s.workCtx, turn)
	}()
}

// authorizeActionRef 只放行规则包登记过的动作引用。
//
// 契约 §4：actionRef "必须引用后端当前允许的动作，不能内嵌任意执行代码"——
// 模型给出的引用一律不可信，未登记就是不执行。
//
// 授权面来自**规则包**而不是一张手写名单：契约 §5.2 要求
// "DC 与属性由规则包从允许的动作中解析并校验"，把授权与规则放在同一处，
// 就不会出现"名单里有、规则里没有"（或反过来）的分叉。
func authorizeActionRef(ref string, rs domain.Ruleset) string {
	if ref == "" {
		return ""
	}
	if _, ok := rs.Lookup(ref); ok {
		return ref
	}
	return ""
}

// validateTurnInput 校验并归一化回合输入（T19）。
//
// 选项输入的三条规则：
//  1. 选项必须来自当前分支头——分支头推进后，上一轮留下的选项即过期，不得执行。
//  2. 选项必须真实存在于该回合（不能凭空构造 optionId）。
//  3. 客户端改写了选项文本时按自由输入处理：改写后的意图不再由后端授权，
//     因此丢弃引用，也不沿用原选项携带的动作。
//
// 文本一律以服务端保存的选项原文为准，不信任客户端回传的文本。
//
// 返回值里的 *Option 是命中的选项（含其后端授权过的动作引用），供受理阶段
// 判断"这一轮要不要先掷骰"。自由输入时为 nil。
func (s *TurnService) validateTurnInput(headID string, in domain.TurnInput) (domain.TurnInput, *domain.Option, error) {
	// 注记与选项模式都是“演绎方式”的指令，它们不参与选项引用校验，但必须一路带着走到提交。
	note, err := normalizePlayerNote(in.Note)
	if err != nil {
		return domain.TurnInput{}, nil, err
	}
	options, err := normalizeOptionsMode(in.Options)
	if err != nil {
		return domain.TurnInput{}, nil, err
	}
	in.Note, in.Options = note, options

	if in.Kind == "action" {
		if in.ActionRef == "" {
			return domain.TurnInput{}, nil, Err("ACTION_REQUIRED", "缺少规则动作", 400)
		}
		return domain.TurnInput{Kind: "action", Text: in.Text, Note: in.Note, Options: in.Options, ActionRef: in.ActionRef}, &domain.Option{ActionRef: in.ActionRef}, nil
	}
	if in.Kind != "option" {
		return in, nil, nil
	}
	if in.OptionRef == nil {
		return domain.TurnInput{}, nil, Err("STALE_OPTION", "选项引用缺失", 409)
	}
	ref := in.OptionRef
	if ref.NodeID != headID {
		// Background cognition and user memory edits add non-dialogue nodes.
		// Options remain tied to the last actual dialogue, not the maintenance node.
		cursor := headID
		for cursor != ref.NodeID {
			node, err := s.store.GetNode(cursor)
			if err != nil || (node.Kind != domain.NodeKindMemoryChange && node.Kind != domain.NodeKindDirectorEvent) || node.ParentID == "" {
				return domain.TurnInput{}, nil, Err("STALE_OPTION", "该选项已过期，请基于当前回合重新选择", 409)
			}
			cursor = node.ParentID
		}
	}
	node, err := s.store.GetNode(ref.NodeID)
	if err != nil {
		return domain.TurnInput{}, nil, Err("STALE_OPTION", "选项来源节点不存在", 409)
	}
	var tc domain.TurnContent
	if err := json.Unmarshal([]byte(node.ContentJSON), &tc); err != nil {
		return domain.TurnInput{}, nil, Err("STALE_OPTION", "选项来源无法解析", 409)
	}
	var matched *domain.Option
	for i := range tc.Options {
		if tc.Options[i].OptionID == ref.OptionID {
			matched = &tc.Options[i]
			break
		}
	}
	if matched == nil {
		return domain.TurnInput{}, nil, Err("STALE_OPTION", "该选项不存在于当前回合", 409)
	}
	if in.Text != matched.Text {
		return domain.TurnInput{Kind: "text", Text: in.Text, Note: in.Note, Options: in.Options}, nil, nil
	}
	return domain.TurnInput{
		Kind:      "option",
		Text:      matched.Text,
		Note:      in.Note,
		Options:   in.Options,
		OptionRef: &domain.OptionRef{NodeID: ref.NodeID, OptionID: ref.OptionID},
	}, matched, nil
}

// maxPlayerNoteRunes 是玩家注记的长度上限。
//
// 注记会随每轮请求进入上下文，且会抄进节点内容：不设上限时，一段粘贴进来的长文
// 会变成“每个回合都背一份”的预算消耗。超限直接拒绝而不是静默截断——玩家写的
// 要求被悄悄削掉一半，比明确报错更坑。
const maxPlayerNoteRunes = 2000

func normalizePlayerNote(note string) (string, error) {
	note = strings.TrimSpace(note)
	if note == "" {
		return "", nil
	}
	if utf8.RuneCountInString(note) > maxPlayerNoteRunes {
		return "", Err("NOTE_TOO_LONG", fmt.Sprintf("注记最多 %d 个字符", maxPlayerNoteRunes), 400)
	}
	return note, nil
}

// normalizeOptionsMode 校验选项呈现模式。
//
// 未知取值一律拒绝而不是归一到 auto：模式决定“要不要给玩家选择”，猜错了会直接
// 改变玩法体验，而客户端却以为自己设置生效了。
func normalizeOptionsMode(mode string) (string, error) {
	if strings.TrimSpace(mode) == "" {
		return domain.OptionsAuto, nil
	}
	switch mode {
	case domain.OptionsAuto, domain.OptionsAlways, domain.OptionsNever:
		return mode, nil
	default:
		return "", Err("BAD_REQUEST", "选项模式只能是 auto、always 或 never", 400)
	}
}
