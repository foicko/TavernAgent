package cardmarkers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// 契约测试：标记语法是跨语言共享的，唯一来源是 testdata/contract/markers.json。
// Go 与 TS 直接读它，Kotlin 由脚本生成，避免"改一处忘另一处"。
type markerCase struct {
	ID         string              `json:"id"`
	Text       string              `json:"text"`
	Images     []string            `json:"images"`
	HUD        []map[string]string `json:"hud"`
	Scene      *string             `json:"scene"`
	VarRemoved bool                `json:"varRemoved"`
}

func loadMarkerCases(t *testing.T) []markerCase {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "contract", "markers.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	var doc struct {
		Cases []markerCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse contract: %v", err)
	}
	if len(doc.Cases) == 0 {
		t.Fatal("contract has no cases")
	}
	return doc.Cases
}

func TestMarkerContract(t *testing.T) {
	for _, tc := range loadMarkerCases(t) {
		t.Run(tc.ID, func(t *testing.T) {
			images := []string{}
			for _, seg := range SplitImages(tc.Text) {
				if seg.Type == "image" {
					images = append(images, seg.AssetKey)
				}
			}
			wantImages := tc.Images
			if wantImages == nil {
				wantImages = []string{}
			}
			if !reflect.DeepEqual(images, wantImages) {
				t.Fatalf("images = %v, want %v", images, wantImages)
			}

			clean, hints := Extract(tc.Text)
			hud := []map[string]string{}
			scene := ""
			for _, h := range hints {
				switch h.Kind {
				case KindHUD:
					hud = append(hud, h.Values)
				case KindScene:
					scene = h.Text
				}
			}
			wantHUD := tc.HUD
			if wantHUD == nil {
				wantHUD = []map[string]string{}
			}
			if !reflect.DeepEqual(hud, wantHUD) {
				t.Fatalf("hud = %v, want %v", hud, wantHUD)
			}
			wantScene := ""
			if tc.Scene != nil {
				wantScene = *tc.Scene
			}
			if scene != wantScene {
				t.Fatalf("scene = %q, want %q", scene, wantScene)
			}
			if tc.VarRemoved && strings.Contains(clean, "[VAR:") {
				t.Fatalf("VAR marker not stripped: clean=%q", clean)
			}
		})
	}
}
