package application

import (
	"encoding/json"
	"strings"

	"tavernagent/internal/cardmarkers"
	"tavernagent/internal/domain"
)

// 内联分段投影：把正文里的表现类标记（`[IMG:key]` / `<img cmd="key">`）
// 切成结构化段，使渲染端不再持有标记语法（此前 Go / TypeScript / C# 各一份）。
//
// 取舍：分段在**读取时**补齐而不是提交时写入。理由是已存在的历史数据
// （本特性之前提交的回合）也要能渲染图片，而迁移历史 JSON 的成本与风险
// 都高于每次投影时解析一遍；且绝大多数正文没有图片标记，快路径只是一次
// 子串检查。

// hasImageMarker 是快路径：只有真的出现表现类标记才进入解析。
func hasImageMarker(text string) bool {
	if text == "" {
		return false
	}
	return strings.Contains(text, "[IMG:") || strings.Contains(text, "[img:") ||
		strings.Contains(text, "<img") || strings.Contains(text, "<IMG")
}

// hasImageMarkerJSON 是对序列化后 JSON 的快路径：encoding/json 默认把 '<' 转义成
// Unicode 转义序列（反斜杠 + u003c），提交落库的正文里 <img 永远以转义形态出现。
func hasImageMarkerJSON(raw string) bool {
	return hasImageMarker(raw) ||
		strings.Contains(raw, jsonEscapedLT+"img") || strings.Contains(raw, jsonEscapedLT+"IMG")
}

// jsonEscapedLT 是 encoding/json 对 '<' 的转义形态（拼接书写，避免被当作字面量 '<' 读错）。
const jsonEscapedLT = `\` + `u003c`

// attachInlineSegments 为单个节点补内联分段；没有变化时原样返回。
//
// 序列化按"逐字段替换"完成：只重写 blocks，未知的顶层字段原样保留——
// 节点内容是对外契约，投影不得成为丢字段的地方。
func attachInlineSegments(node *domain.PlotNode) *domain.PlotNode {
	if node == nil || node.ContentJSON == "" || !hasImageMarkerJSON(node.ContentJSON) {
		return node
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(node.ContentJSON), &top); err != nil {
		return node
	}
	rawBlocks, ok := top["blocks"]
	if !ok {
		return node
	}
	var blocks []map[string]json.RawMessage
	if err := json.Unmarshal(rawBlocks, &blocks); err != nil || len(blocks) == 0 {
		return node
	}
	changed := false
	for _, block := range blocks {
		rawText, ok := block["text"]
		if !ok {
			continue
		}
		var text string
		if err := json.Unmarshal(rawText, &text); err != nil || !hasImageMarker(text) {
			continue
		}
		segments := cardmarkers.SplitImages(text)
		if len(segments) == 1 && segments[0].Type == "text" {
			continue // 标记不可解析：保持原样，不做无意义的改写
		}
		encoded, err := json.Marshal(segments)
		if err != nil {
			continue
		}
		block["segments"] = encoded
		changed = true
	}
	if !changed {
		return node
	}
	encodedBlocks, err := json.Marshal(blocks)
	if err != nil {
		return node
	}
	top["blocks"] = encodedBlocks
	encodedTop, err := json.Marshal(top)
	if err != nil {
		return node
	}
	clone := *node
	clone.ContentJSON = string(encodedTop)
	return &clone
}

// attachInlineSegmentsAll 批量补齐（视图窗口内的节点）。
func attachInlineSegmentsAll(nodes []*domain.PlotNode) []*domain.PlotNode {
	for i, n := range nodes {
		nodes[i] = attachInlineSegments(n)
	}
	return nodes
}
