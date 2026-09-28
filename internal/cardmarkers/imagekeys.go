package cardmarkers

import (
	"regexp"
	"sort"
	"strings"
)

// ImageAsset 是卡内图片资源的最小信息（供 CollectImageKeys 使用）。
//
// 放在这里而不是 application：会话的角色模板 JSON 与卡库条目都能映射成它，
// 而两处都需要"这张卡有哪些图片键"这一份规则——重复实现必然漂移。
type ImageAsset struct {
	Name string
	Ref  string
	Ext  string
}

// ImageKeyLimit 是写进提示词契约的图片资源键上限。真实卡可能有上千张立绘
// （同一角色 × 表情 × 变体），全列会挤占上下文；优先列卡正文实际引用过的键。
const ImageKeyLimit = 80

// imgTagKeyRe / imgBracketKeyRe 与 SplitImages 的语法一致，仅用于从卡正文里
// 挑出"作者自己用过的键"作为优先项。
var (
	imgTagKeyRe     = regexp.MustCompile(`(?i)<img\s*[-\w]*\s*=\s*"([^"]{1,200})"[^>]*>`)
	imgBracketKeyRe = regexp.MustCompile(`\[IMG:([^\[\]\r\n]{1,200})\]`)
)

// CollectImageKeys 收集本卡可用的图片资源键：
//  1. 先列 cardText 里实际引用过的键（模型最可能需要的那些），按出现顺序；
//  2. 再按资源名排序补齐；
//  3. 去重并截断到 limit（limit<=0 时用 ImageKeyLimit）。
//
// 只收图片类资源（按扩展名判定）；名称缺失时回退落地 ref。返回值可为 nil（无图片资源）。
func CollectImageKeys(assets []ImageAsset, cardText string, limit int) []string {
	if limit <= 0 {
		limit = ImageKeyLimit
	}
	usable := make(map[string]bool, len(assets))
	for _, a := range assets {
		switch strings.ToLower(strings.TrimPrefix(a.Ext, ".")) {
		case "png", "webp", "jpg", "jpeg", "gif", "avif":
		default:
			continue
		}
		key := strings.TrimSpace(a.Name)
		if key == "" {
			key = strings.TrimSpace(a.Ref)
		}
		if key != "" {
			usable[key] = true
		}
	}
	if len(usable) == 0 {
		return nil
	}

	out := make([]string, 0, min(len(usable), limit))
	seen := make(map[string]bool, len(usable))
	add := func(key string) {
		if len(out) >= limit || seen[key] || !usable[key] {
			return
		}
		seen[key] = true
		out = append(out, key)
	}
	for _, m := range imgTagKeyRe.FindAllStringSubmatch(cardText, -1) {
		add(strings.TrimSpace(m[1]))
	}
	for _, m := range imgBracketKeyRe.FindAllStringSubmatch(cardText, -1) {
		add(strings.TrimSpace(m[1]))
	}
	rest := make([]string, 0, len(usable))
	for key := range usable {
		if !seen[key] {
			rest = append(rest, key)
		}
	}
	sort.Strings(rest)
	for _, key := range rest {
		add(key)
	}
	return out
}
