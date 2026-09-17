// 记忆管理：列表、路径内有效性与用户修订（纠正/置顶/隐藏）。
//
// 修订采用 copy-on-write：不改动原记录，而是在当前分支上新建一条覆盖记录。
// 原因见 domain.MemoryRecord 的注释——原记录挂在共同祖先上会被多条分支共享，
// 原地修改会让另一分支也看到修订（违反 T17）。
package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"tavernagent/internal/util/id"
)

// MemoryPatch 是记忆修订请求。字段为指针：nil 表示该项不变。
type MemoryPatch struct {
	Content             *string `json:"content,omitempty"`
	Pinned              *bool   `json:"pinned,omitempty"`
	Hidden              *bool   `json:"hidden,omitempty"`
	SubjectKey          *string `json:"subjectKey,omitempty"`
	ExpectedCharacterID string  `json:"expectedCharacterId,omitempty"`
	ExpectedHeadID      string  `json:"expectedHeadId,omitempty"`
	ExpectedVersion     *int64  `json:"expectedVersion,omitempty"`
	IdempotencyKey      string  `json:"idempotencyKey,omitempty"`
}

// MemoryView 是记忆的管理视图。
// Effective 表示该记录在查询所用的路径上是否生效（被覆盖的原记录为 false）。
type MemoryView struct {
	*domain.MemoryRecord
	Effective bool `json:"effective"`
	Protected bool `json:"protected"`
}

// ListAt shows only the inspected path, including hidden/latest revisions for
// recovery. Unrevealed secrets and other characters' private records stay out.
func (s *MemoryService) ListAt(sessionID, branchID, nodeID string) ([]MemoryView, domain.MemoryUsage, error) {
	if branchID == "" {
		branches, err := s.store.ListBranches(sessionID)
		if err != nil {
			return nil, domain.MemoryUsage{}, memoryReadError(err, "读取分支失败")
		}
		if len(branches) == 0 {
			return nil, domain.MemoryUsage{}, Err("NOT_FOUND", "会话或分支不存在", 404)
		}
		branchID = branches[0].BranchID
	}
	b, err := s.store.GetBranch(branchID)
	if err != nil {
		return nil, domain.MemoryUsage{}, memoryReadError(err, "读取分支失败")
	}
	if b == nil || b.SessionID != sessionID {
		return nil, domain.MemoryUsage{}, Err("NOT_FOUND", "分支不存在", 404)
	}
	if nodeID == "" {
		nodeID = b.HeadNodeID
	}
	n, err := s.store.GetNode(nodeID)
	if err != nil {
		return nil, domain.MemoryUsage{}, memoryReadError(err, "读取查看节点失败")
	}
	if n == nil || n.SessionID != sessionID {
		return nil, domain.MemoryUsage{}, Err("NOT_FOUND", "查看节点不存在", 404)
	}
	records, err := s.store.ProjectedMemories(nodeID)
	if err != nil {
		return nil, domain.MemoryUsage{}, memoryReadError(err, "读取路径记忆失败")
	}
	snap, err := s.store.StateAt(nodeID)
	if errors.Is(err, ports.ErrNoState) {
		// 该位置没有状态投影（稀疏导入数据）：按全新状态继续，
		// 可见性判定退化为默认值，而不是把"无状态"报成存储故障。
		snap = &domain.StateSnapshot{NodeID: nodeID, StateJSON: "{}"}
	} else if err != nil {
		return nil, domain.MemoryUsage{}, memoryReadError(err, "读取人物状态失败")
	}
	state, err := domain.UnmarshalWorld(snap.StateJSON)
	if err != nil {
		return nil, domain.MemoryUsage{}, err
	}
	out := []MemoryView{}
	for _, m := range domain.ApplyMemoryOverlays(records) {
		probe := *m
		probe.Hidden = false
		if domain.MemoryVisibleTo(&probe, state) {
			out = append(out, MemoryView{MemoryRecord: m, Effective: true, Protected: domain.MemoryProtected(m, state)})
		}
	}
	return out, domain.ScanMemoryUsage(records, state), nil
}

// MemoryService 提供记忆的管理操作。
type MemoryService struct {
	store ports.MemoryDeps
}

// NewMemoryService 创建记忆管理服务。
func NewMemoryService(store ports.Store) *MemoryService {
	return &MemoryService{store: store}
}

// List 返回会话内全部记忆（含原始记录与覆盖记录）。
// branchID 非空时按该分支的祖先链解析覆盖关系，并标记每条记录是否生效。
func (s *MemoryService) List(sessionID, branchID string) ([]MemoryView, error) {
	all, err := s.store.ListMemories(sessionID)
	if err != nil {
		return nil, Err("STORAGE_UNAVAILABLE", "读取记忆失败: "+err.Error(), 503)
	}
	views := make([]MemoryView, 0, len(all))
	if branchID == "" {
		for _, m := range all {
			views = append(views, MemoryView{MemoryRecord: m, Effective: true})
		}
		return views, nil
	}

	chain, err := s.branchChain(sessionID, branchID)
	if err != nil {
		return nil, err
	}
	visible, err := s.store.MemoriesInChain(chain)
	if err != nil {
		return nil, Err("STORAGE_UNAVAILABLE", "读取路径记忆失败: "+err.Error(), 503)
	}
	effective := map[string]bool{}
	for _, m := range domain.ApplyMemoryOverlays(visible) {
		effective[m.MemoryID] = true
	}
	for _, m := range all {
		views = append(views, MemoryView{MemoryRecord: m, Effective: effective[m.MemoryID]})
	}
	return views, nil
}

// Overlay 在当前分支上生成一条覆盖记录并返回它。
//
// memoryID 可以是原始记录，也可以是某个覆盖记录：函数会先沿覆盖链解析出
// 该分支上当前生效的版本，再以它为基准应用 patch，避免用户编辑到已被取代的旧版本。
//
// 修订提交为当前分支的新维护节点，共享前缀与历史快照保持不变。
func (s *MemoryService) Overlay(sessionID, branchID, memoryID string, patch MemoryPatch) (*domain.MemoryRecord, error) {
	return s.OverlayContext(context.Background(), sessionID, branchID, memoryID, patch)
}

func (s *MemoryService) OverlayContext(ctx context.Context, sessionID, branchID, memoryID string, patch MemoryPatch) (*domain.MemoryRecord, error) {
	if branchID == "" {
		return nil, Err("BAD_REQUEST", "缺少分支 ID", 400)
	}
	if _, err := requireCharacter(s.store, sessionID, patch.ExpectedCharacterID); err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(struct {
		MemoryID string
		Patch    MemoryPatch
	}{memoryID, patch})
	payloadHash := hashString(string(raw))
	batchID := id.New()
	if patch.IdempotencyKey != "" {
		batchID = "manual_" + hashString(sessionID+"\x00"+branchID+"\x00"+patch.IdempotencyKey)
		previous, err := s.store.FindMemoryBatch(batchID, sessionID, branchID, payloadHash)
		if err != nil {
			return nil, Err("STORAGE_UNAVAILABLE", "读取记忆幂等记录失败", 503)
		}
		if previous != nil {
			if previous.ConflictCode != "" {
				return nil, Err(previous.ConflictCode, "幂等键已用于其他修订", 409)
			}
			return s.batchMemory(previous.NewHeadID)
		}
	}
	head, err := s.store.GetBranch(branchID)
	if err != nil {
		return nil, memoryReadError(err, "读取分支失败")
	}
	if head == nil || head.SessionID != sessionID {
		return nil, Err("NOT_FOUND", "分支不存在", 404)
	}
	if (patch.ExpectedHeadID != "" && patch.ExpectedHeadID != head.HeadNodeID) || (patch.ExpectedVersion != nil && *patch.ExpectedVersion != head.Version) {
		return nil, Err("HEAD_CONFLICT", "分支已变化，请刷新后修订", 409)
	}

	visible, err := s.store.ProjectedMemories(head.HeadNodeID)
	if err != nil {
		return nil, Err("STORAGE_UNAVAILABLE", "读取路径记忆失败: "+err.Error(), 503)
	}

	byID := make(map[string]*domain.MemoryRecord, len(visible))
	for _, m := range visible {
		byID[m.MemoryID] = m
	}
	base := byID[memoryID]
	if base == nil {
		// Older clients may name a superseded revision. Resolve that uncommon
		// path against the immutable history; normal edits use the projection.
		visible, err = pathMemories(s.store, head.HeadNodeID)
		if err != nil {
			return nil, memoryReadError(err, "读取记忆修订历史失败")
		}
		for _, m := range visible {
			if m.MemoryID == memoryID {
				base = m
				break
			}
		}
		if base == nil {
			return nil, Err("MEMORY_NOT_FOUND", "记忆不存在或不在当前路径上", 404)
		}
	}

	// 沿覆盖链找到该分支上当前生效的版本（用户看到的就是这一版）。
	current := base
	seen := map[string]bool{}
	for {
		if seen[current.MemoryID] {
			return nil, Err("MEMORY_CORRUPT", "记忆修订链成环", 500)
		}
		seen[current.MemoryID] = true
		next := findSuperseding(visible, current.MemoryID)
		if next == nil {
			break
		}
		current = next
	}
	snapshot, err := s.store.StateAt(head.HeadNodeID)
	if err != nil {
		return nil, memoryReadError(err, "读取人物状态失败")
	}
	state, err := domain.UnmarshalWorld(snapshot.StateJSON)
	if err != nil {
		return nil, Err("STORAGE_UNAVAILABLE", "人物状态损坏", 503)
	}
	visibility := *current
	visibility.Hidden = false // Hidden records remain editable for restoration.
	if !domain.MemoryVisibleTo(&visibility, state) {
		return nil, Err("MEMORY_NOT_FOUND", "记忆不存在或当前不可见", 404)
	}

	headNode, _ := s.store.GetNode(head.HeadNodeID)
	curTurn := 0
	if headNode != nil {
		curTurn = headNode.TurnNumber
	}
	next := *current
	next.MemoryID = id.New()
	next.Supersedes = current.MemoryID
	next.SourceNodeID = id.New()
	next.CreatedTurn = curTurn
	next.ValidFromTurn = curTurn
	current.ValidUntilTurn = curTurn
	if patch.SubjectKey != nil {
		next.SubjectKey = domain.NormalizeSubjectKey(*patch.SubjectKey)
	}
	if patch.Content != nil {
		content := strings.TrimSpace(*patch.Content)
		if content == "" || utf8.RuneCountInString(content) > domain.MaxMemoryContentRunes {
			return nil, Err("BAD_REQUEST", "记忆内容须为 1～1200 个字符", 422)
		}
		next.Content = content
		// A user correction is not supported by the model's previous quotation.
		next.Evidence = nil
	}
	if patch.Pinned != nil {
		next.Pinned = *patch.Pinned
	}
	if patch.Hidden != nil {
		next.Hidden = *patch.Hidden
	}

	b := &ports.MemoryBatch{BatchID: batchID, SessionID: sessionID, BranchID: branchID, ExpectedHeadID: head.HeadNodeID, ExpectedVersion: head.Version,
		NodeID: next.SourceNodeID, Memories: []*domain.MemoryRecord{&next}, PayloadHash: payloadHash, Reason: "memory.revised"}
	b.Events = memoryEvents(b.Memories)
	res, err := commitMemoryBatch(ctx, s.store, b)
	if err != nil {
		return nil, err
	}
	if res.AlreadyDone {
		return s.batchMemory(res.NewHeadID)
	}
	return &next, nil
}

func pathMemories(store ports.MemoryStore, nodeID string) ([]*domain.MemoryRecord, error) {
	candidates, err := store.MemoriesOnPath(nodeID)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.MemoryRecord, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, c.Memory)
	}
	return out, nil
}

func memoryEvents(records []*domain.MemoryRecord) []*domain.DomainEvent {
	out := make([]*domain.DomainEvent, 0, len(records))
	for _, m := range records {
		typ := domain.EventMemoryAdd
		if m.Supersedes != "" {
			typ = domain.EventMemoryRevised
		}
		if m.Hidden {
			typ = domain.EventMemoryHidden
		} else if m.Pinned {
			typ = domain.EventMemoryPinned
		}
		raw, _ := json.Marshal(domain.MemoryAddPayload{Memory: *m})
		out = append(out, &domain.DomainEvent{Type: typ, PayloadJSON: string(raw)})
	}
	return out
}

func commitMemoryBatch(ctx context.Context, store ports.MemoryBatchStore, b *ports.MemoryBatch) (*ports.CommitResult, error) {
	res, err := store.CommitMemoryBatch(ctx, b)
	if err != nil {
		return nil, Err("STORAGE_UNAVAILABLE", "提交记忆变化失败: "+err.Error(), 503)
	}
	if res.ConflictCode != "" {
		return nil, Err(res.ConflictCode, "当前分支正在变化，请稍后重试", 409)
	}
	return res, nil
}

func (s *MemoryService) Usage(sessionID, branchID string) (domain.MemoryUsage, error) {
	b, err := s.store.GetBranch(branchID)
	if err != nil || b == nil || b.SessionID != sessionID {
		if err != nil {
			return domain.MemoryUsage{}, memoryReadError(err, "读取分支失败")
		}
		return domain.MemoryUsage{}, Err("NOT_FOUND", "分支不存在", 404)
	}
	records, err := s.store.ProjectedMemories(b.HeadNodeID)
	if err != nil {
		return domain.MemoryUsage{}, err
	}
	sn, err := s.store.StateAt(b.HeadNodeID)
	if err != nil {
		return domain.MemoryUsage{}, err
	}
	state, err := domain.UnmarshalWorld(sn.StateJSON)
	if err != nil {
		return domain.MemoryUsage{}, err
	}
	return domain.ScanMemoryUsage(records, state), nil
}

func (s *MemoryService) Organize(ctx context.Context, sessionID, branchID, expectedCharacter string) (domain.MemoryOrganization, error) {
	if _, err := requireCharacter(s.store, sessionID, expectedCharacter); err != nil {
		return domain.MemoryOrganization{}, err
	}
	b, err := s.store.GetBranch(branchID)
	if err != nil {
		return domain.MemoryOrganization{}, memoryReadError(err, "读取分支失败")
	}
	if b == nil || b.SessionID != sessionID {
		return domain.MemoryOrganization{}, Err("NOT_FOUND", "分支不存在", 404)
	}
	records, err := s.store.ProjectedMemories(b.HeadNodeID)
	if err != nil {
		return domain.MemoryOrganization{}, err
	}
	snap, err := s.store.StateAt(b.HeadNodeID)
	if err != nil {
		return domain.MemoryOrganization{}, err
	}
	state, err := domain.UnmarshalWorld(snap.StateJSON)
	if err != nil {
		return domain.MemoryOrganization{}, err
	}
	plan := domain.PlanMemoryOrganization(records, state)
	if len(plan.Merges) == 0 {
		return plan, nil
	}
	nodeID := id.New()
	memories, err := domain.BuildOrganizedMemories(plan, records, state, nodeID)
	if err != nil {
		return plan, err
	}
	raw, _ := json.Marshal(plan)
	_, err = commitMemoryBatch(ctx, s.store, &ports.MemoryBatch{BatchID: id.New(), SessionID: sessionID, BranchID: branchID, ExpectedHeadID: b.HeadNodeID, ExpectedVersion: b.Version,
		NodeID: nodeID, Memories: memories, Events: memoryEvents(memories), PayloadHash: hashString(string(raw)), Reason: "memory.organized"})
	return plan, err
}

// branchChain 返回分支当前头节点的祖先链（含头节点）。
func (s *MemoryService) branchChain(sessionID, branchID string) ([]string, error) {
	b, err := s.store.GetBranch(branchID)
	if err != nil || b == nil || b.SessionID != sessionID {
		return nil, Err("NOT_FOUND", "分支不存在", 404)
	}
	nodes, err := s.store.AncestorChain(b.HeadNodeID, true)
	if err != nil {
		return nil, Err("STORAGE_UNAVAILABLE", "读取分支路径失败: "+err.Error(), 503)
	}
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, n.NodeID)
	}
	return out, nil
}

// findSuperseding 在给定记录集合中找出取代 targetID 的那条记录；没有则返回 nil。
// 只应在「已经按路径过滤过」的集合里查找，否则会跨分支命中。
func findSuperseding(records []*domain.MemoryRecord, targetID string) *domain.MemoryRecord {
	for _, r := range records {
		if r.Supersedes == targetID {
			return r
		}
	}
	return nil
}

func (s *MemoryService) batchMemory(nodeID string) (*domain.MemoryRecord, error) {
	records, err := s.store.MemoriesInChain([]string{nodeID})
	if err != nil {
		return nil, err
	}
	for _, m := range records {
		if m.SourceNodeID == nodeID {
			return m, nil
		}
	}
	return nil, Err("STORAGE_UNAVAILABLE", "已提交的记忆修订记录缺失", 503)
}

func memoryReadError(err error, message string) error {
	if errors.Is(err, ports.ErrNotFound) || errors.Is(err, domain.ErrItemNotFound) {
		return Err("NOT_FOUND", "会话、分支或节点不存在", 404)
	}
	return Err("STORAGE_UNAVAILABLE", message, 503)
}
