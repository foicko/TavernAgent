package http

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tavernagent/internal/adapters/providers/mock"
	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/application"
	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/ports"
)

const testCard = `{
  "schemaVersion": 1,
  "cardId": "card_elena",
  "name": "Elena",
  "characters": [
    {"characterId": "npc_elena", "name": "Elena", "participant": true}
  ]
}`

// postJSON 发送 JSON POST 并返回状态码与响应体。
func postJSON(t *testing.T, url, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// memoryScript 返回带 memory_add 提议的脚本（服务层已有同类脚本，此处独立一份）。
func memoryScript() []mock.Item {
	return []mock.Item{
		mock.Frame(`{"v":1,"seq":1,"type":"block","kind":"narration","speakerId":null,"text":"她低声说起了家乡。"}`),
		mock.Frame(`{"v":1,"seq":2,"type":"final","proposals":[{"proposalId":"p1","type":"memory_add","text":"她来自北方，很怕冷。","confidence":"reported","entityIds":["npc_elena"]}],"options":[]}`),
	}
}

func happyScript() []mock.Item {
	items := []mock.Item{
		mock.Frame(`{"v":1,"seq":1,"type":"block","kind":"dialogue","speakerId":"npc_elena","text":"你好。"}`),
		mock.Frame(`{"v":1,"seq":2,"type":"final","proposals":[{"proposalId":"p1","type":"relationship_delta","characterId":"npc_elena","field":"trust","delta":3}],"options":[{"optionId":"o1","intent":"clever","text":"你好，老板娘。"}]}`),
	}
	return items
}

func TestNativeCardFileImportCanCreateSession(t *testing.T) {
	_, base := newTestServer(t)
	var native map[string]any
	if err := json.Unmarshal([]byte(testCard), &native); err != nil {
		t.Fatal(err)
	}
	native["openingVariants"] = []map[string]string{{"variantId": "gate", "title": "门廊", "text": "灯塔的大门敞开着。"}}
	native["extensions"] = map[string]any{"custom": "preserve me"}
	native["lorebookRefs"] = []map[string]any{{"name": "航海资料", "entries": []map[string]any{{"title": "灯塔", "keys": []string{"灯塔"}, "content": "黄昏亮灯。", "enabled": true}}}}
	raw, _ := json.Marshal(native)
	status, body := postJSON(t, base+"/api/v1/cards/import", string(raw))
	if status != 200 {
		t.Fatalf("import %d: %s", status, body)
	}
	var preview struct {
		Name          string              `json:"name"`
		Format        string              `json:"format"`
		CharacterJSON string              `json:"characterJson"`
		Openings      []map[string]string `json:"openings"`
	}
	if err := json.Unmarshal([]byte(body), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Name != "Elena" || preview.Format != "native" || len(preview.Openings) != 1 || preview.CharacterJSON != string(raw) {
		t.Fatalf("native data was discarded: %+v", preview)
	}
	status, body = postJSON(t, base+"/api/v1/sessions", `{"playerName":"旅人","characterJson":`+str(preview.CharacterJSON)+`}`)
	if status != 201 {
		t.Fatalf("imported card cannot start: %d %s", status, body)
	}
}

func TestLorebookHTTPIsBoundedAndValidatesQuery(t *testing.T) {
	_, base := newTestServer(t)
	status, body := postJSON(t, base+"/api/v1/sessions", `{"playerName":"旅人","openingText":"清晨。","characterJson":`+str(testCard)+`}`)
	if status != 201 {
		t.Fatalf("create status %d: %s", status, body)
	}
	var created struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		query  string
		status int
	}{{"", 200}, {"?limit=99999", 200}, {"?offset=-1", 400}, {"?limit=no", 400}, {"?offset=100001", 400}} {
		resp, err := http.Get(base + "/api/v1/sessions/" + created.SessionID + "/lorebook" + check.query)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != check.status {
			t.Fatalf("%s status=%d body=%s", check.query, resp.StatusCode, raw)
		}
		if check.status == 200 && !strings.Contains(string(raw), `"entries":[]`) {
			t.Fatalf("empty list must be an array: %s", raw)
		}
	}
}

func TestSessionCreateAndTurnSSE(t *testing.T) {
	_, base := newTestServer(t)

	// 创建会话。
	body := `{"title":"酒馆相遇","playerName":"旅人","openingText":"你推开门。","characterJson":` + str(testCard) + `}`
	req, _ := http.NewRequest("POST", base+"/api/v1/sessions", strings.NewReader(body))
	req.Header.Set("Origin", base)
	resp := post(t, req)
	if resp.status != 201 {
		t.Fatalf("create status = %d body=%s", resp.status, resp.body)
	}
	var created struct {
		SessionID string `json:"sessionId"`
		BranchID  string `json:"branchId"`
		RootNode  string `json:"rootNodeId"`
	}
	json.Unmarshal([]byte(resp.body), &created)

	// 受理回合。
	turnReq := `{"idempotencyKey":"k1","expectedHeadId":"` + created.RootNode + `","expectedVersion":0,"input":{"kind":"text","text":"你好啊"}}`
	req2, _ := http.NewRequest("POST", base+"/api/v1/sessions/"+created.SessionID+"/branches/"+created.BranchID+"/turns", strings.NewReader(turnReq))
	req2.Header.Set("Origin", base)
	resp2 := post(t, req2)
	if resp2.status != 202 {
		t.Fatalf("accept status = %d body=%s", resp2.status, resp2.body)
	}
	var accepted struct {
		TurnID string `json:"turnId"`
	}
	json.Unmarshal([]byte(resp2.body), &accepted)

	// 断开重连：已提交后从持久化事件重放（block.appended 与 turn.committed 都在 outbox）。
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req3, _ := http.NewRequestWithContext(ctx, "GET", base+"/api/v1/turns/"+accepted.TurnID+"/events", nil)
	client := &http.Client{}
	sseResp, err := client.Do(req3)
	if err != nil {
		t.Fatalf("sse: %v", err)
	}
	defer sseResp.Body.Close()
	sc := bufio.NewScanner(sseResp.Body)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	var sawBlock, sawCommitted bool
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "event: block.appended") {
			sawBlock = true
		}
		if strings.HasPrefix(line, "event: turn.committed") {
			sawCommitted = true
			break
		}
	}
	if !sawBlock || !sawCommitted {
		t.Fatalf("sse missing events: block=%v committed=%v", sawBlock, sawCommitted)
	}

	// 查询回合终态：已提交且有结果节点。
	req4, _ := http.NewRequest("GET", base+"/api/v1/turns/"+accepted.TurnID, nil)
	resp4 := post(t, req4)
	var turnView map[string]any
	json.Unmarshal([]byte(resp4.body), &turnView)
	if turnView["status"] != "committed" {
		t.Fatalf("turn status = %v", turnView["status"])
	}
	if turnView["resultNode"] == nil {
		t.Fatalf("resultNode missing: %s", resp4.body)
	}
}

// T28：跨来源写请求被拒绝。
func TestCrossOriginWriteRejected(t *testing.T) {
	_, base := newTestServer(t)
	body := `{"title":"x","characterJson":` + str(testCard) + `}`
	req, _ := http.NewRequest("POST", base+"/api/v1/sessions", strings.NewReader(body))
	req.Header.Set("Origin", "http://evil.example")
	resp := post(t, req)
	if resp.status != 403 {
		t.Fatalf("status = %d, want 403", resp.status)
	}
	if !strings.Contains(resp.body, "FORBIDDEN") {
		t.Fatalf("body = %s", resp.body)
	}
}

// 局域网私有网络（RFC 1918）允许访问与写操作，公网 IP 拒绝。
func TestLANAccessAllowed(t *testing.T) {
	_, base := newTestServer(t)
	body := `{"title":"lan-test","characterJson":` + str(testCard) + `,"openingText":"开场对话"}`

	// 1. Same-origin LAN access is allowed.
	req1, _ := http.NewRequest("POST", base+"/api/v1/sessions", strings.NewReader(body))
	req1.Host = "192.168.5.8:8890"
	req1.Header.Set("Origin", "http://192.168.5.8:8890")
	resp1 := post(t, req1)
	if resp1.status != 201 {
		t.Fatalf("status = %d, want 201 for same-origin LAN, body=%s", resp1.status, resp1.body)
	}

	// 2. A different private origin is no longer implicitly trusted.
	req2, _ := http.NewRequest("POST", base+"/api/v1/sessions", strings.NewReader(body))
	req2.Header.Set("Origin", "http://10.0.1.20:5173")
	resp2 := post(t, req2)
	if resp2.status != 403 {
		t.Fatalf("status = %d, want 403 for an unrelated private origin, body=%s", resp2.status, resp2.body)
	}

	// 3. 公网 IP Origin 拒绝
	req3, _ := http.NewRequest("POST", base+"/api/v1/sessions", strings.NewReader(body))
	req3.Header.Set("Origin", "http://203.0.113.50:5173")
	resp3 := post(t, req3)
	if resp3.status != 403 {
		t.Fatalf("status = %d, want 403 (public IP origin should be rejected), body=%s", resp3.status, resp3.body)
	}
}

type resp struct {
	status int
	body   string
}

func post(t *testing.T, req *http.Request) resp {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second}
	r, err := client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer r.Body.Close()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return resp{status: r.StatusCode, body: string(b)}
}

func str(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// M3：记忆管理接口。走真实 HTTP 路径覆盖路由接线与安全层。
func TestMemoryAPI(t *testing.T) {
	_, base := newTestServerWith(t, memoryScript())

	// 建会话（happyScript 里的 final 带 memory_add 提议）。
	cardBody, _ := json.Marshal(map[string]any{
		"characterJson": testCard, "playerName": "旅人", "title": "记忆接口",
		"openingText": "你推开酒馆的门，老板娘抬起了头。",
	})
	code, body := postJSON(t, base+"/api/v1/sessions", string(cardBody))
	if code != 201 {
		t.Fatalf("create session: %d %s", code, body)
	}
	var created struct {
		SessionID string `json:"sessionId"`
		BranchID  string `json:"branchId"`
		RootID    string `json:"rootNodeId"`
		Version   int64  `json:"branchVersion"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// 跑一轮，让模型提议的记忆落库。
	turnBody, _ := json.Marshal(map[string]any{
		"idempotencyKey": "mem-api-1", "expectedHeadId": created.RootID,
		"expectedVersion": created.Version,
		"input":           map[string]any{"kind": "text", "text": "你好。"},
	})
	code, body = postJSON(t, base+"/api/v1/sessions/"+created.SessionID+"/branches/"+created.BranchID+"/turns", string(turnBody))
	if code != 202 {
		t.Fatalf("accept turn: %d %s", code, body)
	}
	var accepted struct {
		TurnID string `json:"turnId"`
	}
	_ = json.Unmarshal([]byte(body), &accepted)
	waitTurnDone(t, base, accepted.TurnID)

	// 列表。
	code, body = getJSON(t, base+"/api/v1/sessions/"+created.SessionID+"/memories")
	if code != 200 {
		t.Fatalf("list memories: %d %s", code, body)
	}
	var listed struct {
		Memories []struct {
			MemoryID  string `json:"memoryId"`
			Content   string `json:"content"`
			Effective bool   `json:"effective"`
		} `json:"memories"`
	}
	if err := json.Unmarshal([]byte(body), &listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed.Memories) == 0 {
		t.Fatalf("模型提议的记忆未落库: %s", body)
	}
	target := listed.Memories[0]

	// 按分支查询：应带上 effective 标记。
	code, body = getJSON(t, base+"/api/v1/sessions/"+created.SessionID+"/memories?branchId="+created.BranchID)
	if code != 200 || !strings.Contains(body, `"effective":true`) {
		t.Fatalf("按分支列表应含 effective 标记: %d %s", code, body)
	}

	// 修订：新建覆盖记录。
	code, body = patchJSON(t, base+"/api/v1/sessions/"+created.SessionID+"/branches/"+created.BranchID+
		"/memories/"+target.MemoryID, `{"content":"修订后的记忆。","pinned":true}`)
	if code != 200 {
		t.Fatalf("patch memory: %d %s", code, body)
	}
	var overlay struct {
		MemoryID   string `json:"memoryId"`
		Content    string `json:"content"`
		Pinned     bool   `json:"pinned"`
		Supersedes string `json:"supersedes"`
	}
	if err := json.Unmarshal([]byte(body), &overlay); err != nil {
		t.Fatalf("decode patch: %v", err)
	}
	if overlay.Content != "修订后的记忆。" || !overlay.Pinned {
		t.Fatalf("修订未生效: %+v", overlay)
	}
	// copy-on-write：新记录取代旧记录，而不是原地改。
	if overlay.Supersedes != target.MemoryID || overlay.MemoryID == target.MemoryID {
		t.Fatalf("应为覆盖记录: %+v（原记录 %s）", overlay, target.MemoryID)
	}

	// 修订后：原记录不再生效，覆盖记录生效。
	_, body = getJSON(t, base+"/api/v1/sessions/"+created.SessionID+"/memories?branchId="+created.BranchID)
	var after struct {
		Memories []struct {
			MemoryID  string `json:"memoryId"`
			Effective bool   `json:"effective"`
		} `json:"memories"`
	}
	_ = json.Unmarshal([]byte(body), &after)
	byID := map[string]bool{}
	for _, m := range after.Memories {
		byID[m.MemoryID] = m.Effective
	}
	if byID[target.MemoryID] {
		t.Fatalf("原记录修订后不应生效: %s", body)
	}
	if !byID[overlay.MemoryID] {
		t.Fatalf("覆盖记录应生效: %s", body)
	}

	// 不存在的记忆 → 404。
	code, _ = patchJSON(t, base+"/api/v1/sessions/"+created.SessionID+"/branches/"+created.BranchID+
		"/memories/nope", `{"pinned":true}`)
	if code != 404 {
		t.Fatalf("不存在的记忆应返回 404，实际 %d", code)
	}
	// 空内容 → 4xx。
	code, _ = patchJSON(t, base+"/api/v1/sessions/"+created.SessionID+"/branches/"+created.BranchID+
		"/memories/"+target.MemoryID, `{"content":"   "}`)
	if code < 400 || code >= 500 {
		t.Fatalf("空内容应返回 4xx，实际 %d", code)
	}
}

func getJSON(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func patchJSON(t *testing.T, url, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPatch, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH %s: %v", url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func waitTurnDone(t *testing.T, base, turnID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, body := getJSON(t, base+"/api/v1/turns/"+turnID)
		if strings.Contains(body, `"committed"`) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("回合未在期限内提交")
}

// createTestSession 建会话并返回 ID/分支/根节点/版本。
type testSession struct {
	SessionID string `json:"sessionId"`
	BranchID  string `json:"branchId"`
	RootID    string `json:"rootNodeId"`
	Version   int64  `json:"branchVersion"`
}

func createTestSession(t *testing.T, base, title string) testSession {
	t.Helper()
	cardBody, _ := json.Marshal(map[string]any{
		"characterJson": testCard, "playerName": "旅人", "title": title,
		"openingText": "你推开酒馆的门，老板娘抬起了头。",
	})
	code, body := postJSON(t, base+"/api/v1/sessions", string(cardBody))
	if code != 201 {
		t.Fatalf("create session: %d %s", code, body)
	}
	var out testSession
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	return out
}

// runTurnOnBranch 提交一轮并返回结果节点 ID。
func runTurnOnBranch(t *testing.T, base string, s testSession, key, text string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"idempotencyKey": key, "expectedHeadId": s.RootID, "expectedVersion": s.Version,
		"input": map[string]any{"kind": "text", "text": text},
	})
	code, resp := postJSON(t, base+"/api/v1/sessions/"+s.SessionID+"/branches/"+s.BranchID+"/turns", string(body))
	if code != 202 {
		t.Fatalf("accept turn: %d %s", code, resp)
	}
	var acc struct {
		TurnID string `json:"turnId"`
	}
	_ = json.Unmarshal([]byte(resp), &acc)
	waitTurnDone(t, base, acc.TurnID)
	code, resp = getJSON(t, base+"/api/v1/turns/"+acc.TurnID)
	if code != 200 {
		t.Fatalf("get turn: %d %s", code, resp)
	}
	var view struct {
		Status       string `json:"status"`
		ResultNodeID string `json:"resultNodeId"`
	}
	_ = json.Unmarshal([]byte(resp), &view)
	if view.ResultNodeID == "" {
		t.Fatalf("回合无结果节点: %s", resp)
	}
	return view.ResultNodeID
}

// 分支与候选版本的 HTTP 路径（E1/E2/E3）。
func TestBranchForkAndDeriveAPI(t *testing.T) {
	_, base := newTestServer(t)
	sess := createTestSession(t, base, "分支接口")
	first := runTurnOnBranch(t, base, sess, "br-1", "你好。")

	// fork：从既有节点建分支。
	forkBody, _ := json.Marshal(map[string]any{"fromNodeId": first, "name": "另一条线"})
	code, body := postJSON(t, base+"/api/v1/sessions/"+sess.SessionID+"/branches", string(forkBody))
	if code != 201 {
		t.Fatalf("fork: %d %s", code, body)
	}
	var forked struct {
		Branch struct {
			BranchID   string `json:"branchId"`
			HeadNodeID string `json:"headNodeId"`
			Version    int64  `json:"version"`
			Name       string `json:"name"`
		} `json:"branch"`
	}
	if err := json.Unmarshal([]byte(body), &forked); err != nil {
		t.Fatalf("decode fork: %v", err)
	}
	if forked.Branch.HeadNodeID != first || forked.Branch.Name != "另一条线" {
		t.Fatalf("分叉结果不符: %+v", forked.Branch)
	}

	// 跨会话分叉被拒。
	code, _ = postJSON(t, base+"/api/v1/sessions/sess_other/branches", string(forkBody))
	if code == 201 {
		t.Fatalf("跨会话分叉应被拒绝")
	}

	// regenerate：从 parent(N) 建候选分支并生成。
	regenBody, _ := json.Marshal(map[string]any{"nodeId": first})
	code, body = postJSON(t, base+"/api/v1/sessions/"+sess.SessionID+"/branches/"+sess.BranchID+"/regenerations", string(regenBody))
	if code != 202 {
		t.Fatalf("regenerate: %d %s", code, body)
	}
	var regen struct {
		BranchID string `json:"branchId"`
		TurnID   string `json:"turnId"`
	}
	if err := json.Unmarshal([]byte(body), &regen); err != nil {
		t.Fatalf("decode regen: %v", err)
	}
	if regen.BranchID == sess.BranchID {
		t.Fatalf("重生成应在新的候选分支上进行")
	}
	waitTurnDone(t, base, regen.TurnID)

	// 候选与原件是兄弟：nodes 接口返回两个候选。
	code, body = getJSON(t, base+"/api/v1/nodes/"+first)
	if code != 200 {
		t.Fatalf("get node: %d %s", code, body)
	}
	var nodeView struct {
		Node struct {
			NodeID string `json:"nodeId"`
		} `json:"node"`
		Candidates []struct {
			NodeID   string `json:"nodeId"`
			ParentID string `json:"parentId"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(body), &nodeView); err != nil {
		t.Fatalf("decode node: %v", err)
	}
	if len(nodeView.Candidates) != 2 {
		t.Fatalf("候选数 = %d, want 2（原回合 + 重生成）", len(nodeView.Candidates))
	}
	for _, c := range nodeView.Candidates {
		if c.ParentID != sess.RootID {
			t.Fatalf("候选父节点应为 %s，实际 %s", sess.RootID, c.ParentID)
		}
	}

	// edit blocks：编辑助手正文 → 纯叙事候选，同步返回节点。
	editBody, _ := json.Marshal(map[string]any{
		"nodeId": first,
		"blocks": []map[string]any{{"kind": "narration", "text": "她什么也没说。"}},
	})
	code, body = postJSON(t, base+"/api/v1/sessions/"+sess.SessionID+"/branches/"+sess.BranchID+"/edits", string(editBody))
	if code != 201 {
		t.Fatalf("edit assistant: %d %s", code, body)
	}
	var edited struct {
		NodeID   string `json:"nodeId"`
		Mode     string `json:"mode"`
		BranchID string `json:"branchId"`
	}
	if err := json.Unmarshal([]byte(body), &edited); err != nil {
		t.Fatalf("decode edit: %v", err)
	}
	if edited.Mode != "narrative" || edited.NodeID == "" {
		t.Fatalf("编辑正文应降级为纯叙事候选: %+v", edited)
	}
	if edited.BranchID == sess.BranchID || edited.BranchID == regen.BranchID {
		t.Fatalf("编辑应产生独立候选分支")
	}

	// 同时给 input 与 blocks → 4xx。
	ambiguous, _ := json.Marshal(map[string]any{
		"nodeId": first,
		"input":  map[string]any{"kind": "text", "text": "x"},
		"blocks": []map[string]any{{"kind": "narration", "text": "y"}},
	})
	code, _ = postJSON(t, base+"/api/v1/sessions/"+sess.SessionID+"/branches/"+sess.BranchID+"/edits", string(ambiguous))
	if code < 400 || code >= 500 {
		t.Fatalf("同时编辑两者应返回 4xx，实际 %d", code)
	}

	// 两者都不给 → 4xx。
	empty, _ := json.Marshal(map[string]any{"nodeId": first})
	code, _ = postJSON(t, base+"/api/v1/sessions/"+sess.SessionID+"/branches/"+sess.BranchID+"/edits", string(empty))
	if code < 400 || code >= 500 {
		t.Fatalf("空编辑应返回 4xx，实际 %d", code)
	}

	// 根节点不能重生成。
	rootBody, _ := json.Marshal(map[string]any{"nodeId": sess.RootID})
	code, _ = postJSON(t, base+"/api/v1/sessions/"+sess.SessionID+"/branches/"+sess.BranchID+"/regenerations", string(rootBody))
	if code < 400 || code >= 500 {
		t.Fatalf("根节点重生成应返回 4xx，实际 %d", code)
	}
}

// 会话视图支持按分支与查看节点读取（branch 与 viewNodeId 分离）。
func TestGetSessionScopedViews(t *testing.T) {
	_, base := newTestServer(t)
	sess := createTestSession(t, base, "视图接口")
	first := runTurnOnBranch(t, base, sess, "v-1", "你好。")
	second := runTurnOnBranch(t, base, testSession{
		SessionID: sess.SessionID, BranchID: sess.BranchID, RootID: first, Version: 1,
	}, "v-2", "继续。")

	// 默认：分支头。
	_, body := getJSON(t, base+"/api/v1/sessions/"+sess.SessionID)
	var view struct {
		Branch struct {
			HeadNodeID string `json:"headNodeId"`
			Version    int64  `json:"version"`
		} `json:"branch"`
		ViewNodeID string `json:"viewNodeId"`
		Branches   []struct {
			BranchID string `json:"branchId"`
		} `json:"branches"`
	}
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.Branch.HeadNodeID != second || view.ViewNodeID != second {
		t.Fatalf("默认应查看分支头: %+v", view)
	}
	if len(view.Branches) == 0 {
		t.Fatalf("应返回分支列表")
	}

	// 查看历史节点：viewNode 改变，但分支头不动（浏览不推进写入指针）。
	_, body = getJSON(t, base+"/api/v1/sessions/"+sess.SessionID+"?viewNodeId="+first)
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatalf("decode view: %v", err)
	}
	if view.ViewNodeID != first {
		t.Fatalf("查看位置未切换: %s", view.ViewNodeID)
	}
	if view.Branch.HeadNodeID != second {
		t.Fatalf("浏览历史不应改动分支头: %s", view.Branch.HeadNodeID)
	}

	// 不存在的分支 → 404。
	code, _ := getJSON(t, base+"/api/v1/sessions/"+sess.SessionID+"?branchId=branch_nope")
	if code != 404 {
		t.Fatalf("未知分支应 404，实际 %d", code)
	}
	// 不存在的节点 → 404。
	code, _ = getJSON(t, base+"/api/v1/sessions/"+sess.SessionID+"?viewNodeId=node_nope")
	if code != 404 {
		t.Fatalf("未知节点应 404，实际 %d", code)
	}
}

// 过期选项在 HTTP 层返回 409 与 STALE_OPTION。
func TestStaleOptionHTTP(t *testing.T) {
	_, base := newTestServer(t)
	sess := createTestSession(t, base, "选项接口")
	first := runTurnOnBranch(t, base, sess, "so-1", "你好。")

	// 取首轮选项。
	_, body := getJSON(t, base+"/api/v1/nodes/"+first)
	var nodeView struct {
		Node struct {
			ContentJSON string `json:"contentJson"`
		} `json:"node"`
	}
	if err := json.Unmarshal([]byte(body), &nodeView); err != nil {
		t.Fatalf("decode node: %v", err)
	}
	var tc struct {
		Options []struct {
			OptionID string `json:"optionId"`
			Text     string `json:"text"`
		} `json:"options"`
	}
	if err := json.Unmarshal([]byte(nodeView.Node.ContentJSON), &tc); err != nil {
		t.Fatalf("decode content: %v", err)
	}
	if len(tc.Options) == 0 {
		t.Fatalf("首轮应产生选项")
	}

	// 推进分支头。
	second := runTurnOnBranch(t, base, testSession{
		SessionID: sess.SessionID, BranchID: sess.BranchID, RootID: first, Version: 1,
	}, "so-2", "继续。")
	if second == "" {
		t.Fatalf("第二轮未提交")
	}

	// 用首轮选项提交 → 409 STALE_OPTION。
	stale, _ := json.Marshal(map[string]any{
		"idempotencyKey": "so-3", "expectedHeadId": second, "expectedVersion": 2,
		"input": map[string]any{
			"kind": "option", "text": tc.Options[0].Text,
			"optionRef": map[string]any{"nodeId": first, "optionId": tc.Options[0].OptionID},
		},
	})
	code, body := postJSON(t, base+"/api/v1/sessions/"+sess.SessionID+"/branches/"+sess.BranchID+"/turns", string(stale))
	if code != 409 {
		t.Fatalf("过期选项应 409，实际 %d %s", code, body)
	}
	if !strings.Contains(body, "STALE_OPTION") {
		t.Fatalf("错误码应含 STALE_OPTION: %s", body)
	}
}

// 剧情包导出/导入接口（F5/F6）。
func TestExportImportAPI(t *testing.T) {
	_, base := newTestServer(t)
	sess := createTestSession(t, base, "剧情包接口")
	runTurnOnBranch(t, base, sess, "pk-1", "你好。")

	// 导出：应返回 ZIP 附件。
	resp, err := http.Get(base + "/api/v1/sessions/" + sess.SessionID + "/export")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	packBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("export status = %d: %s", resp.StatusCode, packBytes)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "zip") {
		t.Fatalf("Content-Type = %q, want zip", ct)
	}
	cd := resp.Header.Get("Content-Disposition")
	if !strings.Contains(cd, ".tavernpack") || !strings.Contains(cd, "filename*=") {
		t.Fatalf("下载头不合格（需含文件名与 RFC 5987 回退）: %q", cd)
	}
	if len(packBytes) < 100 || !strings.HasPrefix(string(packBytes[:2]), "PK") {
		t.Fatalf("导出内容不像 ZIP: %d 字节", len(packBytes))
	}

	// 单分支导出也应成功。
	resp2, err := http.Get(base + "/api/v1/sessions/" + sess.SessionID + "/export?branchId=" + sess.BranchID)
	if err != nil {
		t.Fatalf("export branch: %v", err)
	}
	branchPack, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != 200 || len(branchPack) == 0 {
		t.Fatalf("单分支导出失败: %d", resp2.StatusCode)
	}

	// 不存在的会话 → 404。
	resp3, _ := http.Get(base + "/api/v1/sessions/sess_nope/export")
	io.Copy(io.Discard, resp3.Body)
	resp3.Body.Close()
	if resp3.StatusCode != 404 {
		t.Fatalf("未知会话导出应 404，实际 %d", resp3.StatusCode)
	}

	// 导入：应新建会话。
	code, body := postRaw(t, base+"/api/v1/sessions/import", packBytes)
	if code != 201 {
		t.Fatalf("import: %d %s", code, body)
	}
	var imported struct {
		SessionID string         `json:"sessionId"`
		Title     string         `json:"title"`
		Counts    map[string]int `json:"counts"`
	}
	if err := json.Unmarshal([]byte(body), &imported); err != nil {
		t.Fatalf("decode import: %v", err)
	}
	if imported.SessionID == "" || imported.SessionID == sess.SessionID {
		t.Fatalf("导入应分配新会话 ID: %q", imported.SessionID)
	}
	if imported.Counts["nodes"] == 0 {
		t.Fatalf("导入结果缺少统计: %s", body)
	}

	// 导入后的会话可读且分支指针正常。
	code, body = getJSON(t, base+"/api/v1/sessions/"+imported.SessionID)
	if code != 200 {
		t.Fatalf("读取导入会话失败: %d %s", code, body)
	}
	if !strings.Contains(body, imported.SessionID) {
		t.Fatalf("会话视图不含自身 ID")
	}

	// 非 ZIP 内容 → 400 INVALID_PACK。
	code, body = postRaw(t, base+"/api/v1/sessions/import", []byte("definitely not a zip"))
	if code != 400 {
		t.Fatalf("非法包应 400，实际 %d %s", code, body)
	}
	if !strings.Contains(body, "INVALID_PACK") {
		t.Fatalf("错误码应为 INVALID_PACK: %s", body)
	}
}

// postRaw 发送原始字节体（剧情包不是 JSON）。
func postRaw(t *testing.T, url string, body []byte) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/zip")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestCardImport_JSON(t *testing.T) {
	_, base := newTestServer(t)

	rawV2 := `{
		"spec": "chara_card_v2",
		"spec_version": "2.0",
		"data": {
			"name": "艾莲娜",
			"description": "神秘的占星术士",
			"first_mes": "夜空中繁星闪烁。"
		}
	}`

	req, _ := http.NewRequest(http.MethodPost, base+"/api/v1/cards/import", strings.NewReader(rawV2))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/v1/cards/import: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body: %s", resp.StatusCode, string(b))
	}

	var res struct {
		Format        string              `json:"format"`
		Name          string              `json:"name"`
		CharacterJSON string              `json:"characterJson"`
		Openings      []map[string]string `json:"openings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if res.Name != "艾莲娜" {
		t.Fatalf("name = %q, want '艾莲娜'", res.Name)
	}
	if res.Format != "chara_card_v2" {
		t.Fatalf("format = %q, want 'chara_card_v2'", res.Format)
	}
	if len(res.Openings) == 0 || res.Openings[0]["text"] != "夜空中繁星闪烁。" {
		t.Fatalf("openings = %+v", res.Openings)
	}
	if res.CharacterJSON == "" {
		t.Fatalf("characterJson should not be empty")
	}
}

func TestCardImport_Multipart(t *testing.T) {
	_, base := newTestServer(t)

	rawV2 := `{
		"spec": "chara_card_v2",
		"spec_version": "2.0",
		"data": {
			"name": "艾莲娜",
			"description": "神秘的占星术士",
			"first_mes": "夜空中繁星闪烁。"
		}
	}`

	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	fw, err := w.CreateFormFile("file", "elena.json")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := io.WriteString(fw, rawV2); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	w.Close()

	req, _ := http.NewRequest(http.MethodPost, base+"/api/v1/cards/import", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/v1/cards/import: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		resBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body: %s", resp.StatusCode, string(resBody))
	}

	var res struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.Name != "艾莲娜" {
		t.Fatalf("name = %q, want '艾莲娜'", res.Name)
	}
}

func buildTestPNGWithText(chunkType, keyword, text string) []byte {
	var b []byte
	b = append(b, 0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A)
	ihdr := make([]byte, 13)
	appendChunk := func(ctype string, data []byte) {
		length := make([]byte, 4)
		binary.BigEndian.PutUint32(length, uint32(len(data)))
		b = append(b, length...)
		b = append(b, []byte(ctype)...)
		b = append(b, data...)
		b = append(b, 0, 0, 0, 0)
	}
	appendChunk("IHDR", ihdr)
	appendChunk(chunkType, append(append([]byte(keyword), 0), []byte(text)...))
	appendChunk("IEND", nil)
	return b
}

func TestCardImport_PNG(t *testing.T) {
	_, base := newTestServer(t)

	spec := map[string]any{
		"spec":         "chara_card_v2",
		"spec_version": "2.0",
		"data": map[string]any{
			"name":        "卡萝尔",
			"description": "林中精灵巡林客",
			"first_mes":   "树叶在风中沙沙作响。",
		},
	}
	raw, _ := json.Marshal(spec)
	b64 := base64.StdEncoding.EncodeToString(raw)
	pngBytes := buildTestPNGWithText("tEXt", "chara", b64)

	req, _ := http.NewRequest(http.MethodPost, base+"/api/v1/cards/import", bytes.NewReader(pngBytes))
	req.Header.Set("Content-Type", "image/png")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/v1/cards/import: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body: %s", resp.StatusCode, string(b))
	}

	var res struct {
		Name   string `json:"name"`
		Source string `json:"source"`
		Format string `json:"format"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.Name != "卡萝尔" {
		t.Fatalf("name = %q, want '卡萝尔'", res.Name)
	}
	if res.Source != "png" {
		t.Fatalf("source = %q, want 'png'", res.Source)
	}
}

func TestLANPairingAuth(t *testing.T) {
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	bus := application.NewEventBus(st)
	sessSvc := application.NewSessionService(st)
	compiler := ctxpkg.New(st, ctxpkg.DefaultOptions())
	turnSvc := application.NewTurnService(st, mock.New(happyScript()), compiler, bus)
	branchSvc := application.NewBranchService(st, turnSvc)
	memSvc := application.NewMemoryService(st)
	archSvc := application.NewArchiveService(st, "test")

	const testPIN = "654321"
	s := mustNew(t, Deps{
		Sessions: sessSvc, Turns: turnSvc, Branches: branchSvc,
		Memories: memSvc, Archive: archSvc, Bus: bus,
		AuthPIN: testPIN,
	})

	handler := s.Handler()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Test transport only: explicitly simulate a LAN connection peer.
		if peer := r.Header.Get("X-Test-Peer"); peer != "" {
			r.RemoteAddr = peer
		}
		handler.ServeHTTP(w, r)
	}))
	defer srv.Close()

	// 1. 本机回环请求（Host: 127.0.0.1:port）-> 自动放行免配对
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/sessions", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("loopback request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("loopback should succeed without PIN, got status %d", resp.StatusCode)
	}

	// 2. 模拟非本机局域网请求（Host: 192.168.1.100:8890）无 Token -> 401 AUTH_REQUIRED
	lanReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/sessions", nil)
	lanReq.Host = "192.168.1.100:8890"
	lanReq.Header.Set("X-Test-Peer", "192.168.1.20:1234")
	resp2, err := http.DefaultClient.Do(lanReq)
	if err != nil {
		t.Fatalf("lan request failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("lan without token should be 401, got %d", resp2.StatusCode)
	}

	// 3. 错误 PIN 配对 -> 401 INVALID_PIN
	badPairBody := bytes.NewReader([]byte(`{"pin":"000000"}`))
	pairReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/auth/pair", badPairBody)
	pairReq.Host = "192.168.1.100:8890"
	pairReq.Header.Set("X-Test-Peer", "192.168.1.20:1234")
	resp3, err := http.DefaultClient.Do(pairReq)
	if err != nil {
		t.Fatalf("pair request failed: %v", err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong PIN should be 401, got %d", resp3.StatusCode)
	}

	// 4. 正确 PIN 配对 -> 200 返回 token
	goodPairBody := bytes.NewReader([]byte(`{"pin":"654321"}`))
	pairReq2, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/auth/pair", goodPairBody)
	pairReq2.Host = "192.168.1.100:8890"
	pairReq2.Header.Set("X-Test-Peer", "192.168.1.20:1234")
	resp4, err := http.DefaultClient.Do(pairReq2)
	if err != nil {
		t.Fatalf("pair request failed: %v", err)
	}
	defer resp4.Body.Close()
	if resp4.StatusCode != http.StatusOK {
		t.Fatalf("correct PIN should be 200, got %d", resp4.StatusCode)
	}

	var pairRes struct {
		OK    bool   `json:"ok"`
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp4.Body).Decode(&pairRes); err != nil {
		t.Fatalf("decode pair response: %v", err)
	}
	if !pairRes.OK || pairRes.Token == "" {
		t.Fatalf("expected valid token, got %+v", pairRes)
	}

	// 5. 局域网请求携带配对生成的 Token -> 200 放行
	authedReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/sessions", nil)
	authedReq.Host = "192.168.1.100:8890"
	authedReq.Header.Set("X-Test-Peer", "192.168.1.20:1234")
	authedReq.Header.Set("Authorization", "Bearer "+pairRes.Token)
	resp5, err := http.DefaultClient.Do(authedReq)
	if err != nil {
		t.Fatalf("authed request failed: %v", err)
	}
	defer resp5.Body.Close()
	if resp5.StatusCode != http.StatusOK {
		t.Fatalf("authed lan request should be 200, got %d", resp5.StatusCode)
	}
}

// 导入响应的键名是跨语言契约：前端 web/src/app/types.ts 的 CardPreview 按这些名字读取，
// 一旦错位（历史上 postHistory/version 就与 DTO 不一致），预览里的字段会静默恒为空。
// 这里同时钉住「该有的键存在」与「旧错名不再出现」。
func TestCardImport_ResponseKeysMatchFrontendDTO(t *testing.T) {
	_, base := newTestServer(t)

	rawV2 := `{
		"spec": "chara_card_v2",
		"spec_version": "2.0",
		"data": {
			"name": "契约卡",
			"description": "描述",
			"personality": "性子急",
			"scenario": "雨夜",
			"first_mes": "你好",
			"mes_example": "<START>",
			"system_prompt": "规则",
			"post_history_instructions": "收尾要求",
			"creator_notes": "创作者留言",
			"creator": "作者",
			"character_version": "1.2",
			"nickname": "小卡",
			"tags": ["测试"]
		}
	}`

	req, _ := http.NewRequest(http.MethodPost, base+"/api/v1/cards/import", strings.NewReader(rawV2))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/v1/cards/import: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d: %s", resp.StatusCode, string(b))
	}

	var res map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("decode: %v", err)
	}

	required := []string{
		"format", "specVersion", "source", "name", "avatar", "description", "personality",
		"scenario", "firstMes", "mesExample", "systemPrompt", "postHistoryInstructions",
		"creatorNotes", "tags", "creator", "characterVersion", "nickname",
		"supported", "ignored", "warnings", "characterJson", "openings",
	}
	for _, key := range required {
		if _, ok := res[key]; !ok {
			t.Errorf("导入响应缺少前端 DTO 需要的键 %q", key)
		}
	}
	for _, legacy := range []string{"postHistory", "version"} {
		if _, ok := res[legacy]; ok {
			t.Errorf("导入响应仍带旧错名 %q，前端按 DTO 读取会取到空值", legacy)
		}
	}

	var postHistory, version string
	if err := json.Unmarshal(res["postHistoryInstructions"], &postHistory); err != nil || postHistory != "收尾要求" {
		t.Errorf("postHistoryInstructions = %q (err=%v)", postHistory, err)
	}
	if err := json.Unmarshal(res["characterVersion"], &version); err != nil || version != "1.2" {
		t.Errorf("characterVersion = %q (err=%v)", version, err)
	}
}
