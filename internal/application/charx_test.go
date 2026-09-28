package application

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tavernagent/internal/adapters/cardassets"
	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/ports"
)

// charxEntry 是构造测试归档用的一项。
type charxEntry struct {
	Name string
	Data []byte
}

// buildCharx 把若干条目写成临时 .charx（ZIP）并返回路径。
func buildCharx(t *testing.T, entries []charxEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "card.charx")
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

// tinyPNG 生成一张可解码的小 PNG，用于图标资源。
func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x * 30), G: 120, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func charxV3Card(t *testing.T) []byte {
	t.Helper()
	raw := map[string]any{
		"spec": "chara_card_v3", "spec_version": "3.0",
		"data": map[string]any{
			"name":        "测试公主",
			"description": "一位热爱魔法的公主。<img cmd=\"anis_smile\">",
			"first_mes":   "你好，我是公主。<img cmd=\"anis_smile\">",
			"assets": []any{
				map[string]any{"type": "icon", "uri": "embeded://assets/icon/image/1.png", "name": "icon1", "ext": "png"},
				map[string]any{"type": "x-risu-asset", "uri": "embeded://assets/other/image/2.webp", "name": "anis_smile", "ext": "webp"},
			},
			"character_book": map[string]any{
				"name": "设定集",
				"entries": []any{
					map[string]any{"keys": []any{"酒馆"}, "content": "酒馆在东街", "enabled": true},
				},
			},
		},
	}
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestImportCardCharx_Basic(t *testing.T) {
	path := buildCharx(t, []charxEntry{
		{Name: "card.json", Data: charxV3Card(t)},
		{Name: "module.risum", Data: []byte("\x00\x01risu-module")},
		{Name: "assets/icon/image/1.png", Data: tinyPNG(t)},
		{Name: "assets/other/image/2.webp", Data: []byte("RIFF....WEBPVP8 ")},
		{Name: "x_meta/1.json", Data: []byte(`{"Title":"icon"}`)},
	})

	rep, err := ImportCardCharx(path)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Format != "charx" {
		t.Fatalf("format = %q，期望 charx", rep.Format)
	}
	if rep.Source != "charx" {
		t.Fatalf("source = %q", rep.Source)
	}
	if rep.SpecVersion != "3.0" {
		t.Fatalf("specVersion = %q", rep.SpecVersion)
	}
	if rep.Card.Name != "测试公主" {
		t.Fatalf("name = %q", rep.Card.Name)
	}
	if !strings.HasPrefix(rep.Card.Avatar, "data:image/") {
		t.Fatalf("avatar 未生成缩略图: %q", rep.Card.Avatar)
	}
	if len(rep.CharxAssets) != 2 {
		t.Fatalf("charxAssets = %d，期望 2", len(rep.CharxAssets))
	}
	for _, a := range rep.CharxAssets {
		if a.Ref == "" {
			t.Fatalf("资源 %q 未回填 Ref", a.Name)
		}
	}
	if len(rep.Card.LorebookRefs) != 1 || len(rep.Card.LorebookRefs[0].Entries) != 1 {
		t.Fatalf("世界书未正确导入: %+v", rep.Card.LorebookRefs)
	}
	if !hasWarning(rep.Warnings, "module.risum") {
		t.Fatalf("缺少 module.risum 告警: %v", rep.Warnings)
	}
	if !hasWarning(rep.Warnings, "<img cmd") {
		t.Fatalf("缺少 <img cmd> 告警: %v", rep.Warnings)
	}
}

func TestImportCardCharx_MissingCardJSON(t *testing.T) {
	path := buildCharx(t, []charxEntry{
		{Name: "assets/icon/image/1.png", Data: tinyPNG(t)},
	})
	if _, err := ImportCardCharx(path); err == nil {
		t.Fatal("缺少 card.json 应报错")
	}
}

func TestImportCardCharx_NotAZip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not.charx")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportCardCharx(path); err == nil {
		t.Fatal("非 ZIP 应报错")
	}
}

// SaveReport 应把 charx 资源流式落地到卡资源存储，且文件名与 Ref 一致。
func TestCardService_PersistsCharxAssets(t *testing.T) {
	path := buildCharx(t, []charxEntry{
		{Name: "card.json", Data: charxV3Card(t)},
		{Name: "assets/icon/image/1.png", Data: tinyPNG(t)},
		{Name: "assets/other/image/2.webp", Data: []byte("RIFF....WEBPVP8 ")},
	})
	rep, err := ImportCardCharx(path)
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
	svc := NewCardService(st, assets)
	entry, err := svc.SaveReport(rep)
	if err != nil {
		t.Fatal(err)
	}

	names, err := assets.ListAssets(entry.CardID)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != len(rep.CharxAssets) {
		t.Fatalf("落地资源数 = %d，期望 %d (%v)", len(names), len(rep.CharxAssets), names)
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
}

func hasWarning(warnings []string, needle string) bool {
	for _, w := range warnings {
		if strings.Contains(w, needle) {
			return true
		}
	}
	return false
}

// 单项超限的资源跳过、总量超限即停止落地：头部声明的解压大小就是计量依据，
// 不能靠 io.LimitReader 静默截断后当作成功保存（那样会落下损坏的图片）。
func TestCardService_CharxAssetSizeCaps(t *testing.T) {
	const declared = 60 << 20 // 每项声明 60 MB：未超单项上限，但 35 项合计超过总量上限
	assetsDecl := []any{
		map[string]any{"type": "x-risu-asset", "uri": "embeded://assets/big.png", "name": "big", "ext": "png"},
	}
	for i := 0; i < 35; i++ {
		assetsDecl = append(assetsDecl, map[string]any{
			"type": "x-risu-asset", "uri": fmt.Sprintf("embeded://assets/p%02d.png", i), "name": fmt.Sprintf("p%02d", i), "ext": "png",
		})
	}
	card, err := json.Marshal(map[string]any{
		"spec": "chara_card_v3", "spec_version": "3.0",
		"data": map[string]any{"name": "大卡", "description": "d", "first_mes": "hi", "assets": assetsDecl},
	})
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "big.charx")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("card.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(card); err != nil {
		t.Fatal(err)
	}
	// CreateRaw 只写头部声明的大小，实际只放 1 字节：预检在读取前就应拦下。
	rawEntry := func(name string, size uint64) {
		hw, err := zw.CreateRaw(&zip.FileHeader{Name: name, Method: zip.Store, CompressedSize64: 1, UncompressedSize64: size})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := hw.Write([]byte{0}); err != nil {
			t.Fatal(err)
		}
	}
	rawEntry("assets/big.png", maxCardArchiveEntryBytes+1)
	for i := 0; i < 35; i++ {
		rawEntry(fmt.Sprintf("assets/p%02d.png", i), declared)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	rep, err := ImportCardCharx(path)
	if err != nil {
		t.Fatal(err)
	}
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	store, err := cardassets.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewCardService(st, store).SaveReport(rep); err != nil {
		t.Fatal(err)
	}
	if !hasWarning(rep.Warnings, "单项上限") {
		t.Fatalf("超过单项上限的资源应告警: %v", rep.Warnings)
	}
	if !hasWarning(rep.Warnings, "解压总量超过") {
		t.Fatalf("超过总量上限应告警: %v", rep.Warnings)
	}
}
