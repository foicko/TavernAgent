// Package macros 处理角色卡生态的 {{char}} / {{user}} 占位符。
//
// 单独成叶子包，是因为两个方向都要用它，而它们互不依赖：
//   - 会话创建（application）：卡文本在进入世界状态、开场正文、世界书与提示词前展开；
//   - 提示词编译（context）：持久化的卡 JSON 保留原始占位符（卡身份 CardIdentity
//     按原始字节计算，改写卡 JSON 会改变会话身份），因此秘密内容必须在**读取侧**
//     展开，否则模型会看到字面的 {{char}}/{{user}}。
package macros

import (
	"regexp"
	"strings"
)

// cardMacroRe 匹配 {{char}} / {{user}}（大小写与内部空格不敏感）。
var cardMacroRe = regexp.MustCompile(`\{\{\s*(?i:char|user)\s*\}\}`)

// Expand 把 {{char}} / {{user}} 替换为实际称呼（大小写与空格不敏感）。
func Expand(text, charName, userName string) string {
	if text == "" || !strings.Contains(text, "{{") {
		return text
	}
	return cardMacroRe.ReplaceAllStringFunc(text, func(match string) string {
		body := strings.ToLower(strings.TrimSpace(match[2 : len(match)-2]))
		if body == "char" {
			return charName
		}
		return userName
	})
}
