package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"tavernagent/internal/adapters/providers/mock"
	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/application"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"testing"
)

func TestDirectorHTTPContractsAndAuth(t *testing.T) {
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	sessions := application.NewSessionService(st)
	setup, err := sessions.Setup(context.Background(), &application.SessionSetupRequest{Title: "导演接口", CharacterJSON: testCard, OpeningText: "你推开酒馆的门。", Player: application.Player{Name: "旅人"}})
	if err != nil {
		t.Fatal(err)
	}
	bus := application.NewEventBus(st)
	director := application.NewDirectorService(st, nil, nil, bus, mock.NewDemo())
	defer director.Close()
	server := mustNew(t, Deps{Sessions: sessions, Director: director, Bus: bus, Addr: "127.0.0.1:8890", AuthPIN: "123456", AuthToken: "paired"})
	base := "/api/v1/sessions/" + setup.Session.SessionID + "/branches/" + setup.Branch.BranchID + "/director"
	call := func(method, path string, body any, remote bool) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(method, "http://127.0.0.1:8890"+path, bytes.NewReader(raw))
		req.RemoteAddr = "127.0.0.1:1111"
		if remote {
			req.RemoteAddr = "192.168.1.10:1111"
		}
		req.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		server.Handler().ServeHTTP(out, req)
		return out
	}
	if res := call("GET", base, nil, true); res.Code != 401 {
		t.Fatalf("unpaired director read: %d", res.Code)
	}
	if res := call("GET", base, nil, false); res.Code != 200 {
		t.Fatalf("director read: %d %s", res.Code, res.Body)
	}
	plan := domain.DirectorPlan{Title: "初见", Beats: []domain.DirectorBeat{{BeatID: "first", Title: "相识", Instruction: "交换称呼", CompletionCriteria: "说出姓名"}}}
	res := call("PUT", base+"/draft", application.SaveDirectorDraftRequest{ExpectedCharacterID: "card_elena", Plan: plan}, false)
	if res.Code != 200 {
		t.Fatalf("save: %d %s", res.Code, res.Body)
	}
	var draft domain.DirectorDraft
	json.Unmarshal(res.Body.Bytes(), &draft)
	res = call("POST", base+"/commands", application.DirectorCommandRequest{ExpectedCharacterID: "card_elena", ExpectedHeadID: setup.RootNode.NodeID, ExpectedVersion: 0, IdempotencyKey: "activate", Action: "activate", DraftVersion: draft.Version}, false)
	if res.Code != 200 {
		t.Fatalf("activate: %d %s", res.Code, res.Body)
	}
	var result application.DirectorCommandResult
	json.Unmarshal(res.Body.Bytes(), &result)
	res = call("GET", base+"?viewNodeId="+setup.RootNode.NodeID, nil, false)
	var historical application.DirectorView
	json.Unmarshal(res.Body.Bytes(), &historical)
	if historical.State != nil || !historical.ReadOnly || historical.Draft != nil {
		t.Fatal("historical endpoint leaked current state")
	}
	res = call("PUT", base+"/draft", map[string]any{"expectedCharacterId": "other", "plan": plan}, false)
	if res.Code != 400 {
		t.Fatal("wrong character was accepted")
	}
	res = call("POST", base+"/commands", map[string]any{"action": "pause", "unexpected": true}, false)
	if res.Code != 400 {
		t.Fatal("unknown request field was accepted")
	}
	res = call("GET", "/api/v1/director-requests/missing", nil, false)
	if res.Code != 404 {
		t.Fatalf("missing request: %d %s", res.Code, res.Body)
	}
	res = call("GET", base, nil, false)
	var current application.DirectorView
	json.Unmarshal(res.Body.Bytes(), &current)
	res = call("POST", base+"/messages", application.DirectorMessageRequest{ExpectedCharacterID: "card_elena", ExpectedHeadID: result.NodeID, ExpectedDraftVersion: current.Draft.Version, IdempotencyKey: "discussion", Text: "讨论接下来的转折"}, false)
	if res.Code != 202 {
		t.Fatalf("discuss: %d %s", res.Code, res.Body)
	}
	var accepted struct {
		RequestID string `json:"requestId"`
		Status    string `json:"status"`
		EventsURL string `json:"eventsUrl"`
	}
	json.Unmarshal(res.Body.Bytes(), &accepted)
	if accepted.RequestID == "" || accepted.EventsURL == "" || accepted.Status == "" {
		t.Fatal("missing recoverable request URLs")
	}
	res = call("GET", "/api/v1/director-requests/"+accepted.RequestID, nil, false)
	if res.Code != 200 {
		t.Fatal("request status unavailable")
	}
	res = call("POST", "/api/v1/director-requests/"+accepted.RequestID+"/cancel", map[string]string{"expectedCharacterId": "card_elena"}, false)
	if res.Code != 200 {
		t.Fatalf("cancel: %d %s", res.Code, res.Body)
	}
}
