package application

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tavernagent/internal/adapters/cardassets"
	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// buildTacard 把若干条目写成临时 .tacard（ZIP）并返回路径。
func buildTacard(t *testing.T, entries []charxEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "card.tacard")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, e := range entries {
		w, err := zw.Create(e.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(e.Data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// tacardDocJSON 构造一份完整的 v1 清单，覆盖元信息/角色/开场/世界书/资产/UI 契约。
func tacardDocJSON(t *testing.T) []byte {
	t.Helper()
	doc := map[string]any{
		"spec": "tacard/v1",
		"metadata": map[string]any{
			"id": "char_soraka_test", "name": "索拉卡 (Soraka)", "version": "1.0.0",
			"author": "TavernAgent Community", "description": "来自天界众星的温柔治愈者。",
			"tags": []any{"英雄联盟", "治愈"}, "language": "zh-CN",
		},
		"character": map[string]any{
			"name": "索拉卡", "aliases": []any{"Soraka", "星之子"},
			"avatar_asset": "avatar",
			"description":  "拥有浅紫色长发的治愈者。",
			"personality":  "温和慈悲。", "scenario": "巨神峰避风所。",
			"system_prompt":             "扮演索拉卡。",
			"post_history_instructions": "神态变化时用 [IMG:Soraka_smile]。",
		},
		"openings": []any{
			map[string]any{"id": "open_morning", "title": "破晓的小憩", "text": "晨光里她微笑。\n\n[IMG:Soraka_smile]"},
			map[string]any{"id": "open_market", "title": "山脚集市", "text": "集市暖香。"},
		},
		"lorebook": map[string]any{
			"name": "巨神峰设定集",
			"entries": []any{
				map[string]any{
					"id": "lore_mount", "keys": []any{"巨神峰", "Targon"},
					"secondary_keys": []any{"星灵"}, "content": "巨神峰是符文之地最高的山脉。",
					"constant": false, "order": 100, "position": "before_char",
				},
				map[string]any{
					"id": "lore_celestial", "keys": []any{},
					"content":  "索拉卡放弃不朽降临凡间。",
					"constant": true, "order": 10, "position": "after_char",
				},
			},
		},
		"assets": []any{
			map[string]any{"id": "avatar", "file": "assets/avatar.png", "type": "avatar", "label": "主头像"},
			map[string]any{"id": "Soraka_smile", "file": "assets/Soraka_smile.png", "type": "emotion", "category": "happy"},
			map[string]any{"id": "missing_scene", "file": "assets/scene_missing.png", "type": "scene"},
		},
		"ui_contract": map[string]any{
			"theme_tokens":      map[string]any{"accent": "#7C3AED", "bg": "#0C0E14", "font": "system-ui"},
			"default_variables": map[string]any{"trust_level": "0", "encountered_location": "mount_targon_base"},
		},
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestImportCardTacard_Basic(t *testing.T) {
	png := tinyPNG(t)
	path := buildTacard(t, []charxEntry{
		{Name: "tacard.json", Data: tacardDocJSON(t)},
		// 兼容层 card.json 必须被忽略（不是权威）。
		{Name: "card.json", Data: []byte(`{"spec":"chara_card_v3","data":{"name":"兼容层名字"}}`)},
		{Name: "assets/avatar.png", Data: png},
		{Name: "assets/Soraka_smile.png", Data: png},
		{Name: "assets/unreferenced.png", Data: png},
	})

	if !IsTacardArchive(path) {
		t.Fatal("含 tacard.json 的归档应被识别为 tacard")
	}
	rep, err := ImportCardTacard(path)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Format != "tacard" || rep.Source != "tacard" {
		t.Fatalf("format/source = %q/%q", rep.Format, rep.Source)
	}
	if rep.SpecVersion != "tacard/v1" {
		t.Fatalf("specVersion = %q", rep.SpecVersion)
	}
	if rep.Card.CardID != "char_soraka_test" {
		t.Fatalf("cardId = %q", rep.Card.CardID)
	}
	if rep.Card.Name != "索拉卡" {
		t.Fatalf("name = %q", rep.Card.Name)
	}
	if rep.Card.CreatorNotes != "来自天界众星的温柔治愈者。" {
		t.Fatalf("creatorNotes = %q", rep.Card.CreatorNotes)
	}
	if rep.Card.Creator != "TavernAgent Community" || rep.Card.Version != "1.0.0" {
		t.Fatalf("creator/version = %q/%q", rep.Card.Creator, rep.Card.Version)
	}
	if !strings.HasPrefix(rep.Card.Avatar, "data:image/") {
		t.Fatalf("avatar 未生成缩略图: %.32q", rep.Card.Avatar)
	}
	if len(rep.Card.Characters) != 1 || rep.Card.Characters[0].Avatar != rep.Card.Avatar {
		t.Fatalf("角色级头像未与卡级对齐: %+v", rep.Card.Characters)
	}
	if len(rep.Card.Characters[0].Aliases) != 2 {
		t.Fatalf("aliases = %v", rep.Card.Characters[0].Aliases)
	}
	if len(rep.Card.OpeningVariants) != 2 || rep.Card.FirstMes == "" {
		t.Fatalf("开场未正确导入: %+v", rep.Card.OpeningVariants)
	}
	if rep.Card.OpeningVariants[0].VariantID != "open_morning" || rep.Card.OpeningVariants[0].Title != "破晓的小憩" {
		t.Fatalf("开场元信息丢失: %+v", rep.Card.OpeningVariants[0])
	}

	// 世界书：常驻与层级顺序必须保真。
	if len(rep.Card.LorebookRefs) != 1 {
		t.Fatalf("lorebookRefs = %d", len(rep.Card.LorebookRefs))
	}
	entries := rep.Card.LorebookRefs[0].Entries
	if len(entries) != 2 {
		t.Fatalf("世界书条目 = %d，期望 2", len(entries))
	}
	var celestial domain.LorebookEntry
	foundCelestial := false
	for _, e := range entries {
		if e.EntryID == "lore_celestial" {
			celestial = e
			foundCelestial = true
		}
	}
	if !foundCelestial || !celestial.Constant || celestial.Order != 10 || celestial.Position != "after_char" {
		t.Fatalf("常驻条目元信息丢失: %+v", celestial)
	}

	// 资源：三项声明，两项有文件；缺文件的应清空可服务 URI。
	if len(rep.CharxAssets) != 2 {
		t.Fatalf("待落地资源 = %d，期望 2 (%+v)", len(rep.CharxAssets), rep.CharxAssets)
	}
	if !hasWarning(rep.Warnings, "缺少对应文件") {
		t.Fatalf("缺少资源文件应告警: %v", rep.Warnings)
	}
	if !hasWarning(rep.Warnings, "未被清单引用") {
		t.Fatalf("未引用资源应告警: %v", rep.Warnings)
	}
	var avatarAsset *CardAsset
	for i := range rep.Card.Assets {
		if rep.Card.Assets[i].Name == "avatar" {
			avatarAsset = &rep.Card.Assets[i]
		}
	}
	if avatarAsset == nil || avatarAsset.Ref == "" {
		t.Fatalf("avatar 资源未回填 Ref: %+v", avatarAsset)
	}

	// UI 契约：主题 token 与变量初值。
	if rep.Card.UIProfile == nil {
		t.Fatal("缺少 UIProfile")
	}
	if rep.Card.UIProfile.ThemeTokens["accent"] != "#7C3AED" {
		t.Fatalf("themeTokens = %+v", rep.Card.UIProfile.ThemeTokens)
	}
	if rep.Card.UIProfile.VarDefaults["trust_level"] != "0" {
		t.Fatalf("varDefaults = %+v", rep.Card.UIProfile.VarDefaults)
	}
	if rep.Card.UIProfile.Capabilities.Source != "tacard" {
		t.Fatalf("capabilities.source = %q", rep.Card.UIProfile.Capabilities.Source)
	}
}

// avatar_asset 指向一个 emotion 资源（无 type=avatar）时，仍应按 id 精确生成头像。
func TestImportCardTacard_AvatarByAssetID(t *testing.T) {
	doc := map[string]any{
		"spec":     "tacard/v1",
		"metadata": map[string]any{"id": "char_avatar_test", "name": "头像测试"},
		"character": map[string]any{
			"name": "头像测试", "avatar_asset": "Soraka_smile", "description": "人设",
		},
		"assets": []any{
			map[string]any{"id": "Soraka_smile", "file": "assets/Soraka_smile.png", "type": "emotion"},
		},
	}
	b, _ := json.Marshal(doc)
	path := buildTacard(t, []charxEntry{
		{Name: "tacard.json", Data: b},
		{Name: "assets/Soraka_smile.png", Data: tinyPNG(t)},
	})
	rep, err := ImportCardTacard(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(rep.Card.Avatar, "data:image/") {
		t.Fatalf("按 avatar_asset id 未生成头像: %.32q", rep.Card.Avatar)
	}
}

func TestImportCardTacard_RejectsForbiddenScript(t *testing.T) {
	path := buildTacard(t, []charxEntry{
		{Name: "tacard.json", Data: tacardDocJSON(t)},
		{Name: "assets/avatar.png", Data: tinyPNG(t)},
		{Name: "module.risum", Data: []byte("\x00risu")},
		{Name: "scripts/hack.lua", Data: []byte("os.execute('rm -rf /')")},
	})
	_, err := ImportCardTacard(path)
	if err == nil {
		t.Fatal("含脚本文件应拒绝导入")
	}
	if !strings.Contains(err.Error(), "零脚本") || !strings.Contains(err.Error(), "hack.lua") {
		t.Fatalf("错误信息应说明零脚本策略与违禁文件: %v", err)
	}
}

func TestImportCardTacard_MissingManifest(t *testing.T) {
	path := buildTacard(t, []charxEntry{
		{Name: "assets/avatar.png", Data: tinyPNG(t)},
	})
	if IsTacardArchive(path) {
		t.Fatal("缺少 tacard.json 不应识别为 tacard")
	}
	if _, err := ImportCardTacard(path); err == nil {
		t.Fatal("缺少 tacard.json 应报错")
	}
}

func TestImportCardTacard_SpecMismatch(t *testing.T) {
	doc := map[string]any{"spec": "chara_card_v3", "character": map[string]any{"name": "x"}}
	b, _ := json.Marshal(doc)
	path := buildTacard(t, []charxEntry{{Name: "tacard.json", Data: b}})
	if _, err := ImportCardTacard(path); err == nil {
		t.Fatal("非 tacard 规范应报错")
	}
}

// IsTacardArchive 不能把普通 charx（只有 card.json）误判为 tacard。
func TestIsTacardArchive_IgnoresCharx(t *testing.T) {
	path := buildCharx(t, []charxEntry{
		{Name: "card.json", Data: charxV3Card(t)},
		{Name: "assets/icon/image/1.png", Data: tinyPNG(t)},
	})
	if IsTacardArchive(path) {
		t.Fatal("charx 不应被识别为 tacard")
	}
}

func TestNormalizeTacardAssetPath(t *testing.T) {
	cases := map[string]string{
		"assets/avatar.webp": "assets/avatar.webp",
		"./assets/a.png":     "assets/a.png",
		"\\assets\\a.png":    "assets/a.png",
		"avatar.webp":        "assets/avatar.webp",
		"assets/../evil.png": "",
		"":                   "",
	}
	for in, want := range cases {
		if got := normalizeTacardAssetPath(in); got != want {
			t.Fatalf("normalizeTacardAssetPath(%q) = %q，期望 %q", in, got, want)
		}
	}
}

// SaveReport 应把 tacard 资源流式落地到卡资源存储，且可通过卡资源接口读取。
func TestCardService_PersistsTacardAssets(t *testing.T) {
	png := tinyPNG(t)
	path := buildTacard(t, []charxEntry{
		{Name: "tacard.json", Data: tacardDocJSON(t)},
		{Name: "assets/avatar.png", Data: png},
		{Name: "assets/Soraka_smile.png", Data: png},
	})
	rep, err := ImportCardTacard(path)
	if err != nil {
		t.Fatal(err)
	}

	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	assets, err := cardassets.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	entry, err := NewCardService(st, assets).SaveReport(rep)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Format != "tacard" {
		t.Fatalf("entry.format = %q", entry.Format)
	}
	for _, a := range rep.CharxAssets {
		rc, mime, err := assets.OpenAsset(entry.CardID, a.Ref)
		if err != nil {
			t.Fatalf("打开资源 %q 失败: %v", a.Ref, err)
		}
		rc.Close()
		if mime == "" {
			t.Fatalf("资源 %q 缺少 MIME", a.Ref)
		}
	}
	// 归一化 JSON 重新解析（卡库取卡预览的路径）后仍应保留 UI 契约。
	reparsed, err := ImportCardJSON(entry.CharacterJSON)
	if err != nil {
		t.Fatalf("卡库 JSON 无法重新解析: %v", err)
	}
	if reparsed.Card.UIProfile == nil || reparsed.Card.UIProfile.ThemeTokens["accent"] != "#7C3AED" {
		t.Fatalf("重新解析后 UI 契约丢失: %+v", reparsed.Card.UIProfile)
	}
	if reparsed.Card.UIProfile.Capabilities.Source != "tacard" {
		t.Fatalf("重新解析后能力来源漂移: %q", reparsed.Card.UIProfile.Capabilities.Source)
	}
}
