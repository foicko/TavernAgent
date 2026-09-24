package application

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

// CardCapabilities 汇总一张卡声明的 RisuAI/charx 特性，用于导入兼容报告与
// 后续的「卡片 UI 映射」——把卡作者的意图映射到本应用的原生组件，而不是执行卡内代码。
//
// 全部字段来自 card.json（含 extensions.risuai）与卡片文本，**不解析 module.risum**：
// 后者是 RisuAI 的 RPack 二进制，其解码表随 RisuAI 仓库以 GPL-3.0 分发，本项目（MIT）
// 不内嵌该表，因此只登记「存在」而无法读取其内容。这里登记的能力足以驱动标记契约与
// 兼容报告；Lua/regex 的细节留待将来以干净室实现或显式授权后再补。
type CardCapabilities struct {
	Source              string   `json:"source,omitempty"`
	HasModule           bool     `json:"hasModule,omitempty"`
	HasBackgroundHTML   bool     `json:"hasBackgroundHTML,omitempty"`
	HasDefaultVariables bool     `json:"hasDefaultVariables,omitempty"`
	HasSDData           bool     `json:"hasSDData,omitempty"`
	HasDepthPrompt      bool     `json:"hasDepthPrompt,omitempty"`
	HasVits             bool     `json:"hasVits,omitempty"`
	HasAdditionalText   bool     `json:"hasAdditionalText,omitempty"`
	ViewScreen          string   `json:"viewScreen,omitempty"`
	LowLevelAccess      bool     `json:"lowLevelAccess,omitempty"`
	AssetCount          int      `json:"assetCount,omitempty"`
	ImageAssetCount     int      `json:"imageAssetCount,omitempty"`
	AudioAssetCount     int      `json:"audioAssetCount,omitempty"`
	LorebookEntries     int      `json:"lorebookEntries,omitempty"`
	ImgCmd              bool     `json:"imgCmd,omitempty"`
	MarkerFamilies      []string `json:"markerFamilies,omitempty"`
}

// CardUIProfile 是「卡片 UI 映射」的静态档案：导入时从卡里抽取一次，随卡 JSON 冻结，
// 供编译层生成标记契约、供前端渲染状态面板。当前承载能力清单与变量初值；
// 场景/语言/HUD 字段等留待对应阶段填充。
type CardUIProfile struct {
	Capabilities CardCapabilities  `json:"capabilities"`
	VarDefaults  map[string]string `json:"varDefaults,omitempty"`
}

// markerFamilyColonRe 匹配 [Name:...] 形态的卡内标记（如 [CeresStatus:date=…]）。
var markerFamilyColonRe = regexp.MustCompile(`\[([A-Za-z][A-Za-z0-9_]{1,31}):`)
var markerFamilyTagRe = regexp.MustCompile(`\[([A-Za-z][A-Za-z0-9_]{1,31})\]`)

// risuExtensions 是 card.json 里 extensions.risuai（以及并列的 depth_prompt）的关注子集。
// 未知键忽略；值可能为空字符串/空数组，表示卡作者未使用。
type risuExtensions struct {
	BackgroundHTML   string          `json:"backgroundHTML"`
	DefaultVariables string          `json:"defaultVariables"`
	ViewScreen       string          `json:"viewScreen"`
	SDData           json.RawMessage `json:"sdData"`
	LowLevelAccess   bool            `json:"lowLevelAccess"`
	Vits             json.RawMessage `json:"vits"`
	AdditionalText   json.RawMessage `json:"additionalText"`
}

// DetectCardUIProfile 从导入报告抽取卡片 UI 档案。
//
// hasModule 由调用方告知（charx 归档是否含 module.risum）；其余能力全部从
// card.json 的 extensions 与卡片文本推断。rep 必须已完成解析（rep.Card 非空）。
func DetectCardUIProfile(rep *ImportReport, hasModule bool) *CardUIProfile {
	if rep == nil || rep.Card == nil {
		return nil
	}
	caps := CardCapabilities{
		Source:    strings.TrimSpace(rep.Source),
		HasModule: hasModule,
	}
	if rep.Format == "charx" {
		caps.Source = "charx"
	}

	ext := parseRisuExtensions(rep.RawJSON)
	caps.HasBackgroundHTML = strings.TrimSpace(ext.BackgroundHTML) != ""
	caps.HasDefaultVariables = strings.TrimSpace(ext.DefaultVariables) != ""
	caps.HasSDData = len(bytesTrimmed(ext.SDData)) > 0 && string(bytesTrimmed(ext.SDData)) != "[]"
	caps.HasVits = len(bytesTrimmed(ext.Vits)) > 0 && string(bytesTrimmed(ext.Vits)) != "{}"
	caps.HasAdditionalText = len(bytesTrimmed(ext.AdditionalText)) > 0 && string(bytesTrimmed(ext.AdditionalText)) != "[]"
	caps.LowLevelAccess = ext.LowLevelAccess
	if vs := strings.TrimSpace(ext.ViewScreen); vs != "" && vs != "none" {
		caps.ViewScreen = vs
	}
	caps.HasDepthPrompt = hasDepthPrompt(rep.RawJSON)

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

	if !caps.hasAny() {
		// 无任何 RisuAI 特性：不生成档案，保持原生卡 JSON 的字节稳定。
		return nil
	}
	profile := &CardUIProfile{Capabilities: caps}
	if vars := parseDefaultVariables(ext.DefaultVariables); len(vars) > 0 {
		profile.VarDefaults = vars
	}
	return profile
}

// hasAny 判断能力清单是否包含任何实质内容（Source 恒有值，故不计入）。
func (c CardCapabilities) hasAny() bool {
	return c.HasModule || c.HasBackgroundHTML || c.HasDefaultVariables ||
		c.HasSDData || c.HasDepthPrompt || c.HasVits || c.HasAdditionalText ||
		c.ViewScreen != "" || c.LowLevelAccess || c.AssetCount > 0 ||
		c.LorebookEntries > 0 || c.ImgCmd || len(c.MarkerFamilies) > 0
}

// attachUIProfile 计算并挂载卡片 UI 档案（幂等，按并集合并）。
//
// 两处调用：解析原始卡 JSON 时（charx 归档随后还会用 hasModule=true 再调一次），
// 以及从卡库重新解析归一化 JSON 时。后者已经带冻结的 UIProfile，而重新推断只能看到
// 归一化后的文本（extensions 已丢），所以必须按并集合并而非覆盖，避免清空已冻结能力。
func attachUIProfile(rep *ImportReport, hasModule bool) {
	if rep == nil || rep.Card == nil {
		return
	}
	detected := DetectCardUIProfile(rep, hasModule)
	if rep.Card.UIProfile == nil {
		rep.Card.UIProfile = detected // 可能为 nil：无特性时不写档案
		return
	}
	if detected == nil {
		return
	}
	existing := rep.Card.UIProfile
	mergeCapabilities(&existing.Capabilities, detected.Capabilities)
	if len(existing.VarDefaults) == 0 {
		existing.VarDefaults = detected.VarDefaults
	}
	rep.Card.UIProfile = existing
}

// mergeCapabilities 把 src 的实质内容并入 dst（布尔取或、计数取大、字符串取先非空）。
func mergeCapabilities(dst *CardCapabilities, src CardCapabilities) {
	dst.HasModule = dst.HasModule || src.HasModule
	dst.HasBackgroundHTML = dst.HasBackgroundHTML || src.HasBackgroundHTML
	dst.HasDefaultVariables = dst.HasDefaultVariables || src.HasDefaultVariables
	dst.HasSDData = dst.HasSDData || src.HasSDData
	dst.HasDepthPrompt = dst.HasDepthPrompt || src.HasDepthPrompt
	dst.HasVits = dst.HasVits || src.HasVits
	dst.HasAdditionalText = dst.HasAdditionalText || src.HasAdditionalText
	dst.LowLevelAccess = dst.LowLevelAccess || src.LowLevelAccess
	dst.ImgCmd = dst.ImgCmd || src.ImgCmd
	if dst.ViewScreen == "" {
		dst.ViewScreen = src.ViewScreen
	}
	if dst.Source == "" {
		dst.Source = src.Source
	}
	dst.AssetCount = maxInt(dst.AssetCount, src.AssetCount)
	dst.ImageAssetCount = maxInt(dst.ImageAssetCount, src.ImageAssetCount)
	dst.AudioAssetCount = maxInt(dst.AudioAssetCount, src.AudioAssetCount)
	dst.LorebookEntries = maxInt(dst.LorebookEntries, src.LorebookEntries)
	dst.MarkerFamilies = unionSorted(dst.MarkerFamilies, src.MarkerFamilies)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func unionSorted(a, b []string) []string {
	if len(b) == 0 {
		return a
	}
	set := make(map[string]bool, len(a)+len(b))
	for _, s := range a {
		set[s] = true
	}
	for _, s := range b {
		set[s] = true
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// parseRisuExtensions 从原始卡 JSON 中提取 extensions.risuai 与 extensions.depth_prompt。
// V3 在 data.extensions 下，旧版/原生卡可能在顶层 extensions；两处都尝试。
func parseRisuExtensions(raw string) risuExtensions {
	var out risuExtensions
	if strings.TrimSpace(raw) == "" {
		return out
	}
	var env struct {
		Data struct {
			Extensions map[string]json.RawMessage `json:"extensions"`
		} `json:"data"`
		Extensions map[string]json.RawMessage `json:"extensions"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		return out
	}
	apply := func(m map[string]json.RawMessage) {
		if m == nil {
			return
		}
		if r, ok := m["risuai"]; ok {
			var e risuExtensions
			if json.Unmarshal(r, &e) == nil {
				if e.BackgroundHTML != "" {
					out.BackgroundHTML = e.BackgroundHTML
				}
				if e.DefaultVariables != "" {
					out.DefaultVariables = e.DefaultVariables
				}
				if e.ViewScreen != "" {
					out.ViewScreen = e.ViewScreen
				}
				if len(bytesTrimmed(e.SDData)) > 0 {
					out.SDData = e.SDData
				}
				if len(bytesTrimmed(e.Vits)) > 0 {
					out.Vits = e.Vits
				}
				if len(bytesTrimmed(e.AdditionalText)) > 0 {
					out.AdditionalText = e.AdditionalText
				}
				out.LowLevelAccess = out.LowLevelAccess || e.LowLevelAccess
			}
		}
	}
	apply(env.Extensions)
	apply(env.Data.Extensions)
	return out
}

// hasDepthPrompt 检测 extensions 是否带非空 depth_prompt（V3 的深度提示）。
func hasDepthPrompt(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	var env struct {
		Data struct {
			Extensions map[string]json.RawMessage `json:"extensions"`
		} `json:"data"`
		Extensions map[string]json.RawMessage `json:"extensions"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		return false
	}
	check := func(m map[string]json.RawMessage) bool {
		r, ok := m["depth_prompt"]
		if !ok {
			return false
		}
		var dp struct {
			Prompt string `json:"prompt"`
		}
		if json.Unmarshal(r, &dp) != nil {
			return false
		}
		return strings.TrimSpace(dp.Prompt) != ""
	}
	return check(env.Extensions) || check(env.Data.Extensions)
}

// parseDefaultVariables 解析 defaultVariables（RisuAI 存为 JSON 对象字符串）。
// 非对象或解析失败时返回 nil；只保留字符串值。
func parseDefaultVariables(raw string) map[string]string {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "{") {
		return nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return nil
	}
	out := make(map[string]string, len(obj))
	for k, v := range obj {
		var s string
		if json.Unmarshal(v, &s) == nil && s != "" {
			out[k] = s
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// cardUIText 汇总卡内可能出现标记的文本字段，供标记族与 <img cmd> 检测。
func cardUIText(card *CharacterCard) string {
	var b strings.Builder
	write := func(s string) {
		if s != "" {
			b.WriteString(s)
			b.WriteByte('\n')
		}
	}
	write(card.Description)
	write(card.Personality)
	write(card.Scenario)
	write(card.PostHistory)
	write(card.SystemPrompt)
	for _, v := range card.OpeningVariants {
		write(v.Text)
	}
	for _, lb := range card.LorebookRefs {
		for _, e := range lb.Entries {
			write(e.Content)
		}
	}
	return b.String()
}

// detectMarkerFamilies 收集卡内出现的 [Name:…] / [Name] 标记族名（去重、排序）。
// 这些是卡作者要求模型输出的自定义标记（如 [CeresStatus:…]），用于生成别名提示。
// 只收含大写字母的名字：纯小写方括号在正文里多为普通引用（如 [lowercase]），误报率高。
func detectMarkerFamilies(text string) []string {
	set := map[string]bool{}
	consider := func(name string) {
		if hasUpper(name) {
			set[name] = true
		}
	}
	for _, m := range markerFamilyColonRe.FindAllStringSubmatch(text, -1) {
		consider(m[1])
	}
	for _, m := range markerFamilyTagRe.FindAllStringSubmatch(text, -1) {
		consider(m[1])
	}
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func bytesTrimmed(raw json.RawMessage) json.RawMessage {
	return json.RawMessage(strings.TrimSpace(string(raw)))
}

func hasUpper(s string) bool {
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			return true
		}
	}
	return false
}
