package application

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

// 原生卡也带 first_mes 等外键，不能被误判成 V1。
func TestImportCardJSON_NativeNotMistakenForV1(t *testing.T) {
	raw := `{"schemaVersion":2,"name":"原生卡","characters":[{"name":"原生卡","description":"人设","participant":true}],"first_mes":"原生开场"}`
	rep, err := ImportCardJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Format != "native" {
		t.Fatalf("format = %q，原生卡被误判", rep.Format)
	}
	if rep.Card.Name != "原生卡" {
		t.Fatalf("name = %q", rep.Card.Name)
	}
}

// V3 群聊开场不参与单角色叙事，但要在报告里说明而不是静默丢弃。
func TestImportCardPNG_V3GroupOnlyGreetingsReported(t *testing.T) {
	v3raw, err := json.Marshal(map[string]any{
		"spec": "chara_card_v3", "spec_version": "3.0",
		"data": map[string]any{
			"name": "群聊卡", "first_mes": "你好",
			"group_only_greetings": []any{"大家好"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	pngData := buildPNGMetadataOnly(t, pngTextChunk{
		Type: "tEXt", Keyword: "ccv3", Text: base64.StdEncoding.EncodeToString(v3raw),
	})
	rep, err := ImportCardPNG(pngData)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Format != "chara_card_v3" {
		t.Fatalf("format = %q", rep.Format)
	}
	found := false
	for _, ig := range rep.Ignored {
		if strings.Contains(ig, "group_only_greetings") {
			found = true
		}
	}
	if !found {
		t.Fatalf("ignored = %v", rep.Ignored)
	}
}

// V2 世界书条目要落到 LorebookRefs，且导入条数在报告里可核对。
func TestImportCardPNG_CharacterBookImported(t *testing.T) {
	payload := v2Payload(t, map[string]any{
		"name":      "带书卡",
		"first_mes": "你好",
		"character_book": map[string]any{
			"name": "设定集",
			"entries": []any{
				map[string]any{"keys": []any{"酒馆"}, "content": "酒馆在东街", "enabled": true},
				map[string]any{"keys": []any{"空"}, "content": "   "},
			},
		},
	})
	rep, err := importV2PNG(t, payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Card.LorebookRefs) != 1 {
		t.Fatalf("lorebookRefs = %d", len(rep.Card.LorebookRefs))
	}
	if got := len(rep.Card.LorebookRefs[0].Entries); got != 1 {
		t.Fatalf("空内容条目应被跳过，得到 %d 条", got)
	}
	if rep.Card.LorebookRefs[0].Entries[0].EntryID == "" {
		t.Fatal("条目应带稳定 entryId")
	}
}

// importV2PNG 是「V2 载荷 → PNG → 导入」的便捷封装。
func importV2PNG(t *testing.T, base64Payload string) (*ImportReport, error) {
	t.Helper()
	return ImportCardPNG(buildPNGMetadataOnly(t, pngTextChunk{
		Type: "tEXt", Keyword: "chara", Text: base64Payload,
	}))
}

// 原生卡原样透传以保留未知扩展，但 PNG 卡的头像来自像素、不在原始串里，
// 需要补写回顶层 avatar，否则这类卡的头像会整个丢掉。
func TestImportCardPNG_NativeCardKeepsExtensionsAndGainsAvatar(t *testing.T) {
	raw := `{"schemaVersion":2,"cardId":"native_png","name":"原生图卡","description":"人设",` +
		`"characters":[{"characterId":"npc_a","name":"原生图卡","description":"人设","participant":true}],` +
		`"extensions":{"custom":[1,2]}}`
	pngData := buildDecodablePNG(t, 320, 320, pngTextChunk{Type: "tEXt", Keyword: "chara", Text: raw})

	rep, err := ImportCardPNG(pngData)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Format != "native" {
		t.Fatalf("format = %q", rep.Format)
	}
	cardJSON := rep.CardJSON()
	if !strings.Contains(cardJSON, `"extensions"`) {
		t.Fatalf("未知扩展应保留：%.120q", cardJSON)
	}
	if !strings.Contains(cardJSON, `"avatar":"data:image/`) {
		t.Fatalf("PNG 卡的头像应补写进卡片 JSON：%.200q", cardJSON)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(cardJSON), &parsed); err != nil {
		t.Fatalf("补写后仍是合法 JSON：%v", err)
	}
	if parsed["name"] != "原生图卡" {
		t.Fatalf("name = %v", parsed["name"])
	}
}

// 无头像时原生卡必须逐字保留原始串（含缩进与未知字段）。
func TestCardJSON_NativeWithoutAvatarIsByteIdentical(t *testing.T) {
	raw := "{\n  \"schemaVersion\": 2,\n  \"name\": \"纯文本卡\",\n  \"characters\": []\n}"
	rep, err := ImportCardJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := rep.CardJSON(); got != raw {
		t.Fatalf("原始串被改写：%q", got)
	}
}

// ---- 缩略图纯函数边界 ----

func TestFitWithin_KeepsSmallImages(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 50))
	got := fitWithin(img, avatarMaxEdge)
	if got.Bounds().Dx() != 100 || got.Bounds().Dy() != 50 {
		t.Fatalf("小图不应被放大: %v", got.Bounds())
	}
}

func TestFitWithin_NeverZeroDimension(t *testing.T) {
	// 极端长条图缩到长边 640 时短边四舍五入后可能为 0，必须兜底为 1。
	img := image.NewRGBA(image.Rect(0, 0, 1, 4000))
	got := fitWithin(img, avatarMaxEdge)
	if got.Bounds().Dx() < 1 || got.Bounds().Dy() != avatarMaxEdge {
		t.Fatalf("bounds = %v", got.Bounds())
	}
}

func TestFitWithin_NilAndEmpty(t *testing.T) {
	if fitWithin(nil, avatarMaxEdge) != nil {
		t.Fatal("nil 输入应返回 nil")
	}
	if fitWithin(image.NewRGBA(image.Rect(0, 0, 0, 0)), avatarMaxEdge) != nil {
		t.Fatal("空图应返回 nil")
	}
}

func TestBuildAvatarDataURL_RejectsNonPNG(t *testing.T) {
	if s := buildAvatarDataURL([]byte("not a png")); s != "" {
		t.Fatalf("非 PNG 应返回空串，得到 %.32q", s)
	}
	if s := buildAvatarDataURL(nil); s != "" {
		t.Fatalf("空输入应返回空串，得到 %.32q", s)
	}
}

// 尺寸已在限度内的小图直接沿用原 PNG：重新编码只会让它变大。
func TestBuildAvatarDataURL_KeepsSmallOriginalBytes(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(x * 4), uint8(y * 4), 200, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	got := buildAvatarDataURL(buf.Bytes())
	if want := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()); got != want {
		t.Fatalf("小图应原样沿用，得到前缀 %.32q", got)
	}
}

// 独立的 Creator / Notes 文本块是早期工具的写法：卡片载荷没写创作者信息时，
// 这些块是唯一来源，不能再被当成「只记录存在性」扔掉。
func TestImportCardPNG_BackfillsCreatorAndNotesChunks(t *testing.T) {
	payload := v2Payload(t, map[string]any{"name": "老工具卡", "first_mes": "你好"})
	pngData := buildPNGMetadataOnly(t,
		pngTextChunk{Type: "tEXt", Keyword: "chara", Text: payload},
		pngTextChunk{Type: "tEXt", Keyword: "Creator", Text: "Pieruto"},
		pngTextChunk{Type: "tEXt", Keyword: "Notes", Text: "开场写给中文玩家"},
	)
	rep, err := ImportCardPNG(pngData)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Card.Creator != "Pieruto" {
		t.Fatalf("Creator 未回填: %q", rep.Card.Creator)
	}
	if rep.Card.CreatorNotes != "开场写给中文玩家" {
		t.Fatalf("Notes 未回填: %q", rep.Card.CreatorNotes)
	}
	if rep.Card.Characters[0].CreatorNotes != "开场写给中文玩家" {
		t.Fatal("CreatorNotes 应同步到单角色卡的角色条目")
	}
}

// 卡片自身写了创作者信息时以卡片为准。
func TestImportCardPNG_ChunkMetadataDoesNotOverrideCard(t *testing.T) {
	payload := v2Payload(t, map[string]any{"name": "卡内优先", "creator": "卡片里的作者", "creator_notes": "卡片里的留言"})
	pngData := buildPNGMetadataOnly(t,
		pngTextChunk{Type: "tEXt", Keyword: "chara", Text: payload},
		pngTextChunk{Type: "tEXt", Keyword: "Creator", Text: "文本块里的作者"},
		pngTextChunk{Type: "tEXt", Keyword: "Notes", Text: "文本块里的留言"},
	)
	rep, err := ImportCardPNG(pngData)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Card.Creator != "卡片里的作者" || rep.Card.CreatorNotes != "卡片里的留言" {
		t.Fatalf("卡片字段被文本块覆盖: creator=%q notes=%q", rep.Card.Creator, rep.Card.CreatorNotes)
	}
}

// 开场编号是展示契约的一部分（前端下拉直接显示这些标题），
// 且只由后端决定：默认开场占 a 位，备选从 b 起，空开场跳过但会占位。
func TestImportCardPNG_OpeningTitlesAreStable(t *testing.T) {
	payload := v2Payload(t, map[string]any{
		"name":                "多开场卡",
		"first_mes":           "默认开场正文",
		"alternate_greetings": []any{"备选一", "   ", "备选三"},
	})
	pngData := buildPNGMetadataOnly(t, pngTextChunk{Type: "tEXt", Keyword: "chara", Text: payload})
	rep, err := ImportCardPNG(pngData)
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	var variantIDs []string
	for _, ov := range rep.Card.OpeningVariants {
		titles = append(titles, ov.Title)
		variantIDs = append(variantIDs, ov.VariantID)
	}
	want := []string{"默认开场", "开场b", "开场d"}
	if len(titles) != len(want) {
		t.Fatalf("开场数 = %d (%v)，期望 %d", len(titles), titles, len(want))
	}
	for i := range want {
		if titles[i] != want[i] {
			t.Fatalf("开场标题 = %v，期望 %v", titles, want)
		}
	}
	// 每个变体都必须有非空且互不相同的 variantId：
	// 前端下拉用 value 回传，空值会让「选了备选却走默认开场」。
	seen := map[string]bool{}
	for i, id := range variantIDs {
		if id == "" {
			t.Fatalf("第 %d 个开场缺少 variantId", i)
		}
		if seen[id] {
			t.Fatalf("variantId 重复: %q", id)
		}
		seen[id] = true
	}
}
