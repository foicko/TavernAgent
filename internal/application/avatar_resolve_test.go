package application

import (
	"testing"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// fakeCardStore 只实现 GetCard：其余方法通过内嵌接口保持 nil（本测试不会调用）。
type fakeCardStore struct {
	ports.CardStore
	entry *domain.CharacterCardEntry
	err   error
}

func (f fakeCardStore) GetCard(string) (*domain.CharacterCardEntry, error) {
	return f.entry, f.err
}

// charx 卡若无法生成头像缩略图，状态里会残留 embeded:// 归档引用，前端加载不出图片。
// resolveEmbeddedAvatars 必须按卡片 assets 的 uri↔ref 把它改写成可加载的资源 URL。
func TestResolveEmbeddedAvatars_RewritesToAssetURL(t *testing.T) {
	const cardJSON = `{"schemaVersion":2,"cardId":"c1","name":"x",` +
		`"characters":[{"characterId":"npc_1","name":"x"}],` +
		`"assets":[{"name":"iconx","type":"icon","ref":"iconx.png","uri":"embeded://assets/icon/image/iconx.png"}]}`
	store := fakeCardStore{entry: &domain.CharacterCardEntry{CardID: "c1", CharacterJSON: cardJSON}}

	ws := domain.NewWorldState()
	ws.Characters["npc_1"] = domain.CharacterInfo{
		CharacterID: "npc_1", Name: "x", Avatar: "embeded://assets/icon/image/iconx.png",
	}

	if !resolveEmbeddedAvatars(store, "c1", ws) {
		t.Fatal("有 embeded:// 头像时应报告改动")
	}
	if got := ws.Characters["npc_1"].Avatar; got != "/api/v1/cards/c1/assets/iconx.png" {
		t.Fatalf("avatar = %q", got)
	}
}

// 已经是 data:/http(s) 的头像不应被改写，也不应触发卡片读取。
func TestResolveEmbeddedAvatars_LeavesResolvableAvatars(t *testing.T) {
	store := fakeCardStore{err: ports.ErrNotFound}
	ws := domain.NewWorldState()
	ws.Characters["npc_1"] = domain.CharacterInfo{
		CharacterID: "npc_1", Avatar: "data:image/png;base64,AAAA",
	}
	if resolveEmbeddedAvatars(store, "c1", ws) {
		t.Fatal("可加载的头像不应被改写")
	}
	if got := ws.Characters["npc_1"].Avatar; got != "data:image/png;base64,AAAA" {
		t.Fatalf("avatar = %q", got)
	}
}
