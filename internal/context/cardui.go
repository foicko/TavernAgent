package context

import "strings"

// cardUIInfo 是编译层需要的卡片 UI 档案子集（根节点 cardUI 字段）。
// 完整档案在 application.CardUIProfile；这里只取生成契约所需的部分，避免跨层依赖。
type cardUIInfo struct {
	Capabilities struct {
		MarkerFamilies []string `json:"markerFamilies"`
	} `json:"capabilities"`
	VarDefaults map[string]string `json:"varDefaults"`
	// ImageKeys 是本卡可用的图片资源键（由 application 侧收集并随根节点冻结）。
	ImageKeys []string `json:"imageKeys"`
}

// CardUIContract 是「卡片界面标记」的静态契约文本。
//
// 只在会话冻结的卡片带 UI 档案时注入，位置在静态前缀（KV Cache 稳定）：
// 它把卡作者自定义的标记语法（如 [CeresStatus:…]）归一到本应用的标准标记，
// 使各端能用原生组件渲染状态/场景，而不必执行卡里的 HTML/CSS/JS。
//
// 必须与 internal/cardmarkers 的语法、web/src/lib/cardMarkers.ts 的解析保持一致。
const CardUIContract = `【卡片界面标记】
本角色卡可能带有自定义的状态/场景标记。请改用下列**标准标记**表达同等信息，写在 block 帧的 text 字段里（不要写进 final 帧）：
- 场景：[SCENE:地点或场景标题]
- 状态面板：[HUD:名称=值|名称=值]
- 会话变量：[VAR:键=值|键=值]
- 图片：只能用系统列出的资源键，写成 <img cmd="资源键">（等价写法 [IMG:资源键]）；不得自造或改写键名。

规则：
1. 每条标记单独完整、用方括号包裹；键值用 | 分隔、= 连接；值内不要出现 [ ] | 换行。
2. 状态确实变化时才输出 [HUD]；场景切换时才输出 [SCENE]；每轮各自最多一次。
3. 不要输出任何 HTML/CSS/JS，也不要输出卡片里的其它标记语法。
4. 标记不是台词：不要把它们读出来，也不要让角色知道它们的存在。
`

// cardUIContractBlock 渲染本会话的卡片界面标记契约；无档案时返回空串。
// 附带的原始标记清单是别名提示：告诉模型卡作者原本要它输出哪些标记，请改用标准标记。
func cardUIContractBlock(sc sessionContext) string {
	if sc.CardUI == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(CardUIContract)
	if fams := sc.CardUI.Capabilities.MarkerFamilies; len(fams) > 0 {
		b.WriteString("本卡作者原本要求输出这些自定义标记：")
		b.WriteString(strings.Join(fams, "、"))
		b.WriteString("。请把它们表达的状态改用上面的标准标记输出，不要再输出这些原始标记。\n")
	}
	// 图片资源键必须逐字复用：客户端按资源名匹配，写错或自造键名会让该处图片渲染不出来。
	if keys := sc.CardUI.ImageKeys; len(keys) > 0 {
		b.WriteString("本卡可用的图片资源键（只能使用下列键，逐字复制，不要自造、不要改写、不要翻译）：\n")
		b.WriteString(strings.Join(keys, "、"))
		b.WriteString("\n")
	}
	return b.String()
}
