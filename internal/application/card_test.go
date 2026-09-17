package application

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

// ---- 原生卡解析（openingVariants / secrets） ----

func TestParseCharacterCardWithVariantsAndSecrets(t *testing.T) {
	raw := `{
		"name":"艾莲娜","description":"酒馆老板娘",
		"secrets":[{"title":"身世","content":"她是前任城主的养女"}],
		"openingVariants":[
			{"variantId":"v1","title":"酒馆之夜","text":"雨水敲打着窗棂……"},
			{"title":"晨光","text":"清晨的薄雾里……"}
		]
	}`
	card, err := ParseCharacterCard(raw)
	if err != nil {
		t.Fatal(err)
	}
	if card.Name != "艾莲娜" {
		t.Fatalf("name = %q", card.Name)
	}
	if len(card.Secrets) != 1 || card.Secrets[0].Content != "她是前任城主的养女" {
		t.Fatalf("secrets = %+v", card.Secrets)
	}
	if len(card.OpeningVariants) != 2 {
		t.Fatalf("variants = %d", len(card.OpeningVariants))
	}
	if card.OpeningVariants[1].VariantID == "" {
		t.Fatal("未补默认 variantId")
	}
	if card.CardID == "" {
		t.Fatal("未补默认 cardId")
	}
	if card.RawOriginal != raw {
		t.Fatal("RawOriginal 应保留原始串")
	}
}

func TestSecretLeakWarnings(t *testing.T) {
	// 泄漏：秘密内容原样出现在公开字段 → 必须告警（宽松子串匹配的确定性契约）。
	rawLeak := `{
		"name":"艾莲娜",
		"description":"她最看重父亲留下的几缕往事。",
		"secrets":[{"title":"怀表来历","content":"怀表来自她父亲"}],
		"openingVariants":[{"title":"默认","text":"怀表来自她父亲，那是最重要的东西。"}]
	}`
	card, err := ParseCharacterCard(rawLeak)
	if err != nil {
		t.Fatal(err)
	}
	if len(card.ScanSecretLeakWarnings()) == 0 {
		t.Fatal("秘密内容出现在开场中，应检测到泄漏警告")
	}

	// 未泄漏：公开字段不包含秘密文本 → 无告警。
	rawClean := `{
		"name":"艾莲娜",
		"description":"她经营着一家小酒馆。",
		"secrets":[{"title":"怀表来历","content":"怀表来自她父亲"}]
	}`
	cardClean, err := ParseCharacterCard(rawClean)
	if err != nil {
		t.Fatal(err)
	}
	if warns := cardClean.ScanSecretLeakWarnings(); len(warns) != 0 {
		t.Fatalf("不应告警: %v", warns)
	}
}

func TestCleanMarkup(t *testing.T) {
	in := `他轻声说 <b>晚安</b> <script>alert(1)</script>`
	got := cleanMarkup(in)
	if strings.Contains(got, "<") || strings.Contains(got, "script") {
		t.Fatalf("cleanMarkup 未剥离标记: %q", got)
	}
}

// ---- PNG V2 导入 ----

// pngTextChunk 是一段待写入 PNG 的文本块（tEXt/iTXt/zTXt）。
type pngTextChunk struct {
	Type    string
	Keyword string
	Text    string
}

// appendPNGChunk 追加一个带真实 CRC 的 chunk（png.Decode 会校验 CRC）。
func appendPNGChunk(dst []byte, ctype string, data []byte) []byte {
	length := make([]byte, 4)
	binary.BigEndian.PutUint32(length, uint32(len(data)))
	payload := make([]byte, 0, len(ctype)+len(data)+4)
	payload = append(payload, ctype...)
	payload = append(payload, data...)
	crc := crc32.ChecksumIEEE(payload)
	payload = binary.BigEndian.AppendUint32(payload, crc)
	return append(append(dst, length...), payload...)
}

// buildPNGMetadataOnly 构造只含元数据的最小 PNG（签名+IHDR+文本块+IEND）。
// IHDR 未描述真实像素，因此无法解码像素——用于只验证元数据解析的用例。
func buildPNGMetadataOnly(t *testing.T, texts ...pngTextChunk) []byte {
	t.Helper()
	b := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	b = appendPNGChunk(b, "IHDR", make([]byte, 13))
	for _, c := range texts {
		b = appendPNGChunk(b, c.Type, append(append([]byte(c.Keyword), 0), []byte(c.Text)...))
	}
	return appendPNGChunk(b, "IEND", nil)
}

// buildPNGWithText 兼容旧用例：单个文本块、无像素。
func buildPNGWithText(t *testing.T, chunkType, keyword, text string) []byte {
	t.Helper()
	return buildPNGMetadataOnly(t, pngTextChunk{Type: chunkType, Keyword: keyword, Text: text})
}

// buildDecodablePNG 生成一张像素可解码的真实 PNG，并在 IHDR 之后插入文本块。
// 头像缩略图链路需要真实像素才能走通。
func buildDecodablePNG(t *testing.T, w, h int, texts ...pngTextChunk) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(x % 251), uint8(y % 241), 96, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	base := buf.Bytes()
	// 插入点取 IHDR 之后：8 字节签名 + 4 长度 + 4 类型 + 13 数据 + 4 CRC。
	at := 8 + 4 + 4 + 13 + 4
	if len(base) < at {
		t.Fatal("png.Encode 输出异常")
	}
	out := append([]byte{}, base[:at]...)
	for _, c := range texts {
		out = appendPNGChunk(out, c.Type, append(append([]byte(c.Keyword), 0), []byte(c.Text)...))
	}
	return append(out, base[at:]...)
}

func v2Payload(t *testing.T, data map[string]any) string {
	t.Helper()
	spec := map[string]any{"spec": "chara_card_v2", "spec_version": "2.0", "data": data}
	raw, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func TestImportCardPNG_V2(t *testing.T) {
	payload := v2Payload(t, map[string]any{
		"name":        "艾莲娜",
		"description": "酒馆老板娘，脾气不好但心软。",
		"personality": "毒舌、护短",
		"scenario":    "雨夜酒馆内",
		"first_mes":   "你浑身湿透地推开门，艾莲娜头也不抬：「打烊了。」",
		"alternate_greetings": []any{
			"晨光洒进酒馆，她正在擦杯子。",
		},
	})
	pngData := buildPNGWithText(t, "tEXt", "chara", payload)
	rep, err := ImportCardPNG(pngData)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Format != "chara_card_v2" {
		t.Fatalf("format = %q", rep.Format)
	}
	c := rep.Card
	if c.Name != "艾莲娜" {
		t.Fatalf("name = %q", c.Name)
	}
	if len(c.Characters) != 1 {
		t.Fatalf("characters = %d", len(c.Characters))
	}
	if !strings.Contains(c.Characters[0].Description, "毒舌") {
		t.Fatalf("description 未拼接 personality: %q", c.Characters[0].Description)
	}
	if len(c.OpeningVariants) != 2 {
		t.Fatalf("开场变体数 = %d，应为 2", len(c.OpeningVariants))
	}
	if c.OpeningVariants[0].Title != "默认开场" || c.OpeningVariants[1].Title != "开场b" {
		t.Fatalf("变体标题异常: %q %q", c.OpeningVariants[0].Title, c.OpeningVariants[1].Title)
	}
	if len(rep.Ignored) == 0 {
		t.Fatal("应有未启用字段说明")
	}
}

func TestImportCardPNG_zTXtAndITXt(t *testing.T) {
	payload := v2Payload(t, map[string]any{"name": "zTXt 卡", "first_mes": "你好"})
	// zTXt：compression_method(1B=0) + zlib 压缩数据（关键字由 buildPNGWithText 前置）
	var zbody bytesBuffer
	zw := zlib.NewWriter(&zbody)
	_, _ = zw.Write([]byte(payload))
	_ = zw.Close()
	ztxt := append([]byte{0}, zbody.Bytes()...)
	pngZ := buildPNGWithText(t, "zTXt", "chara", string(ztxt))
	repZ, err := ImportCardPNG(pngZ)
	if err != nil {
		t.Fatalf("zTXt 导入失败: %v", err)
	}
	if repZ.Card.Name != "zTXt 卡" {
		t.Fatalf("zTXt name = %q", repZ.Card.Name)
	}

	// iTXt：flag(0) method(0) lang\0 translated\0 + text
	itxt := append([]byte{0, 0, 0, 0}, []byte(payload)...)
	pngI := buildPNGWithText(t, "iTXt", "chara", string(itxt))
	repI, err := ImportCardPNG(pngI)
	if err != nil {
		t.Fatalf("iTXt 导入失败: %v", err)
	}
	if repI.Card.Name != "zTXt 卡" {
		t.Fatalf("iTXt name = %q", repI.Card.Name)
	}
}

type bytesBuffer struct{ b []byte }

func (w *bytesBuffer) Write(p []byte) (int, error) { w.b = append(w.b, p...); return len(p), nil }
func (w *bytesBuffer) Bytes() []byte               { return w.b }

func TestImportCardPNG_RejectsNonPNG(t *testing.T) {
	if _, err := ImportCardPNG([]byte("not a png")); err == nil {
		t.Fatal("非 PNG 应报错")
	}
}

func TestImportCardPNG_Base64Unmarshal(t *testing.T) {
	// base64 载荷须先经 v2Payload 编码，这里复用确保路径完整。
	payload := v2Payload(t, map[string]any{"name": "base64 卡", "first_mes": "hi"})
	pngData := buildPNGWithText(t, "tEXt", "chara", payload)
	rep, err := ImportCardPNG(pngData)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Card.Name != "base64 卡" {
		t.Fatalf("name = %q", rep.Card.Name)
	}
}

func TestImportCardPNG_ScriptLikeWarning(t *testing.T) {
	payload := v2Payload(t, map[string]any{
		"name":      "危险卡",
		"first_mes": `<script>alert(1)</script>你醒了过来。`,
	})
	pngData := buildPNGWithText(t, "tEXt", "chara", payload)
	rep, err := ImportCardPNG(pngData)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range rep.Warnings {
		if strings.Contains(w, "脚本") || strings.Contains(w, "HTML") {
			found = true
		}
	}
	if !found {
		t.Fatalf("应提示脚本/HTML 风险: %v", rep.Warnings)
	}
}

// ---- 头像缩略图：卡片体积必须与原始 PNG 解耦 ----

// 整幅 PNG 内联进卡片会让建会话请求体、世界快照与浏览器本地存储同时膨胀；
// 这里约束缩略图的长边与体积上限。
func TestImportCardPNG_AvatarIsBoundedThumbnail(t *testing.T) {
	payload := v2Payload(t, map[string]any{"name": "大图卡", "first_mes": "你好"})
	pngData := buildDecodablePNG(t, 1600, 2400, pngTextChunk{Type: "tEXt", Keyword: "chara", Text: payload})

	rep, err := ImportCardPNG(pngData)
	if err != nil {
		t.Fatal(err)
	}
	const jpegPrefix = "data:image/jpeg;base64,"
	if !strings.HasPrefix(rep.Card.Avatar, jpegPrefix) {
		t.Fatalf("不透明图像应输出 JPEG 缩略图，实际 %q", clip(rep.Card.Avatar, 32))
	}
	if len(rep.Card.Avatar) > 256<<10 {
		t.Fatalf("头像 data URL 过大：%d 字节", len(rep.Card.Avatar))
	}
	bin, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(rep.Card.Avatar, jpegPrefix))
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(bin))
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Bounds().Dy(); got != avatarMaxEdge {
		t.Fatalf("缩略图长边 = %d，应为 %d", got, avatarMaxEdge)
	}
	if got := img.Bounds().Dx(); got != 427 { // 1600:2400 等比缩到高 640
		t.Fatalf("缩略图宽 = %d，应为 427", got)
	}
	if len(rep.Card.Characters) != 1 || rep.Card.Characters[0].Avatar != rep.Card.Avatar {
		t.Fatal("单角色卡的头像应同步到角色条目")
	}
}

// 透明图像必须保留 PNG：转 JPEG 会让透明区域在深色界面上变成色块。
func TestImportCardPNG_AvatarKeepsAlpha(t *testing.T) {
	payload := v2Payload(t, map[string]any{"name": "透明卡", "first_mes": "你好"})
	img := image.NewRGBA(image.Rect(0, 0, 800, 800))
	for y := 0; y < 800; y++ {
		for x := 0; x < 800; x++ {
			img.SetRGBA(x, y, color.RGBA{200, 40, 40, uint8(x % 256)})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	// 带 alpha 的图直接作为卡片 PNG（仍需文本块）。
	withText := insertTextChunk(t, buf.Bytes(), pngTextChunk{Type: "tEXt", Keyword: "chara", Text: payload})
	rep, err := ImportCardPNG(withText)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(rep.Card.Avatar, "data:image/png;base64,") {
		t.Fatalf("带透明通道的图像应保留 PNG，实际 %q", clip(rep.Card.Avatar, 32))
	}
	if rep.Card.Avatar == "" {
		t.Fatal("应生成头像")
	}
}

// insertTextChunk 在既有 PNG 的 IHDR 之后插入文本块（供自定义像素的用例复用）。
func insertTextChunk(t *testing.T, base []byte, c pngTextChunk) []byte {
	t.Helper()
	at := 8 + 4 + 4 + 13 + 4
	if len(base) < at {
		t.Fatal("PNG 过短")
	}
	out := append([]byte{}, base[:at]...)
	out = appendPNGChunk(out, c.Type, append(append([]byte(c.Keyword), 0), []byte(c.Text)...))
	return append(out, base[at:]...)
}

// ---- PNG 文本块编码 ----

// tEXt 规范写 Latin-1，但角色卡工具普遍直接写 UTF-8 明文 JSON；
// 若按 Latin-1 解释，中文会整体乱码并导致卡片无法导入。
func TestImportCardPNG_UTF8PlainTextChunk(t *testing.T) {
	raw := `{"spec":"chara_card_v2","spec_version":"2.0","data":{"name":"真琴","first_mes":"「打烊了。」","description":"中文人设"}}`
	pngData := buildPNGMetadataOnly(t, pngTextChunk{Type: "tEXt", Keyword: "chara", Text: raw})
	rep, err := ImportCardPNG(pngData)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Card.Name != "真琴" {
		t.Fatalf("name = %q（UTF-8 文本块被错误解释）", rep.Card.Name)
	}
	if !strings.Contains(rep.Card.Description, "中文人设") {
		t.Fatalf("description = %q", rep.Card.Description)
	}
}

// V3 卡（SillyTavern 1.12+）同时写 chara（降级 V2）与 ccv3（完整 V3），必须取 V3。
func TestImportCardPNG_PrefersCCV3OverChara(t *testing.T) {
	v2 := v2Payload(t, map[string]any{"name": "旧版名"})
	v3raw, err := json.Marshal(map[string]any{
		"spec": "chara_card_v3", "spec_version": "3.0",
		"data": map[string]any{
			"name":      "新版名",
			"first_mes": "V3 开场",
			"assets": []any{
				map[string]any{"type": "icon", "uri": "https://example.invalid/icon.png", "name": "icon"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	pngData := buildPNGMetadataOnly(t,
		pngTextChunk{Type: "tEXt", Keyword: "chara", Text: v2},
		pngTextChunk{Type: "tEXt", Keyword: "ccv3", Text: base64.StdEncoding.EncodeToString(v3raw)},
	)
	rep, err := ImportCardPNG(pngData)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Format != "chara_card_v3" {
		t.Fatalf("format = %q", rep.Format)
	}
	if rep.Card.Name != "新版名" {
		t.Fatalf("name = %q（应取 ccv3 载荷）", rep.Card.Name)
	}
	if rep.Card.Avatar != "https://example.invalid/icon.png" {
		t.Fatalf("V3 assets 图标未映射为头像: %q", clip(rep.Card.Avatar, 60))
	}
	if len(rep.Card.OpeningVariants) != 1 || rep.Card.OpeningVariants[0].Text != "V3 开场" {
		t.Fatalf("开场 = %+v", rep.Card.OpeningVariants)
	}
}

// 无像素可解码时不应让导入失败：其余字段照常返回，只补一条告警。
func TestImportCardPNG_UndecodablePixelsStillImports(t *testing.T) {
	payload := v2Payload(t, map[string]any{"name": "无图卡", "first_mes": "你好"})
	rep, err := ImportCardPNG(buildPNGWithText(t, "tEXt", "chara", payload))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Card.Name != "无图卡" {
		t.Fatalf("name = %q", rep.Card.Name)
	}
	if rep.Card.Avatar != "" {
		t.Fatalf("不应凭空生成头像: %q", clip(rep.Card.Avatar, 40))
	}
	found := false
	for _, w := range rep.Warnings {
		if strings.Contains(w, "头像") {
			found = true
		}
	}
	if !found {
		t.Fatalf("应提示头像缺失: %v", rep.Warnings)
	}
}

// 单个野块损坏不该让能读的卡片失败。
func TestImportCardPNG_BrokenTextChunkIsSkipped(t *testing.T) {
	payload := v2Payload(t, map[string]any{"name": "正常卡", "first_mes": "你好"})
	pngData := buildPNGMetadataOnly(t,
		pngTextChunk{Type: "zTXt", Keyword: "Software", Text: "不是 zlib 数据"},
		pngTextChunk{Type: "tEXt", Keyword: "chara", Text: payload},
	)
	rep, err := ImportCardPNG(pngData)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Card.Name != "正常卡" {
		t.Fatalf("name = %q", rep.Card.Name)
	}
	found := false
	for _, w := range rep.Warnings {
		if strings.Contains(w, "zTXt") {
			found = true
		}
	}
	if !found {
		t.Fatalf("应记录被跳过的文本块: %v", rep.Warnings)
	}
}

// ---- 宽容解码 ----

func TestImportCardJSON_TolerantBase64(t *testing.T) {
	raw := `{"spec":"chara_card_v2","spec_version":"2.0","data":{"name":"换行卡","first_mes":"你好"}}`
	cases := map[string]string{
		"标准填充":    base64.StdEncoding.EncodeToString([]byte(raw)),
		"去填充":     base64.RawStdEncoding.EncodeToString([]byte(raw)),
		"含换行与空格":  wrapBase64(base64.StdEncoding.EncodeToString([]byte(raw))),
		"URL 安全表": base64.URLEncoding.EncodeToString([]byte(raw)),
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			rep, err := ImportCardJSON(payload)
			if err != nil {
				t.Fatalf("导入失败: %v", err)
			}
			if rep.Card.Name != "换行卡" {
				t.Fatalf("name = %q", rep.Card.Name)
			}
		})
	}
}

func wrapBase64(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && i%40 == 0 {
			b.WriteString("\r\n  ")
		}
		b.WriteRune(r)
	}
	return b.String()
}

// 带 BOM 的 JSON 会被标准库判为语法错误，导入侧须先剥离。
func TestImportCardJSON_StripsBOM(t *testing.T) {
	raw := "\uFEFF" + `{"spec":"chara_card_v2","spec_version":"2.0","data":{"name":"BOM 卡","first_mes":"你好"}}`
	rep, err := ImportCardJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Card.Name != "BOM 卡" {
		t.Fatalf("name = %q", rep.Card.Name)
	}
}

// ---- V1 扁平卡 ----

func TestImportCardJSON_V1FlatCard(t *testing.T) {
	raw := `{
		"name":"莉莉","description":"面包店学徒","personality":"腼腆",
		"scenario":"小镇的清晨","first_mes":"炉火刚亮起来。","mes_example":"<START>"
	}`
	rep, err := ImportCardJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Format != "tavern_v1" {
		t.Fatalf("format = %q", rep.Format)
	}
	if rep.Card.Name != "莉莉" {
		t.Fatalf("name = %q", rep.Card.Name)
	}
	if len(rep.Card.OpeningVariants) != 1 || rep.Card.OpeningVariants[0].Text != "炉火刚亮起来。" {
		t.Fatalf("开场 = %+v", rep.Card.OpeningVariants)
	}
	if len(rep.Card.Characters) != 1 || !strings.Contains(rep.Card.Characters[0].Description, "腼腆") {
		t.Fatalf("角色描述 = %+v", rep.Card.Characters)
	}
}
