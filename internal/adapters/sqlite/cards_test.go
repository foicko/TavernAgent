package sqlite

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// 卡库是用户资产：可覆盖、可删除，且删除只影响卡库本身。
func TestCardLibraryLifecycle(t *testing.T) {
	s := newTestStore(t)
	created := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	card := &domain.CharacterCardEntry{
		CardID: "card_a", Name: "Elena", ShortName: "Elena", Format: "native",
		CharacterJSON: `{"cardId":"card_a","name":"Elena"}`, ContentHash: "h1",
		CreatedAt: created, UpdatedAt: created,
	}
	if err := s.SaveCard(card); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := s.GetCard("card_a")
	if err != nil || got.Name != "Elena" || got.CharacterJSON == "" {
		t.Fatalf("get: %+v %v", got, err)
	}
	if !got.CreatedAt.Equal(created) {
		t.Fatalf("created_at round-trip: got %v want %v", got.CreatedAt, created)
	}
	if got.LastUsedAt != nil {
		t.Fatalf("fresh card must have no lastUsedAt: %v", got.LastUsedAt)
	}

	// 列表刻意不下发兆级 characterJson。
	list, err := s.ListCards()
	if err != nil || len(list) != 1 || list[0].CharacterJSON != "" {
		t.Fatalf("list: %+v %v", list, err)
	}

	// 同一张卡重复导入是覆盖：不新增行，保留创建时间，更新内容。
	card2 := &domain.CharacterCardEntry{
		CardID: "card_a", Name: "Elena v2", CharacterJSON: `{"name":"Elena v2"}`,
		ContentHash: "h2", UpdatedAt: time.Now(),
	}
	if err := s.SaveCard(card2); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	list, _ = s.ListCards()
	if len(list) != 1 || list[0].Name != "Elena v2" {
		t.Fatalf("upsert must not duplicate: %+v", list)
	}
	got, _ = s.GetCard("card_a")
	if !got.CreatedAt.Equal(created) {
		t.Fatalf("upsert must preserve created_at: got %v want %v", got.CreatedAt, created)
	}

	used := time.Now().UTC()
	if err := s.TouchCard("card_a", used); err != nil {
		t.Fatalf("touch: %v", err)
	}
	got, _ = s.GetCard("card_a")
	if got.LastUsedAt == nil || !got.LastUsedAt.Equal(used) {
		t.Fatalf("lastUsedAt: %+v", got.LastUsedAt)
	}

	if err := s.DeleteCard("card_a"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetCard("card_a"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("deleted card still readable: %v", err)
	}
	if err := s.DeleteCard("card_a"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("second delete must report not found: %v", err)
	}
	if err := s.TouchCard("card_a", used); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("touch missing card: %v", err)
	}
}

// 卡库是用户资产：满库时显式拒绝新卡，但已存在的卡仍可更新。
func TestCardLibraryRejectsOverCapacity(t *testing.T) {
	s := newTestStore(t)
	mk := func(i int) *domain.CharacterCardEntry {
		id := "card_" + strconv.Itoa(i)
		return &domain.CharacterCardEntry{CardID: id, Name: id, CharacterJSON: `{}`, ContentHash: id}
	}
	for i := 0; i < maxCards; i++ {
		if err := s.SaveCard(mk(i)); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}
	if err := s.SaveCard(mk(maxCards)); !errors.Is(err, ports.ErrCardLibraryFull) {
		t.Fatalf("over capacity must be rejected, got %v", err)
	}
	// 已存在的卡是 upsert，不受上限影响。
	update := mk(0)
	update.Name = "renamed"
	if err := s.SaveCard(update); err != nil {
		t.Fatalf("update existing card at capacity: %v", err)
	}
	if got, _ := s.GetCard("card_0"); got.Name != "renamed" {
		t.Fatalf("update dropped: %+v", got)
	}
}

// 从卡库删卡绝不触碰会话模板：打开旧故事的会话仍能取回它的角色模板。
func TestDeletingCardDoesNotTouchSessionTemplates(t *testing.T) {
	s := newTestStore(t)
	const cardJSON = `{"schemaVersion":2,"cardId":"card_keep","name":"守护者","characters":[{"characterId":"npc","name":"守护者"}]}`
	_, _, _ = seedSession(t, s)
	tpl := &domain.TemplateVersion{
		TemplateVersionID: "tpl_keep", Kind: domain.TemplateCharacter, SchemaVersion: 1,
		Content: cardJSON, ContentHash: "hash_keep",
	}
	if _, err := s.db.Exec(`INSERT INTO template_versions(template_version_id, kind, schema_version, content, content_hash, created_at) VALUES(?,?,?,?,?,?)`,
		tpl.TemplateVersionID, string(tpl.Kind), tpl.SchemaVersion, tpl.Content, tpl.ContentHash, s.now()); err != nil {
		t.Fatalf("seed template: %v", err)
	}
	if err := s.SaveCard(&domain.CharacterCardEntry{CardID: "card_keep", Name: "守护者", CharacterJSON: cardJSON}); err != nil {
		t.Fatalf("save card: %v", err)
	}
	if err := s.DeleteCard("card_keep"); err != nil {
		t.Fatalf("delete card: %v", err)
	}
	kept, err := s.GetTemplateVersion("tpl_keep")
	if err != nil || kept.Content != cardJSON {
		t.Fatalf("session template must survive card deletion: %+v %v", kept, err)
	}
}
