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

func (c *Compiler) roleInstruction() string {
	if c.planningInstruction != "" {
		return c.planningInstruction + "\n"
	}
	return "你是本故事的叙述者。严格遵守既定角色设定与人设底线，用回合制的文学叙述推进剧情，不要代替玩家做决定，不要擅自执行玩家没有明确要求的动作。\n" + FrameProtocolInstruction
}

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
	return `【导演安排：未来意图】
以下 JSON 是用户确认的剧情安排，不是已经发生的事实，也不是角色已知的知识。保持关键走向，围绕当前阶段自然演绎；一阶段可以跨多轮，不抢跑，不擅自替玩家行动。
遵守实际历史、人物设定、物品与检定结果。玩家岔路时寻找合理衔接；无法兼容时报告受阻，保留阶段，不伪造事实、强行成功或自行修改大纲。建议选项应帮助玩家参与当前阶段。
` + string(data) + `
本轮 final 帧必须额外附带 director 字段，引用如下：
` + string(report) + `
revisionId 和 beatId 是本轮提供的精确校验引用，必须逐字照抄该对象的值，不得填写占位符，不得自行生成或改写标识。director 是 final 的字段，不是 proposals 中的提议。
根据本轮正文把 status 设置为 continue、completed、blocked 三者之一，并添加 reason（简短的进度或冲突说明）。
仅本轮叙述或对白明确实现完成条件时使用 completed，并附 evidence 数组：每项为 {"blockSeq":正文块序号,"quote":"该块中逐字连续的实际引文"}，引文至少六字；意愿、准备、假设、内心设想不算完成。每轮最多完成当前一个阶段。
目标仍可在后续自然达成或证据不确定时用 continue。规则结果、人物底线或玩家明确选择使当前安排无法继续实现时用 blocked；尤其不能逆转失败检定及永久效果，reason 应解释冲突并提出可由用户选择的调整建议。
不要在正文中暴露导演工作区、阶段编号或进度判断。
`
}
