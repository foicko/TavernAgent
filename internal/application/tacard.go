package application

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"tavernagent/internal/domain"
)

// .tacard 是 TavernAgent 原生角色卡容器（规范 docs/TACARD_SPEC.md）：标准 ZIP，
// 根级 tacard.json 为唯一权威清单，assets/** 为声明式静态媒体。
//
// 与 charx 的本质区别是「零脚本 + 声明式契约」：归档内不得出现任何可执行/脚本文件；
// 视觉与状态一律由 [IMG:id]/[SCENE]/[HUD]/[VAR] 标记与 ui_contract 驱动，宿主不执行卡内代码。
// 因此 tacard.json 而非（可选的）兼容 card.json 才是解析入口。
const (
	tacardCardEntry = "tacard.json"
	// maxTacardCardBytes 是 tacard.json 解压后的上限（防压缩炸弹）。
	maxTacardCardBytes = 8 << 20
	// tacardSpecV1 是本实现支持的规范版本。
	tacardSpecV1 = "tacard/v1"
)

// maxAvatarCandidates 限制头像候选的尝试次数：每个候选可能读满 64MB，
// 声明大量候选且都解不出图时会造成内存/CPU 放大。
const maxAvatarCandidates = 8

// tacardForbiddenExt 是规范第 5 节禁入的可执行/脚本文件扩展名（小写，不含点）。
// 命中任意一项即整卡拒绝，而不是静默忽略：零脚本策略是 .tacard 的安全契约。
var tacardForbiddenExt = map[string]bool{
	"exe": true, "dll": true, "so": true, "dylib": true,
	"sh": true, "bat": true, "cmd": true, "ps1": true,
	"risum": true, "lua": true, "js": true, "mjs": true, "cjs": true,
	"wasm": true, "py": true, "jar": true, "com": true, "scr": true, "vbs": true,
}

// tacardDoc 是 tacard.json 的顶层结构。未知字段忽略（规范向前兼容）。
type tacardDoc struct {
	Spec       string           `json:"spec"`
	Metadata   tacardMetadata   `json:"metadata"`
	Character  tacardCharacter  `json:"character"`
	Openings   []tacardOpening  `json:"openings"`
	Lorebook   tacardLorebook   `json:"lorebook"`
	Assets     []tacardAsset    `json:"assets"`
	UIContract tacardUIContract `json:"ui_contract"`
}

type tacardMetadata struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Author      string   `json:"author"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

type tacardCharacter struct {
	Name                    string   `json:"name"`
	Aliases                 []string `json:"aliases"`
	AvatarAsset             string   `json:"avatar_asset"`
	Description             string   `json:"description"`
	Personality             string   `json:"personality"`
	Scenario                string   `json:"scenario"`
	SystemPrompt            string   `json:"system_prompt"`
	PostHistoryInstructions string   `json:"post_history_instructions"`
}

type tacardOpening struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

type tacardLorebook struct {
	Name    string                `json:"name"`
	Entries []tacardLorebookEntry `json:"entries"`
}

type tacardLorebookEntry struct {
	ID            string   `json:"id"`
	Keys          []string `json:"keys"`
	SecondaryKeys []string `json:"secondary_keys"`
	Content       string   `json:"content"`
	Constant      bool     `json:"constant"`
	Order         int      `json:"order"`
	Position      string   `json:"position"`
}

type tacardAsset struct {
	ID   string `json:"id"`
	File string `json:"file"`
	Type string `json:"type"`
}

type tacardUIContract struct {
	ThemeTokens      map[string]string          `json:"theme_tokens"`
	DefaultVariables map[string]json.RawMessage `json:"default_variables"`
}

// IsTacardArchive 报告 path 是否是一个 .tacard 归档（ZIP 且含 tacard.json）。
// 只读取 ZIP 中央目录，不解压内容，对超大文件也安全。
func IsTacardArchive(archivePath string) bool {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return false
	}
	defer zr.Close()
	return findTacardCardEntry(&zr.Reader) != nil
}

// ImportCardTacard 解析 .tacard 归档并返回兼容报告。
//
// 以 tacard.json 为唯一权威：兼容层 card.json（若存在）一律忽略。
// 归档资源与 charx 复用同一套「登记清单 → 建卡成功后流式落地」管线，避免解析阶段写盘产生孤儿。
func ImportCardTacard(archivePath string) (*ImportReport, error) {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, fmt.Errorf("tacard 归档无法打开: %w", err)
	}
	defer zr.Close()

	if bad := tacardForbiddenEntries(&zr.Reader); len(bad) > 0 {
		return nil, fmt.Errorf(
			"tacard 含规范禁入的可执行/脚本文件（零脚本策略），已拒绝导入: %s",
			strings.Join(bad, "、"))
	}

	entry := findTacardCardEntry(&zr.Reader)
	if entry == nil {
		return nil, errors.New("tacard 归档中未找到 tacard.json")
	}
	if entry.UncompressedSize64 > maxTacardCardBytes {
		return nil, fmt.Errorf("tacard.json 解压后超过 %d MB 上限", maxTacardCardBytes>>20)
	}
	payload, err := readZipEntry(entry, maxTacardCardBytes)
	if err != nil {
		return nil, fmt.Errorf("读取 tacard.json 失败: %w", err)
	}

	var doc tacardDoc
	if err := json.Unmarshal(payload, &doc); err != nil {
		return nil, fmt.Errorf("tacard.json 解析失败: %w", err)
	}
	if err := checkTacardSpec(doc.Spec); err != nil {
		return nil, err
	}
	if strings.TrimSpace(doc.Character.Name) == "" && strings.TrimSpace(doc.Metadata.Name) == "" {
		return nil, errors.New("tacard 缺少角色名称（character.name / metadata.name 均为空）")
	}

	rep := &ImportReport{
		Format:           "tacard",
		Source:           "tacard",
		SpecVersion:      strings.TrimSpace(doc.Spec),
		RawJSON:          string(payload),
		CharxArchivePath: archivePath, // 复用 charx 的流式资源落地管线（见 CardService.persistCharxAssets）
	}
	if rep.SpecVersion == "" {
		rep.SpecVersion = tacardSpecV1
		rep.Warnings = append(rep.Warnings, "tacard.json 缺少 spec 字段，已按 tacard/v1 解析")
	} else if rep.SpecVersion != tacardSpecV1 {
		rep.Warnings = append(rep.Warnings,
			fmt.Sprintf("规范版本 %s 高于本实现支持的 %s：未知字段已忽略", rep.SpecVersion, tacardSpecV1))
	}
	rep.Card = mapTacardDoc(&doc)
	normalizeCard(rep.Card)
	rep.warn()
	collectTacardAssets(&zr.Reader, rep)
	attachTacardAvatar(rep, &zr.Reader, doc.Character.AvatarAsset)
	attachTacardUIProfile(rep, &doc)
	rep.warnTacard(&zr.Reader)
	return rep, nil
}

// checkTacardSpec 校验规范版本：接受 tacard/v1（含缺省）；明确是别的格式则拒绝；
// 同族更高版本放行但提示未知字段已忽略，保证向前兼容。
func checkTacardSpec(spec string) error {
	spec = strings.TrimSpace(spec)
	switch {
	case spec == "" || spec == tacardSpecV1:
		return nil
	case strings.HasPrefix(spec, "tacard/"):
		return nil
	default:
		return fmt.Errorf("不支持的卡片规范: %s（期望 %s）", spec, tacardSpecV1)
	}
}

// mapTacardDoc 把 tacard.json 映射到原生 CharacterCard。
func mapTacardDoc(doc *tacardDoc) *CharacterCard {
	name := strings.TrimSpace(doc.Character.Name)
	if name == "" {
		name = strings.TrimSpace(doc.Metadata.Name)
	}
	card := &CharacterCard{
		SchemaVersion: 2,
		CardID:        tacardCardID(doc.Metadata.ID),
		Name:          name,
		Description:   strings.TrimSpace(doc.Character.Description),
		Personality:   strings.TrimSpace(doc.Character.Personality),
		Scenario:      strings.TrimSpace(doc.Character.Scenario),
		SystemPrompt:  strings.TrimSpace(doc.Character.SystemPrompt),
		PostHistory:   strings.TrimSpace(doc.Character.PostHistoryInstructions),
		CreatorNotes:  strings.TrimSpace(doc.Metadata.Description),
		Tags:          cleanTacardStrings(doc.Metadata.Tags),
		Creator:       strings.TrimSpace(doc.Metadata.Author),
		Version:       strings.TrimSpace(doc.Metadata.Version),
		Nickname:      strings.Join(cleanTacardStrings(doc.Character.Aliases), ", "),
	}
	// 开场：openings 是有序剧本列表，首条即默认开场（first_mes 等效）。
	for _, op := range doc.Openings {
		text := strings.TrimSpace(op.Text)
		if text == "" {
			continue
		}
		card.OpeningVariants = append(card.OpeningVariants, OpeningVariant{
			VariantID: strings.TrimSpace(op.ID),
			Title:     strings.TrimSpace(op.Title),
			Text:      text,
		})
		if card.FirstMes == "" {
			card.FirstMes = text
		}
	}
	// 世界书：保留常驻（constant）与层级顺序（order）。
	if entries := mapTacardLorebook(doc.Lorebook.Entries); len(entries) > 0 {
		bookName := strings.TrimSpace(doc.Lorebook.Name)
		if bookName == "" {
			bookName = name + " 的世界书"
		}
		card.LorebookRefs = append(card.LorebookRefs, LorebookRef{
			RefID:   "lb_" + hashString(bookName)[:8],
			Name:    bookName,
			Schema:  "tacard",
			Entries: entries,
		})
	}
	// 资产清单：id 即正文 [IMG:id] 的键（与 charx 的 assets[].name 同义）。
	for _, a := range doc.Assets {
		id := strings.TrimSpace(a.ID)
		file := normalizeTacardAssetPath(a.File)
		if id == "" || file == "" {
			continue
		}
		card.Assets = append(card.Assets, CardAsset{
			Name: id,
			Type: strings.TrimSpace(a.Type),
			URI:  file,
			Ext:  strings.TrimPrefix(path.Ext(file), "."),
		})
	}
	mainID := ""
	if name != "" {
		mainID = "npc_" + hashString(name)[:8]
	}
	card.Characters = []domain.CharacterInfo{{
		CharacterID:  mainID,
		Name:         name,
		Aliases:      cleanTacardStrings(doc.Character.Aliases),
		Description:  card.Description,
		Participant:  true,
		Personality:  card.Personality,
		Scenario:     card.Scenario,
		SystemPrompt: card.SystemPrompt,
		PostHistory:  card.PostHistory,
		CreatorNotes: card.CreatorNotes,
		Tags:         card.Tags,
		Creator:      card.Creator,
		Version:      card.Version,
		Nickname:     card.Nickname,
	}}
	return card
}

// mapTacardLorebook 把 tacard 世界书条目映射为领域条目，保留常驻与顺序。
func mapTacardLorebook(entries []tacardLorebookEntry) []domain.LorebookEntry {
	out := make([]domain.LorebookEntry, 0, len(entries))
	for _, e := range entries {
		content := strings.TrimSpace(e.Content)
		if content == "" {
			continue
		}
		keys := cleanTacardStrings(e.Keys)
		id := strings.TrimSpace(e.ID)
		if id == "" {
			id = "lbe_" + hashString(strings.Join(keys, "|") + ":" + content)[:8]
		}
		out = append(out, domain.LorebookEntry{
			EntryID:       id,
			Keys:          keys,
			SecondaryKeys: cleanTacardStrings(e.SecondaryKeys),
			Content:       content,
			Enabled:       true,
			Constant:      e.Constant,
			Order:         e.Order,
			Position:      strings.ToLower(strings.TrimSpace(e.Position)),
		})
	}
	return out
}

// collectTacardAssets 把清单条目与归档内 assets/** 对齐，登记待落地资源并回填 Ref。
// 缺失文件清空 URI：它没有可服务的字节，留着会让前端渲染出断裂的图片。
func collectTacardAssets(zr *zip.Reader, rep *ImportReport) {
	card := rep.Card
	if card == nil || len(card.Assets) == 0 {
		return
	}
	byPath := tacardZipIndex(zr)
	used := map[string]bool{}
	refByPath := map[string]string{}
	var missing int
	for i := range card.Assets {
		a := &card.Assets[i]
		if _, ok := byPath[a.URI]; !ok {
			a.URI = ""
			missing++
			continue
		}
		ext := strings.TrimPrefix(a.Ext, ".")
		if ref, seen := refByPath[a.URI]; seen {
			a.Ref = ref
			a.Ext = ext
			continue
		}
		ref := uniqueAssetFile(a.Name, a.URI, ext, used)
		refByPath[a.URI] = ref
		a.Ref = ref
		a.Ext = ext
		rep.CharxAssets = append(rep.CharxAssets, CharxAsset{Name: a.Name, ZipPath: a.URI, Ref: ref, Ext: ext})
	}
	if missing > 0 {
		rep.Warnings = append(rep.Warnings,
			fmt.Sprintf("tacard 声明了 %d 项资源但归档中缺少对应文件，相关图片无法显示", missing))
	}
}

// attachTacardAvatar 用 avatar_asset（或 type=avatar、任意图片）生成有界头像缩略图。
func attachTacardAvatar(rep *ImportReport, zr *zip.Reader, avatarAsset string) {
	if rep == nil || rep.Card == nil {
		return
	}
	byPath := tacardZipIndex(zr)
	var candidates []string
	addByName := func(id string) {
		for _, a := range rep.Card.Assets {
			if a.Name == id && a.URI != "" {
				candidates = append(candidates, a.URI)
			}
		}
	}
	if id := strings.TrimSpace(avatarAsset); id != "" {
		addByName(id)
	}
	for _, a := range rep.Card.Assets {
		if strings.EqualFold(strings.TrimSpace(a.Type), "avatar") {
			addByName(a.Name)
		}
	}
	for _, a := range rep.Card.Assets {
		if isTacardImageAsset(a) && a.URI != "" {
			candidates = append(candidates, a.URI)
		}
	}

	tried := map[string]bool{}
	attempts := 0
	for _, p := range candidates {
		if attempts >= maxAvatarCandidates {
			break
		}
		if p == "" || tried[p] {
			continue
		}
		tried[p] = true
		attempts++
		f, ok := byPath[p]
		if !ok {
			continue
		}
		data, err := readZipEntry(f, maxCharxEntryBytes)
		if err != nil {
			continue
		}
		if avatar := buildAvatarDataURL(data); avatar != "" {
			rep.Card.Avatar = avatar
			if len(rep.Card.Characters) == 1 {
				rep.Card.Characters[0].Avatar = avatar
			}
			return
		}
	}
	if id := strings.TrimSpace(avatarAsset); id != "" {
		rep.Warnings = append(rep.Warnings,
			"tacard 的 avatar_asset「"+id+"」缺失或无法解码，未生成头像缩略图（其余字段已正常导入）")
	}
}

// attachTacardUIProfile 由 ui_contract 生成卡片 UI 档案：主题 token 与变量初值。
// 静态契约是 .tacard 允许的「声明式意图」，因此不需要像 charx 那样从 HTML 里白名单抽取。
func attachTacardUIProfile(rep *ImportReport, doc *tacardDoc) {
	if rep == nil || rep.Card == nil {
		return
	}
	caps := CardCapabilities{Source: "tacard"}
	text := cardUIText(rep.Card)
	caps.ImgCmd = strings.Contains(text, "<img")
	caps.MarkerFamilies = detectMarkerFamilies(text)

	img, audio := 0, 0
	for _, a := range rep.Card.Assets {
		switch strings.ToLower(strings.TrimPrefix(a.Ext, ".")) {
		case "mp3", "wav", "ogg", "m4a", "flac":
			audio++
		case "png", "webp", "jpg", "jpeg", "gif", "avif":
			img++
		}
	}
	caps.AssetCount = len(rep.Card.Assets)
	caps.ImageAssetCount = img
	caps.AudioAssetCount = audio
	for _, lb := range rep.Card.LorebookRefs {
		caps.LorebookEntries += len(lb.Entries)
	}

	theme := sanitizeTacardTheme(doc.UIContract.ThemeTokens)
	vars := parseTacardVariables(doc.UIContract.DefaultVariables)
	caps.HasDefaultVariables = len(vars) > 0
	if !caps.hasAny() && len(theme) == 0 {
		return
	}
	profile := &CardUIProfile{Capabilities: caps}
	if len(theme) > 0 {
		profile.ThemeTokens = theme
	}
	if len(vars) > 0 {
		profile.VarDefaults = vars
	}
	rep.Card.UIProfile = profile
}

// sanitizeTacardTheme 校验 ui_contract.theme_tokens：颜色必须是固定格式，字体名只允许安全字符。
func sanitizeTacardTheme(raw map[string]string) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	out := map[string]string{}
	for k, v := range raw {
		key := normalizeThemeKey(strings.ToLower(strings.TrimSpace(k)))
		if key == "" {
			continue
		}
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if key == "font" {
			if !isFontValue(v) {
				continue
			}
		} else if !themeColorRe.MatchString(v) {
			continue
		}
		if _, exists := out[key]; !exists {
			out[key] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// parseTacardVariables 解析 ui_contract.default_variables，只保留可安全字符串化的标量初值。
func parseTacardVariables(raw map[string]json.RawMessage) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	out := map[string]string{}
	for k, v := range raw {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if s, ok := scalarToString(v); ok {
			out[k] = s
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// warnTacard 补充 tacard 专属的兼容提示。
func (r *ImportReport) warnTacard(zr *zip.Reader) {
	var assetFiles, otherFiles int
	hasCompat := false
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := strings.TrimPrefix(strings.ReplaceAll(f.Name, "\\", "/"), "./")
		switch {
		case name == tacardCardEntry:
		case name == charxCardEntry:
			hasCompat = true
		case strings.HasPrefix(name, "assets/"):
			assetFiles++
		default:
			otherFiles++
		}
	}
	if hasCompat {
		r.Supported = append(r.Supported, "兼容层 card.json（按 tacard.json 权威解析，已忽略）")
	}
	if len(r.CharxAssets) > 0 {
		r.Supported = append(r.Supported, fmt.Sprintf("内嵌资源 %d 项（[IMG:id] 可渲染）", len(r.CharxAssets)))
	}
	if extra := assetFiles - len(r.CharxAssets); extra > 0 {
		r.Warnings = append(r.Warnings, fmt.Sprintf("tacard 的 assets/ 含 %d 个未被清单引用的文件，已忽略", extra))
	}
	if otherFiles > 0 {
		r.Warnings = append(r.Warnings, fmt.Sprintf("tacard 含 %d 个 assets/ 之外的非资源文件，已忽略", otherFiles))
	}
	if r.Card != nil && len(r.Card.OpeningVariants) == 0 {
		r.Warnings = append(r.Warnings, "tacard 未提供任何开场白，创建故事时需手动填写第一幕")
	}
}

// findTacardCardEntry 定位 tacard.json：优先根级，其次最浅的同名条目。
func findTacardCardEntry(zr *zip.Reader) *zip.File {
	var fallback *zip.File
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := strings.TrimPrefix(strings.ReplaceAll(f.Name, "\\", "/"), "./")
		if name == tacardCardEntry {
			return f
		}
		if strings.EqualFold(path.Base(name), tacardCardEntry) && fallback == nil {
			fallback = f
		}
	}
	return fallback
}

// tacardForbiddenEntries 列出归档中命中零脚本黑名单的条目（去重、排序）。
func tacardForbiddenEntries(zr *zip.Reader) []string {
	var bad []string
	seen := map[string]bool{}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := strings.TrimPrefix(strings.ReplaceAll(f.Name, "\\", "/"), "./")
		ext := strings.ToLower(strings.TrimPrefix(path.Ext(name), "."))
		if tacardForbiddenExt[ext] && !seen[name] {
			seen[name] = true
			bad = append(bad, name)
		}
	}
	sort.Strings(bad)
	return bad
}

// tacardZipIndex 建立归档内路径 → 条目的索引（归一化分隔符与前导 ./）。
func tacardZipIndex(zr *zip.Reader) map[string]*zip.File {
	byPath := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		byPath[strings.TrimPrefix(strings.ReplaceAll(f.Name, "\\", "/"), "./")] = f
	}
	return byPath
}

// tacardCardID 校验外部声明的卡 ID：含路径分隔符/控制字符或过长时返回空串，
// 交由 normalizeCard 由内容派生稳定 ID，避免把危险标识带进卡库主键与资源目录。
func tacardCardID(raw string) string {
	id := strings.TrimSpace(raw)
	if id == "" || len(id) > 128 {
		return ""
	}
	for _, r := range id {
		if r < 0x20 || r == '/' || r == '\\' || r == '?' || r == '#' || r == '%' {
			return ""
		}
	}
	return id
}

// normalizeTacardAssetPath 归一化资源路径：统一分隔符、拒绝穿越、补齐 assets/ 前缀。
func normalizeTacardAssetPath(raw string) string {
	p := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "/")
	if p == "" || strings.Contains(p, "..") {
		return ""
	}
	if !strings.HasPrefix(p, "assets/") {
		p = "assets/" + p
	}
	return p
}

// isTacardImageAsset 判断资源是否为可作头像候选的图片类型。
func isTacardImageAsset(a CardAsset) bool {
	switch strings.ToLower(strings.TrimPrefix(a.Ext, ".")) {
	case "png", "webp", "jpg", "jpeg", "gif", "avif":
		return true
	}
	return strings.EqualFold(strings.TrimSpace(a.Type), "avatar") ||
		strings.EqualFold(strings.TrimSpace(a.Type), "emotion")
}

// cleanTacardStrings 去空、去首尾空白；结果为空时返回 nil。
func cleanTacardStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
