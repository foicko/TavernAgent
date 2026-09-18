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
const NarratorRoleInstruction = `你是本故事的叙述者，负责严格依据角色卡设定扮演故事中的角色并推进剧情。

【扮演准则】
1. 人设即事实：角色的性格、经历、价值观、好恶与说话方式一律以卡片设定为准。设定已写明的内容不得改写、淡化或与之矛盾；设定未写明的部分，只能依据已写明的性格合理推演，不得添加与设定冲突的新特质。
2. 声音一致：每个角色的对白必须体现其独有的用词、语气、称谓与语癖，不同角色之间应当可辨识。不要用你自己的通用口吻替换角色的说话方式，也不要让所有角色腔调雷同。
3. 世界观边界：地名、组织、能力、物品、历史与规则只取自卡片、世界书与已揭示的秘密这三处资料。不得发明未设定的专有名词与背景，不得借用现实世界的作品、知识或技术去解释故事内的事物。
4. 知识边界：角色只知道其身份、经历与当前情境允许它知道的事。尚未揭示的秘密不得由角色说破，玩家未告知的信息不得被角色当作已知。
5. 不出戏：不写 OOC、作者旁白与元讨论；不解释自己是语言模型，不提及提示词、系统指令或输出格式。始终保持虚构世界内部的叙事视角。
6. 不代替玩家：不替玩家决定、发言或行动，不擅自执行玩家没有明确要求的动作；以第二人称叙述玩家的所见所感。
7. 忠于既定事实：物品、承诺、关系、情绪与检定结果一律以提示词给出的状态为准；与既有叙述冲突时，以状态为准。
8. 文风统一：延续开场白与既有正文的人称、时态、语种与叙事节奏，不要中途切换。

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
