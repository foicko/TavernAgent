package application

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"

	"tavernagent/internal/adapters/cardassets"
	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/ports"
)

// v3PayloadB64 构造一份内嵌 V3 卡片载荷（base64）。
func v3PayloadB64(t *testing.T, assets []map[string]any) string {
	t.Helper()
	spec := map[string]any{
		"spec":         "chara_card_v3",
		"spec_version": "3.0",
		"data": map[string]any{
			"name":        "Ext Asset Card",
			"description": "desc",
			"first_mes":   `你好 <img src="dressed_happy">`,
			"assets":      assets,
		},
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// 内嵌在 PNG tEXt 块里的伪 WebP 字节。
func fakeWebP() []byte {
	b := []byte("RIFF\x00\x00\x00\x00WEBPVP8 ")
	return append(b, bytes.Repeat([]byte{0x5A}, 64)...)
}

// PNG 的 chara-ext-asset_:N 扩展块应被提取为待落地资源，并回填 assets[].Ref；
// 缺少数据块的声明应被清空 URI（前端退回纯文本，而不是渲染断裂图片）。
func TestImportCardPNG_ExtAssets(t *testing.T) {
	webp := fakeWebP()
	png := buildPNGMetadataOnly(t,
		pngTextChunk{Type: "tEXt", Keyword: "ccv3", Text: v3PayloadB64(t, []map[string]any{
			{"type": "x-risu-asset", "name": "dressed_happy1.png", "uri": "__asset:0"},
			{"type": "x-risu-asset", "name": "missing.png", "uri": "__asset:9"},
		})},
		pngTextChunk{Type: "tEXt", Keyword: "chara-ext-asset_:0", Text: base64.StdEncoding.EncodeToString(webp)},
	)
	rep, err := ImportCardPNG(png)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.EmbeddedAssets) != 1 {
		t.Fatalf("EmbeddedAssets = %d，期望 1 (%+v)", len(rep.EmbeddedAssets), rep.EmbeddedAssets)
	}
	ea := rep.EmbeddedAssets[0]
	if ea.Name != "dressed_happy1.png" || ea.Ref == "" || ea.Ext != "png" {
		t.Errorf("EmbeddedAsset = %+v", ea)
	}
	if !bytes.Equal(ea.Data, webp) {
		t.Errorf("资源字节不一致：len=%d 期望 %d", len(ea.Data), len(webp))
	}

	byName := map[string]CardAsset{}
	for _, a := range rep.Card.Assets {
		byName[a.Name] = a
	}
	if byName["dressed_happy1.png"].Ref == "" {
		t.Error("已提取资源的 Ref 未回填")
	}
	if byName["missing.png"].URI != "" {
		t.Errorf("缺失数据块的资源 URI 应清空，实际 %q", byName["missing.png"].URI)
	}
	if !hasWarning(rep.Warnings, "缺少对应数据块") {
		t.Errorf("缺少缺失资源告警: %v", rep.Warnings)
	}
}

// SaveReport 应把内嵌资源写入卡资源存储，且文件名与 Ref 一致、MIME 可识别。
func TestCardService_PersistsEmbeddedAssets(t *testing.T) {
	png := buildPNGMetadataOnly(t,
		pngTextChunk{Type: "tEXt", Keyword: "ccv3", Text: v3PayloadB64(t, []map[string]any{
			{"type": "x-risu-asset", "name": "dressed_happy1.png", "uri": "__asset:0"},
		})},
		pngTextChunk{Type: "tEXt", Keyword: "chara-ext-asset_:0", Text: base64.StdEncoding.EncodeToString(fakeWebP())},
	)
	rep, err := ImportCardPNG(png)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.EmbeddedAssets) != 1 {
		t.Fatalf("EmbeddedAssets = %d", len(rep.EmbeddedAssets))
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

	names, err := assets.ListAssets(entry.CardID)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != rep.EmbeddedAssets[0].Ref {
		t.Fatalf("落地资源 = %v，期望 [%s]", names, rep.EmbeddedAssets[0].Ref)
	}
	rc, mime, err := assets.OpenAsset(entry.CardID, rep.EmbeddedAssets[0].Ref)
	if err != nil {
		t.Fatalf("打开资源失败: %v", err)
	}
	defer rc.Close()
	if mime != "image/webp" {
		t.Errorf("MIME = %q，期望 image/webp（按内容嗅探）", mime)
	}
}

// __asset:N 形式的解析：只接受非负整数序号。
func TestPNGAssetIndex(t *testing.T) {
	cases := map[string]struct {
		idx int
		ok  bool
	}{
		"__asset:0":   {0, true},
		"__asset:12":  {12, true},
		" __asset:3":  {3, true},
		"__asset:":    {0, false},
		"__asset:x":   {0, false},
		"embeded://a": {0, false},
		"":            {0, false},
	}
	for in, want := range cases {
		got, ok := pngAssetIndex(in)
		if ok != want.ok || (ok && got != want.idx) {
			t.Errorf("pngAssetIndex(%q) = (%d,%v)，期望 (%d,%v)", in, got, ok, want.idx, want.ok)
		}
	}
}
