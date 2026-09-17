package application

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	"tavernagent/internal/domain"
)

// ImportReport 是角色卡导入的兼容报告（字段支持表 + 兼容提示 + 潜在秘密泄漏警告）。
type ImportReport struct {
	Format      string         `json:"format"`      // native | chara_card_v2 | chara_card_v3 | unknown
	SpecVersion string         `json:"specVersion"` // 卡片 spec_version（未知时为空）
	Source      string         `json:"source"`      // png | json
	Supported   []string       `json:"supported"`   // 映射到原生 schema 的字段
	Ignored     []string       `json:"ignored"`     // 识别但未启用的字段（含说明）
	Warnings    []string       `json:"warnings"`    // 兼容提示 / 秘密泄漏 / 危险标记
	RawJSON     string         `json:"rawJson"`     // 卡内嵌原始 JSON（诊断用，不回传敏感值）
	Card        *CharacterCard `json:"-"`
}

// 卡片元数据关键字（SillyTavern 兼容；tEXt/iTXt/zTXt 统一提取）。
const (
	metaKeyChara   = "chara" // V2：base64 的 chara_card_v2 JSON
	metaKeyCCV3    = "ccv3"  // V3：base64 的 chara_card_v3 JSON
	metaKeyCreator = "Creator"
	metaKeyNotes   = "Notes"
	metaKeyComment = "Comment"
	maxMetaChunk   = 64 << 20 // 单个元数据块上限，防膨胀攻击
)

// ImportCardPNG 从 PNG 字节流提取卡片元数据并解码为原生角色卡（纯 Go，无外部依赖）。
// 只读取 tEXt/iTXt/zTXt chunk；像素仅用于生成有界头像缩略图，不随卡片保存。
// 外部脚本/HTML 一律按纯文本保留，不执行。
func ImportCardPNG(pngData []byte) (*ImportReport, error) {
	meta, chunkWarns, err := readPNGMeta(pngData)
	if err != nil {
		return nil, err
	}
	payload, key := cardPayloadFromMeta(meta)
	if key == "" {
		if len(chunkWarns) > 0 {
			return nil, fmt.Errorf("PNG 中未找到 chara/ccv3 角色卡元数据（%s）", chunkWarns[0])
		}
		return nil, errors.New("PNG 中未找到 chara/ccv3 角色卡元数据")
	}
	rep, err := decodeCardJSON(payload, "png")
	if err != nil {
		return nil, err
	}
	rep.Warnings = append(rep.Warnings, chunkWarns...)
	backfillChunkMetadata(rep, meta)
	if key == metaKeyCCV3 {
		if _, alsoV2 := meta[metaKeyChara]; alsoV2 {
			rep.Warnings = append(rep.Warnings, "该 PNG 同时内嵌 chara（V2）与 ccv3（V3）两份载荷，已按 V3 导入")
		}
	}
	attachPNGAvatar(rep, pngData)
	return rep, nil
}

// backfillChunkMetadata 用独立的 Creator / Notes / Comment 文本块补齐卡片元数据。
// 这些关键字是早期角色卡工具的写法：卡片载荷里没有创作者信息时，它们是唯一来源。
// 卡片自身写了值时以卡片为准，不覆盖。
func backfillChunkMetadata(rep *ImportReport, meta map[string]string) {
	if rep == nil || rep.Card == nil {
		return
	}
	notes := strings.TrimSpace(meta[metaKeyNotes])
	if notes == "" {
		notes = strings.TrimSpace(meta[metaKeyComment])
	}
	if notes != "" && strings.TrimSpace(rep.Card.CreatorNotes) == "" {
		rep.Card.CreatorNotes = notes
		if len(rep.Card.Characters) == 1 {
			rep.Card.Characters[0].CreatorNotes = notes
		}
		rep.Supported = append(rep.Supported, "Notes/Comment 文本块（创作者留言）")
	}
	creator := strings.TrimSpace(meta[metaKeyCreator])
	if creator != "" && strings.TrimSpace(rep.Card.Creator) == "" {
		rep.Card.Creator = creator
		if len(rep.Card.Characters) == 1 {
			rep.Card.Characters[0].Creator = creator
		}
		rep.Supported = append(rep.Supported, "Creator 文本块")
	}
}

// cardPayloadFromMeta 选取卡片载荷。V3 卡（SillyTavern 1.12+）会同时写 chara 与 ccv3：
// chara 是降级后的 V2 副本，ccv3 才是完整 V3（含 assets、群聊开场等）。
// 因此优先取 ccv3，缺失时再退回 chara。
func cardPayloadFromMeta(meta map[string]string) (string, string) {
	if v, ok := meta[metaKeyCCV3]; ok && strings.TrimSpace(v) != "" {
		return v, metaKeyCCV3
	}
	if v, ok := meta[metaKeyChara]; ok && strings.TrimSpace(v) != "" {
		return v, metaKeyChara
	}
	return "", ""
}

// CardJSON 返回交给建会话接口的卡片 JSON。
//
// 非原生卡直接序列化标准化结果；原生卡保留原始串以携带未知扩展，
// 但 PNG 卡的头像来自像素、不在原始串里，需要补写回顶层 avatar。
// 单角色卡的角色级头像由 ParseCharacterCard 的归一化回填，不必重复写入。
func (r *ImportReport) CardJSON() string {
	if r == nil || r.Card == nil {
		return ""
	}
	if r.Format != "native" {
		b, err := json.Marshal(r.Card)
		if err != nil {
			return ""
		}
		return string(b)
	}
	if r.Card.Avatar == "" {
		return r.RawJSON
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(r.RawJSON), &obj); err != nil {
		return r.RawJSON
	}
	avatar, err := json.Marshal(r.Card.Avatar)
	if err != nil {
		return r.RawJSON
	}
	obj["avatar"] = avatar
	out, err := json.Marshal(obj)
	if err != nil {
		return r.RawJSON
	}
	return string(out)
}

// attachPNGAvatar 生成有界头像缩略图并挂到卡片上。
// 整幅 PNG（可达数 MB）只作为缩略图输入，不进入卡片 JSON：否则建会话请求体、
// 世界快照与浏览器本地存储都会被同一张图撑爆。像素无法解码时只告警不中断导入。
func attachPNGAvatar(rep *ImportReport, pngData []byte) {
	if rep == nil || rep.Card == nil || len(pngData) == 0 {
		return
	}
	avatar := buildAvatarDataURL(pngData)
	if avatar == "" {
		rep.Warnings = append(rep.Warnings, "角色卡 PNG 像素无法解码，未生成头像缩略图（其余字段已正常导入）")
		return
	}
	rep.Card.Avatar = avatar
	if len(rep.Card.Characters) == 1 {
		rep.Card.Characters[0].Avatar = avatar
	}
}

// ImportCardJSON 解析原生角色卡 JSON（也兼容直接粘贴 V1/V2/V3 JSON）。
func ImportCardJSON(raw string) (*ImportReport, error) {
	return decodeCardJSON(raw, "json")
}

// decodeCardJSON 处理卡内嵌载荷：base64（chunk 形式）或原始 JSON（chara_card_v2/v3）。
func decodeCardJSON(payload string, source string) (*ImportReport, error) {
	raw := stripBOM([]byte(payload))
	if !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
		decoded, err := decodeCardBase64(payload)
		if err != nil {
			return nil, fmt.Errorf("卡片负载解码失败（非 JSON 且非 base64）: %w", err)
		}
		raw = stripBOM(decoded)
	}
	rep := &ImportReport{Source: source, RawJSON: string(raw)}
	if err := fillCardFromSpec(raw, rep); err != nil {
		return nil, err
	}
	normalizeCard(rep.Card)
	rep.warn()
	return rep, nil
}

// stripBOM 去掉 UTF-8 BOM：带 BOM 的 JSON 会被标准库直接判为语法错误。
func stripBOM(b []byte) []byte {
	return bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
}

// decodeCardBase64 宽容解码卡内嵌 base64。
// 不同导出工具会写出标准表 / URL 安全表、带或不带填充、含换行空白等变体，
// 这些都应被接受——否则「卡就在手里但导入失败」。
func decodeCardBase64(payload string) ([]byte, error) {
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r':
			return -1
		}
		return r
	}, strings.TrimSpace(payload))
	if cleaned == "" {
		return nil, errors.New("空载荷")
	}
	normalized := strings.NewReplacer("-", "+", "_", "/").Replace(cleaned)
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding} {
		if b, err := enc.DecodeString(normalized); err == nil {
			return b, nil
		}
	}
	for _, enc := range []*base64.Encoding{base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(cleaned); err == nil {
			return b, nil
		}
	}
	_, err := base64.StdEncoding.DecodeString(normalized)
	return nil, err
}

// v2Spec 是 chara_card_v2 / chara_card_v3 的标准信封（技术契约 §12.1 兼容）。
// 两版共用 data 段，V3 只在其上追加字段；未知扩展保留于原始串，不因读取失败而丢弃。
type v2Spec struct {
	Spec    string `json:"spec"`
	SpecVer string `json:"spec_version"`
	Data    v2Data `json:"data"`
}

// v1Spec 是最早的 TavernAI 扁平卡：字段直接平铺在顶层，没有 spec/data 信封。
type v1Spec struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Personality  string `json:"personality"`
	Scenario     string `json:"scenario"`
	FirstMes     string `json:"first_mes"`
	MesExample   string `json:"mes_example"`
	CreatorNotes string `json:"creator_notes"`
	Avatar       string `json:"avatar"`
}

type v2Data struct {
	Name                    string           `json:"name"`
	Description             string           `json:"description"`
	Personality             string           `json:"personality"`
	Scenario                string           `json:"scenario"`
	FirstMes                string           `json:"first_mes"`
	MesExample              string           `json:"mes_example"`
	SystemPrompt            string           `json:"system_prompt"`
	PostHistoryInstructions string           `json:"post_history_instructions"`
	AlternateGreetings      []string         `json:"alternate_greetings"`
	GroupOnlyGreetings      []string         `json:"group_only_greetings"`
	Tags                    []string         `json:"tags"`
	Creator                 string           `json:"creator"`
	CharacterVersion        string           `json:"character_version"`
	CreatorNotes            string           `json:"creator_notes"`
	Nickname                string           `json:"nickname"`
	Avatar                  string           `json:"avatar"`
	Assets                  []v3Asset        `json:"assets"`
	CharacterBook           *v2CharacterBook `json:"character_book"`
}

type v3Asset struct {
	Type string `json:"type"`
	URI  string `json:"uri"`
	Name string `json:"name"`
}

// v2CharacterBook 是 chara_card_v2/v3 的世界书结构。
// 本版只提取「引用」信息（名称/条目数）以供追溯；条目内容注入见 M3 世界书链路。
type v2CharacterBook struct {
	Name    string        `json:"name"`
	Entries []v2BookEntry `json:"entries"`
}

type v2BookEntry struct {
	Keys          []string `json:"keys"`
	SecondaryKeys []string `json:"secondary_keys"`
	Selective     bool     `json:"selective"`
	Name          string   `json:"name"`
	Comment       string   `json:"comment"`
	Content       string   `json:"content"`
	Enabled       *bool    `json:"enabled"`
}

// mapBookEntries 把 V2/V3 世界书条目规范化为领域条目。
// enabled 缺省视为启用（V2 生态中省略该字段等价于开启）；entryId 用内容派生，保证稳定。
func mapBookEntries(entries []v2BookEntry) []domain.LorebookEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]domain.LorebookEntry, 0, len(entries))
	for _, e := range entries {
		content := strings.TrimSpace(e.Content)
		if content == "" {
			continue
		}
		keys := make([]string, 0, len(e.Keys))
		for _, k := range e.Keys {
			if k = strings.TrimSpace(k); k != "" {
				keys = append(keys, k)
			}
		}
		enabled := true
		if e.Enabled != nil {
			enabled = *e.Enabled
		}
		title := strings.TrimSpace(e.Name)
		if title == "" {
			title = strings.TrimSpace(e.Comment)
		}
		var secondary []string
		if e.Selective {
			for _, key := range e.SecondaryKeys {
				if key = strings.TrimSpace(key); key != "" {
					secondary = append(secondary, key)
				}
			}
		}
		out = append(out, domain.LorebookEntry{
			EntryID:       "lbe_" + hashString(strings.Join(keys, "|") + ":" + content)[:8],
			Title:         title,
			Keys:          keys,
			SecondaryKeys: secondary,
			Content:       content,
			Enabled:       enabled,
		})
	}
	return out
}

// fillCardFromSpec 判定卡片格式（原生 / V1 扁平 / V2 / V3）并映射到原生 CharacterCard，
// 同时记录字段支持表。
func fillCardFromSpec(raw []byte, rep *ImportReport) error {
	var spec v2Spec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return fmt.Errorf("角色卡内容无法解析: %w", err)
	}
	if spec.Spec == "" || spec.Spec == "native" {
		// 解 JSON 成功不能证明是 V2 信封：原生对象同样能解出，只是 data 段全空。
		// 先分辨「V1 扁平卡」，再按原生 schema 解析。
		if looksLikeV1(raw) {
			return fillFromV1(raw, rep)
		}
		card, err := ParseCharacterCard(string(raw))
		if err != nil {
			return fmt.Errorf("角色卡内容无法解析: %w", err)
		}
		if strings.TrimSpace(card.Name) == "" && len(card.Characters) == 0 {
			return errors.New("原生角色卡缺少姓名与角色定义")
		}
		rep.Format = "native"
		rep.Card = card
		rep.Supported = []string{"schemaVersion", "cardId", "name", "description", "characters", "items", "initialState", "secrets", "openingVariants", "lorebookRefs", "rules"}
		return nil
	}
	rep.SpecVersion = spec.SpecVer
	switch spec.Spec {
	case "chara_card_v2":
		rep.Format = "chara_card_v2"
	case "chara_card_v3":
		rep.Format = "chara_card_v3"
	default:
		return fmt.Errorf("不支持的角色卡格式: %s", spec.Spec)
	}
	return mapSpecData(spec.Data, raw, rep)
}

// looksLikeV1 识别 TavernAI 早期扁平卡。
// 原生卡也可以带 first_mes 等外键（导入时会写回卡级），所以必须先确认
// 原生特征缺失，再看 V1 时代专有的对话字段是否存在。
func looksLikeV1(raw []byte) bool {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	for _, nativeKey := range []string{
		"characters", "schemaVersion", "cardId", "openingVariants", "initialState", "lorebookRefs", "rules", "secrets",
	} {
		if _, ok := probe[nativeKey]; ok {
			return false
		}
	}
	for _, v1Key := range []string{"first_mes", "mes_example", "personality", "scenario"} {
		if _, ok := probe[v1Key]; ok {
			return true
		}
	}
	return false
}

// fillFromV1 把扁平卡归一成 V2 的 data 形状后复用同一条映射路径。
// V1 没有世界书与结构化物品，缺失字段保持为空。
func fillFromV1(raw []byte, rep *ImportReport) error {
	var v1 v1Spec
	if err := json.Unmarshal(raw, &v1); err != nil {
		return fmt.Errorf("角色卡内容无法解析: %w", err)
	}
	if strings.TrimSpace(v1.Name) == "" && strings.TrimSpace(v1.FirstMes) == "" {
		return errors.New("角色卡缺少姓名与开场")
	}
	rep.Format = "tavern_v1"
	rep.SpecVersion = "1.0"
	return mapSpecData(v2Data{
		Name:         v1.Name,
		Description:  v1.Description,
		Personality:  v1.Personality,
		Scenario:     v1.Scenario,
		FirstMes:     v1.FirstMes,
		MesExample:   v1.MesExample,
		CreatorNotes: v1.CreatorNotes,
		Avatar:       v1.Avatar,
	}, raw, rep)
}

// mapSpecData 把 V1/V2/V3 的字段映射到原生角色卡，并填写兼容报告。
func mapSpecData(d v2Data, raw []byte, rep *ImportReport) error {
	avatar := strings.TrimSpace(d.Avatar)
	if avatar == "" {
		for _, ast := range d.Assets {
			if ast.Type == "icon" || strings.HasPrefix(ast.URI, "data:image/") || strings.HasPrefix(ast.URI, "http") {
				avatar = ast.URI
				break
			}
		}
	}

	card := &CharacterCard{
		SchemaVersion: 2,
		Name:          d.Name,
		Avatar:        avatar,
		Personality:   d.Personality,
		Scenario:      d.Scenario,
		FirstMes:      d.FirstMes,
		MesExample:    d.MesExample,
		SystemPrompt:  d.SystemPrompt,
		PostHistory:   d.PostHistoryInstructions,
		CreatorNotes:  d.CreatorNotes,
		Tags:          d.Tags,
		Creator:       d.Creator,
		Version:       d.CharacterVersion,
		Nickname:      d.Nickname,
		RawOriginal:   string(raw),
	}

	desc := strings.TrimSpace(strings.Join([]string{d.Description, d.Personality, d.Scenario}, "\n\n"))
	card.Description = desc
	mainID := ""
	if d.Name != "" {
		mainID = "npc_" + hashString(d.Name)[:8]
	}
	card.Characters = []domain.CharacterInfo{{
		CharacterID:  mainID,
		Name:         d.Name,
		Description:  desc,
		Participant:  true,
		Avatar:       avatar,
		Personality:  d.Personality,
		Scenario:     d.Scenario,
		MesExample:   d.MesExample,
		SystemPrompt: d.SystemPrompt,
		PostHistory:  d.PostHistoryInstructions,
		CreatorNotes: d.CreatorNotes,
		Tags:         d.Tags,
		Creator:      d.Creator,
		Version:      d.CharacterVersion,
		Nickname:     d.Nickname,
	}}
	card.Items = nil // V2 卡不提供结构化物品；若 config 有自定义扩展在 M2 解析

	// 世界书：引用 + 条目内容一并落库，供上下文按关键词命中注入（M3 世界书链路）。
	if d.CharacterBook != nil {
		schema := "v2"
		if rep.Format == "chara_card_v3" {
			schema = "v3"
		}
		bookName := strings.TrimSpace(d.CharacterBook.Name)
		if bookName == "" {
			bookName = d.Name + " 的世界书"
		}
		card.LorebookRefs = append(card.LorebookRefs, LorebookRef{
			RefID:   "lb_" + hashString(bookName)[:8],
			Name:    bookName,
			Schema:  schema,
			Entries: mapBookEntries(d.CharacterBook.Entries),
		})
	}

	// 开场：first_mes 为默认开场，alternate_greetings 为备选变体。
	for i, g := range append([]string{d.FirstMes}, d.AlternateGreetings...) {
		if strings.TrimSpace(g) == "" {
			continue
		}
		title := "开场" + runeLabel(i)
		if i == 0 && d.FirstMes != "" {
			title = "默认开场"
		}
		card.OpeningVariants = append(card.OpeningVariants, OpeningVariant{VariantID: "", Title: title, Text: g})
	}

	rep.Card = card
	rep.Supported = []string{"name", "description", "personality", "scenario", "first_mes", "alternate_greetings", "tags", "nickname", "creator_notes"}
	if avatar != "" {
		rep.Supported = append(rep.Supported, "avatar")
	}
	if d.Creator != "" {
		rep.Supported = append(rep.Supported, "creator")
	}
	if d.CharacterVersion != "" {
		rep.Supported = append(rep.Supported, "character_version")
	}
	rep.Ignored = []string{
		"mes_example（示例对白：保留原文，本版不用于训练）",
		"system_prompt（规则模板 M3 启用前不注入）",
		"post_history_instructions（同上）",
	}
	if d.CharacterBook != nil {
		rep.Supported = append(rep.Supported, "character_book（世界书：引用与条目已导入，按关键词命中注入）")
		rep.Ignored = append(rep.Ignored, fmt.Sprintf(
			"character_book.entries（%d 条中导入 %d 条：空内容条目已跳过）",
			len(d.CharacterBook.Entries), len(card.LorebookRefs[len(card.LorebookRefs)-1].Entries)))
	} else {
		rep.Ignored = append(rep.Ignored, "character_book（未提供）")
	}
	if len(d.GroupOnlyGreetings) > 0 {
		rep.Ignored = append(rep.Ignored, fmt.Sprintf("group_only_greetings（%d 条群聊开场：本版为单角色叙事）", len(d.GroupOnlyGreetings)))
	}
	if len(d.AlternateGreetings) == 0 {
		rep.Ignored = append(rep.Ignored, "alternate_greetings（未提供）")
	}
	if rep.Format == "chara_card_v3" {
		rep.Warnings = append(rep.Warnings, "spec 为 chara_card_v3：基础字段与 assets 已按 V3 导入；depth_prompt 等扩展仍保留于原卡不参与注入")
	}
	return nil
}

// runeLabel 给开场变体编号。索引 i 是「默认开场 + 备选」合并后的序号，
// 因此 0 号（默认开场）没有字母，第一个备选从 b 起——a 位被默认开场占掉，
// 于是标题形如「默认开场 / 开场b / 开场c」。前端不再自行编号，这里是唯一规则。
func runeLabel(i int) string {
	if i == 0 {
		return ""
	}
	const base = 'b'
	if i-1 < 26 {
		return string(rune(base + i - 1))
	}
	return fmt.Sprintf("%d", i)
}

// warn 附加 HTML/脚本检测与秘密泄漏提示（脚本/HTML 按纯文本保留，不执行）。
func (r *ImportReport) warn() {
	c := r.Card
	leaks := c.ScanSecretLeakWarnings()
	r.Warnings = append(r.Warnings, leaks...)
	for _, f := range []struct{ name, val string }{
		{"描述", c.Description}, {"开场", firstOpening(c)}, {"物品", joinItems(c)},
	} {
		if containsScriptLike(f.val) {
			r.Warnings = append(r.Warnings, "「"+f.name+"」包含疑似 HTML/脚本标记，已按纯文本保留，不会作为指令执行")
		}
	}
}

func firstOpening(c *CharacterCard) string {
	if len(c.OpeningVariants) > 0 {
		return c.OpeningVariants[0].Text
	}
	return ""
}

func joinItems(c *CharacterCard) string {
	var b strings.Builder
	for _, it := range c.Items {
		b.WriteString(it.Name)
		b.WriteByte(' ')
	}
	return b.String()
}

// containsScriptLike 宽松检测可执行/脚本化标记（防御性提示，不参与执行）。
var (
	htmlTagRe = regexp.MustCompile(`(?i)<\s*(script|iframe|object|embed|style)\b[^>]*>`)
	evtRe     = regexp.MustCompile(`(?i)\son(load|error|click|mouseover|focus|submit)\s*=`)

	// 兼容旧用法（tEXt 关键字集）
	knownMetaKeys = map[string]bool{
		metaKeyChara: true, metaKeyCCV3: true,
		metaKeyCreator: true, metaKeyNotes: true, metaKeyComment: true,
	}
)

func containsScriptLike(s string) bool {
	return htmlTagRe.MatchString(s) || evtRe.MatchString(s)
}

// ---- PNG chunk 读取（纯 Go，仅标准库） ----

// readPNGMeta 扫描 PNG 各 chunk，提取文本元数据（tEXt/zTXt/iTXt）。
// 不验证 CRC（宽松导入），但校验 chunk 类型与长度边界，防止越界与膨胀。
// 单个文本块的编码错误记为警告而不是错误：相邻的野块不该让能读的卡片导入失败。
func readPNGMeta(data []byte) (map[string]string, []string, error) {
	if len(data) < 24 {
		return nil, nil, errors.New("不是有效的 PNG（长度不足）")
	}
	sig := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	if !bytes.Equal(data[:8], sig) {
		return nil, nil, errors.New("不是有效的 PNG（签名不匹配）")
	}
	meta := map[string]string{}
	var warns []string
	pos := 8
	for pos+8 <= len(data) {
		length := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		ctype := string(data[pos+4 : pos+8])
		if length < 0 || pos+8+length > len(data) {
			return nil, warns, errors.New("PNG chunk 长度越界或数据被截断")
		}
		body := data[pos+8 : pos+8+length]
		if ctype == "tEXt" || ctype == "zTXt" || ctype == "iTXt" {
			if err := readTextChunk(ctype, body, meta); err != nil {
				warns = append(warns, fmt.Sprintf("%s 文本块读取失败，已跳过：%v", ctype, err))
			}
		}
		if ctype == "IEND" {
			break
		}
		pos += 12 + length
	}
	if len(meta) == 0 {
		return nil, warns, errors.New("PNG 未包含文本元数据")
	}
	return meta, warns, nil
}

// decodePNGText 解释 tEXt/zTXt 的文本字节。
// 规范要求 Latin-1，但角色卡工具普遍直接写 UTF-8（中文卡全靠这一点）；
// 按 UTF-8 优先解释，不是合法 UTF-8 时再退回 Latin-1。
func decodePNGText(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	return latin1ToString(b)
}

func readTextChunk(ctype string, body []byte, out map[string]string) error {
	nul := bytes.IndexByte(body, 0)
	if nul < 1 || nul > 79 {
		return errors.New("无效的文本 chunk：缺少关键字终止符")
	}
	key := string(body[:nul])
	rest := body[nul+1:]
	var text string
	switch ctype {
	case "tEXt":
		text = decodePNGText(rest)
	case "zTXt":
		if len(rest) < 2 {
			return errors.New("无效的 zTXt chunk")
		}
		if rest[0] != 0 {
			return errors.New("不支持的 zTXt 压缩方法")
		}
		zr, err := zlib.NewReader(bytes.NewReader(rest[1:]))
		if err != nil {
			return fmt.Errorf("zTXt 解压失败: %w", err)
		}
		defer zr.Close()
		b, err := io.ReadAll(io.LimitReader(zr, maxMetaChunk))
		if err != nil {
			return fmt.Errorf("zTXt 读取失败: %w", err)
		}
		text = decodePNGText(b)
	case "iTXt":
		if len(rest) < 3 {
			return errors.New("无效的 iTXt chunk")
		}
		compFlag, compMethod := rest[0], rest[1]
		l := rest[2:]
		nulA := bytes.IndexByte(l, 0) // language tag 终止
		if nulA < 0 {
			return errors.New("无效的 iTXt chunk：语言标签未终止")
		}
		l = l[nulA+1:]
		nulB := bytes.IndexByte(l, 0) // translated keyword 终止
		if nulB < 0 {
			return errors.New("无效的 iTXt chunk：翻译关键字未终止")
		}
		payload := l[nulB+1:]
		if len(payload) > maxMetaChunk {
			return errors.New("iTXt chunk 超过大小上限")
		}
		switch {
		case compFlag == 1 && compMethod == 0:
			zr, err := zlib.NewReader(bytes.NewReader(payload))
			if err != nil {
				return fmt.Errorf("iTXt 解压失败: %w", err)
			}
			defer zr.Close()
			b, err := io.ReadAll(io.LimitReader(zr, maxMetaChunk))
			if err != nil {
				return fmt.Errorf("iTXt 读取失败: %w", err)
			}
			text = decodePNGText(b)
		case compFlag == 0:
			text = decodePNGText(payload)
		default:
			return errors.New("不支持的 iTXt 压缩配置")
		}
	default:
		return nil
	}
	if key == metaKeyChara || key == metaKeyCCV3 || knownMetaKeys[key] {
		out[key] = text
	}
	return nil
}

// latin1ToString 按 Latin-1（ISO-8859-1）解释字节（tEXt/zTXt 的官方编码；
// base64 载荷为 ASCII，直接转换即可）。
func latin1ToString(b []byte) string {
	out := make([]rune, 0, len(b))
	for _, c := range b {
		out = append(out, rune(c))
	}
	return string(out)
}
