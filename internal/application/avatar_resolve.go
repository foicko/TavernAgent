package application

import (
	"net/url"
	"strings"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// CardAssetURL 返回卡内嵌资源的展示 URL（与 HTTP 路由一致）。
//
// 展示字段（头像等）用它把卡内归档引用换成前端可直接加载的地址；这里只产出
// 相对路径，由客户端拼接自己的服务地址（桌面 / 局域网 / 浏览器都适用）。
func CardAssetURL(cardID, ref string) string {
	return "/api/v1/cards/" + url.PathEscape(cardID) + "/assets/" + url.PathEscape(ref)
}

// resolveEmbeddedAvatars 把世界状态里指向卡内归档的头像（embeded://…）改写为可加载的
// 资源 URL，并返回是否有改动。
//
// charx 卡的头像来自归档内的图标资源；图标格式无法生成缩略图时（例如 AVIF），卡里会
// 保留原始的 embeded:// 引用，前端拿到它加载不出任何图片。这里按卡片的 assets
// （uri ↔ ref）映射成资源 URL，让已有会话也能显示头像，而不必重建会话。
func resolveEmbeddedAvatars(store ports.CardStore, cardID string, ws *domain.WorldState) bool {
	if ws == nil || cardID == "" || len(ws.Characters) == 0 {
		return false
	}
	need := false
	for _, c := range ws.Characters {
		if strings.HasPrefix(c.Avatar, "embeded://") {
			need = true
			break
		}
	}
	if !need {
		return false
	}
	entry, err := store.GetCard(cardID)
	if err != nil || entry == nil || strings.TrimSpace(entry.CharacterJSON) == "" {
		return false
	}
	card, err := ParseCharacterCard(entry.CharacterJSON)
	if err != nil || card == nil || len(card.Assets) == 0 {
		return false
	}
	byURI := make(map[string]string, len(card.Assets))
	for _, a := range card.Assets {
		if a.URI != "" && a.Ref != "" {
			byURI[a.URI] = CardAssetURL(cardID, a.Ref)
		}
	}
	if len(byURI) == 0 {
		return false
	}
	changed := false
	for id, c := range ws.Characters {
		if c.Avatar == "" {
			continue
		}
		if u, ok := byURI[c.Avatar]; ok {
			c.Avatar = u
			ws.Characters[id] = c
			changed = true
		}
	}
	return changed
}
