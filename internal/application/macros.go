package application

import (
	"regexp"
	"strings"
)

// 角色卡宏：V2/V3 生态用 {{char}} / {{user}} 指代角色与玩家。
//
// 卡片文本是「给模型看的模板」，只有在双方称呼都确定之后才谈得上展开。
// 展开点放在会话创建（Setup）：之后进入世界状态、开场正文、世界书与提示词的
// 都是可读文本，模型不会再把宏当成实体名照抄一遍。
var cardMacroRe = regexp.MustCompile(`\{\{\s*(?i:char|user)\s*\}\}`)

// expandCardMacros 把 {{char}} / {{user}} 替换为实际称呼（大小写与空格不敏感）。
func expandCardMacros(text, charName, userName string) string {
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

// ExpandMacros 展开卡内全部面向模型的文本。
// 秘密与开场同样包含宏：它们会分别进入解锁后的上下文与首条正文。
// 规则表达式的 RevealWhen 不做替换——那是规则语法，不是文案。
func (c *CharacterCard) ExpandMacros(userName string) {
	if c == nil {
		return
	}
	expand := func(s string) string { return expandCardMacros(s, c.Name, userName) }

	c.Description = expand(c.Description)
	c.Personality = expand(c.Personality)
	c.Scenario = expand(c.Scenario)
	c.FirstMes = expand(c.FirstMes)
	c.MesExample = expand(c.MesExample)
	c.SystemPrompt = expand(c.SystemPrompt)
	c.PostHistory = expand(c.PostHistory)
	c.CreatorNotes = expand(c.CreatorNotes)

	for i := range c.Characters {
		ch := &c.Characters[i]
		ch.Description = expand(ch.Description)
		ch.Personality = expand(ch.Personality)
		ch.Scenario = expand(ch.Scenario)
		ch.MesExample = expand(ch.MesExample)
		ch.SystemPrompt = expand(ch.SystemPrompt)
		ch.PostHistory = expand(ch.PostHistory)
		ch.CreatorNotes = expand(ch.CreatorNotes)
	}
	for i := range c.Secrets {
		c.Secrets[i].Content = expand(c.Secrets[i].Content)
	}
	for i := range c.OpeningVariants {
		c.OpeningVariants[i].Text = expand(c.OpeningVariants[i].Text)
	}
	for i := range c.LorebookRefs {
		entries := c.LorebookRefs[i].Entries
		for j := range entries {
			entries[j].Title = expand(entries[j].Title)
			entries[j].Content = expand(entries[j].Content)
			// 关键词也要展开：命中靠的是正文里的实际称呼。
			for k := range entries[j].Keys {
				entries[j].Keys[k] = expand(entries[j].Keys[k])
			}
			for k := range entries[j].SecondaryKeys {
				entries[j].SecondaryKeys[k] = expand(entries[j].SecondaryKeys[k])
			}
		}
	}
}
