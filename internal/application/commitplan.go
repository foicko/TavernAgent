package application

import (
	"encoding/json"
	"fmt"
	"strings"

	"tavernagent/internal/domain"
	"tavernagent/internal/protocol"
)

// planData 是校验后、提交前的完整计划数据。
type planData struct {
	Events      []*domain.DomainEvent
	Memories    []*domain.MemoryRecord
	NewState    *domain.WorldState
	Blocks      []domain.TextBlock
	Options     []domain.Option
	Mood        *domain.Mood
	ContentJSON string
}

// RulesetVersion 是首版简化规则的版本标识。
const RulesetVersion = "ruleset.simplified.v1"

// buildPlan 校验模型提议并生成领域事件与提交后状态。
// mode 为解析模式（structured | narrative），记入节点来源用于重放与计量。
// nodeID 是本次提交将要创建的剧情节点 ID：记忆的 source_node_id 与 ID 都由它派生，
// 保证记忆可回溯到来源节点、且 ID 全局唯一（模型给的 proposalId 只在单次尝试内唯一）。
// 硬操作（item_transfer/consume 等）在 M0 未授权（C08：必须有动作授权），一律拒绝。
// planContext 是 buildPlan 需要的会话级规则材料。
//
// 它们不属于"这一回合的输入"，而是"这个会话用哪套规则"：
// 规则包（授权面 + 检定）与秘密定义（条件揭示）。
type planContext struct {
	ruleset domain.Ruleset
	// RulesetVersion 写进事件与快照。它与 ruleset 分开传，因为旧数据
	// （迁移前的空串）要回退到当前常量，二者不总是一一对应。
	RulesetVersion  string
	Secrets         []domain.SecretDef
	Checks          []domain.CheckResult
	HistoryExcerpts []domain.EvidenceExcerpt
	VisibleMemories []*domain.MemoryRecord
	CurrentTurn     int
	// InjectedMemories 是本次生成注入上下文的记忆（展示快照）。
	// 它随节点落库，是“为什么它会这么演”的依据。
	InjectedMemories []domain.MemoryRef
	// PreviousTurnHadOptions 表示上一层回合是否**呈现过**选项。
	// auto 模式据此避免连续两轮都给选项——抉择之后必有一轮承接后果，
	// 那本身就不再是新的抉择点（详见 buildPlan 里的门槛）。
	PreviousTurnHadOptions bool
}

func buildPlan(base *domain.WorldState, draft protocol.TurnDraft, input domain.TurnInput, mode, nodeID string, pc planContext) (*planData, error) {
	rulesetVersion := pc.RulesetVersion
	ruleset := pc.ruleset
	pd := &planData{NewState: base.Clone()}
	pd.Blocks = make([]domain.TextBlock, 0, len(draft.Blocks))
	for _, b := range draft.Blocks {
		pd.Blocks = append(pd.Blocks, domain.TextBlock{Kind: string(b.Kind), SpeakerID: derefStr(b.SpeakerID), Text: b.Text})
	}
	pd.Options = make([]domain.Option, 0, len(draft.Options))
	for _, o := range draft.Options {
		pd.Options = append(pd.Options, domain.Option{
			OptionID: o.OptionID, Intent: o.Intent, Text: o.Text,
			// 未授权的动作引用不写入选项（技术契约 §4）：模型自造的 actionRef
			// 不能进入授权面，否则 M4 引入动作执行后会被误执行。
			ActionRef: authorizeActionRef(o.ActionRef, ruleset),
		})
	}
	// OptionsNever 是硬保证：明确要求关闭选项时，即使模型仍写了四个也一律清空。
	// 仅在提示词里约定是不够的——模型不听话时，玩家看到的就是“我明明关了还在弹”。
	//
	// auto 另有一道**确定性**门槛：不连续两轮都给选项。提示词里的判据（“只在关键
	// 节点给”）在真实模型上不够硬——紧张场景里它倾向于认为每轮都是抉择点，而读者
	// 得到的体验就是选项变成了固定配菜。抉择之后必然有一轮要承接后果，那本身就不是
	// 新的抉择点，所以这条规则是叙事上的，不是随手挑的窗口。
	optionsMode := domain.NormalizeOptionsMode(input.Options)
	suppressed := 0
	if optionsMode == domain.OptionsNever {
		pd.Options = nil
	} else if optionsMode == domain.OptionsAuto && pc.PreviousTurnHadOptions && len(pd.Options) > 0 {
		suppressed = len(pd.Options)
		pd.Options = nil
	}

	// 关系增量同轮同角色同维度聚合（契约 §5.1）。
	type charField struct {
		char  string
		field domain.RelationshipField
	}
	aggDeltas := map[charField][]int{}
	proposalOrder := []charField{}

	probe := pd.NewState.Clone()
	ruleEvents, err := validatedRuleEvents(base, pc.Checks, rulesetVersion)
	if err != nil {
		return nil, err
	}
	pd.Events = append(pd.Events, ruleEvents...)
	if _, err := domain.ApplyEvents(probe, ruleEvents); err != nil {
		return nil, err
	}

	for _, p := range draft.Proposals {
		switch p.Type {
		case "relationship_delta":
			if _, ok := probe.Characters[p.CharacterID]; !ok || p.CharacterID == "player" {
				continue
			}
			f := domain.RelationshipField(p.Field)
			if f != domain.FieldAffection && f != domain.FieldTrust && f != domain.FieldAlertness {
				continue
			}
			cf := charField{char: p.CharacterID, field: f}
			if _, seen := aggDeltas[cf]; !seen {
				proposalOrder = append(proposalOrder, cf)
			}
			aggDeltas[cf] = append(aggDeltas[cf], p.Delta)
		case "mood_set":
			if _, ok := probe.Characters[p.CharacterID]; !ok || p.CharacterID == "player" {
				continue
			}
			mood := domain.CharacterMood{MoodCode: p.MoodCode, Text: p.Text}
			if pd.Mood == nil {
				pd.Mood = &domain.Mood{CharacterID: p.CharacterID, MoodCode: p.MoodCode, Text: p.Text}
			}
			payload, _ := json.Marshal(map[string]any{"characterId": p.CharacterID, "mood": mood})
			pd.Events = append(pd.Events, &domain.DomainEvent{
				Type: domain.EventMoodSet, PayloadJSON: string(payload), RulesetVersion: rulesetVersion,
			})
		case "memory_add":
			// 来源化认知记录（M3）：写入 memory_records 并留事件。
			content := strings.TrimSpace(p.Text)
			if content == "" {
				continue
			}
			kind := normalizeMemoryKind(p.Confidence)
			if p.MemoryKind != "" {
				kind = normalizeMemoryKind(p.MemoryKind)
			}
			if kind == domain.MemoryInferred && strings.TrimSpace(p.Reasoning) == "" && strings.TrimSpace(p.SourceQuote) != "" {
				kind = domain.MemoryObserved
			}

			resolvedEntities := make([]string, 0, len(p.EntityIDs))
			for _, eid := range p.EntityIDs {
				resolved := domain.ResolveEntityID(eid, probe)
				if _, ok := probe.Characters[resolved]; ok {
					resolvedEntities = append(resolvedEntities, resolved)
				} else if _, ok := probe.Items[resolved]; ok {
					resolvedEntities = append(resolvedEntities, resolved)
				} else if _, ok := probe.Promises[resolved]; ok {
					resolvedEntities = append(resolvedEntities, resolved)
				}
			}
			resolvedOwners := make([]string, 0, len(p.Participants))
			for _, oid := range p.Participants {
				resolved := domain.ResolveEntityID(oid, probe)
				if _, ok := probe.Characters[resolved]; ok {
					resolvedOwners = append(resolvedOwners, resolved)
				}
			}

			excerpts := make([]domain.EvidenceExcerpt, 0, len(pc.HistoryExcerpts)+1+len(draft.Blocks))
			excerpts = append(excerpts, pc.HistoryExcerpts...)
			excerpts = append(excerpts, domain.EvidenceExcerpt{NodeID: nodeID, Text: input.Text})
			for _, b := range draft.Blocks {
				excerpts = append(excerpts, domain.EvidenceExcerpt{NodeID: nodeID, Text: b.Text})
			}
			mem, err := domain.ValidateCognitiveMemory(domain.CognitiveMemory{
				Content: content, EntityIDs: resolvedEntities,
				OwnerIDs: resolvedOwners, SourceQuote: p.SourceQuote, Reasoning: p.Reasoning,
				Confidence: p.EvidenceConfidence, SubjectKey: p.SubjectKey,
			}, kind, probe, excerpts)
			if err != nil {
				return nil, fmt.Errorf("invalid memory proposal: %w", err)
			}
			mem.MemoryID, mem.SourceNodeID = "mem_"+nodeID+"_"+p.ProposalID, nodeID
			mem.CreatedTurn = pc.CurrentTurn
			mem.ValidFromTurn = pc.CurrentTurn

			if mem.SubjectKey != "" {
				var supersededID string
				for _, prev := range pd.Memories {
					if prev.SubjectKey == mem.SubjectKey {
						supersededID = prev.MemoryID
						prev.ValidUntilTurn = pc.CurrentTurn
					}
				}
				if supersededID == "" && len(pc.VisibleMemories) > 0 {
					activePath := domain.ApplyMemoryOverlays(pc.VisibleMemories)
					for _, prev := range activePath {
						if prev.SubjectKey == mem.SubjectKey {
							supersededID = prev.MemoryID
							break
						}
					}
				}
				if supersededID != "" {
					mem.Supersedes = supersededID
				}
			}

			pd.Memories = append(pd.Memories, mem)
			payload, _ := json.Marshal(domain.MemoryAddPayload{Memory: *mem})
			pd.Events = append(pd.Events, &domain.DomainEvent{
				Type: domain.EventMemoryAdd, PayloadJSON: string(payload), RulesetVersion: rulesetVersion,
			})
		case "item_grant", "item_transfer", "item_consume":
			if !authorizedItemProposal(p, pc.Checks) {
				return nil, fmt.Errorf("%w: item change requires an exact rule receipt", ErrOperationRejected)
			}
			// The authoritative effect was already added from the receipt.
		case "promise_propose":
			content := strings.TrimSpace(p.Text)
			if content != "" {
				promID := "prom_" + nodeID + "_" + p.ProposalID
				if _, exists := probe.Promises[promID]; !exists {
					participants := nonNilStrings(p.Participants)
					if len(participants) == 0 && p.CharacterID != "" {
						participants = []string{p.CharacterID}
					}
					prom := domain.Promise{
						PromiseID:      promID,
						ParticipantIDs: participants,
						Content:        content,
						SourceNodeID:   nodeID,
						State:          domain.PromiseProposed,
					}
					probe.Promises[promID] = prom
					payload, _ := json.Marshal(domain.PromiseProposePayload{Promise: prom})
					pd.Events = append(pd.Events, &domain.DomainEvent{
						Type: domain.EventPromisePropose, PayloadJSON: string(payload), RulesetVersion: rulesetVersion,
					})
				}
			}
		case "goal_set":
			if _, ok := probe.Characters[p.CharacterID]; !ok || p.CharacterID == "player" || len([]rune(strings.TrimSpace(p.Text))) > 400 {
				continue
			}
			payload, _ := json.Marshal(map[string]any{"goalId": "goal_" + p.CharacterID, "characterId": p.CharacterID, "text": strings.TrimSpace(p.Text)})
			pd.Events = append(pd.Events, &domain.DomainEvent{Type: domain.EventGoalSet, PayloadJSON: string(payload), RulesetVersion: rulesetVersion})
		case "secret_propose", "promise_settle", "scene_propose", "milestone_propose":
			// 模型可以提议解锁秘密或结算，但无权自证达成；跳过不报错
			continue
		default:
			// 未知提议类型忽略跳过，不中断整轮文学成果提交
			continue
		}
	}

	for _, cf := range proposalOrder {
		sum := domain.AggregateDeltas(aggDeltas[cf])
		if sum == 0 {
			continue
		}
		applied, err := probe.ApplyRelationshipDelta(cf.char, cf.field, sum, domain.AffectionMax, domain.AffectionMax)
		if err != nil {
			return nil, err
		}
		payload, _ := json.Marshal(domain.RelationshipDeltaPayload{
			CharacterID: cf.char, Field: string(cf.field), Delta: sum, Applied: applied,
		})
		pd.Events = append(pd.Events, &domain.DomainEvent{
			Type: domain.EventRelationshipDelta, PayloadJSON: string(payload), RulesetVersion: rulesetVersion,
		})
	}

	// 状态只有一处实现：用与重放完全相同的入口应用本次事件。
	// 早期实现在这里手写一遍转移（改 Moods、调 ApplyRelationshipDelta），
	// 与 domain.ApplyEvent 构成两份必须永远一致的逻辑——改一处忘一处就会
	// 让提交出的快照与重放结果悄悄分叉，而没有任何机制会报错。
	if _, err := domain.ApplyEvents(pd.NewState, pd.Events); err != nil {
		return nil, fmt.Errorf("apply events to new state: %w", err)
	}

	// 条件揭示（M4d）：对每个未解锁且有条件的秘密，按**本回合效果生效后**
	// 的状态求值。这决定了时序：条件在第 N 轮达成 → 解锁事件落在第 N 轮
	// （与其它效果同事务，提交与重放必然一致）→ 内容最早在第 N+1 轮的
	// 上下文出现。第 N 轮的正文不可能"声称已获得"——它的上下文在解锁判定
	// 之前就编译完了，里面根本没有这条内容（T25 的另一半）。
	//
	// 求值失败必须让整轮失败：静默跳过会让秘密永远无法揭示，而且没人知道为什么。
	var unlockEvents []*domain.DomainEvent
	for _, def := range pc.Secrets {
		if def.RevealWhen == nil || pd.NewState.IsSecretUnlocked(def.SecretID) {
			continue
		}
		ok, err := def.RevealWhen.Eval(ruleContextOf(pd.NewState))
		if err != nil {
			return nil, fmt.Errorf("%w: 秘密 %q 条件求值失败: %v", ErrOperationRejected, def.SecretID, err)
		}
		if !ok {
			continue
		}
		payload, _ := json.Marshal(domain.SecretUnlockPayload{SecretID: def.SecretID, Title: def.Title})
		unlockEvents = append(unlockEvents, &domain.DomainEvent{
			Type: domain.EventSecretUnlock, PayloadJSON: string(payload), RulesetVersion: rulesetVersion,
		})
	}
	if len(unlockEvents) > 0 {
		if _, err := domain.ApplyEvents(pd.NewState, unlockEvents); err != nil {
			return nil, fmt.Errorf("apply secret unlock events: %w", err)
		}
		pd.Events = append(pd.Events, unlockEvents...)
	}

	// 提交内容 JSON。
	tc := domain.TurnContent{
		InputKind: input.Kind, InputText: input.Text, InputNote: input.Note,
		OptionRef: input.OptionRef, ActionRef: input.ActionRef,
		Blocks: pd.Blocks, Options: pd.Options, Mood: pd.Mood, Checks: pc.Checks,
		// 变化摘要从**已生效的事件**推导，与真正落库的东西同源：
		// 这一层是给玩家核验用的，一旦与事件不一致就从证据变成了掩护。
		Changes:          domain.SummarizeChanges(pd.Events, pd.NewState),
		InjectedMemories: pc.InjectedMemories,
		OptionsMode:      optionsMode,
		// 收起台阶也要留痕：读者分得出“本轮本无抉择”与“系统没有摆出选项”。
		SuppressedOptions: suppressed,
		Provenance:        &domain.TurnProvidence{Mode: normalizeMode(mode)},
	}
	cb, err := json.Marshal(tc)
	if err != nil {
		return nil, err
	}
	pd.ContentJSON = string(cb)
	return pd, nil
}

// normalizeMode 归一化解析模式：只有明确的 narrative 才记为 narrative，
// 其余（含空值）一律按 structured 处理（契约 §4：模式记录在请求与节点中）。
func normalizeMode(mode string) string {
	if mode == protocol.ModeNarrative {
		return protocol.ModeNarrative
	}
	return protocol.ModeStructured
}

// normalizeMemoryKind 归一化置信类别（契约 §5：memory_add 需给出「置信类别」）。
// 未知或缺失取值按 observed 处理：observed 是最保守的类别，
// inferred 才需要显式声明（推断不当作世界事实）。
func normalizeMemoryKind(v string) domain.MemoryKind {
	switch domain.MemoryKind(v) {
	case domain.MemoryReported:
		return domain.MemoryReported
	case domain.MemoryInferred:
		return domain.MemoryInferred
	default:
		return domain.MemoryObserved
	}
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
