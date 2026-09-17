package application

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"sort"

	"tavernagent/internal/domain"
)

func validateBundleRules(b *domain.SessionBundle, root *domain.PlotNode) error {
	_, err := bundleRules(b, root)
	return err
}

func bundleRules(b *domain.SessionBundle, root *domain.PlotNode) (domain.Ruleset, error) {
	var refs struct {
		Templates map[string]struct {
			ID string `json:"templateVersionId"`
		} `json:"templates"`
	}
	if err := json.Unmarshal([]byte(root.ContentJSON), &refs); err != nil {
		return domain.Ruleset{}, Err("INVALID_PACK", "根模板引用无效", 400)
	}
	ref := refs.Templates["rules"].ID
	for _, tpl := range b.Templates {
		if tpl.TemplateVersionID != ref || ref == "" {
			continue
		}
		var rs domain.Ruleset
		if err := json.Unmarshal([]byte(tpl.Content), &rs); err != nil {
			return domain.Ruleset{}, Err("INVALID_PACK", "规则模板无效", 400)
		}
		if err := domain.ValidateRuleset(rs); err != nil {
			return domain.Ruleset{}, Err("INVALID_PACK", err.Error(), 400)
		}
		if rs.Version != b.RulesetVersion || (b.Session.RulesetVersion != "" && rs.Version != b.Session.RulesetVersion) {
			return domain.Ruleset{}, Err("UNSUPPORTED_RULESET", "规则模板与会话版本不一致", 409)
		}
		return rs, nil
	}
	if (b.RulesetVersion != "" && b.RulesetVersion != RulesetVersion) || (b.Session.RulesetVersion != "" && b.Session.RulesetVersion != RulesetVersion) {
		return domain.Ruleset{}, Err("UNSUPPORTED_RULESET", "该规则版本缺少可验证的规则模板", 409)
	}
	return domain.DefaultRuleset(), nil
}

func validateBundleReferences(b *domain.SessionBundle, nodes map[string]*domain.PlotNode) error {
	bad := func(message string) error { return Err("INVALID_PACK", message, 400) }
	ancestor := func(from, to string) bool {
		for n := nodes[to]; n != nil; n = nodes[n.ParentID] {
			if n.NodeID == from {
				return true
			}
		}
		return false
	}
	memories := map[string]*domain.MemoryRecord{}
	for _, m := range b.Memories {
		memories[m.MemoryID] = m
	}
	for _, m := range b.Memories {
		seen := map[string]bool{}
		for current := m; current != nil; current = memories[current.Supersedes] {
			if seen[current.MemoryID] {
				return bad("记忆修订链存在环")
			}
			seen[current.MemoryID] = true
		}
		if old := memories[m.Supersedes]; old != nil && !ancestor(old.SourceNodeID, m.SourceNodeID) {
			return bad("记忆修订引用了其他分支")
		}
		seen = map[string]bool{}
		for _, id := range m.MergedFrom {
			old := memories[id]
			if old == nil || id == m.MemoryID || seen[id] || !ancestor(old.SourceNodeID, m.SourceNodeID) {
				return bad("记忆合并来源无效")
			}
			seen[id] = true
		}
		if m.Evidence != nil {
			var excerpts []domain.EvidenceExcerpt
			if m.Evidence.SourceNodeID != "" {
				if !ancestor(m.Evidence.SourceNodeID, m.SourceNodeID) {
					return bad("记忆证据不在来源路径上")
				}
				var tc domain.TurnContent
				if err := json.Unmarshal([]byte(nodes[m.Evidence.SourceNodeID].ContentJSON), &tc); err != nil {
					return bad("记忆证据节点无效")
				}
				excerpts = append(excerpts, domain.EvidenceExcerpt{NodeID: m.Evidence.SourceNodeID, Text: tc.InputText})
				for _, block := range tc.Blocks {
					excerpts = append(excerpts, domain.EvidenceExcerpt{NodeID: m.Evidence.SourceNodeID, Text: block.Text})
				}
			}
			checked, confidence, err := domain.ValidateMemoryEvidence(m.Kind, *m.Evidence, excerpts)
			if err != nil {
				return bad("记忆证据校验失败: " + err.Error())
			}
			checked.AutoDowngraded = checked.AutoDowngraded || m.Evidence.AutoDowngraded
			m.Evidence, m.Confidence = &checked, confidence
		}
	}
	rules, err := bundleRules(b, nodes[b.RootNodeID])
	if err != nil {
		return err
	}
	receipts := map[string]bool{}
	turnNodes := map[string]string{}
	for _, p := range b.Receipts {
		r := p.Receipt
		if r == nil || r.ReceiptID == "" || r.TurnID == "" || receipts[r.ReceiptID] || r.Status != domain.ReceiptCommitted {
			return bad("检定收据无效或重复")
		}
		receipts[r.ReceiptID] = true
		n := nodes[p.NodeID]
		if n == nil || n.Kind != domain.NodeKindTurn || n.ParentID != r.BaseHeadID {
			return bad("检定结果节点与基准不匹配")
		}
		if old := turnNodes[r.TurnID]; old != "" && old != p.NodeID {
			return bad("同一检定回合指向多个结果")
		}
		turnNodes[r.TurnID] = p.NodeID
		cr, err := r.CheckResultOf()
		if err != nil || cr.RollID != r.RollID || cr.ActionID != r.ActionID || cr.RulesetVer != r.RulesetVersion {
			return bad("检定结果字段不一致")
		}
		rule, ok := rules.Lookup(cr.ActionID)
		if !ok || cr.RulesetVer != rules.Version {
			return bad("检定动作未被规则模板授权")
		}
		expected, err := domain.EvaluateCheck(rule, cr.AttributeVal, cr.Natural)
		expected.RollID, expected.RulesetVer = cr.RollID, cr.RulesetVer
		if err != nil || !sameCheck(expected, cr) {
			return bad("检定结果不符合规则")
		}
		var tc domain.TurnContent
		if err := json.Unmarshal([]byte(n.ContentJSON), &tc); err != nil {
			return bad("检定回合正文无效")
		}
		for _, check := range tc.Checks {
			if check.RollID == cr.RollID && !sameCheck(check, cr) {
				return bad("检定正文与收据不一致")
			}
		}
	}
	for _, t := range b.Templates {
		if hashString(t.Content) != t.ContentHash {
			return bad("模板内容指纹不一致")
		}
	}
	cardRef := decodeRootCardRef(nodes[b.RootNodeID].ContentJSON)
	for _, t := range b.Templates {
		if t.TemplateVersionID == cardRef && t.Kind == domain.TemplateCharacter && b.Session.CharacterID != "" && domain.CardIdentity(t.Content) != b.Session.CharacterID {
			return bad("角色归属与根模板不一致")
		}
	}
	return validateBundleSnapshots(b, nodes)
}

// Payload JSON whitespace is not part of a rule's meaning.
func sameCheck(a, b domain.CheckResult) bool {
	aa, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	var av, bv any
	if json.Unmarshal(aa, &av) != nil || json.Unmarshal(bb, &bv) != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}

// Validate optional checkpoints against replay, so an imported snapshot cannot
// silently override its event history. Parent states are released after use.
func validateBundleSnapshots(b *domain.SessionBundle, nodes map[string]*domain.PlotNode) error {
	snapshots := map[string]*domain.StateSnapshot{}
	for _, sn := range b.Snapshots {
		if sn == nil || snapshots[sn.NodeID] != nil {
			return Err("INVALID_PACK", "快照为空或重复", 400)
		}
		if nodes[sn.NodeID] == nil {
			return Err("INVALID_PACK", "快照节点不存在", 400)
		}
		state, err := domain.UnmarshalWorld(sn.StateJSON)
		if err != nil {
			return Err("INVALID_PACK", "快照状态无法解析: "+err.Error(), 400)
		}
		if sn.StateHash != "" && !state.MatchesHash(sn.StateHash) {
			return Err("INVALID_PACK", "快照指纹无效", 400)
		}
		snapshots[sn.NodeID] = sn
	}
	root := snapshots[b.RootNodeID]
	if root == nil {
		return Err("INVALID_PACK", "剧情包缺少根状态快照", 400)
	}
	state, _ := domain.UnmarshalWorld(root.StateJSON)
	var rootContent struct {
		InitialState json.RawMessage `json:"initialState"`
	}
	if err := json.Unmarshal([]byte(nodes[b.RootNodeID].ContentJSON), &rootContent); err != nil {
		return Err("INVALID_PACK", "根节点内容无效", 400)
	}
	// Older archives may omit initialState; when present it must agree with the
	// authoritative root snapshot, including fields omitted by the v1 hash.
	if len(rootContent.InitialState) > 0 {
		declared, err := domain.UnmarshalWorld(string(rootContent.InitialState))
		if err != nil || declared.HashID() != state.HashID() {
			return Err("INVALID_PACK", "根快照与声明的初始状态不一致", 400)
		}
	}
	events := map[string][]*domain.DomainEvent{}
	for _, ev := range b.Events {
		events[ev.NodeID] = append(events[ev.NodeID], ev)
	}
	children := map[string][]string{}
	for _, n := range b.Nodes {
		children[n.ParentID] = append(children[n.ParentID], n.NodeID)
	}
	states := map[string]*domain.WorldState{b.RootNodeID: state}
	queue := []string{b.RootNodeID}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		for _, id := range children[parent] {
			next := states[parent].Clone()
			sort.Slice(events[id], func(i, j int) bool { return events[id][i].EventIndex < events[id][j].EventIndex })
			if err := domain.ReplayEvents(next, events[id]); err != nil {
				return Err("INVALID_PACK", fmt.Sprintf("节点 %s 的事件无法重放: %v", id, err), 400)
			}
			if sn := snapshots[id]; sn != nil {
				actual, _ := domain.UnmarshalWorld(sn.StateJSON)
				if actual.HashID() != next.HashID() {
					return Err("INVALID_PACK", fmt.Sprintf("节点 %s 的快照与事件重放不一致，请保留原包并核验该节点", id), 400)
				}
			}
			states[id] = next
			queue = append(queue, id)
		}
		delete(states, parent)
	}
	return nil
}

// decodeRootCardRef 读取根节点声明的角色卡引用。解析失败时告警并返回空串
// （调用方据此跳过角色归属校验，不因此拒绝整包——后续快照校验会独立判定内容有效性）。
func decodeRootCardRef(contentJSON string) string {
	var root struct {
		CardRef string `json:"cardRef"`
	}
	if err := json.Unmarshal([]byte(contentJSON), &root); err != nil {
		slog.Warn("根节点内容解析失败，跳过角色归属校验", "error", err)
	}
	return root.CardRef
}
