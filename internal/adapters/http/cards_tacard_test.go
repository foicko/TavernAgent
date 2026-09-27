package http

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

// buildTacardBytes 构造一个最小 .tacard（tacard.json + 资源 + 兼容 card.json）。
// 兼容 card.json 用于验证 tacard.json 的优先级：它写了另一个名字，绝不能被采用。
func buildTacardBytes(t *testing.T) []byte {
	t.Helper()
	doc := map[string]any{
		"spec": "tacard/v1",
		"metadata": map[string]any{
			"id": "char_tacard_test", "name": "tacard 少女", "author": "测试者",
		},
		"character": map[string]any{
			"name": "塔卡德", "avatar_asset": "avatar",
			"description": "一位原生卡角色。",
			"personality": "安静，好奇。",
		},
		"openings": []any{map[string]any{"id": "open_a", "title": "初见", "text": "她抬头看向你。[IMG:smile]"}},
		"assets": []any{
			map[string]any{"id": "avatar", "file": "assets/avatar.png", "type": "avatar"},
			map[string]any{"id": "smile", "file": "assets/smile.png", "type": "emotion"},
		},
		"ui_contract": map[string]any{"theme_tokens": map[string]any{"accent": "#3366FF"}},
	}
	docJSON, _ := json.Marshal(doc)

	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 120, G: 80, B: 220, A: 255})
		}
	}
	var pngBuf bytes.Buffer
	_ = png.Encode(&pngBuf, img)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	write := func(name string, data []byte) {
		w, _ := zw.Create(name)
		_, _ = w.Write(data)
	}
	write("tacard.json", docJSON)
	write("card.json", []byte(`{"spec":"chara_card_v3","spec_version":"3.0","data":{"name":"兼容层不该出现"}}`))
	write("assets/avatar.png", pngBuf.Bytes())
	write("assets/smile.png", pngBuf.Bytes())
	_ = zw.Close()
	return buf.Bytes()
}

func TestImportCard_TacardEndToEnd(t *testing.T) {
	ts, base := newTestServer(t)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "card.tacard")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(buildTacardBytes(t)); err != nil {
		t.Fatal(err)
	}
	mw.Close()

	req, _ := http.NewRequest(http.MethodPost, base+"/api/v1/cards", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/v1/cards: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d: %s", resp.StatusCode, string(raw))
	}

	var res struct {
		CardID      string            `json:"cardId"`
		Format      string            `json:"format"`
		Source      string            `json:"source"`
		Name        string            `json:"name"`
		Avatar      string            `json:"avatar"`
		ThemeTokens map[string]string `json:"themeTokens"`
		Assets      []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatal(err)
	}
	if res.Format != "tacard" || res.Source != "tacard" {
		t.Fatalf("format/source = %q/%q", res.Format, res.Source)
	}
	if res.Name != "塔卡德" {
		t.Fatalf("name = %q（兼容层 card.json 不应生效）", res.Name)
	}
	if res.CardID == "" || !strings.HasPrefix(res.Avatar, "data:image/") {
		t.Fatalf("cardId/avatar = %q/%.24q", res.CardID, res.Avatar)
	}
	if res.ThemeTokens["accent"] != "#3366FF" {
		t.Fatalf("themeTokens = %+v", res.ThemeTokens)
	}

	var smileURL string
	for _, a := range res.Assets {
		if a.Name == "smile" {
			smileURL = a.URL
		}
	}
	if smileURL == "" {
		t.Fatalf("未找到 smile 资源 URL: %+v", res.Assets)
	}
	assetResp, err := http.Get(ts.URL + smileURL)
	if err != nil {
		t.Fatal(err)
	}
	defer assetResp.Body.Close()
	if assetResp.StatusCode != 200 {
		t.Fatalf("asset GET status = %d", assetResp.StatusCode)
	}
}
