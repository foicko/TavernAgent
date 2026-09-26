package application

import (
	"encoding/json"
	"testing"

	"tavernagent/internal/domain"
)

// 内联分段在投影时补齐：历史数据（本特性之前提交的回合）也要能渲染图片，
// 且投影不得成为丢字段的地方。
func TestAttachInlineSegments(t *testing.T) {
	t.Run("补齐分段并保留未知顶层字段", func(t *testing.T) {
		content := `{"inputKind":"text","inputText":"你好","blocks":[
			{"kind":"narration","text":"前[IMG:emo_a]后"},
			{"kind":"dialogue","speakerId":"c1","text":"没有标记"}
		],"futureField":{"x":1}}`
		node := &domain.PlotNode{NodeID: "n1", Kind: domain.NodeKindTurn, ContentJSON: content}
		got := attachInlineSegments(node)
		if got == node {
			t.Fatal("含图片标记的节点应产出新节点")
		}
		var top map[string]json.RawMessage
		if err := json.Unmarshal([]byte(got.ContentJSON), &top); err != nil {
			t.Fatalf("结果不是合法 JSON: %v", err)
		}
		if _, ok := top["futureField"]; !ok {
			t.Error("未知顶层字段被投影丢弃")
		}
		var blocks []struct {
			Kind     string                 `json:"kind"`
			Text     string                 `json:"text"`
			Segments []domain.InlineSegment `json:"segments"`
		}
		if err := json.Unmarshal(top["blocks"], &blocks); err != nil {
			t.Fatalf("解析 blocks: %v", err)
		}
		if len(blocks) != 2 {
			t.Fatalf("块数 = %d, 期望 2", len(blocks))
		}
		if len(blocks[0].Segments) != 3 {
			t.Fatalf("首块分段 = %+v", blocks[0].Segments)
		}
		if blocks[0].Segments[1].Type != "image" || blocks[0].Segments[1].AssetKey != "emo_a" {
			t.Errorf("图片段异常: %+v", blocks[0].Segments[1])
		}
		if len(blocks[1].Segments) != 0 {
			t.Errorf("无标记的块不该产生分段: %+v", blocks[1].Segments)
		}
	})

	t.Run("json.Marshal 转义后的 <img cmd> 也能识别", func(t *testing.T) {
		// 真实提交路径用 json.Marshal 落库，'<' 被转义为 <；手写 JSON 的用例测不出这一点。
		cb, err := json.Marshal(map[string]any{"blocks": []map[string]any{
			{"kind": "narration", "text": `她笑了。<img cmd="smile">`},
		}})
		if err != nil {
			t.Fatal(err)
		}
		node := &domain.PlotNode{NodeID: "n3", Kind: domain.NodeKindTurn, ContentJSON: string(cb)}
		if got := attachInlineSegments(node); got == node {
			t.Fatalf("转义后的 <img cmd> 未被识别: %s", cb)
		}
	})

	t.Run("无标记时原样返回同一指针", func(t *testing.T) {
		node := &domain.PlotNode{NodeID: "n2", Kind: domain.NodeKindTurn,
			ContentJSON: `{"blocks":[{"kind":"narration","text":"纯文本"}]}`}
		if got := attachInlineSegments(node); got != node {
			t.Error("无标记的节点应原样返回（避免无意义的序列化）")
		}
	})

	t.Run("内容非法时原样返回", func(t *testing.T) {
		node := &domain.PlotNode{NodeID: "n3", Kind: domain.NodeKindTurn, ContentJSON: "{不是 JSON"}
		if got := attachInlineSegments(node); got != node {
			t.Error("非法内容不应被改写")
		}
	})
}
