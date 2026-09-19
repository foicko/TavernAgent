package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"tavernagent/internal/protocol"
	"tavernagent/internal/util/id"
)

// BranchService 从既有节点派生新分支：主动分叉（fork）与候选版本（regenerate / edit）。
//
// 技术契约 §7 的数据行为：
//
//	fork(X)           校验 X 属于会话，新建 head=X 的 Branch
//	regenerate(N)     读取 parent(N) 状态，新建候选分支；新回复与 N 为兄弟节点
//	edit player(N)    同样从 parent(N) 产生新分支，旧后续不自动复用
//	edit assistant(N) 生成新的候选；不能验证仍支持原效果时转为纯叙事
//
// 两条贯穿性约束：
//  1. 已提交节点一律不改写（评审 D02）。本服务只新增节点与分支，从不 UPDATE 节点。
//  2. 基准状态取 parent(N) 而非 N 应用后的状态——否则数值会被重复扣减一次。
type BranchService struct {
	store interface {
		ports.StoryStore
		ports.SessionStore
	}
	turns    *TurnService
	deriveMu sync.Mutex
}

// NewBranchService 创建分支服务。
func NewBranchService(store ports.Store, turns *TurnService) *BranchService {
	return &BranchService{store: store, turns: turns}
}

// ForkRequest 是主动分叉请求。
type ForkRequest struct {
	FromNodeID          string
	Name                string
	ExpectedCharacterID string
}

// Fork 从既有节点建立新分支（写入位置），不立即生成内容。
// 新分支的 Version 从 0 开始：它还没有提交过任何节点。
func (s *BranchService) Fork(ctx context.Context, sessionID string, req ForkRequest) (*domain.Branch, error) {
	if _, err := requireCharacter(s.store, sessionID, req.ExpectedCharacterID); err != nil {
		return nil, err
	}
	node, err := s.nodeOf(sessionID, req.FromNodeID)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		if node.TurnNumber > 0 {
			name = fmt.Sprintf("分叉 · 第%d轮", node.TurnNumber)
		} else {
			name = "分叉 · 开局"
		}
	}
	b := &domain.Branch{
		BranchID: id.New(), SessionID: sessionID, Name: name,
		HeadNodeID: node.NodeID, Version: 0,
	}
	if err := s.store.CreateBranch(b); err != nil {
		return nil, Err("STORAGE_UNAVAILABLE", "建立分支失败: "+err.Error(), 503)
	}
	return b, nil
}

// DeriveRequest 是"从某回合派生候选"的请求（重生成 / 编辑玩家输入共用）。
type DeriveRequest struct {
	NodeID         string // 被重生成或被编辑的回合节点 N
	Input          *domain.TurnInput
	IdempotencyKey string
	Label          string // 候选分支的展示名后缀
	Recheck        bool
	// Note 是重生成时的引导（非叙事要求）：改节奏、换侧重、让 NPC 先开口……
	// 留空表示“按原来的条件重演”。重生成原本只能盲重掷或手工改写正文，
	// 中间那段“演得不对但说不清怎么改”的处境无路可走。
	Note string
	// Options 覆盖本回合的选项呈现模式；留空则沿用原回合的设置。
	Options string
	// ExpectedCharacterID 透传给回合受理的角色归属校验（M4l，契约 §11.2）。
	ExpectedCharacterID string
}

// DeriveResult 返回新建的候选分支与已受理的回合。
type DeriveResult struct {
	Branch *domain.Branch
	Turn   *domain.TurnRequest
}

// DeriveTurn 从 parent(N) 建立候选分支并受理一次生成。
//
// 输入为空时复用 N 原有输入（重生成）；非空时使用新输入（编辑玩家输入）。
// 无论哪种，N 都保持不变，新回复与 N 成为兄弟节点。
func (s *BranchService) DeriveTurn(ctx context.Context, sessionID string, req DeriveRequest) (*DeriveResult, error) {
	s.deriveMu.Lock()
	defer s.deriveMu.Unlock()
	if _, err := requireCharacter(s.store, sessionID, req.ExpectedCharacterID); err != nil {
		return nil, err
	}
	target, tc, err := s.turnNodeOf(sessionID, req.NodeID)
	if err != nil {
		return nil, err
	}

	input := domain.TurnInput{Kind: tc.InputKind, Text: tc.InputText, Note: tc.InputNote, Options: tc.OptionsMode, OptionRef: tc.OptionRef, ActionRef: tc.ActionRef}
	if req.Input != nil {
		input = *req.Input
		if strings.TrimSpace(input.Text) == "" {
			return nil, Err("INPUT_REQUIRED", "编辑后的输入不能为空", 400)
		}
	}
	// 显式给出的引导与选项模式覆盖原回合的设置；留空表示“按原来的条件重演”。
	if strings.TrimSpace(req.Note) != "" {
		input.Note = req.Note
	}
	if strings.TrimSpace(req.Options) != "" {
		input.Options = req.Options
	}

	key := req.IdempotencyKey
	if key == "" {
		key = "derive_" + id.New()
	}
	mode := protocol.ModeStructured
	if tc.Provenance != nil && tc.Provenance.Mode != "" {
		mode = tc.Provenance.Mode
	}
	input, matched, err := s.turns.validateTurnInput(target.ParentID, input)
	if err != nil {
		return nil, err
	}
	if req.Recheck && (matched == nil || matched.ActionRef == "") {
		return nil, Err("CHECK_REQUIRED", "该回合没有可重新掷骰的规则动作", 400)
	}
	reuseRoll := ""
	if req.Input == nil && !req.Recheck && matched != nil && matched.ActionRef != "" {
		receipts, err := s.turns.store.ReceiptsAtResultNodes([]string{target.NodeID})
		if err != nil {
			return nil, err
		}
		for _, r := range receipts {
			if r.Receipt.ActionID == matched.ActionRef {
				reuseRoll = r.Receipt.RollID
				break
			}
		}
	}
	accept := &TurnAcceptRequest{
		IdempotencyKey:  key,
		ExpectedHeadID:  target.ParentID,
		ExpectedVersion: 0,
		Input:           input,
		Mode:            mode,
		Recheck:         req.Recheck, ReuseRollID: reuseRoll, DeriveNodeID: req.NodeID,
		// M4l：重生成/编辑与普通回合走同一道角色归属守门。
		ExpectedCharacterID: req.ExpectedCharacterID,
	}
	existing, err := s.turns.store.FindTurnByIdempotency(sessionID, key)
	if err != nil {
		return nil, Err("STORAGE_UNAVAILABLE", "读取派生幂等记录失败", 503)
	}
	if existing != nil {
		if existing.PayloadHash != acceptPayloadHash(accept) {
			return nil, Err("IDEMPOTENCY_CONFLICT", "相同幂等键不能用于不同派生请求", 409)
		}
		branch, err := s.store.GetBranch(existing.BranchID)
		if err != nil {
			return nil, err
		}
		return &DeriveResult{Branch: branch, Turn: existing}, nil
	}
	branch, err := s.forkFrom(sessionID, target, req.Label)
	if err != nil {
		return nil, err
	}
	turn, err := s.turns.Accept(ctx, sessionID, branch.BranchID, accept)
	if err != nil {
		return nil, err
	}
	return &DeriveResult{Branch: branch, Turn: turn}, nil
}

// AssistantEditRequest 是"编辑助手正文"请求。
type AssistantEditRequest struct {
	NodeID              string
	Blocks              []domain.TextBlock
	Label               string
	ExpectedCharacterID string
}

// AssistantEditResult 返回候选分支与已提交的候选节点。
type AssistantEditResult struct {
	Branch *domain.Branch
	Node   *domain.PlotNode
}

// EditAssistantText 把改写后的助手正文落为**纯叙事候选**。
//
// 契约 §7 原文：编辑助手正文要生成新的候选，"若不能验证仍支持原效果，
// 则转为纯叙事或重生成"。当前没有规则检定（属 M4），无法判断新文本是否仍
// 支持原来的状态变化，因此这里明确降级：**丢弃原提议**，按零状态变化提交。
// 这样不会出现"叙述改成没给、背包却少了"的矛盾（评审 R01）。
func (s *BranchService) EditAssistantText(ctx context.Context, sessionID string, req AssistantEditRequest) (*AssistantEditResult, error) {
	if _, err := requireCharacter(s.store, sessionID, req.ExpectedCharacterID); err != nil {
		return nil, err
	}
	target, tc, err := s.turnNodeOf(sessionID, req.NodeID)
	if err != nil {
		return nil, err
	}
	blocks := make([]domain.TextBlock, 0, len(req.Blocks))
	for _, b := range req.Blocks {
		if strings.TrimSpace(b.Text) == "" {
			continue
		}
		kind := b.Kind
		if kind == "" {
			kind = string(protocol.BlockNarration)
		}
		blocks = append(blocks, domain.TextBlock{Kind: kind, SpeakerID: b.SpeakerID, Text: b.Text})
	}
	if len(blocks) == 0 {
		return nil, Err("BLOCKS_REQUIRED", "改写后的正文不能为空", 400)
	}

	branch, err := s.forkFrom(sessionID, target, req.Label)
	if err != nil {
		return nil, err
	}
	// 提交走 TurnService 的同一条路径：受理、认领、构建计划、提交事务、
	// 终态事件都与普通回合一致，只是正文由用户给定（早前这里自己复刻了一份
	// 精简版，少了 outbox 事件与失败终态）。
	node, _, err := s.turns.SubmitBlocks(ctx, SubmitBlocksRequest{
		SessionID: sessionID, BranchID: branch.BranchID,
		HeadID:  branch.HeadNodeID,
		Version: branch.Version,
		Blocks:  blocks,
		Input:   domain.TurnInput{Kind: tc.InputKind, Text: tc.InputText, OptionRef: tc.OptionRef},
		Mode:    protocol.ModeNarrative,
	})
	if err != nil {
		return nil, err
	}
	return &AssistantEditResult{Branch: branch, Node: node}, nil
}

// forkFrom 从 target 的父节点建立候选分支。
func (s *BranchService) forkFrom(sessionID string, target *domain.PlotNode, label string) (*domain.Branch, error) {
	if target.ParentID == "" {
		return nil, Err("CANNOT_DERIVE_ROOT", "根节点没有可派生的回合", 400)
	}
	name := fmt.Sprintf("候选 · 第%d轮", target.TurnNumber)
	if strings.TrimSpace(label) != "" {
		name = strings.TrimSpace(label)
	}
	b := &domain.Branch{
		BranchID: id.New(), SessionID: sessionID, Name: name,
		// 候选分支的写入位置是 parent(N)，因此新回复与 N 成为兄弟节点。
		HeadNodeID: target.ParentID, Version: 0,
	}
	if err := s.store.CreateBranch(b); err != nil {
		return nil, Err("STORAGE_UNAVAILABLE", "建立候选分支失败: "+err.Error(), 503)
	}
	return b, nil
}

// nodeOf 读取节点并校验归属会话（防止跨会话操作）。
func (s *BranchService) nodeOf(sessionID, nodeID string) (*domain.PlotNode, error) {
	if strings.TrimSpace(nodeID) == "" {
		return nil, Err("NODE_REQUIRED", "缺少节点 ID", 400)
	}
	node, err := s.store.GetNode(nodeID)
	if err != nil {
		return nil, Err("NOT_FOUND", "节点不存在", 404)
	}
	if node.SessionID != sessionID {
		return nil, Err("NODE_SESSION_MISMATCH", "节点不属于该会话", 400)
	}
	return node, nil
}

// turnNodeOf 读取节点、校验归属会话，并解析其回合内容。
func (s *BranchService) turnNodeOf(sessionID, nodeID string) (*domain.PlotNode, *domain.TurnContent, error) {
	node, err := s.nodeOf(sessionID, nodeID)
	if err != nil {
		return nil, nil, err
	}
	if node.Kind != domain.NodeKindTurn {
		return nil, nil, Err("NODE_NOT_TURN", "该节点不是回合节点", 400)
	}
	var tc domain.TurnContent
	if err := json.Unmarshal([]byte(node.ContentJSON), &tc); err != nil {
		return nil, nil, Err("NODE_CONTENT_INVALID", "回合内容无法解析: "+err.Error(), 500)
	}
	return node, &tc, nil
}
