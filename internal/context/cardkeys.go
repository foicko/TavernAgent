package context

import (
	"encoding/json"
	"strings"

	"tavernagent/internal/cardmarkers"
)

// cardAssetJSON 是角色模板 JSON 里的资源条目（只取"可用键"所需字段）。
type cardAssetJSON struct {
	Name string `json:"name"`
	Ref  string `json:"ref"`
	Ext  string `json:"ext"`
}

// imageAssetsOf 把卡资源条目映射成 cardmarkers 的中性结构。
func imageAssetsOf(assets []cardAssetJSON) []cardmarkers.ImageAsset {
	if len(assets) == 0 {
		return nil
	}
	out := make([]cardmarkers.ImageAsset, 0, len(assets))
	for _, a := range assets {
		out = append(out, cardmarkers.ImageAsset{Name: a.Name, Ref: a.Ref, Ext: a.Ext})
	}
	return out
}

// attachCardImageKeys 从角色模板 JSON 现算可用图片键并写入 sc.CardUI。
// 无卡片档案、已有键、或没有任何图片资源时不改动——契约保持原样。
func attachCardImageKeys(sc *sessionContext, cardJSON string, assets []cardAssetJSON) {
	if sc.CardUI == nil || len(sc.CardUI.ImageKeys) > 0 || len(assets) == 0 {
		return
	}
	sc.CardUI.ImageKeys = cardmarkers.CollectImageKeys(
		imageAssetsOf(assets), cardStringValues(cardJSON), cardmarkers.ImageKeyLimit)
}

// cardStringValues 汇总卡 JSON 里所有字符串值（供"作者引用过哪些图片键"判断）。
//
// 用递归取值而不是正则扫原文：卡 JSON 里的引号是转义过的（`<img cmd=\"x\">`），
// 直接对原文做正则会把键名连同反斜杠一起取出来。这里只看解码后的字符串，
// 与渲染时模型看到的文本一致。
func cardStringValues(raw string) string {
	var v any
	if json.Unmarshal([]byte(raw), &v) != nil {
		return ""
	}
	var b strings.Builder
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case string:
			b.WriteString(t)
			b.WriteByte('\n')
		case []any:
			for _, e := range t {
				walk(e)
			}
		case map[string]any:
			for _, e := range t {
				walk(e)
			}
		}
	}
	walk(v)
	return b.String()
}
