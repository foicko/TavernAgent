package http

import (
	"encoding/json"
	"testing"
)

// 导入即入库：导入的卡必须出现在卡库里，且能直接用它建会话（不再回传 characterJson）。
func TestCardLibraryImportListAndStartSession(t *testing.T) {
	_, base := newTestServer(t)

	status, body := postJSON(t, base+"/api/v1/cards", testCard)
	if status != 200 {
		t.Fatalf("import %d: %s", status, body)
	}
	var preview struct {
		CardID        string `json:"cardId"`
		Name          string `json:"name"`
		CharacterJSON string `json:"characterJson"`
	}
	if err := json.Unmarshal([]byte(body), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.CardID == "" || preview.Name != "Elena" || preview.CharacterJSON == "" {
		t.Fatalf("import response missing cardId/content: %+v", preview)
	}

	status, body = getJSON(t, base+"/api/v1/cards")
	if status != 200 {
		t.Fatalf("list %d: %s", status, body)
	}
	var list struct {
		Cards []struct {
			CardID        string `json:"cardId"`
			Name          string `json:"name"`
			CharacterJSON string `json:"characterJson"`
		} `json:"cards"`
	}
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Cards) != 1 || list.Cards[0].CardID != preview.CardID {
		t.Fatalf("library must contain the imported card: %s", body)
	}
	if list.Cards[0].CharacterJSON != "" {
		t.Fatal("list must not ship the full characterJson")
	}

	status, body = getJSON(t, base+"/api/v1/cards/"+preview.CardID)
	if status != 200 {
		t.Fatalf("get card %d: %s", status, body)
	}
	if !json.Valid([]byte(body)) || body == "" {
		t.Fatalf("get card body invalid: %s", body)
	}

	// 用 cardId 建会话：请求体里没有 characterJson。
	sessionReq, _ := json.Marshal(map[string]any{
		"cardId": preview.CardID, "playerName": "旅人", "openingText": "灯塔的大门敞开着。",
	})
	status, body = postJSON(t, base+"/api/v1/sessions", string(sessionReq))
	if status != 201 {
		t.Fatalf("start by cardId %d: %s", status, body)
	}
	var created struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil || created.SessionID == "" {
		t.Fatalf("session response: %s", body)
	}

	// 删卡不影响已有会话：故事仍在，卡库清空。
	if status, body = deleteJSON(t, base+"/api/v1/cards/"+preview.CardID); status != 200 {
		t.Fatalf("delete %d: %s", status, body)
	}
	if status, _ = getJSON(t, base+"/api/v1/sessions/"+created.SessionID); status != 200 {
		t.Fatalf("session must survive card deletion: %d", status)
	}
	if status, body = getJSON(t, base+"/api/v1/cards"); status != 200 || body != "{\"cards\":[]}\n" {
		t.Fatalf("library should be empty: %d %s", status, body)
	}
}

func TestCardLibraryNotFoundAndIdempotentDelete(t *testing.T) {
	_, base := newTestServer(t)
	if status, _ := getJSON(t, base+"/api/v1/cards/card_missing"); status != 404 {
		t.Fatalf("missing card must be 404, got %d", status)
	}
	// 删不存在的卡按幂等成功处理。
	if status, body := deleteJSON(t, base+"/api/v1/cards/card_missing"); status != 200 {
		t.Fatalf("idempotent delete: %d %s", status, body)
	}
	// 用不存在的 cardId 建会话必须 404，而不是静默创建空会话。
	req, _ := json.Marshal(map[string]any{"cardId": "card_missing", "playerName": "旅人", "openingText": "。"})
	if status, body := postJSON(t, base+"/api/v1/sessions", string(req)); status != 404 {
		t.Fatalf("missing card session must be 404: %d %s", status, body)
	}
}
