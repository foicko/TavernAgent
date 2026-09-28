package http

import (
	"testing"

	"tavernagent/internal/application"
	"tavernagent/internal/domain"
)

// 卡资源清单不得把归档内路径（embeded://…）当图片 URL 回传：
// 归档条目缺失时 ref 为空、URI 仍是归档内路径，客户端拿到它只会显示破图。
func TestCardAssetsBodySkipsUnresolvableEmbeddedURI(t *testing.T) {
	rep := &application.ImportReport{Card: &application.CharacterCard{Assets: []application.CardAsset{
		{Name: "smile", Type: "emotion", Ref: "smile.webp", URI: "embeded://assets/icon/smile.png"},
		{Name: "gone", Type: "emotion", URI: "embeded://assets/icon/gone.png"},
		{Name: "ext", Type: "icon", URI: "https://example.com/i.png"},
	}}}
	out := cardAssetsBody(rep, &domain.CharacterCardEntry{CardID: "card_1"})

	got := map[string]string{}
	for _, a := range out {
		got[a["name"]] = a["url"]
	}
	if got["smile"] != "/api/v1/cards/card_1/assets/smile.webp" {
		t.Errorf("落地资源应给出 assets 路由，得到 %q", got["smile"])
	}
	if got["gone"] != "" {
		t.Errorf("归档内路径不可当图片 URL，得到 %q", got["gone"])
	}
	if got["ext"] != "https://example.com/i.png" {
		t.Errorf("外部地址应原样保留，得到 %q", got["ext"])
	}
}
