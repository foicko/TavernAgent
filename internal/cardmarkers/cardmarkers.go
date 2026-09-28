// Package cardmarkers 解析模型正文里的「卡片 UI 标记」。
//
// 设计目标：把角色卡（尤其是 charx/RisuAI 卡）要求模型输出的自定义标记，归一到
// 一组稳定的、语言中立的标记上，再由各端渲染为原生组件——而不是执行卡里的 HTML/CSS/JS。
//
// 标记分两类：
//   - 状态类（HUD/SCENE/VAR）：承载状态，需由服务端归一化进结构化字段，
//     从正文中**剥离**，避免污染记忆引文与证据；
//   - 表现类（IMG，以及 <img cmd="..."> 等 RisuAI 图像指令）：纯展示，保留在正文中由前端就地渲染。
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

// removedSentinel 标记被剥离的状态类标记所在位置，让 tidy 只收拾它两侧的空白。
const removedSentinel = "\x00RM\x00"

// Extract 从正文中剥离状态类标记，返回清理后的文本与解析出的提示。
//
// 表现类标记（[IMG:…] 与 <img …> 图像指令）不在此处理：它们保留在文本里，由渲染层就地展示。
// 因此没有任意 HTML 注入面——本函数只识别这三种固定形态。
//
// 返回文本保留 `[[` 转义：它会落库，之后还要被 SplitImages 与前端解析器再解析一遍，
// 转义只能在最终渲染时还原一次。在这里还原会让落库的 `[[IMG:x]` 重新变成真正的标记。
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
				return removedSentinel
			}
		case "SCENE":
			if body != "" {
				hints = append(hints, Hint{Kind: KindScene, Text: body})
				return removedSentinel
			}
		case "VAR":
			if v := parsePairs(body); len(v) > 0 {
				hints = append(hints, Hint{Kind: KindVar, Values: v})
				return removedSentinel
			}
		}
		// 无法解析的标记原样保留：宁可让玩家看到原文，也不静默吞掉内容。
		return m
	})
	// 只有在真的剥离了标记时才收拾残留空白：没有标记的文本（可能含普通方括号）
	// 必须原样保留，避免把模型有意写的多空格/制表符悄悄改掉。
	if len(hints) > 0 {
		clean = tidy(clean)
	}
	clean = strings.ReplaceAll(clean, escapeSentinel, "[[")
	return clean, hints
}

// Segment 是正文的一小段：纯文本或卡内图片引用。
type Segment struct {
	Type     string `json:"type"` // text | image
	Text     string `json:"text,omitempty"`
	AssetKey string `json:"assetKey,omitempty"`
}

// imgRe 匹配表现类图片标记。为兼容真实 RisuAI / charx 角色卡，接受多种写法：
//
//	<img cmd="key">           经典写法
//	<img="key">               RisuAI 旧式写法
//	<img_obj="key">           物件/道具图
//	<imgBG="key">             背景图
//	<img src="key" attr="…">  带附加属性的写法（只取第一个双引号值）
//	[IMG:key]                 方括号等价写法
//
// 属性名部分用 [-\w]* 限制，引号值有长度上限；与 Extract 一样用 [^\[\]\r\n]
// 约束方括号形式，避免跨标记匹配。
var imgRe = regexp.MustCompile(`(?i)<img\s*[-\w]*\s*=\s*"([^"]{0,` + strconv.Itoa(maxMarkerBody) + `})"[^>]*>|\[IMG:([^\[\]\r\n]{0,` + strconv.Itoa(maxMarkerBody) + `})\]`)

// SplitImages 把正文切成文本段与图片段。
//
// 这是"表现类标记"的唯一实现：渲染端不再需要持有语法的转义、容错与边界规则，
// 只按段渲染。无法解析的形态（`<img>` 无 cmd、空键、`[[IMG:x]`）原样留在
// 文本段里——宁可让玩家看到原文，也不静默吞掉内容（与 Extract 同一原则）。
//
// 输出保证：相邻文本段已合并；文本为空时不产生空段；无图片标记时返回单个文本段。
func SplitImages(text string) []Segment {
	if text == "" {
		return nil
	}
	if !strings.Contains(text, "[") && !strings.Contains(text, "<") {
		return []Segment{{Type: "text", Text: text}}
	}
	protected := strings.ReplaceAll(text, "[[", escapeSentinel)
	var out []Segment
	last := 0
	for _, loc := range imgRe.FindAllStringSubmatchIndex(protected, -1) {
		key := ""
		if loc[2] >= 0 {
			key = strings.TrimSpace(protected[loc[2]:loc[3]])
		} else if loc[4] >= 0 {
			key = strings.TrimSpace(protected[loc[4]:loc[5]])
		}
		if key == "" {
			continue // 空键不构成图片：留在文本里
		}
		if loc[0] > last {
			out = appendText(out, protected[last:loc[0]])
		}
		out = append(out, Segment{Type: "image", AssetKey: key})
		last = loc[1]
	}
	if len(out) == 0 {
		// 没有可解析的图片：还原转义后原样返回（转义还原不是改写，是分段的前提）。
		return []Segment{{Type: "text", Text: strings.ReplaceAll(protected, escapeSentinel, "[")}}
	}
	if last < len(protected) {
		out = appendText(out, protected[last:])
	}
	for i := range out {
		out[i].Text = strings.ReplaceAll(out[i].Text, escapeSentinel, "[")
	}
	return out
}

// appendText 追加文本段（保留空白：分段不应改变正文的排版）。
func appendText(out []Segment, text string) []Segment {
	if text == "" {
		return out
	}
	return append(out, Segment{Type: "text", Text: text})
}

// ParsePairs 解析 `k=v|k=v` 形式的键值体；忽略空项与无 = 的项。
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

// tidy 收拾剥离标记后残留的空白：只处理被剥离位置两侧的空白（两侧都有空白时留一个空格，
// 否则全部去掉），再去掉整段首尾空白。正文其余位置的缩进与连续空格（Markdown 嵌套列表、
// 代码缩进）必须原样保留。
func tidy(s string) string {
	if s == "" {
		return s
	}
	s = removedRe.ReplaceAllStringFunc(s, func(m string) string {
		first, last := m[0], m[len(m)-1]
		if (first == ' ' || first == '\t') && (last == ' ' || last == '\t') {
			return " "
		}
		return ""
	})
	return strings.TrimSpace(s)
}

// removedRe 匹配一处或多处相邻的剥离位置，连同两侧的空格与制表符。
var removedRe = regexp.MustCompile(`[ \t]*(?:` + regexp.QuoteMeta(removedSentinel) + `[ \t]*)+`)
