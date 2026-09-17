package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"tavernagent/internal/domain"
)

func TestLorebookSessionSnapshotSecretsPaginationAndArchive(t *testing.T) {
	st, _, sessions, _, _, _ := newTestServices(t, happyScript())
	var card map[string]any
	if err := json.Unmarshal([]byte(testCard), &card); err != nil {
		t.Fatal(err)
	}
	entries := make([]domain.LorebookEntry, 105)
	for i := range entries {
		entries[i] = domain.LorebookEntry{Title: fmt.Sprintf("航道 %03d", i), Keys: []string{"灯塔"}, Content: fmt.Sprintf("灯塔旁第 %d 幅公开海图。", i), Enabled: true}
	}
	card["lorebookRefs"] = []domain.Lorebook{{Name: "公开航海资料", Entries: entries}}
	card["secrets"] = []map[string]any{{"secretId": "hidden-lore-test", "title": "上锁档案", "content": "不可检索的藏宝坐标"}}
	raw, _ := json.Marshal(card)
	setup, err := sessions.Setup(context.Background(), &SessionSetupRequest{CharacterJSON: string(raw), Player: Player{Name: "旅人"}, OpeningText: "港口清晨。"})
	if err != nil {
		t.Fatal(err)
	}
	sid := setup.Session.SessionID
	page, err := sessions.Lorebook(sid, "", 0, 99999)
	if err != nil || len(page.Entries) != 100 || !page.HasMore || page.NextOffset != 100 {
		t.Fatalf("bounded first page: %+v %v", page, err)
	}
	last, err := sessions.Lorebook(sid, "", page.NextOffset, 100)
	if err != nil || len(last.Entries) != 5 || last.HasMore || last.Entries[0].EntryIndex != 100 {
		t.Fatalf("last page: %+v %v", last, err)
	}
	secret, err := sessions.Lorebook(sid, "藏宝坐标", 0, 40)
	if err != nil || len(secret.Entries) != 0 {
		t.Fatalf("secret content entered public search: %+v %v", secret, err)
	}
	card["lorebookRefs"] = []domain.Lorebook{{Name: "另一个版本", Entries: []domain.LorebookEntry{{Title: "新版本独有", Content: "第二个版本的独立设定", Enabled: true}}}}
	raw, _ = json.Marshal(card)
	if _, err := sessions.Setup(context.Background(), &SessionSetupRequest{CharacterJSON: string(raw), Player: Player{Name: "旅人"}, OpeningText: "港口清晨。"}); err != nil {
		t.Fatal(err)
	}
	old, err := sessions.Lorebook(sid, "新版本独有", 0, 40)
	if err != nil || len(old.Entries) != 0 {
		t.Fatalf("new template leaked into older session: %+v %v", old, err)
	}
	archive := NewArchiveService(st, "test")
	exported, err := archive.Export(context.Background(), sid, "")
	if err != nil {
		t.Fatal(err)
	}
	imported, err := archive.Import(context.Background(), exported.Data)
	if err != nil {
		t.Fatal(err)
	}
	got, err := sessions.Lorebook(imported.Session.SessionID, "灯塔", 100, 40)
	if err != nil || len(got.Entries) != 5 || got.Entries[0].BookName != "公开航海资料" {
		t.Fatalf("import did not preserve searchable snapshot: %+v %v", got, err)
	}
	for _, test := range []struct {
		sid, query string
		offset     int
		code       string
	}{
		{"missing", "", 0, "NOT_FOUND"}, {sid, "", -1, "INVALID_QUERY"}, {sid, strings.Repeat("字", 513), 0, "INVALID_QUERY"},
	} {
		_, err := sessions.Lorebook(test.sid, test.query, test.offset, 40)
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Code != test.code {
			t.Fatalf("expected %s: %v", test.code, err)
		}
	}
}

func TestV2LorebookTitleAndSelectiveKeys(t *testing.T) {
	entries := mapBookEntries([]v2BookEntry{
		{Comment: "灯塔档案", Keys: []string{"海图"}, SecondaryKeys: []string{"航线"}, Selective: true, Content: "选择性触发的资料"},
		{Name: "主词条", Keys: []string{"灯塔"}, SecondaryKeys: []string{"无效次要词"}, Content: "仅主词触发"},
	})
	if len(entries) != 2 || entries[0].Title != "灯塔档案" || len(entries[0].SecondaryKeys) != 1 || len(entries[1].SecondaryKeys) != 0 || entries[1].Title != "主词条" {
		t.Fatalf("metadata lost: %+v", entries)
	}
}
