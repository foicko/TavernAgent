// Package cardmarkers 解析模型正文里的「卡片 UI 标记」。
//
// 设计目标：把角色卡（尤其是 charx/RisuAI 卡）要求模型输出的自定义标记，归一到
// 一组稳定的、语言中立的标记上，再由各端渲染为原生组件——而不是执行卡里的 HTML/CSS/JS。
//
// 标记分两类：
//   - 状态类（HUD/SCENE/VAR）：承载状态，需由服务端归一化进结构化字段，
//     从正文中**剥离**，避免污染记忆引文与证据；
//   - 表现类（IMG，以及既有 <img cmd="...">）：纯展示，保留在正文中由前端就地渲染。
//
// 语法（v1，方括号，键值用 | 分隔、= 连接）：
//
//	[HUD:label=value|label=value]
//	[SCENE:地点或场景标题]
//	[VAR:key=value|key=value]
//	[IMG:asset_key]            （<img cmd="asset_key"> 的等价写法）
//
// 转义：`[[` 表示一个字面量 `[`（不会被当作标记）。
package cardmarkers

import (
	"regexp"
	"strconv"
	"strings"
)

// Kind 是状态类标记的种类。
type Kind string

const (
	KindHUD   Kind = "hud"
	KindScene Kind = "scene"
	KindVar   Kind = "var"
)

// Hint 是一条已解析的状态类标记。
type Hint struct {
	Kind   Kind              `json:"kind"`
	Values map[string]string `json:"values,omitempty"` // HUD/VAR 的键值
	Text   string            `json:"text,omitempty"`   // SCENE 的标题
}

// maxMarkerBody 限制单条标记的体积，防止异常输入造成大分配。
const maxMarkerBody = 400

// markerRe 匹配状态类标记。用 [^\[\]\r\n] 排除方括号与换行，避免跨标记匹配。
var markerRe = regexp.MustCompile(`(?i)\[(HUD|SCENE|VAR):([^\[\]\r\n]{0,` + strconv.Itoa(maxMarkerBody) + `})\]`)

// escapeSentinel 是 `[[` 的临时占位，提取完成后还原为 `[`。不含 `[`，否则会被 markerRe 误匹配。
const escapeSentinel = "\x00LBRACK\x00"

// Extract 从正文中剥离状态类标记，返回清理后的文本与解析出的提示。
//
// 表现类标记（[IMG:…] 与 <img cmd="…">）不在此处理：它们保留在文本里，由渲染层就地展示。
// 因此没有任意 HTML 注入面——本函数只识别这三种固定形态。
func Extract(text string) (string, []Hint) {
	if text == "" || !strings.Contains(text, "[") {
		return text, nil
	}
	protected := strings.ReplaceAll(text, "[[", escapeSentinel)
	var hints []Hint
	clean := markerRe.ReplaceAllStringFunc(protected, func(m string) string {
		sub := markerRe.FindStringSubmatch(m)
		tag := strings.ToUpper(sub[1])
		body := strings.TrimSpace(sub[2])
		switch tag {
		case "HUD":
			if v := parsePairs(body); len(v) > 0 {
				hints = append(hints, Hint{Kind: KindHUD, Values: v})
				return ""
			}
		case "SCENE":
			if body != "" {
				hints = append(hints, Hint{Kind: KindScene, Text: body})
				return ""
			}
		case "VAR":
			if v := parsePairs(body); len(v) > 0 {
				hints = append(hints, Hint{Kind: KindVar, Values: v})
				return ""
			}
		}
		// 无法解析的标记原样保留：宁可让玩家看到原文，也不静默吞掉内容。
		return m
	})
	clean = strings.ReplaceAll(clean, escapeSentinel, "[")
	return tidy(clean), hints
}

// ExtractBlocks 对多个文本块逐一提取，返回清理后的文本与合并后的提示。
func ExtractBlocks(texts []string) ([]string, []Hint) {
	out := make([]string, len(texts))
	var hints []Hint
	for i, t := range texts {
		clean, h := Extract(t)
		out[i] = clean
		hints = append(hints, h...)
	}
	return out, hints
}

// parsePairs 解析 `k=v|k=v` 形式的键值体；忽略空项与无 = 的项。
func parsePairs(body string) map[string]string {
	if body == "" {
		return nil
	}
	out := map[string]string{}
	for _, part := range strings.Split(body, "|") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// tidy 收拾剥离标记后残留的空白：折叠连续空格、去掉行首尾空白。
func tidy(s string) string {
	if s == "" {
		return s
	}
	s = spacesRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

var spacesRe = regexp.MustCompile(`[ \t]{2,}`)
