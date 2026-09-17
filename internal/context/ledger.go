package context

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"tavernagent/internal/domain"
)

// ============================================================================
// Domain State Ledger (M4h) —— 确定性硬状态下界账本
// ============================================================================
//
// 长文本摘要 (SummaryArtifact) 在多轮演进中天然存在实体遗忘、信物凭空转移、
// 检定翻案的幻觉缺陷。本账本由 Go 领域层直接从状态投影 (WorldState) 编译一段
// 不可被叙事反转的 Authoritative Markdown 硬下界，注入到摘要之前，作为模型的
// 「物理现实 Ground Truth」。渲染是纯函数、只读、确定性、无状态。
// 设计依据：项目现状与待办（docs/PROJECT_STATUS.md）。

// LedgerCheck 是一次欲锚定进账本的历史大检定。
type LedgerCheck struct {
	TurnNumber int
	Result     domain.CheckResult
}

// LedgerRelationDelta 记录一次角色好感/信任/戒备的权威变动。
type LedgerRelationDelta struct {
	TurnNumber  int    `json:"turnNumber"`
	CharacterID string `json:"characterId"`
	Field       string `json:"field"`
	Delta       int    `json:"delta"`
	Applied     int    `json:"applied"`
}

// RenderDomainLedger 渲染权威硬下界块；无任何账本内容时返回空串。
//
// playerName 用于誓言「X 对玩家立誓」的姓名映射（CharacterInfo 名取不到时兜底）。
func RenderDomainLedger(state *domain.WorldState, checks []LedgerCheck, playerName string, relations []LedgerRelationDelta) string {
	if state == nil {
		return ""
	}
	var b strings.Builder
	wrote := false

	if items := renderLedgerItems(state.Items, state.Characters); items != "" {
		b.WriteString(items)
		wrote = true
	}
	if pledges := renderLedgerPromises(state.Promises, state.Characters, playerName); pledges != "" {
		b.WriteString(pledges)
		wrote = true
	}
	if receipts := renderLedgerReceipts(checks); receipts != "" {
		b.WriteString(receipts)
		wrote = true
	}
	if miles := renderLedgerMilestones(state.Milestones); miles != "" {
		b.WriteString(miles)
		wrote = true
	}
	if rels := renderLedgerRelationships(relations, state.Characters, state.Relationships); rels != "" {
		b.WriteString(rels)
		wrote = true
	}
	if !wrote {
		return ""
	}

	var out strings.Builder
	out.WriteString("<domain_state_ledger authoritative=\"true\">\n")
	out.WriteString("### 物理现实与契约硬下界（Data, Strictly Authoritative - 不可被叙事反转）\n")
	out.WriteString(b.String())
	out.WriteString("</domain_state_ledger>")
	return out.String()
}

// renderLedgerRelationships 渲染角色关键关系与好感轨迹账本。
// 按角色分组，仅输出变动转折点（单角色至多保留最新 10 条变动防止超长会话膨胀），
// 并附带当前最新状态。
func renderLedgerRelationships(relations []LedgerRelationDelta, chars map[string]domain.CharacterInfo, currentRels map[string]domain.RelationValue) string {
	if len(relations) == 0 {
		return ""
	}
	grouped := map[string][]LedgerRelationDelta{}
	for _, r := range relations {
		if r.CharacterID == "" {
			continue
		}
		grouped[r.CharacterID] = append(grouped[r.CharacterID], r)
	}
	if len(grouped) == 0 {
		return ""
	}
	charIDs := sortedKeys(grouped)
	var b strings.Builder
	b.WriteString("- 💖 **角色关系轨迹（权威变化记录）**:\n")
	for _, cid := range charIDs {
		deltas := grouped[cid]
		name := cid
		if ch, ok := chars[cid]; ok && ch.Name != "" {
			name = ch.Name
		}
		sort.SliceStable(deltas, func(i, j int) bool {
			return deltas[i].TurnNumber < deltas[j].TurnNumber
		})
		if len(deltas) > 10 {
			deltas = deltas[len(deltas)-10:]
		}
		var parts []string
		for _, d := range deltas {
			fieldLabel := relationFieldLabel(d.Field)
			parts = append(parts, fmt.Sprintf("Turn#%d [%s %+d]", d.TurnNumber, fieldLabel, d.Applied))
		}
		curText := ""
		if cur, ok := currentRels[cid]; ok {
			curText = fmt.Sprintf(" (当前: 好感 %d, 信任 %d, 戒备 %d)", cur.Affection, cur.Trust, cur.Alertness)
		}
		b.WriteString(fmt.Sprintf("  * %s (%s): %s%s\n", name, cid, strings.Join(parts, "; "), curText))
	}
	return b.String()
}

func relationFieldLabel(f string) string {
	switch f {
	case "affection":
		return "好感"
	case "trust":
		return "信任"
	case "alertness":
		return "戒备"
	default:
		return f
	}
}

// renderLedgerItems 渲染物品/信物账本。非玩家持有的物品也一并列出（含场景/地点
// 持有的），因为信物是否易手、归属何方也是硬事实。数量>1 标 ×N，信物强标「无法丢弃」。
func renderLedgerItems(items map[string]domain.ItemInstance, chars map[string]domain.CharacterInfo) string {
	if len(items) == 0 {
		return ""
	}
	entries := make([]domain.ItemInstance, 0, len(items))
	for _, it := range items {
		if it.Quantity <= 0 || it.OwnerID == "consumed" || it.OwnerID == "discard" {
			continue
		}
		entries = append(entries, it)
	}
	if len(entries) == 0 {
		return ""
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Name != entries[j].Name {
			return entries[i].Name < entries[j].Name
		}
		return entries[i].InstanceID < entries[j].InstanceID
	})

	var b strings.Builder
	b.WriteString("- 🎒 **持有物品账本**:\n")
	for _, it := range entries {
		unit := "枚"
		if it.Quantity > 1 {
			unit = "份"
		}
		holder := "地点/场景"
		if it.OwnerID == "player" {
			holder = "玩家"
		} else if it.OwnerID != "" {
			holder = it.OwnerID
			if ch, ok := chars[it.OwnerID]; ok && ch.Name != "" {
				holder = ch.Name
			}
		} else if it.LocationID != "" {
			holder = it.LocationID
		}
		tag := ""
		if it.Keepsake {
			tag = "[信物] "
		}
		line := fmt.Sprintf("  * %s%s (%d%s, 归属: %s)", tag, it.Name, it.Quantity, unit, holder)
		if it.Keepsake {
			line += ", 无法丢弃"
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

// renderLedgerPromises 渲染誓言/约定账本。
// 姓名映射：参与方里非 player 者取 Character 名（兜底 ID）在前，玩家名在后。
func renderLedgerPromises(promises map[string]domain.Promise, chars map[string]domain.CharacterInfo, playerName string) string {
	if len(promises) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("- 📜 **誓言与约定账本**:\n")
	for _, key := range sortedKeys(promises) {
		p := promises[key]
		if p.Content == "" {
			continue
		}
		other := ""
		for _, pid := range p.ParticipantIDs {
			if pid == "player" {
				continue
			}
			if ch, ok := chars[pid]; ok && ch.Name != "" {
				other = ch.Name
			} else {
				other = pid
			}
			break
		}
		target := playerName
		if target == "" {
			target = "player"
		}
		status := promiseLedgerLabel(p.State)
		if status == "" {
			continue // proposed 未定约，跳过
		}
		otherTxt := other
		if otherTxt == "" {
			otherTxt = "参与者"
		}
		b.WriteString(fmt.Sprintf("  * %s对%s立誓: \"%s\" %s\n", otherTxt, target, oneLine(p.Content), status))
	}
	if b.Len() == len("- 📜 **誓言与约定账本**:\n") {
		return ""
	}
	return b.String()
}

func promiseLedgerLabel(s domain.PromiseState) string {
	switch s {
	case domain.PromiseActive:
		return "[生效中 (active)]"
	case domain.PromiseFulfilled:
		return "[已达成 (fulfilled)]"
	case domain.PromiseBroken:
		return "[已破碎 (broken)]"
	case domain.PromiseCancelled:
		return "[已作废 (cancelled)]"
	default:
		return ""
	}
}

// renderLedgerReceipts 渲染历史大检定裁决账本。
func renderLedgerReceipts(checks []LedgerCheck) string {
	filtered := filterLedgerChecks(checks)
	if len(filtered) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("- 🎲 **历史大检定裁决**:\n")
	for _, c := range filtered {
		b.WriteString("  * " + renderLedgerCheck(c) + "\n")
	}
	return b.String()
}

// filterLedgerChecks 应用入选标准并封顶条数：
// critical_success/critical_failure 恒选；普通 failure 仅当 DC>=15；按轮次降序取最近 N=10。
func filterLedgerChecks(checks []LedgerCheck) []LedgerCheck {
	type scored struct {
		turn int
		ck   LedgerCheck
	}
	out := make([]scored, 0, len(checks))
	for _, c := range checks {
		if c.Result.PermanentEffect != "" {
			out = append(out, scored{c.TurnNumber, c})
			continue
		}
		switch c.Result.Outcome {
		case domain.OutcomeCriticalSuccess, domain.OutcomeCriticalFailure:
			out = append(out, scored{c.TurnNumber, c})
		case domain.OutcomeFailure:
			if c.Result.DC >= 15 {
				out = append(out, scored{c.TurnNumber, c})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].turn > out[j].turn })
	res := make([]LedgerCheck, 0, len(out))
	ordinary := 0
	for _, s := range out {
		if s.ck.Result.PermanentEffect == "" {
			if ordinary >= 10 {
				continue
			}
			ordinary++
		}
		res = append(res, s.ck)
	}
	return res
}

// renderLedgerCheck 把一次大检定渲染成一行中文硬事实。
func renderLedgerCheck(c LedgerCheck) string {
	ck := c.Result
	text := fmt.Sprintf("Turn#%d [%s检定 DC%d]: 掷骰 %d%+d=%d → %s", c.TurnNumber,
		ck.Attribute, ck.DC, ck.Natural, ck.AttributeMod, ck.Total, outcomeText(ck.Outcome))
	if ck.PermanentEffect != "" {
		text += "；永久效果：" + oneLine(ck.PermanentEffect)
	}
	return text
}

// renderLedgerMilestones 渲染主线里程碑与世界标志账本。
func renderLedgerMilestones(ms map[string]string) string {
	if len(ms) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("- 🚩 **世界状态里程碑**:\n")
	for _, k := range sortedKeys(ms) {
		b.WriteString(fmt.Sprintf("  * %s (%s)\n", ms[k], k))
	}
	return b.String()
}

// compileLedger 从祖先路径已结算收据编译历史大检定账本（M4h）与关系轨迹，并收集秘密揭示回合。
// ReceiptsOnPath 返回本路径已结算收据，再解析为带回合序号的 LedgerCheck。
// EventsOnPath 读取路径上的关系增量与秘密解锁事件，支撑关系转折点与秘密时序。
// 读取失败或收据损坏时停止本轮准备，避免在缺少权威事实时继续生成。
func (c *Compiler) compileLedger(nodeID string, state *domain.WorldState, sc sessionContext) (string, map[string]int, error) {
	if state == nil {
		return "", nil, nil
	}
	receipts, err := c.store.ReceiptsOnPath(nodeID)
	if err != nil {
		return "", nil, fmt.Errorf("read authoritative ledger: %w", err)
	}
	var checks []LedgerCheck
	for _, rn := range receipts {
		if rn.Receipt == nil {
			continue
		}
		cr, cerr := rn.Receipt.CheckResultOf()
		if cerr != nil || cr.ActionID == "" {
			return "", nil, fmt.Errorf("invalid authoritative receipt %s", rn.Receipt.ReceiptID)
		}
		checks = append(checks, LedgerCheck{TurnNumber: rn.TurnNumber, Result: cr})
	}

	events, err := c.store.EventsOnPath(nodeID, domain.EventRelationshipDelta, domain.EventSecretUnlock)
	if err != nil {
		return "", nil, fmt.Errorf("read authoritative events: %w", err)
	}
	var relations []LedgerRelationDelta
	secretUnlockTurns := map[string]int{}
	for _, en := range events {
		if en.Event == nil {
			continue
		}
		switch en.Event.Type {
		case domain.EventRelationshipDelta:
			var p domain.RelationshipDeltaPayload
			if err := json.Unmarshal([]byte(en.Event.PayloadJSON), &p); err == nil {
				applied := p.Applied
				if applied == 0 && p.Delta != 0 {
					applied = p.Delta
				}
				relations = append(relations, LedgerRelationDelta{
					TurnNumber:  en.TurnNumber,
					CharacterID: p.CharacterID,
					Field:       p.Field,
					Delta:       p.Delta,
					Applied:     applied,
				})
			}
		case domain.EventSecretUnlock:
			var p domain.SecretUnlockPayload
			if err := json.Unmarshal([]byte(en.Event.PayloadJSON), &p); err == nil && p.SecretID != "" {
				if _, ok := secretUnlockTurns[p.SecretID]; !ok {
					secretUnlockTurns[p.SecretID] = en.TurnNumber
				}
			}
		}
	}

	return RenderDomainLedger(state, checks, sc.PlayerName, relations), secretUnlockTurns, nil
}
