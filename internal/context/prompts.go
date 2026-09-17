package context

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// FrameProtocolInstruction 是模型输出协议 v1 的提示词约束（技术契约 §4）。
// 必须与 internal/protocol 的字段名、枚举值、seq 规则保持一致，否则解析器会硬拒绝。
// 字段名必须逐个写清：模型倾向于把 optionId 简写为 id/label，导致 final 帧被拒绝。
const FrameProtocolInstruction = `【输出协议】
你只输出逐行 JSON，每行一个完整对象。不要输出解释、前言、标题或 markdown 代码围栏。

正文块帧：
{"v":1,"seq":N,"type":"block","kind":"narration","speakerId":null,"text":"正文内容"}
kind 只能是 narration（叙述）、dialogue（对白）、inner_monologue（文学心声）三类；
dialogue 需要把 speakerId 填为说话角色 ID，narration 与 inner_monologue 填 null。

最后一个帧必须是：
{"v":1,"seq":M,"type":"final","proposals":[],"options":[]}
其中 options 的元素格式为（最多 4 条）：
{"optionId":"o1","intent":"clever","text":"玩家可以采取的具体行动"}
intent 只能是 aggressive、clever、emotional、chaotic 四者之一。

proposals 的元素格式如下。没有状态变化时写空数组 []，不要使用未列出的类型：
1. 关系变化（好感/信任/戒备的增减）：
{"proposalId":"p1","type":"relationship_delta","characterId":"角色ID","field":"affection","delta":2,"evidenceBlockSeqs":[1]}
field 只能是 affection、trust、alertness；delta 为 -10 到 10 的整数。
2. 心情（角色当下的情绪状态）：
{"proposalId":"p2","type":"mood_set","characterId":"角色ID","moodCode":"calm","text":"简短的情绪描述"}
3. 记忆（仅记录本轮值得长期记住的信息）：
{"proposalId":"p3","type":"memory_add","text":"角色提到习惯在凌晨工作。","memoryKind":"reported","sourceQuote":"我习惯在凌晨工作","evidenceConfidence":"high","entityIds":["角色ID"],"participants":["角色ID"],"subjectKey":"角色ID.habit"}
memoryKind 只能是 observed（见证）、reported（转述）、inferred（推测）；inferred 必须另附 reasoning 推理依据。
subjectKey 为可选的主体属性点分小写键（如 npc_liel.status 或 character.identity），提供时自动覆盖同一主体的旧事实。
sourceQuote 必须直接逐字复制最近几轮对话或本轮正文中的原文片段（与原文一字不差，切勿概括或自行润色词句），high 至少六个字符；缺引文会降级。entityIds 与 participants 必须使用已提供的规范角色 ID（如 npc_xxx 或 player），推测不当作世界事实。
4. 约定提议（提议尚未等同于生效誓言，结算由规则决定）：
{"proposalId":"p4","type":"promise_propose","text":"明天正午在桥头会面","characterId":"角色ID"}
5. 非玩家角色的短期目标：
{"proposalId":"p5","type":"goal_set","characterId":"角色ID","text":"找回遗失的地图"}
物品授予、转移、消耗，誓言结算、场景跳转和里程碑由系统规则收据执行。只演绎收据已列明的效果，不要提出额外硬操作，不能凭正文创造资产或绕过秘密条件。
选项可以附 actionRef，但只能引用当前系统列出的规则动作。

硬性规则：
1. 字段名必须与上面完全一致。不得写成 id、label、content、option 等别名，不得省略或改名 optionId、proposalId。
2. v 恒为 1；seq 从 1 开始连续递增，不得跳号或重复；final 必须是最后一帧，其后不得再有任何内容。
3. proposals 与 options 即使为空也必须写出空数组 []。
4. 每个 text 字段内不要出现换行符，单块不超过 2048 个字符。
5. 除上述 JSON 行以外，不要输出任何其他文字。
`

func (c *Compiler) buildMessages(lore []lorebookHit, memories []*domain.MemoryRecord, ancestors []*domain.PlotNode, summaries []*domain.SummaryArtifact,
	checks []domain.CheckResult, sc sessionContext, state *domain.WorldState, inputText, ledger string, omittedTurns int) []ports.ChatMessage {
	if c.planningInstruction != "" {
		return c.buildPlanningMessages(lore, memories, ancestors, summaries, checks, sc, state, inputText, ledger)
	}
	var req ports.ChatRequest

	if c.options.SplitDynamicContext {
		req.Messages = append(req.Messages, ports.ChatMessage{
			Role: "system", Content: c.staticSystemPrompt(state, sc),
		})
	} else {
		req.Messages = append(req.Messages, ports.ChatMessage{
			Role: "system", Content: c.systemPrompt(state, lore, memories, checks, revealedSecrets(sc.Secrets, state), summaries, sc, ledger),
		})
	}

	// 开场白是故事的第一段正文，必须以 assistant 消息进入上下文，
	// 否则模型看不到故事是怎么开始的。
	if strings.TrimSpace(sc.OpeningText) != "" {
		req.Messages = append(req.Messages, ports.ChatMessage{Role: "assistant", Content: sc.OpeningText})
	}

	// 历史缺口标记：预算丢弃回合时必须在模型看得到的位置说明，否则"看不到"会被
	// 当成"没发生"（借鉴 Reasonix 的 truncation marker；此前缺口完全不可见）。
	if omittedTurns > 0 && countTurns(ancestors) > 0 {
		req.Messages = append(req.Messages, ports.ChatMessage{Role: "system", Content: omittedHistoryNotice(omittedTurns)})
	}
	for _, n := range ancestors {
		if n.Kind != domain.NodeKindTurn {
			continue
		}
		tc, err := parseTurnContent(n.ContentJSON)
		if err != nil {
			continue
		}
		// 未折叠区间的历史节点渲染结果在同分支内保持字节稳定，不随编译次数动态改变，
		// 消除 (turnCount - curTurnIdx) >= pruneDepth 导致每轮历史前缀失效的破坏性因素。
		req.Messages = append(req.Messages, ports.ChatMessage{Role: "user", Content: tc.InputText})
		req.Messages = append(req.Messages, ports.ChatMessage{Role: "assistant", Content: renderBlocksPruned(tc.Blocks, false)})
	}

	// 动态提示词分离（KV Cache 优化）：放在历史轮次之后、最新输入之前，
	// 确保静态系统设定和开场白拥有最大可能的缓存命中前缀。
	if c.options.SplitDynamicContext {
		dyn := c.dynamicContextPrompt(state, lore, memories, checks, revealedSecrets(sc.Secrets, state), summaries, ledger)
		if dyn != "" {
			req.Messages = append(req.Messages, ports.ChatMessage{
				Role:    "system",
				Content: "【当前情境与状态提示】\n" + dyn,
			})
		}
	}

	if instruction := directorPrompt(sc.Director); instruction != "" {
		req.Messages = append(req.Messages, ports.ChatMessage{Role: "system", Content: instruction})
	}
	req.Messages = append(req.Messages, c.planningMessages...)
	// 最新输入。
	if strings.TrimSpace(inputText) == "" {
		inputText = "..."
	}
	if sc.PlayerName != "" && c.planningInstruction == "" {
		inputText = fmt.Sprintf("%s：%s", sc.PlayerName, inputText)
	}
	req.Messages = append(req.Messages, ports.ChatMessage{Role: "user", Content: inputText})
	return req.Messages
}

func (c *Compiler) systemPrompt(state *domain.WorldState, lore []lorebookHit, memories []*domain.MemoryRecord,
	checks []domain.CheckResult, secrets []secretEntry, summaries []*domain.SummaryArtifact, sc sessionContext, ledger string) string {
	var b strings.Builder
	b.WriteString(c.roleInstruction())
	b.WriteString(renderActionRefs(sc.Rules))
	if ledger != "" {
		b.WriteString(ledger + "\n\n")
	}

	// 角色设定：来自世界状态里的角色静态信息。
	// 秘密条目不在此结构中（只存在于角色卡与其 UnlockedSecrets 的 ID 列表），
	// 因此这条注入路径在结构上不可能泄漏未揭示的秘密。
	if state != nil {
		var names []string
		for id, ch := range state.Characters {
			if id == "player" || !ch.Participant || strings.TrimSpace(ch.Description) == "" {
				continue
			}
			names = append(names, id)
		}
		sort.Strings(names)
		if len(names) > 0 {
			b.WriteString("【角色设定】\n")
			for _, id := range names {
				ch := state.Characters[id]
				b.WriteString(fmt.Sprintf("- %s（ID: %s）：%s\n", ch.Name, id, oneLine(ch.Description)))
			}
		}
	}

	// 世界书命中：关键词命中的条目原文。
	b.WriteString(renderLoreSection(lore))
	b.WriteString(renderMemorySection(memories))

	// 检定结果：已经掷过骰、已经写进收据的事实。
	// 原文给出骰值与判定，模型没有任何裁量空间——只给结果会让它描写不出
	// "险胜"还是"轻松"，只给骰值则等于把判定权又交回给了模型。
	if len(checks) > 0 {
		b.WriteString("\n【检定结果】后端已掷骰，必须照此演绎；不要改写数值、不要重新判定成败、不要另造结果。\n")
		for _, ck := range checks {
			b.WriteString("- " + renderCheck(ck) + "\n")
		}
	}

	// 已揭示的秘密/世界观：只在解锁后进入上下文（T25）。
	// 这里**只可能**看到已解锁的条目——过滤发生在 Compile 里，
	// 本函数拿不到未解锁条目的内容，结构上无法泄漏。
	if len(secrets) > 0 {
		b.WriteString(renderSecretsSection(secrets))
	}

	// 相关摘要（T23/T24）：只含来源区间在当前路径上的条目（过滤在 Compile）。
	// 摘要是叙述性回顾，**不是事实来源**——物品/关系/承诺以状态投影为准，
	// 这句话必须写进提示词：模型无法自行区分"摘要说的"和"状态说的"。
	if len(summaries) > 0 {
		b.WriteString("\n【相关摘要】以下是更早剧情的回顾，帮助保持连贯。它只是回顾：物品、承诺与关系等当前事实一律以本提示中的状态信息为准，与摘要冲突时以状态为准。\n")
		for _, a := range summaries {
			txt := strings.TrimSpace(a.Text)
			if strings.HasPrefix(txt, "<story_checkpoint>") {
				b.WriteString(txt + "\n")
			} else {
				b.WriteString("- " + oneLine(txt) + "\n")
			}
		}
	}

	if sc.PlayerName != "" {
		b.WriteString("【玩家】玩家扮演「" + sc.PlayerName + "」")
		if strings.TrimSpace(sc.PlayerRole) != "" {
			b.WriteString("，身份：" + oneLine(sc.PlayerRole))
		}
		if c.planningInstruction == "" {
			b.WriteString("。用第二人称叙述玩家所见所感，不要替玩家做决定。\n")
		} else {
			b.WriteString("。\n")
		}
	}
	b.WriteString(renderCharacterState(state))
	return b.String()
}

func (c *Compiler) staticSystemPrompt(state *domain.WorldState, sc sessionContext) string {
	var b strings.Builder
	b.WriteString(c.roleInstruction())
	b.WriteString(renderActionRefs(sc.Rules))

	if state != nil {
		var names []string
		for id, ch := range state.Characters {
			if id == "player" || !ch.Participant || strings.TrimSpace(ch.Description) == "" {
				continue
			}
			names = append(names, id)
		}
		sort.Strings(names)
		if len(names) > 0 {
			b.WriteString("【角色设定】\n")
			for _, id := range names {
				ch := state.Characters[id]
				b.WriteString(fmt.Sprintf("- %s（ID: %s）：%s\n", ch.Name, id, oneLine(ch.Description)))
			}
		}
	}

	if sc.PlayerName != "" {
		b.WriteString("【玩家】玩家扮演「" + sc.PlayerName + "」")
		if strings.TrimSpace(sc.PlayerRole) != "" {
			b.WriteString("，身份：" + oneLine(sc.PlayerRole))
		}
		if c.planningInstruction == "" {
			b.WriteString("。用第二人称叙述玩家所见所感，不要替玩家做决定。\n")
		} else {
			b.WriteString("。\n")
		}
	}
	return b.String()
}

func (c *Compiler) dynamicContextPrompt(state *domain.WorldState, lore []lorebookHit, memories []*domain.MemoryRecord,
	checks []domain.CheckResult, secrets []secretEntry, summaries []*domain.SummaryArtifact, ledger string) string {
	var b strings.Builder
	// 账本作为权威真值先行，先于世界书与记忆等可压缩材料——它不可被叙事反转。
	if ledger != "" {
		b.WriteString(ledger + "\n")
	}
	b.WriteString(renderLoreSection(lore))
	b.WriteString(renderMemorySection(memories))

	if len(checks) > 0 {
		b.WriteString("\n【检定结果】后端已掷骰，必须照此演绎；不要改写数值、不要重新判定成败、不要另造结果。\n")
		for _, ck := range checks {
			b.WriteString("- " + renderCheck(ck) + "\n")
		}
	}

	if len(secrets) > 0 {
		b.WriteString(renderSecretsSection(secrets))
	}

	if len(summaries) > 0 {
		b.WriteString("\n【相关摘要】以下是更早剧情的回顾，帮助保持连贯。它只是回顾：物品、承诺与关系等当前事实一律以本提示中的状态信息为准，与摘要冲突时以状态为准。\n")
		for _, a := range summaries {
			txt := strings.TrimSpace(a.Text)
			if strings.HasPrefix(txt, "<story_checkpoint>") {
				b.WriteString(txt + "\n")
			} else {
				b.WriteString("- " + oneLine(txt) + "\n")
			}
		}
	}

	b.WriteString(renderCharacterState(state))
	return strings.TrimSpace(b.String())
}

// oneLine 把条目内容压成单行，避免注入段破坏提示词的逐行结构。
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func renderBlocks(blocks []domain.TextBlock) string {
	return renderBlocksPruned(blocks, false)
}

func renderBlocksPruned(blocks []domain.TextBlock, pruneInnerMonologue bool) string {
	var b strings.Builder
	for _, blk := range blocks {
		switch blk.Kind {
		case "dialogue":
			b.WriteString(blk.Text)
		case "inner_monologue":
			if !pruneInnerMonologue {
				b.WriteString("（" + blk.Text + "）")
			}
		default:
			b.WriteString(blk.Text)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func parseTurnContent(content string) (*domain.TurnContent, error) {
	var tc domain.TurnContent
	if err := json.Unmarshal([]byte(content), &tc); err != nil {
		return nil, err
	}
	return &tc, nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// renderCheck 把一次检定渲染成一句中文事实陈述。
func renderCheck(ck domain.CheckResult) string {
	return fmt.Sprintf("%s（%s 属性 %d，修正 %+d）：骰值 %d，总值 %d，难度 %d → %s",
		ck.ActionID, ck.Attribute, ck.AttributeVal, ck.AttributeMod,
		ck.Natural, ck.Total, ck.DC, outcomeText(ck.Outcome))
}

func outcomeText(o domain.CheckOutcome) string {
	switch o {
	case domain.OutcomeCriticalSuccess:
		return "大成功"
	case domain.OutcomeSuccess:
		return "成功"
	case domain.OutcomeFailure:
		return "失败"
	case domain.OutcomeCriticalFailure:
		return "大失败"
	default:
		return "未知"
	}
}

// revealedSecrets 过滤出已解锁的秘密定义，保持卡片声明顺序。
func revealedSecrets(defs []secretEntry, state *domain.WorldState) []secretEntry {
	if state == nil || len(defs) == 0 {
		return nil
	}
	out := make([]secretEntry, 0, len(defs))
	for _, d := range defs {
		if state.IsSecretUnlocked(d.SecretID) {
			out = append(out, d)
		}
	}
	return out
}

// renderLoreSection / renderMemorySection 抽成独立函数：
// 预算裁剪要反复测量这两段的大小（契约 §9.1 的分配顺序），不能靠拼字符串再猜。
func renderLoreSection(lore []lorebookHit) string {
	if len(lore) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("世界设定（与当前情境相关，仅作背景参考；不要直接复述原文，也不要让角色知道玩家本不该知道的信息）：\n")
	for _, h := range lore {
		b.WriteString("- " + oneLine(h.Entry.Content) + "\n")
	}
	return b.String()
}

func renderMemorySection(memories []*domain.MemoryRecord) string {
	if len(memories) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("已知记忆（此前剧情中形成的认知，仅用于保持连续性；不要直接复述，也不要让角色知道玩家本不该知道的信息）：\n")
	for _, m := range memories {
		label := string(m.Kind)
		if m.Kind == domain.MemoryInferred {
			label = "推测，尚未被叙事确证"
		}
		if m.Evidence != nil {
			if m.Evidence.AutoDowngraded {
				label += "，引文不足降级"
			}
			if m.Evidence.Confidence != "" {
				label += ", 置信度=" + m.Evidence.Confidence
			}
		}
		b.WriteString("- [" + label + "] " + oneLine(m.Content) + "\n")
	}
	return b.String()
}

func renderSecretsSection(secrets []secretEntry) string {
	if len(secrets) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n【已揭示的世界观与秘密】以下是此前剧情中已经揭示的事实，角色此刻已知；可用于叙述，但不要一次性倾倒给玩家。\n")
	for _, sec := range secrets {
		turnPrefix := ""
		if sec.UnlockTurn > 0 {
			turnPrefix = fmt.Sprintf("[第%d轮揭示] ", sec.UnlockTurn)
		}
		if sec.Title != "" {
			b.WriteString("- " + turnPrefix + sec.Title + "：" + oneLine(sec.Content) + "\n")
			continue
		}
		b.WriteString("- " + turnPrefix + oneLine(sec.Content) + "\n")
	}
	return b.String()
}

// buildPlanningMessages 组装导演协商模式的消息：不把开场白与历史回合作为
// 多轮对话注入（避免模型产生角色扮演续写惯性），全部材料以只读参考块给出。
func (c *Compiler) buildPlanningMessages(lore []lorebookHit, memories []*domain.MemoryRecord, ancestors []*domain.PlotNode, summaries []*domain.SummaryArtifact,
	checks []domain.CheckResult, sc sessionContext, state *domain.WorldState, inputText, ledger string) []ports.ChatMessage {
	var b strings.Builder
	b.WriteString(c.roleInstruction())
	// 角色设定（只读参考）
	if state != nil {
		var names []string
		for id, ch := range state.Characters {
			if id == "player" || !ch.Participant || strings.TrimSpace(ch.Description) == "" {
				continue
			}
			names = append(names, id)
		}
		sort.Strings(names)
		if len(names) > 0 {
			b.WriteString("【角色设定（只读参考）】\n")
			for _, id := range names {
				ch := state.Characters[id]
				b.WriteString(fmt.Sprintf("- %s（ID: %s）：%s\n", ch.Name, id, oneLine(ch.Description)))
			}
		}
	}
	if sc.PlayerName != "" {
		b.WriteString(fmt.Sprintf("\n【玩家信息】主角名称为「%s」", sc.PlayerName))
		if sc.PlayerRole != "" {
			b.WriteString(fmt.Sprintf("，身份是「%s」", sc.PlayerRole))
		}
		b.WriteString("。\n")
	}
	// 历史剧情简要回顾（纯只读文本，非交互对话）
	if len(summaries) > 0 {
		b.WriteString("\n【故事背景与历史摘要（只读参考）】\n")
		for _, a := range summaries {
			txt := strings.TrimSpace(a.Text)
			b.WriteString("- " + oneLine(txt) + "\n")
		}
	}
	if len(ancestors) > 0 {
		b.WriteString("\n【最近故事进展简述（只读素材，绝不要模仿续写正文）】\n")
		for _, n := range ancestors {
			if tc, err := parseTurnContent(n.ContentJSON); err == nil {
				blocksText := renderBlocksPruned(tc.Blocks, true)
				b.WriteString(fmt.Sprintf("上一幕输入：%s\n上一幕剧情：%s\n", oneLine(tc.InputText), oneLine(blocksText)))
			}
		}
	}
	b.WriteString("\n【当前权威状态与资料（只读；与旧摘要冲突时以此为准）】\n")
	b.WriteString(c.dynamicContextPrompt(state, lore, memories, checks, revealedSecrets(sc.Secrets, state), nil, ledger))
	if strings.TrimSpace(inputText) == "" {
		inputText = "..."
	}
	msgs := []ports.ChatMessage{{Role: "system", Content: b.String()}}
	msgs = append(msgs, c.planningMessages...)
	msgs = append(msgs, ports.ChatMessage{Role: "user", Content: inputText})
	return msgs
}
