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

// buildCharxBytes 构造一个最小 charx 归档（V3 卡 + 图标 + 立绘）。
func buildCharxBytes(t *testing.T) []byte {
	t.Helper()
	card := map[string]any{
		"spec": "chara_card_v3", "spec_version": "3.0",
		"data": map[string]any{
			"name":      "charx 公主",
			"first_mes": "你好。<img cmd=\"smile\">",
			"assets": []any{
				map[string]any{"type": "icon", "uri": "embeded://assets/icon/image/1.png", "name": "icon1", "ext": "png"},
				map[string]any{"type": "x-risu-asset", "uri": "embeded://assets/other/image/2.webp", "name": "smile", "ext": "webp"},
			},
		},
	}
	cardJSON, _ := json.Marshal(card)

	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 10, G: 200, B: 90, A: 255})
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
	write("card.json", cardJSON)
	write("assets/icon/image/1.png", pngBuf.Bytes())
	write("assets/other/image/2.webp", []byte("RIFF....WEBPVP8 "))
	_ = zw.Close()
	return buf.Bytes()
}

func TestImportCard_CharxEndToEnd(t *testing.T) {
	ts, base := newTestServer(t)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "card.charx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(buildCharxBytes(t)); err != nil {
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
		CardID string `json:"cardId"`
		Format string `json:"format"`
		Source string `json:"source"`
		Name   string `json:"name"`
		Avatar string `json:"avatar"`
		Assets []struct {
			Name string `json:"name"`
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatal(err)
	}
	if res.Format != "charx" || res.Source != "charx" {
		t.Fatalf("format/source = %q/%q", res.Format, res.Source)
	}
	if res.CardID == "" || res.Name != "charx 公主" {
		t.Fatalf("cardId/name = %q/%q", res.CardID, res.Name)
	}
	if !strings.HasPrefix(res.Avatar, "data:image/") {
		t.Fatalf("avatar = %q", res.Avatar)
	}
	if len(res.Assets) != 2 {
		t.Fatalf("assets = %+v", res.Assets)
	}

	// 资源应可通过返回的 URL 读取，且 MIME 正确。
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
	if ct := assetResp.Header.Get("Content-Type"); ct != "image/webp" {
		t.Fatalf("asset content-type = %q", ct)
	}
}
