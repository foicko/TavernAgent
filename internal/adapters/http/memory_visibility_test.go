package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/application"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

func TestMemoryAPIUsesVisibleDefaultPathAndGuardsHiddenKnowledge(t *testing.T) {
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	sessions := application.NewSessionService(st)
	setup, err := sessions.Setup(context.Background(), &application.SessionSetupRequest{CharacterJSON: testCard, OpeningText: "开场"})
	if err != nil {
		t.Fatal(err)
	}
	sid, main := setup.Session.SessionID, setup.Branch
	if err := st.CreateBranch(&domain.Branch{BranchID: "sibling", SessionID: sid, Name: "sibling", HeadNodeID: setup.RootNode.NodeID}); err != nil {
		t.Fatal(err)
	}
	write := func(branchID, nodeID string, memories []*domain.MemoryRecord) {
		t.Helper()
		branch, _ := st.GetBranch(branchID)
		for _, m := range memories {
			m.SourceNodeID = nodeID
		}
		result, err := st.CommitMemoryBatch(context.Background(), &ports.MemoryBatch{BatchID: nodeID, PayloadHash: nodeID, NodeID: nodeID, SessionID: sid, BranchID: branchID,
			ExpectedHeadID: branch.HeadNodeID, ExpectedVersion: branch.Version, Memories: memories})
		if err != nil || !result.Committed {
			t.Fatalf("seed memories: %+v %v", result, err)
		}
	}
	write(main.BranchID, "main-memory", []*domain.MemoryRecord{
		{MemoryID: "public", Kind: domain.MemoryObserved, Content: "可见的旅程"},
		{MemoryID: "hidden", Kind: domain.MemoryObserved, Content: "隐藏后可恢复", Hidden: true},
		{MemoryID: "private", Kind: domain.MemoryObserved, Content: "不在场角色的私有记忆", OwnerIDs: []string{"absent"}},
		{MemoryID: "sealed", Kind: domain.MemorySecret, Content: "尚未揭示的秘密", SecretID: "locked"},
	})
	write("sibling", "sibling-memory", []*domain.MemoryRecord{{MemoryID: "side", Kind: domain.MemoryObserved, Content: "另一个分支的往事"}})
	server := mustNew(t, Deps{Sessions: sessions, Memories: application.NewMemoryService(st), Addr: "127.0.0.1:8890"})
	request := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:8890"+path, strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:1234"
		w := httptest.NewRecorder()
		server.Handler().ServeHTTP(w, r)
		return w
	}
	for _, query := range []string{"", "?branchId=" + main.BranchID} {
		w := request(http.MethodGet, "/api/v1/sessions/"+sid+"/memories"+query, "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), "可见的旅程") || !strings.Contains(w.Body.String(), "隐藏后可恢复") {
			t.Fatalf("visible/default path missing: %d %s", w.Code, w.Body.String())
		}
		for _, forbidden := range []string{"不在场角色的私有记忆", "尚未揭示的秘密", "另一个分支的往事"} {
			if strings.Contains(w.Body.String(), forbidden) {
				t.Errorf("memory API disclosed %q", forbidden)
			}
		}
	}
	for _, id := range []string{"private", "sealed", "side"} {
		w := request(http.MethodPatch, "/api/v1/sessions/"+sid+"/branches/"+main.BranchID+"/memories/"+id, `{"hidden":false}`)
		if w.Code != 404 {
			t.Errorf("invisible memory %s was editable: %d %s", id, w.Code, w.Body.String())
		}
	}
	branch, _ := st.GetBranch(main.BranchID)
	if branch.HeadNodeID != "main-memory" {
		t.Fatal("invisible memory mutation moved the branch")
	}
}
