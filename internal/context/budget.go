package context

import (
	"fmt"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// countTurns 统计节点列表中回合节点的数量（预算裁剪的保护下限按回合计）。
func countTurns(nodes []*domain.PlotNode) int {
	n := 0
	for _, node := range nodes {
		if node != nil && node.Kind == domain.NodeKindTurn {
			n++
		}
	}
	return n
}

// ReserveInput leaves space for required caller-supplied continuation messages.
func (c *Compiler) ReserveInput(tokens int) *Compiler {
	copy := *c
	copy.options.SafetyMargin += max(0, tokens)
	return &copy
}

// MessageTokens 是**未校准**的原始估算，用于观测与校准系数自身的计算
// （账本里 estimated 与真实 reported 的比值即系数来源）。预算决策请用
// Compiler.EstimateMessages，它带上真实用量校准。
func MessageTokens(messages []ports.ChatMessage) int {
	total := 0
	for _, m := range messages {
		total += EstimateTokens(m.Content) + 4
	}
	return total
}

// EstimateMessages 是带入校准系数的估算，供预算、尾部策略与摘要输入预算使用。
func (c *Compiler) EstimateMessages(messages []ports.ChatMessage) int {
	total := 0
	for _, m := range messages {
		total += c.tokens(m.Content) + 4
	}
	return total
}

// tokens 是单段文本的校准后估算。
func (c *Compiler) tokens(s string) int {
	return calibrateTokens(EstimateTokens(s), c.options.TokenCalibration)
}

// calibrateTokens 应用真实用量反推的校准系数；factor <= 0 表示不校准。
func calibrateTokens(raw int, factor float64) int {
	if factor <= 0 {
		return raw
	}
	return int(float64(raw)*factor + 0.5)
}

// Budget the final rendered messages, including summaries and message framing.
// Remove low-priority material before recent dialogue; mandatory state and the
// latest input are never silently truncated.
//
// 淘汰顺序（压力从低到高）：
//  1. 世界书（背景参考）
//  2. 记忆（得分低的先丢）
//  3. **不承重的摘要**——它覆盖的区间仍在历史里，丢掉不损失任何原文
//  4. 历史回合（最旧先丢，停在最近 N 轮保护下限）
//  5. **承重摘要**（其区间原文已被折叠移除）→ 不可淘汰，显式报错
//
// 第 3 与第 5 条是"摘要不可静默消失"这条不变量的两侧：历史还在时淘汰摘要
// 是纯收益；历史已不在时淘汰摘要等于删掉一段故事，只能整轮失败并告诉用户
// 怎么把窗口调够。契约 §9.1 对"不可省略材料"的要求同样适用于承重摘要。
func (c *Compiler) fitBundleBudget(lore []lorebookHit, memories []*domain.MemoryRecord, ancestors []*domain.PlotNode,
	summaries []*domain.SummaryArtifact,
	checks []domain.CheckResult, sc sessionContext, state *domain.WorldState, input, ledger string,
) ([]lorebookHit, []*domain.MemoryRecord, []*domain.PlotNode, []*domain.SummaryArtifact, BudgetReport, error) {
	ancestors = append([]*domain.PlotNode{}, ancestors...)
	budget := c.options.InputBudget()
	report := BudgetReport{Enabled: c.options.ContextWindow > 0, Budget: budget}
	measure := func() int {
		return c.EstimateMessages(c.buildMessages(lore, memories, ancestors, summaries, checks, sc, state, input, ledger, report.DroppedHistory))
	}
	report.BeforeTrim = measure()
	report.Total = report.BeforeTrim
	report.Mandatory = c.EstimateMessages(c.buildMessages(nil, nil, nil, nil, checks, sc, state, input, ledger, 0))
	if !report.Enabled {
		return lore, memories, ancestors, summaries, report, nil
	}
	if report.Mandatory > budget {
		material := "必要规则、人设、状态与输入"
		advice := "请缩短输入或调整模型窗口与输出预算。"
		if directorPrompt(sc.Director) != "" {
			material += "（包括当前导演阶段与全局要求）"
			advice = "请缩短导演阶段、全局要求或输入，或调整模型窗口与输出预算；导演安排不会被自动丢弃。"
		} else if c.planningInstruction != "" {
			material += "（包括导演草稿与讨论）"
			advice = "请缩小大纲与本次讨论范围，或调整导演模型的窗口与输出预算。"
		}
		return lore, memories, ancestors, summaries, report, &Error{Code: "CONTEXT_OVER_BUDGET", Message: "上下文预算不足：" + material + "需要 " + itoa(report.Mandatory) + " token，输入预算为 " + itoa(budget) + "。" + advice, Status: 422}
	}
	// 主动压缩阈值（借鉴 Reasonix window*0.80 与 LiveAgent 阶梯保护）：
	// 当总预估 Token 达到模型上下文窗口的 80%，或达到可用输入预算的 75% 时触发主动压缩。
	proactiveThreshold := 0
	if c.options.ContextWindow > 0 {
		proactiveThreshold = int(float64(c.options.ContextWindow) * 0.80)
	}
	report.PrepareCompression = (proactiveThreshold > 0 && report.BeforeTrim >= proactiveThreshold) || (budget > 0 && report.BeforeTrim*4 >= budget*3)
	// ProtectedTurns 是"最近 N 轮原文"的保护下限，与折叠策略共用同一常量：
	// 契约要求摘要只替代其覆盖区间、保留最近对话，因此预算裁剪不得突破它。
	protected := c.options.CompactionPolicy.TailWindowTurns
	if protected <= 0 {
		protected = 20
	}
	spanCache := map[string][2]int{}
	redundant := func(a *domain.SummaryArtifact) bool { return c.summaryRedundant(a, ancestors, spanCache) }
	summaryCost := func() int {
		if len(summaries) == 0 {
			return 0
		}
		withSummaries := c.EstimateMessages(c.buildMessages(nil, nil, nil, summaries, checks, sc, state, input, ledger, 0))
		without := c.EstimateMessages(c.buildMessages(nil, nil, nil, nil, checks, sc, state, input, ledger, 0))
		return max(0, withSummaries-without)
	}
	for report.Total > budget {
		switch {
		case len(lore) > 0:
			lore = lore[:len(lore)-1]
			report.DroppedLorebook++
		case len(memories) > 0:
			memories = memories[:len(memories)-1]
			report.DroppedMemories++
		// 冗余摘要（区间原文仍完整在历史里）先丢：丢掉不损失任何信息。
		case len(summaries) > 0 && redundant(summaries[0]):
			summaries = summaries[1:]
			report.DroppedSummaries++
		// 历史回合先于承重摘要被淘汰：单条摘要承载几十轮的信息密度远高于单个回合，
		// 丢摘要换空间会连旧故事一起丢掉。
		case len(ancestors) > 0 && countTurns(ancestors) > protected:
			ancestors = ancestors[1:]
			report.DroppedHistory++
		default:
			if cost := summaryCost(); cost > 0 {
				report.SummaryTokens = cost
				for _, a := range summaries {
					if !redundant(a) {
						report.LoadBearingSummaries++
					}
				}
				return lore, memories, ancestors, summaries, report, &Error{
					Code: "CONTEXT_OVER_BUDGET",
					Message: "上下文预算不足：历史摘要需要 " + itoa(cost) + " token，输入预算为 " + itoa(budget) +
						" token；它覆盖的原文已折叠移除，丢弃摘要会让模型彻底失去那一段历史。" +
						"请调大模型上下文窗口、降低输出预留，或缩短本次输入。",
					Status: 422,
				}
			}
			return lore, memories, ancestors, summaries, report, &Error{Code: "CONTEXT_OVER_BUDGET", Message: "无法在预算内保留必要上下文（已保护最近 " + itoa(protected) + " 轮原文）", Status: 422}
		}
		report.Total = measure()
	}
	report.ProtectedTurns = min(countTurns(ancestors), protected)
	for _, n := range ancestors {
		if tc, err := parseTurnContent(n.ContentJSON); err == nil {
			report.History += c.tokens(tc.InputText) + c.tokens(renderBlocks(tc.Blocks)) + 8
		}
	}
	return lore, memories, ancestors, summaries, report, nil
}

// omittedHistoryNotice 是历史被预算截断时写给模型的缺口标记。
// 借鉴 Reasonix 的 truncation marker：缺口必须显式说明，否则模型会把
// "看不到"当作"没发生"，在续写里编造那段时间。
func omittedHistoryNotice(turns int) string {
	return fmt.Sprintf("【历史缺口】更早的 %d 轮对话因上下文预算被省略，不要在缺少依据时编造那段时间发生的事。", turns)
}

// ---- 上下文预算（技术契约 §9.1）----

// BudgetReport 回报一次预算裁剪：裁掉了什么、剩多少空间。
// 观测用——"为什么模型没看到这段"应该能从这里找到答案，而不是靠猜。
type BudgetReport struct {
	Enabled          bool `json:"enabled"`
	Budget           int  `json:"budget,omitempty"`
	Mandatory        int  `json:"mandatory,omitempty"`
	History          int  `json:"history,omitempty"`
	DroppedLorebook  int  `json:"droppedLorebook,omitempty"`
	DroppedMemories  int  `json:"droppedMemories,omitempty"`
	DroppedHistory   int  `json:"droppedHistory,omitempty"`
	DroppedSummaries int  `json:"droppedSummaries,omitempty"`
	// ProtectedTurns 是本次裁剪后仍在保护窗口内的最近回合数（观测用）。
	ProtectedTurns int `json:"protectedTurns,omitempty"`
	// SummaryTokens 是摘要区块的实际开销；LoadBearingSummaries 是其中
	// "覆盖区间已折叠移除"的条数——它们不可淘汰，装不下即整轮失败。
	SummaryTokens        int  `json:"summaryTokens,omitempty"`
	LoadBearingSummaries int  `json:"loadBearingSummaries,omitempty"`
	Total                int  `json:"total"`
	BeforeTrim           int  `json:"beforeTrim"`
	PrepareCompression   bool `json:"prepareCompression"`
	// ForcedRecompile 已移除：它此前被计算并序列化，但全仓无任何消费方，
	// 属于"看着像有机制、其实没有"的死信号。压缩策略以 PrepareCompression
	// 与 policy.DecideCompaction 为准。
}

// fitToBudget 把可压缩材料收敛进输入预算。
//
// 契约 §9.1 的分配顺序是：必要宿主规则与行动结果 → 必要人设和当前状态 →
// 最新输入与近期回合 → 相关摘要/记忆。丢东西的顺序正好相反：
// 世界书（背景参考）→ 记忆（得分低的先丢）→ 历史（最旧的先丢）。
//
// 「不可省略」部分超预算时**报错**而不是静默截断——契约明确禁止
// "静默截去核心约束"；用户应当被告知缩短输入或更换模型配置。
func (c *Compiler) fitToBudget(lore []lorebookHit, memories []*domain.MemoryRecord, ancestors []*domain.PlotNode,
	checks []domain.CheckResult, sc sessionContext, state *domain.WorldState, inputText, ledger string,
) ([]lorebookHit, []*domain.MemoryRecord, []*domain.PlotNode, BudgetReport, error) {
	l, m, a, _, report, err := c.fitBundleBudget(lore, memories, ancestors, nil, checks, sc, state, inputText, ledger)
	return l, m, a, report, err
}
