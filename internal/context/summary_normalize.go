package context

import (
	"regexp"
	"strings"
)

// 摘要结构归一化：把模型输出里"已知的层级漂移"机械修回契约结构。
//
// 为什么需要它：实测摘要模型有约三分之一的概率写错层级，且错法高度集中——
// 章节套自身（`<character_dynamics>` 套 `<character_dynamics>`）、把顶层章节
// 塞进别的章节里（`<character_dynamics>` 里出现 `<open_loops>`）。这些是排版
// 结构错误而不是内容错误，整条丢弃等于把一次完整生成白扔。
//
// 做法是**按章节重组**而不是逐标签修补：把每个已知章节的内容片段收集起来、
// 同名章节合并，再按契约顺序重新输出。逐标签搬移做不到这件事——把章节从
// character_dynamics 里提出来会拆出两个同名章节，反而触发"章节不能重复"。
//
// 归一之后仍然走 validateSummaryXML 严格校验：归一不负责放行，只负责把可
// 机械修复的排版问题修好；修不好就继续失败，由调用方重试或退避。"宽容"不会
// 变成"不校验"。

var summaryTagPattern = regexp.MustCompile(`<[^<>]*>`)

// summaryCanonicalOrder 是契约规定的章节顺序。
var summaryCanonicalOrder = []string{"narrative_arc", "character_dynamics", "open_loops", "milestones"}

var summarySectionNames = map[string]bool{
	"narrative_arc": true, "character_dynamics": true, "open_loops": true, "milestones": true,
}

var summarySubsectionNames = map[string]bool{"mindset": true, "hidden_tension": true}

// NormalizeSummaryXML 返回归一后的摘要 XML 与"是否发生过修复"。
func NormalizeSummaryXML(raw string) (string, bool) {
	text := stripSummaryFences(strings.TrimSpace(raw))
	start := strings.Index(text, "<story_checkpoint>")
	if start < 0 {
		return text, false
	}
	text = text[start:]
	if end := strings.LastIndex(text, "</story_checkpoint>"); end >= 0 {
		text = text[:end]
	}
	inner := strings.TrimPrefix(text, "<story_checkpoint>")

	sections := map[string]*strings.Builder{}
	order := []string{}
	ensure := func(name string) *strings.Builder {
		if b, ok := sections[name]; ok {
			return b
		}
		b := &strings.Builder{}
		sections[name] = b
		order = append(order, name)
		return b
	}
	cur := ""              // 当前所在的已知章节
	openSubs := []string{} // 已写出、等待闭合的子章节标签
	changed := false

	put := func(run string) {
		if cur == "" || strings.TrimSpace(run) == "" {
			return // 根节点下的游离文本丢弃（校验器同样禁止）
		}
		ensure(cur).WriteString(run)
	}
	cursor := 0
	for _, loc := range summaryTagPattern.FindAllStringIndex(inner, -1) {
		run := inner[cursor:loc[0]]
		tag := inner[loc[0]:loc[1]]
		cursor = loc[1]
		if strings.HasPrefix(tag, "<!--") || strings.HasPrefix(tag, "<?") || strings.HasPrefix(tag, "<!") {
			changed = true
			continue // 注释/指令：校验器禁止，丢弃
		}
		closing := strings.HasPrefix(tag, "</")
		name := summaryTagName(tag)
		switch {
		case closing:
			if len(openSubs) > 0 && openSubs[len(openSubs)-1] == name {
				ensure("character_dynamics").WriteString("</" + name + ">")
				openSubs = openSubs[:len(openSubs)-1]
				put(run)
				continue
			}
			if name == cur {
				put(run)
				cur = ""
				continue
			}
			changed = true // 多余或错位的闭合标签：丢弃
		case summarySectionNames[name]:
			if cur == name {
				changed = true // 章节套自身：内容合并进同一章节
				put(run)
				continue
			}
			put(run)
			cur = name
			ensure(name)
		case summarySubsectionNames[name]:
			// 子章节只允许出现在 character_dynamics 下；出现在别处时一并归位。
			if cur != "character_dynamics" {
				changed = true
				cur = "character_dynamics"
				ensure("character_dynamics")
			}
			put(run)
			ensure("character_dynamics").WriteString(tag)
			openSubs = append(openSubs, name)
		default:
			changed = true // 未知标签：去掉标签，保留其中的文本
			put(run)
		}
	}
	put(inner[cursor:])
	for i := len(openSubs) - 1; i >= 0; i-- {
		ensure("character_dynamics").WriteString("</" + openSubs[i] + ">")
		changed = true
	}

	var out strings.Builder
	out.WriteString("<story_checkpoint>")
	emitted := false
	writeSection := func(name string) {
		b := sections[name]
		if b == nil || b.Len() == 0 {
			return
		}
		emitted = true
		out.WriteString("<" + name + ">")
		out.WriteString(b.String())
		out.WriteString("</" + name + ">")
	}
	for _, name := range summaryCanonicalOrder {
		writeSection(name)
	}
	for _, name := range order { // 契约外的未知章节名不会出现在这里（未知标签已丢弃）
		found := false
		for _, canonical := range summaryCanonicalOrder {
			if canonical == name {
				found = true
			}
		}
		if !found {
			writeSection(name)
		}
	}
	out.WriteString("</story_checkpoint>")
	return out.String(), changed || !emitted
}

func summaryTagName(tag string) string {
	name := strings.TrimSpace(strings.TrimPrefix(tag, "</"))
	name = strings.TrimSpace(strings.TrimPrefix(name, "<"))
	name = strings.TrimSuffix(name, ">")
	if idx := strings.IndexAny(name, " \t\r\n/"); idx >= 0 {
		name = name[:idx]
	}
	return name
}

// ParseAndNormalizeSummary 先机械归一，再严格校验。
// normalized 表示是否发生过修复（用于计数与日志：宽容不能静默）。
func ParseAndNormalizeSummary(rawText string) (text string, normalized bool, err error) {
	text, err = ParseCompactionSummary(rawText)
	if err == nil {
		return text, false, nil
	}
	fixed, changed := NormalizeSummaryXML(rawText)
	if !changed {
		return "", false, err
	}
	fixedText, fixedErr := ParseCompactionSummary(fixed)
	if fixedErr != nil {
		// 归一没能救回来（例如真的缺章节）：返回原始错误，方便修复重试时复述。
		return "", false, err
	}
	return fixedText, true, nil
}

func stripSummaryFences(text string) string {
	if !strings.HasPrefix(text, "```") {
		return text
	}
	if idx := strings.Index(text, "\n"); idx != -1 {
		text = strings.TrimSpace(text[idx+1:])
	}
	if strings.HasSuffix(text, "```") {
		text = strings.TrimSpace(text[:len(text)-3])
	}
	return text
}
