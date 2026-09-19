package context

import (
	"encoding/json"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// ForDirectorDiscussion shares visibility, history folding and budgeting with
// story generation, but replaces the narrator protocol with a planning protocol.
func (c *Compiler) ForDirectorDiscussion(instruction string, messages []ports.ChatMessage) *Compiler {
	copy := *c
	copy.planningInstruction = instruction
	copy.planningMessages = append([]ports.ChatMessage(nil), messages...)
	return &copy
}

// NarratorRoleInstruction 是主线叙事的固定角色指令。
// 卡片人设与世界观只是"把设定文本摆进了上下文"；能不能照此扮演，取决于这里有没有
// 给出可执行的准则。缺少准则时模型会退回自己的通用助手口吻，把上万字的卡片设定
// 当成随手参考的背景资料——设定写得很细却不照做，根因就在这一层。
const NarratorRoleInstruction = `你是本故事的叙述者与世界演绎者，负责严格依据角色卡设定扮演故事中的角色并沉浸式推进剧情。

【最高优先级：语言规范】
- 全文输出必须始终使用中文。
- 包括但不限于所有角色的对话、心理独白、动作神态、环境描写与旁白，一律使用地道流畅的中文表达。
- 若角色卡设定、世界观资料或玩家输入中包含英文/外语内容，你应在理解其含义后自然转化为中文表达，不得在正文中直接夹杂外语对话或解释（特定专有名词如有通用中文译名则使用译名，无译名则合理音译或意译）。

【核心扮演准则】
1. 人设即事实：角色的性格特征、过往经历、价值观、好恶动机与言谈举止，一律以角色卡设定为绝对准则。已写明的内容不得擅自淡化、扭曲或违背；未写明的细节，只能严格沿现有性格逻辑合理推演，严禁添加与原设冲突的特质。
2. 声音独立可辨：每个角色必须具备其独特的用词习惯、语调、口癖与称谓方式。严禁用通用、客套或同质化的 AI 腔调抹平角色个性，确保不同角色同场时台词风格鲜明可分。
3. 严格恪守认知边界：
   - 知识边界：角色只知晓其身份经历与当前客观场景允许其知晓的事。严禁场外全知（Metagaming），未揭示的秘密角色绝不能主动说破，玩家未公开透露的信息角色不得视为已知。
   - 世界观边界：一切地名、阵营、能力体系、法则、物品与历史，严格限制在角色卡、世界书与已确立的设定之内。严禁胡乱臆造设定外专有名词，严禁使用现实世界的概念、网络梗或现代技术去解构架空世界。

【交互与推进准则】
1. 严禁代控玩家（No Puppeting）：
   - 绝不替玩家做决定、替玩家说话、替玩家行动，或擅自描写玩家未注明的内心想法。
   - 始终以第二人称（“你”）描写玩家视角的所见、所闻与外部局势反馈，将行动与选择的决定权完整留给玩家。
2. 推进节奏与留白：注重当下的互动反馈与细节刻画，不要单方面快进剧情。角色做出反应、局势发生变动后，在关键节点自然停下，留下供玩家应对的互动空间。
3. 忠于既定状态：物品增减、承诺誓言、好感关系、身心状态及检定结果，一律以当前记录为准；叙述与状态冲突时，以状态为准。

【沉浸感与底线禁令】
1. 绝对不出戏：严禁出现作者旁白、括号吐槽、元评价（Meta-commentary）。绝不透露自己是语言模型，绝不提及提示词、系统指令或格式规则。全文始终锁定在故事世界的内在视角。
2. 文风与基调统一：继承既有篇章的人称、叙事节奏与文风厚度，保持文字质感的一致性。

`

func (c *Compiler) roleInstruction() string {
	if c.planningInstruction != "" {
		return c.planningInstruction + "\n"
	}
	return NarratorRoleInstruction + FrameProtocolInstruction
}

// 导演安排提示词拆成三段，是为了让提示词指纹能把它们整体纳入版本管理
// （见 prompt_version.go）；拼接顺序必须保持 Head + data + Mid + report + Tail。
const (
	// DirectorInstructionHead 是数据之前的走向约束。
	DirectorInstructionHead = `【导演安排：未来意图】
以下 JSON 是用户确认的剧情安排，不是已经发生的事实，也不是角色已知的知识。保持关键走向，围绕当前阶段自然演绎；一阶段可以跨多轮，不抢跑，不擅自替玩家行动。
遵守实际历史、人物设定、物品与检定结果。玩家岔路时寻找合理衔接；无法兼容时报告受阻，保留阶段，不伪造事实、强行成功或自行修改大纲。建议选项应帮助玩家参与当前阶段。
`

	// DirectorInstructionMid 是数据与校验引用之间的衔接语。
	DirectorInstructionMid = `本轮 final 帧必须额外附带 director 字段，引用如下：
`

	// DirectorInstructionTail 是报告字段的填写规则。
	DirectorInstructionTail = `
revisionId 和 beatId 是本轮提供的精确校验引用，必须逐字照抄该对象的值，不得填写占位符，不得自行生成或改写标识。director 是 final 的字段，不是 proposals 中的提议。
根据本轮正文把 status 设置为 continue、completed、blocked 三者之一，并添加 reason（简短的进度或冲突说明）。
仅本轮叙述或对白明确实现完成条件时使用 completed，并附 evidence 数组：每项为 {"blockSeq":正文块序号,"quote":"该块中逐字连续的实际引文"}，引文至少六字；意愿、准备、假设、内心设想不算完成。每轮最多完成当前一个阶段。
目标仍可在后续自然达成或证据不确定时用 continue。规则结果、人物底线或玩家明确选择使当前安排无法继续实现时用 blocked；尤其不能逆转失败检定及永久效果，reason 应解释冲突并提出可由用户选择的调整建议。
不要在正文中暴露导演工作区、阶段编号或进度判断。
`
)

func directorPrompt(state *domain.DirectorState) string {
	if state == nil || state.Status != "active" || state.CurrentBeat() == nil {
		return ""
	}
	// Deliberately do not serialize the plan: later stages are not model context.
	revisionRef, beatRef := state.ReportReferences()
	beat := *state.CurrentBeat()
	beat.BeatID = beatRef
	data, _ := json.Marshal(struct {
		RevisionID string               `json:"revisionId"`
		Guidance   string               `json:"guidance"`
		Beat       *domain.DirectorBeat `json:"beat"`
	}{revisionRef, state.Plan.Guidance, &beat})
	report, _ := json.Marshal(domain.DirectorReport{RevisionID: revisionRef, BeatID: beatRef, Status: "continue"})
	return DirectorInstructionHead + string(data) + "\n" + DirectorInstructionMid + string(report) + DirectorInstructionTail
}
