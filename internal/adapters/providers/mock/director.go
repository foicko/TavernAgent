package mock

import (
	"context"
	"encoding/json"
	"strings"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// Demo supports the application's planning UI without changing scripted tests.
// It is explicit about its fixed story and never fabricates semantic completion.
type Demo struct{ *Provider }

func NewDemo() *Demo { return &Demo{Provider: New(DefaultStoryScript())} }

func (p *Demo) Stream(ctx context.Context, req ports.ChatRequest, sink ports.StreamSink) error {
	var report *domain.DirectorReport
	for _, m := range req.Messages {
		if m.Role != "system" {
			continue
		}
		if strings.HasPrefix(m.Content, "【导演协商】") {
			plan := domain.DirectorPlan{Title: "重逢与转折 · 演示大纲", Guidance: "保持自然节奏，保留玩家选择。", Beats: []domain.DirectorBeat{
				{BeatID: "demo_meeting", Title: "重新相识", Instruction: "让角色与玩家交换近况，留出回应空间。", CompletionCriteria: "双方已经谈及各自的近况。"},
				{BeatID: "demo_difference", Title: "出现分歧", Instruction: "围绕一件旧事出现不同理解，邀请玩家表达立场。", CompletionCriteria: "分歧已经被明确说出，玩家作出回应。"},
				{BeatID: "demo_resolution", Title: "达成共识", Instruction: "根据玩家态度找到缓和关系的契机。", CompletionCriteria: "双方明确表达了愿意继续交流。"},
			}}
			out, _ := json.Marshal(map[string]any{"reply": "当前使用离线 Mock 演示。我提供了一份可编辑的三阶段示例；请按你的故事修改后确认启用。配置真实模型后，可以根据你的具体要求协商大纲。", "plan": plan})
			if err := ctx.Err(); err != nil {
				return err
			}
			return sink.Chunk(out)
		}
		if strings.HasPrefix(m.Content, "【导演安排：未来意图】") {
			for _, line := range strings.Split(m.Content, "\n") {
				var data struct {
					RevisionID string              `json:"revisionId"`
					Beat       domain.DirectorBeat `json:"beat"`
				}
				if json.Unmarshal([]byte(line), &data) == nil && data.RevisionID != "" {
					report = &domain.DirectorReport{RevisionID: data.RevisionID, BeatID: data.Beat.BeatID, Status: "continue", Reason: "离线演示使用固定正文，请手动调整阶段；真实模型会根据剧情判断进度。"}
					break
				}
			}
		}
	}
	if report == nil {
		return p.Provider.Stream(ctx, req, sink)
	}
	items := DefaultStoryScript()
	var final map[string]any
	if err := json.Unmarshal([]byte(items[len(items)-1].Line), &final); err != nil {
		return err
	}
	final["director"] = report
	raw, _ := json.Marshal(final)
	items[len(items)-1] = Frame(string(raw))
	return New(items).Stream(ctx, req, sink)
}
