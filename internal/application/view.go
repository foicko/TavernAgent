package application

import (
	"encoding/json"
	"errors"
	"sort"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// 会话读取视图。
//
// 组装逻辑放在应用层而不是 HTTP handler：可见性、分支与查看位置的区分、
// 候选分组都是业务规则，散落在适配器里迟早会与领域规则分叉
// （此前 handler 直接持有 Store 自己拼 JSON）。

// BranchView 是分支的展示视图（刻意不含 sessionId：它由外层会话决定）。
type BranchView struct {
	BranchID     string `json:"branchId"`
	Name         string `json:"name"`
	HeadNodeID   string `json:"headNodeId"`
	Version      int64  `json:"version"`
	ActiveTurnID string `json:"activeTurnId,omitempty"`
}

// SessionView 是一个会话的完整读取视图。
type SessionView struct {
	Director    *domain.DirectorSummary `json:"director,omitempty"`
	SessionID   string                  `json:"sessionId"`
	CharacterID string                  `json:"characterId"`
	Title       string                  `json:"title"`
	RootNodeID  string                  `json:"rootNodeId"`
	Branch      BranchView              `json:"branch"`
	Branches    []*domain.Branch        `json:"branches"`
	ViewNodeID  string                  `json:"viewNodeId"`
	ViewNode    *domain.PlotNode        `json:"viewNode"`
	HeadNode    *domain.PlotNode        `json:"headNode"`
	Nodes       []*domain.PlotNode      `json:"nodes"`
	// CandidateGroups 是 parentId → 该父节点下的回合候选（仅当多于一个时出现）。
	CandidateGroups map[string][]*domain.PlotNode `json:"candidateGroups"`
	State           json.RawMessage               `json:"state"`
	// Secrets 是「世界观/秘密」区块（M4d）：已揭示的带内容，未揭示的只有标题。
	Secrets []SecretView `json:"secrets,omitempty"`
	// Outline 是「剧情大纲」区块（M4f）：里程碑 + 角色目标 + 当前场景。
	Outline *OutlineView `json:"outline,omitempty"`
	// ActiveSummary 是当前查看路径上生效的演义交接快照（M4b）。
	ActiveSummary *domain.SummaryArtifact `json:"activeSummary,omitempty"`
	Actions       []ActionView            `json:"actions"`
	// HasMore 表示窗口之外还有更早的回合（前端据此决定是否允许向上翻页）。
	HasMore bool `json:"hasMore"`
	// OldestTurnID 是窗口内最早的回合节点，用作向上翻页的游标。
	OldestTurnID string `json:"oldestTurnId,omitempty"`
}

// ViewQuery 是会话视图的读取参数。
type ViewQuery struct {
	// BranchID 是写入位置（空表示默认分支）。
	BranchID string
	// ViewNodeID 是查看位置（空表示分支头）。
	ViewNodeID string
	// Limit 是窗口内最多返回的回合数（<=0 用 DefaultViewLimit）。
	Limit int
	// Before 是向上翻页游标：只取该节点**之前**的回合（空表示从最新开始）。
	Before string
}

// DefaultViewLimit 是首屏返回的回合数。
//
// 不返回整条链：万级节点的会话在前端要渲染 10,000+ 个回合元素，实测首屏
// 7 秒（见 docs/PERF_BASELINE.md）。历史按 before 游标向上翻页加载。
const DefaultViewLimit = 100

// MaxViewLimit 是单次请求的回合数上限，避免"窗口化"被一个参数绕过去。
const MaxViewLimit = 500

// OutlineEntry 是剧情大纲里的一条主线里程碑（M4f）。
type OutlineEntry struct {
	MilestoneID string `json:"milestoneId"`
	Description string `json:"description"`
}

// GoalEntry 是一个角色的短期目标（M4f）。
type GoalEntry struct {
	GoalID      string `json:"goalId"`
	CharacterID string `json:"characterId"`
	Text        string `json:"text"`
}

// OutlineView 是「剧情大纲」区块（M4f）。
//
// 规范里"大纲"没有更细的定义；这里取状态投影里**已经存在**的主线里程碑、
// 角色目标与当前场景——它们由规则事件推进，天然随分支隔离，
// 不引入新的存储或新的用户输入面。
type OutlineView struct {
	Milestones []OutlineEntry `json:"milestones"`
	Goals      []GoalEntry    `json:"goals"`
	Scene      string         `json:"scene,omitempty"`
}

// SecretView 是秘密的展示视图。
//
// 未揭示的条目**不回传内容**：前端是玩家可见面，把未揭示内容发给前端
// 等于剧透，也违背 T25「未授权字段不进入可见面」的精神。
type SecretView struct {
	SecretID string `json:"secretId"`
	Title    string `json:"title,omitempty"`
	Order    int    `json:"order,omitempty"`
	// Content 只在已揭示时非空。
	Content  string `json:"content,omitempty"`
	Revealed bool   `json:"revealed"`
}

// NodeView 是单节点的读取视图（inspect，不推进任何分支）。
type NodeView struct {
	Node       *domain.PlotNode   `json:"node"`
	State      json.RawMessage    `json:"state"`
	Candidates []*domain.PlotNode `json:"candidates"`
}

// Sessions 列出全部会话。
func (s *SessionService) Sessions() ([]*domain.Session, error) {
	return s.store.ListSessions()
}

// Branches 列出会话的分支。
func (s *SessionService) Branches(sessionID string) ([]*domain.Branch, error) {
	branches, err := s.store.ListBranches(sessionID)
	if err != nil {
		return nil, Err("STORAGE_UNAVAILABLE", "读取分支失败", 503)
	}
	return branches, nil
}

// RequireBranch 校验分支存在且属于该会话（写操作的前置条件）。
func (s *SessionService) RequireBranch(sessionID, branchID string) error {
	b, err := s.store.GetBranch(branchID)
	if err != nil {
		return Err("NOT_FOUND", "分支不存在", 404)
	}
	if b.SessionID != sessionID {
		return Err("BRANCH_SESSION_MISMATCH", "分支不属于该会话", 400)
	}
	return nil
}

// View 组装会话读取视图。
//
// 写入位置（BranchID）与查看位置（ViewNodeID）分离：浏览历史只改查看位置，
// 不推进任何分支指针（技术契约 §7）。
//
// 节点按窗口返回（最近 Limit 轮 + 根节点），并通过 HasMore / OldestTurnID
// 支持向上翻页——把整条链一次交给前端在万级节点下不可用。
func (s *SessionService) View(sessionID string, q ViewQuery) (*SessionView, error) {
	sess, err := s.store.GetSession(sessionID)
	if err != nil {
		return nil, Err("NOT_FOUND", "会话不存在", 404)
	}
	branches, err := s.store.ListBranches(sessionID)
	if err != nil || len(branches) == 0 {
		return nil, Err("STORAGE_UNAVAILABLE", "读取分支失败", 503)
	}

	branch := branches[0]
	if q.BranchID != "" {
		found := false
		for _, b := range branches {
			if b.BranchID == q.BranchID {
				branch, found = b, true
				break
			}
		}
		if !found {
			return nil, Err("BRANCH_NOT_FOUND", "分支不存在", 404)
		}
	}

	viewID := q.ViewNodeID
	if viewID == "" {
		viewID = branch.HeadNodeID
	}
	viewNode, err := s.store.GetNode(viewID)
	if err != nil {
		return nil, Err("NODE_NOT_FOUND", "节点不存在", 404)
	}
	if viewNode.SessionID != sessionID {
		return nil, Err("NODE_SESSION_MISMATCH", "节点不属于该会话", 400)
	}
	head, err := s.store.GetNode(branch.HeadNodeID)
	if err != nil {
		return nil, Err("STORAGE_UNAVAILABLE", "读取节点失败", 503)
	}

	limit := q.Limit
	if limit <= 0 {
		limit = DefaultViewLimit
	}
	if limit > MaxViewLimit {
		limit = MaxViewLimit
	}

	// 翻页游标：从 Before 的父节点往上取，避免把 Before 自己重复带回。
	anchor := viewNode.NodeID
	if q.Before != "" {
		bn, berr := s.store.GetNode(q.Before)
		if berr != nil || bn.SessionID != sessionID {
			return nil, Err("INVALID_CURSOR", "历史游标不属于该会话", 400)
		}
		ok, err := s.store.IsAncestor(q.Before, viewNode.NodeID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, Err("INVALID_CURSOR", "历史游标不在查看路径上", 400)
		}
		anchor = bn.ParentID
	}

	var turns []*domain.PlotNode
	hasMore := false
	if anchor != "" {
		// 多取一条用于判断是否还有更早的回合。
		got, aerr := s.store.RecentTurnNodes(anchor, limit+1)
		if aerr != nil {
			return nil, Err("STORAGE_UNAVAILABLE", "读取回合失败", 503)
		}
		if len(got) > limit {
			hasMore = true
			got = got[len(got)-limit:]
		}
		turns = got
	}

	// 根节点始终携带：开场白是故事的第一段正文，前端据它起头。
	root, rerr := s.store.GetNode(sess.RootNodeID)
	var nodes []*domain.PlotNode
	if rerr == nil && root != nil {
		nodes = append(nodes, root)
	}
	nodes = append(nodes, turns...)

	oldest := ""
	if len(turns) > 0 {
		oldest = turns[0].NodeID
	}

	// 状态投影：查看历史节点时得到的是"那一刻"的状态（经检查点 + 重放）。
	var state json.RawMessage
	switch sn, serr := s.store.StateAt(viewNode.NodeID); {
	case serr == nil && sn != nil:
		state = json.RawMessage(sn.StateJSON)
	case errors.Is(serr, ports.ErrNoState):
		// 祖先链上没有快照（稀疏导入数据）：按全新状态降级，正文与分支照常可读。
	case serr != nil:
		// 不要把底层失败一律压成"状态损坏"：真实原因必须带出去，
		// 否则调用方只会看到 "unexpected end of JSON input" 这种无从下手的提示。
		return nil, Err("STATE_CORRUPT", "状态投影读取失败: "+serr.Error(), 500)
	}

	// 状态投影解析一次，秘密（M4d）与大纲（M4f）共用：
	// 两者的揭示/达成状态都来自"这个查看位置那一刻的状态"。
	ws := domain.NewWorldState()
	if len(state) > 0 {
		parsed, uerr := domain.UnmarshalWorld(string(state))
		if uerr != nil {
			return nil, Err("STATE_CORRUPT", "状态投影解析失败: "+uerr.Error(), 500)
		}
		ws = parsed
	}
	defs := sessionSecretDefs(s.store, sessionID)
	secretViews := make([]SecretView, 0, len(defs))
	for _, d := range defs {
		revealed := ws.IsSecretUnlocked(d.SecretID)
		sv := SecretView{SecretID: d.SecretID, Title: d.Title, Order: d.Order, Revealed: revealed}
		if revealed {
			sv.Content = d.Content
		}
		secretViews = append(secretViews, sv)
	}

	// 剧情大纲：全部来自状态投影（M4f）。
	outline := outlineOf(ws)
	director, err := s.store.DirectorAt(viewNode.NodeID)
	if err != nil {
		return nil, Err("STORAGE_UNAVAILABLE", "读取导演状态失败", 503)
	}

	// 演义交接快照与摘要（M4b）：获取当前查看位置生效的最新摘要
	var activeSummary *domain.SummaryArtifact
	if sums, serr := s.store.SummariesOnPath(viewNode.NodeID); serr == nil && len(sums) > 0 {
		activeSummary = sums[len(sums)-1]
	}

	rules, err := rulesForSession(s.store, sessionID, domain.DefaultRuleset())
	if err != nil {
		return nil, err
	}
	return &SessionView{
		Actions:         actionViews(rules),
		SessionID:       sess.SessionID,
		CharacterID:     sess.CharacterID,
		Secrets:         secretViews,
		Outline:         outline,
		Director:        director.Summary(),
		ActiveSummary:   activeSummary,
		Title:           sess.Title,
		RootNodeID:      sess.RootNodeID,
		Branch:          BranchView{BranchID: branch.BranchID, Name: branch.Name, HeadNodeID: branch.HeadNodeID, Version: branch.Version, ActiveTurnID: branch.ActiveTurnID},
		Branches:        branches,
		ViewNodeID:      viewNode.NodeID,
		ViewNode:        viewNode,
		HeadNode:        head,
		Nodes:           nodes,
		CandidateGroups: candidateGroups(s, sessionID, nodes),
		State:           state,
		HasMore:         hasMore,
		OldestTurnID:    oldest,
	}, nil
}

// NodeView 组装单节点读取视图：状态投影 + 同父候选。
func (s *SessionService) NodeView(nodeID string) (*NodeView, error) {
	node, err := s.store.GetNode(nodeID)
	if err != nil {
		return nil, Err("NOT_FOUND", "节点不存在", 404)
	}
	var state json.RawMessage
	if sn, serr := s.store.StateAt(nodeID); serr == nil && sn != nil {
		state = json.RawMessage(sn.StateJSON)
	}
	candidates := []*domain.PlotNode{}
	if node.ParentID != "" {
		if kids, _, kerr := s.store.ChildWindow(node.SessionID, []string{node.ParentID}, 100, true); kerr == nil {
			for _, k := range kids[node.ParentID] {
				if k.Kind == domain.NodeKindTurn {
					candidates = append(candidates, k)
				}
			}
		}
	}
	return &NodeView{Node: node, State: state, Candidates: candidates}, nil
}

// candidateGroups 找出链上"有多版本"的回合（同父兄弟），按父节点分组。
// 一次批量查询，避免对链上每个回合各查一次（N+1）。
func candidateGroups(s *SessionService, sessionID string, nodes []*domain.PlotNode) map[string][]*domain.PlotNode {
	parentIDs := make([]string, 0, len(nodes))
	seen := map[string]bool{}
	for _, n := range nodes {
		if n.Kind == domain.NodeKindTurn && n.ParentID != "" && !seen[n.ParentID] {
			seen[n.ParentID] = true
			parentIDs = append(parentIDs, n.ParentID)
		}
	}
	out := map[string][]*domain.PlotNode{}
	kids, _, err := s.store.ChildWindow(sessionID, parentIDs, 300, true)
	if err != nil {
		return out
	}
	for pid, group := range kids {
		turns := make([]*domain.PlotNode, 0, len(group))
		for _, k := range group {
			if k.Kind == domain.NodeKindTurn {
				turns = append(turns, k)
			}
		}
		if len(turns) > 1 {
			out[pid] = turns
		}
	}
	return out
}

// ---- 分支图谱（M4e）----

// GraphNode 是图谱里的一个节点：只带画树需要的元数据，不带正文。
// 正文在故事列里已经有了，图谱复制一份只会让响应变大、渲染变慢。
type GraphNode struct {
	NodeID      string `json:"nodeId"`
	ParentID    string `json:"parentId,omitempty"`
	Kind        string `json:"kind"`
	TurnNumber  int    `json:"turnNumber,omitempty"`
	Depth       int    `json:"depth"`
	ChildCount  int    `json:"childCount,omitempty"` // >0 表示有未展开的子树
	IsCandidate bool   `json:"isCandidate,omitempty"`
}

// GraphView 是以 target 为中心的一圈子图。
//
// 契约的指标是"图谱可交互时间 p95 ≤ 500ms，且不渲染全部节点"——
// 万级节点的会话一次性给出整棵树本身就违背了这个前提，
// 因此这里只取目标节点的上方 up 层与下方 down 层。
type GraphView struct {
	SessionID string      `json:"sessionId"`
	Target    string      `json:"targetNodeId"`
	Nodes     []GraphNode `json:"nodes"`
	// Truncated 表示目标周围还有更多层未返回（前端据此提供"加载更多"）。
	Truncated bool `json:"truncated"`
}

// Graph 组装分支图谱子图。
//
// up 沿父链向上（不含 target），down 沿子树向下（含 target 的全部子节点，
// 每层展开）。candidate（同父兄弟回合）总是包含——它们是图谱存在的意义。
func (s *SessionService) Graph(sessionID, target string, up, down int) (*GraphView, error) {
	if up < 0 {
		up = 0
	}
	if down < 0 {
		down = 0
	}
	if up > 50 {
		up = 50
	}
	if down > 10 {
		down = 10
	}
	targetNode, err := s.store.GetNode(target)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) || errors.Is(err, domain.ErrItemNotFound) {
			return nil, Err("NOT_FOUND", "节点不存在", 404)
		}
		return nil, Err("STORAGE_UNAVAILABLE", "读取目标节点失败", 503)
	}
	if targetNode.SessionID != sessionID {
		return nil, Err("NODE_SESSION_MISMATCH", "节点不属于该会话", 400)
	}

	view := &GraphView{SessionID: sessionID, Target: target}
	seen := map[string]bool{}
	add := func(n *domain.PlotNode, isCandidate bool) {
		if n == nil || seen[n.NodeID] {
			return
		}
		if len(view.Nodes) >= 300 {
			view.Truncated = true
			return
		}
		seen[n.NodeID] = true
		view.Nodes = append(view.Nodes, GraphNode{
			NodeID: n.NodeID, ParentID: n.ParentID, Kind: string(n.Kind),
			TurnNumber: n.TurnNumber, Depth: n.Depth, IsCandidate: isCandidate,
		})
	}
	add(targetNode, false)

	// 向上：父链（root 方向）。
	cur := targetNode
	for i := 0; i < up && cur.ParentID != ""; i++ {
		p, err := s.store.GetNode(cur.ParentID)
		if err != nil {
			return nil, Err("STORAGE_UNAVAILABLE", "读取父节点失败", 503)
		}
		add(p, false)
		cur = p
	}

	// 向下：逐层展开目标子树（候选版本天然包含在子树里）。
	frontier := []*domain.PlotNode{targetNode}
	for level := 0; level < down; level++ {
		if len(frontier) == 0 || len(view.Nodes) >= 300 {
			break
		}
		parentIDs := make([]string, 0, len(frontier))
		for _, n := range frontier {
			parentIDs = append(parentIDs, n.NodeID)
		}
		kids, truncated, err := s.store.ChildWindow(sessionID, parentIDs, 300-len(view.Nodes), false)
		view.Truncated = view.Truncated || truncated
		if err != nil {
			return nil, Err("STORAGE_UNAVAILABLE", "读取子节点失败", 503)
		}
		next := make([]*domain.PlotNode, 0, len(frontier))
		for _, pid := range parentIDs {
			for _, k := range kids[pid] {
				add(k, k.Kind == domain.NodeKindTurn)
				next = append(next, k)
			}
		}
		frontier = next
	}

	// 候选补全：子图里每个节点的其余子节点（同父兄弟回合）也要可见，
	// 否则向上翻到历史节点时看不到当时发生过分叉——那是图谱的核心价值。
	if len(view.Nodes) > 0 && len(view.Nodes) < 300 {
		ids := make([]string, 0, len(view.Nodes))
		for _, n := range view.Nodes {
			ids = append(ids, n.NodeID)
		}
		kids, truncated, err := s.store.ChildWindow(sessionID, ids, 300-len(view.Nodes), false)
		view.Truncated = view.Truncated || truncated
		if err != nil {
			return nil, Err("STORAGE_UNAVAILABLE", "读取候选节点失败", 503)
		}
		for _, n := range view.Nodes {
			for _, k := range kids[n.NodeID] {
				if k.Kind != domain.NodeKindTurn {
					continue
				}
				add(k, true)
			}
		}
	}

	// 每个节点的子节点数：用来渲染"可展开"标记，而不是把整棵树搬出去。
	if len(view.Nodes) > 0 {
		ids := make([]string, 0, len(view.Nodes))
		for _, n := range view.Nodes {
			ids = append(ids, n.NodeID)
		}
		counts, err := s.store.ChildCounts(sessionID, ids)
		if err != nil {
			return nil, Err("STORAGE_UNAVAILABLE", "读取节点数量失败", 503)
		}
		{
			for i := range view.Nodes {
				view.Nodes[i].ChildCount = counts[view.Nodes[i].NodeID]
				shown := 0
				for _, child := range view.Nodes {
					if child.ParentID == view.Nodes[i].NodeID {
						shown++
					}
				}
				if shown < view.Nodes[i].ChildCount {
					view.Truncated = true
				}
			}
		}
	}
	view.Truncated = view.Truncated || cur.ParentID != ""
	return view, nil
}

// outlineOf 从状态投影组装剧情大纲（M4f）。
// 排序用 ID 保持稳定：同一状态必须得到同一份大纲（可复现）。
func outlineOf(state *domain.WorldState) *OutlineView {
	if state == nil {
		return nil
	}
	out := &OutlineView{Milestones: []OutlineEntry{}, Goals: []GoalEntry{}}
	for _, mid := range sortedOutlineKeys(state.Milestones) {
		out.Milestones = append(out.Milestones, OutlineEntry{MilestoneID: mid, Description: state.Milestones[mid]})
	}
	for _, gid := range sortedOutlineKeys(state.Goals) {
		g := state.Goals[gid]
		out.Goals = append(out.Goals, GoalEntry{GoalID: gid, CharacterID: g.CharacterID, Text: g.Text})
	}
	if state.Scene != nil && state.Scene.Title != "" {
		out.Scene = state.Scene.Title
	} else if state.Scene != nil && state.Scene.LocationID != "" {
		out.Scene = state.Scene.LocationID
	}
	return out
}

// sortedOutlineKeys 是 map key 的稳定排序（应用层本地实现，
// 避免为几个排序调用引入对编译层的依赖）。
func sortedOutlineKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
