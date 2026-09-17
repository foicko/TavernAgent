package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"tavernagent/internal/domain"
)

// OpeningVariant 是备选开场（原生开局支持各自的地点/背包覆盖）。
type OpeningVariant struct {
	VariantID    string                 `json:"variantId"`
	Title        string                 `json:"title,omitempty"`        // 展示名（地点/情境）
	Text         string                 `json:"text"`                   // 开场正文（first_mes 等效）
	InitialState *CharacterInitialState `json:"initialState,omitempty"` // 该开场的初始状态覆盖
}

// CharacterInitialState 是原生卡声明的初始世界状态。
// 传统卡缺省时使用基础初始状态，不让模型擅自生成资产（设计 §4.1）。
type CharacterInitialState struct {
	Scene         *domain.Scene                   `json:"scene,omitempty"`
	Items         []domain.ItemInstance           `json:"items,omitempty"`
	Relationships map[string]domain.RelationValue `json:"relationships,omitempty"`
	Moods         map[string]domain.CharacterMood `json:"moods,omitempty"`
}

// SecretItem 是秘密/世界观条目。与人设分字段：揭示前不进入模型上下文与玩家可见文本。
type SecretItem struct {
	SecretID   string `json:"secretId,omitempty"`
	Title      string `json:"title,omitempty"`
	Content    string `json:"content"`
	RevealWhen string `json:"revealWhen,omitempty"` // 揭示条件（规则引擎占位）
	Order      int    `json:"order,omitempty"`
}

// LorebookRef 是世界书引用 + 条目内容，与领域类型同构（类型别名，避免两份 JSON 形状漂移）。
// 条目内容自 M3 起参与上下文注入（关键词命中）。
type LorebookRef = domain.Lorebook

// CharacterCard 是首版原生角色卡 schema（schemaVersion=2）。
// 兼容导入（V2/V3 PNG）会映射到本结构，并生成兼容报告（ImportReport）。
type CharacterCard struct {
	SchemaVersion   int                    `json:"schemaVersion"`
	CardID          string                 `json:"cardId"`
	Name            string                 `json:"name"`
	Description     string                 `json:"description"`
	Avatar          string                 `json:"avatar,omitempty"`
	Personality     string                 `json:"personality,omitempty"`
	Scenario        string                 `json:"scenario,omitempty"`
	FirstMes        string                 `json:"first_mes,omitempty"`
	MesExample      string                 `json:"mes_example,omitempty"`
	SystemPrompt    string                 `json:"system_prompt,omitempty"`
	PostHistory     string                 `json:"post_history_instructions,omitempty"`
	CreatorNotes    string                 `json:"creator_notes,omitempty"`
	Tags            []string               `json:"tags,omitempty"`
	Creator         string                 `json:"creator,omitempty"`
	Version         string                 `json:"character_version,omitempty"`
	Nickname        string                 `json:"nickname,omitempty"`
	Characters      []domain.CharacterInfo `json:"characters"`
	Items           []domain.ItemInstance  `json:"items"`
	InitialState    *CharacterInitialState `json:"initialState,omitempty"`
	Secrets         []SecretItem           `json:"secrets,omitempty"`
	OpeningVariants []OpeningVariant       `json:"openingVariants,omitempty"`
	LorebookRefs    []LorebookRef          `json:"lorebookRefs,omitempty"`
	Rules           *domain.Ruleset        `json:"rules,omitempty"`
	RawOriginal     string                 `json:"-"` // 原始导入内容（资源保留，不回传）
}

// ParseCharacterCard 解析原生角色卡 JSON；未知字段不丢失（原样保留于 RawOriginal）。
func ParseCharacterCard(raw string) (*CharacterCard, error) {
	var card CharacterCard
	if err := json.Unmarshal([]byte(raw), &card); err != nil {
		return nil, err
	}
	card.RawOriginal = raw
	normalizeCard(&card)
	return &card, nil
}

// normalizeCard 补默认卡 ID、角色 ID、变体 ID；保持稳定派生（同一输入 → 同一 ID）。
func normalizeCard(card *CharacterCard) {
	if card.Name == "" && len(card.Characters) > 0 {
		card.Name = card.Characters[0].Name
	}
	if card.CardID == "" {
		card.CardID = domain.CardIdentity(card.RawOriginal)
		if card.CardID == "" {
			card.CardID = hashString(card.RawOriginal + ":" + card.Name)[:16]
		}
	}
	// 外部卡片 ID 可以很短；派生标识不应依赖它至少有六个字节。
	prefix := card.CardID
	if len(prefix) < 6 {
		prefix = hashString(prefix)
	}
	prefix = prefix[:6]
	for i := range card.Characters {
		if card.Characters[i].CharacterID == "" {
			card.Characters[i].CharacterID = "npc_" + prefix + "_" + string(rune('a'+i))
		}
	}
	// 单角色卡的描述回退：卡级 description 是公开人设，角色级缺失时补上，
	// 否则该人设不会进入提示词。secrets 不参与此回退（可见性边界不变）。
	if card.Description != "" && len(card.Characters) == 1 &&
		strings.TrimSpace(card.Characters[0].Description) == "" {
		card.Characters[0].Description = card.Description
	}
	// 头像与扩展结构化人设字段在单角色卡时的双向回退与对齐
	if len(card.Characters) == 1 {
		c0 := &card.Characters[0]
		if card.Avatar != "" && c0.Avatar == "" {
			c0.Avatar = card.Avatar
		} else if c0.Avatar != "" && card.Avatar == "" {
			card.Avatar = c0.Avatar
		}
		if card.Personality != "" && c0.Personality == "" {
			c0.Personality = card.Personality
		}
		if card.Scenario != "" && c0.Scenario == "" {
			c0.Scenario = card.Scenario
		}
		if card.MesExample != "" && c0.MesExample == "" {
			c0.MesExample = card.MesExample
		}
		if card.SystemPrompt != "" && c0.SystemPrompt == "" {
			c0.SystemPrompt = card.SystemPrompt
		}
		if card.PostHistory != "" && c0.PostHistory == "" {
			c0.PostHistory = card.PostHistory
		}
		if card.CreatorNotes != "" && c0.CreatorNotes == "" {
			c0.CreatorNotes = card.CreatorNotes
		}
		if len(card.Tags) > 0 && len(c0.Tags) == 0 {
			c0.Tags = card.Tags
		}
		if card.Creator != "" && c0.Creator == "" {
			c0.Creator = card.Creator
		}
		if card.Version != "" && c0.Version == "" {
			c0.Version = card.Version
		}
		if card.Nickname != "" && c0.Nickname == "" {
			c0.Nickname = card.Nickname
		}
	}
	for i := range card.OpeningVariants {
		v := &card.OpeningVariants[i]
		if v.VariantID == "" {
			v.VariantID = "open_" + prefix + "_" + string(rune('a'+i))
		}
	}
	for i := range card.Secrets {
		s := &card.Secrets[i]
		if s.SecretID == "" {
			s.SecretID = "secret_" + prefix + "_" + string(rune('a'+i))
		}
	}
}

// ScanSecretLeakWarnings 检查秘密条目内容是否出现在公开字段（描述/开场），提示潜在泄漏。
// 只做宽松子串检查，作为导入预览警告而非硬拦截。
func (c *CharacterCard) ScanSecretLeakWarnings() []string {
	if len(c.Secrets) == 0 {
		return nil
	}
	public := []string{c.Description}
	for _, it := range c.Items {
		public = append(public, it.Name)
	}
	for _, v := range c.OpeningVariants {
		public = append(public, v.Text)
	}
	var warns []string
	for _, s := range c.Secrets {
		key := cleanMarkup(s.Content)
		if key == "" {
			continue
		}
		for _, pub := range public {
			if strings.Contains(pub, key) || strings.Contains(key, pub) && len(pub) >= 8 {
				warns = append(warns, "秘密条目「"+s.Title+"」的内容疑似与公开字段重复，可能泄漏")
				break
			}
		}
	}
	return warns
}

// cleanMarkup 去除常见 HTML/脚本标记后返回纯文本片段（用于泄漏对比）。
func cleanMarkup(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// ResolvedOpening 是已确定的开场选择（文本 + 可选变体引用）。
type ResolvedOpening struct {
	Variant   *OpeningVariant `json:"-"`
	Text      string          `json:"text"`
	VariantID string          `json:"variantId,omitempty"`
}

// ResolveOpening 按 variantID 选择开场；缺省回退第一个变体；显式文本（兼容旧接口）优先。
// 找不到指定变体时返回错误（402 级让 HTTP 层转 CARD_VARIANT_NOT_FOUND）。
func (c *CharacterCard) ResolveOpening(variantID, explicitText string) (*ResolvedOpening, error) {
	if explicitText != "" {
		return &ResolvedOpening{Text: explicitText}, nil
	}
	if variantID != "" {
		for i := range c.OpeningVariants {
			if c.OpeningVariants[i].VariantID == variantID {
				v := c.OpeningVariants[i]
				return &ResolvedOpening{Variant: &v, Text: v.Text, VariantID: v.VariantID}, nil
			}
		}
	}
	if len(c.OpeningVariants) > 0 {
		v := c.OpeningVariants[0]
		return &ResolvedOpening{Variant: &v, Text: v.Text, VariantID: v.VariantID}, nil
	}
	return nil, errors.New("角色卡未提供任何开场变体")
}

// BuildInitialState 构造会话初始世界状态：角色 + 卡片初始状态 + 变体覆盖 + 玩家背包。
// 数据链路约定：initialState 存在时作为权威初始状态（顶替顶级 items）；
// 变体 initialState 在其上增量覆盖（物品只能新增，关系/情绪/场景整体替换）。
func (c *CharacterCard) BuildInitialState(player domain.CharacterInfo, backpack []domain.ItemInstance, resolved *ResolvedOpening) (*domain.WorldState, error) {
	state := domain.NewWorldState()
	if player.CharacterID != "" {
		state.Characters[player.CharacterID] = player
	}
	for _, ch := range c.Characters {
		if strings.TrimSpace(ch.CharacterID) == "" || ch.CharacterID == "player" || ch.CharacterID == "consumed" {
			return nil, fmt.Errorf("角色 ID %q 为空或使用了保留名称", ch.CharacterID)
		}
		if _, exists := state.Characters[ch.CharacterID]; exists {
			return nil, fmt.Errorf("角色 ID %q 重复", ch.CharacterID)
		}
		state.Characters[ch.CharacterID] = ch
	}
	if c.InitialState != nil {
		if err := applyInitialState(state, c.InitialState); err != nil {
			return nil, err
		}
	} else {
		// 无结构化初始状态时，使用卡顶级 items（传统最小集）。
		for _, it := range c.Items {
			if err := state.ApplyItemGrant(it); err != nil {
				return nil, fmt.Errorf("初始物品非法: %w", err)
			}
		}
	}
	if resolved != nil && resolved.Variant != nil && resolved.Variant.InitialState != nil {
		if err := applyInitialState(state, resolved.Variant.InitialState); err != nil {
			return nil, err
		}
	}
	for _, it := range backpack {
		it.OwnerID = "player"
		if err := state.ApplyItemGrant(it); err != nil {
			return nil, fmt.Errorf("玩家初始背包非法: %w", err)
		}
	}
	return state, nil
}

// applyInitialState 在副本上应用结构化初始状态（关系/情绪整体替换，物品仅新增）。
func applyInitialState(state *domain.WorldState, is *CharacterInitialState) error {
	if is.Scene != nil {
		sc := *is.Scene
		state.Scene = &sc
	}
	for cid, rv := range is.Relationships {
		if _, ok := state.Characters[cid]; !ok || cid == "player" {
			return fmt.Errorf("初始关系引用了未知或非法角色 %q", cid)
		}
		if rv.Affection < domain.AffectionMin || rv.Affection > domain.AffectionMax ||
			rv.Trust < domain.ScaleMin || rv.Trust > domain.ScaleMax ||
			rv.Alertness < domain.ScaleMin || rv.Alertness > domain.ScaleMax {
			return fmt.Errorf("角色 %q 的初始关系数值超出合法范围", cid)
		}
		state.Relationships[cid] = rv
	}
	for cid, m := range is.Moods {
		if _, ok := state.Characters[cid]; !ok || cid == "player" {
			return fmt.Errorf("初始情绪引用了未知或非法角色 %q", cid)
		}
		state.Moods[cid] = m
	}
	for _, it := range is.Items {
		if _, exists := state.Items[it.InstanceID]; exists {
			return fmt.Errorf("初始物品 %q 重复声明", it.InstanceID)
		}
		if err := state.ApplyItemGrant(it); err != nil {
			return err
		}
	}
	return nil
}
