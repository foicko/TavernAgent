package http

// 删除会话的接口级验收：数据与列表都要消失，重复删除 404，跨站来源被拒。
//
// 与 server_test.go 分开：那个文件早已超过架构门禁的 800 行上限（基线豁免），
// 新用例单独成文件，避免把基线继续往上推。

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// 删除会话：数据与列表都要消失，重复删除返回 404，他人会话不受影响。
func TestDeleteSessionAPI(t *testing.T) {
	_, base := newTestServer(t)
	victim := createTestSession(t, base, "待删除故事")
	keeper := createTestSession(t, base, "保留故事")
	runTurnOnBranch(t, base, victim, "del-1", "我推开了门。")
	runTurnOnBranch(t, base, keeper, "keep-1", "我留在原地。")

	// 删除前：拿到过导出包（证明数据确实存在）。
	before, err := http.Get(base + "/api/v1/sessions/" + victim.SessionID + "/export")
	if err != nil {
		t.Fatalf("export before delete: %v", err)
	}
	io.Copy(io.Discard, before.Body)
	before.Body.Close()
	if before.StatusCode != 200 {
		t.Fatalf("删除前导出应成功，实际 %d", before.StatusCode)
	}

	code, body := deleteJSON(t, base+"/api/v1/sessions/"+victim.SessionID)
	if code != 200 {
		t.Fatalf("删除会话: %d %s", code, body)
	}
	if !strings.Contains(body, `"ok":true`) {
		t.Fatalf("删除响应体不符: %s", body)
	}

	// 视图、导出、列表三处都必须看不到它了。
	if code, _ := getJSON(t, base+"/api/v1/sessions/"+victim.SessionID); code != 404 {
		t.Fatalf("已删除会话的视图应 404，实际 %d", code)
	}
	if resp, err := http.Get(base + "/api/v1/sessions/" + victim.SessionID + "/export"); err == nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 404 {
			t.Fatalf("已删除会话的导出应 404，实际 %d", resp.StatusCode)
		}
	}
	code, listBody := getJSON(t, base+"/api/v1/sessions")
	if code != 200 {
		t.Fatalf("列会话: %d %s", code, listBody)
	}
	if strings.Contains(listBody, victim.SessionID) {
		t.Fatalf("删除后列表仍包含该会话: %s", listBody)
	}
	if !strings.Contains(listBody, keeper.SessionID) {
		t.Fatalf("删除他人会话不应影响保留会话: %s", listBody)
	}

	// 重复删除：404，而不是假装成功。
	if code, body := deleteJSON(t, base+"/api/v1/sessions/"+victim.SessionID); code != 404 {
		t.Fatalf("重复删除应 404，实际 %d %s", code, body)
	}
}

// 写请求都带 Origin 校验，DELETE 也不例外：跨站来源必须被拒绝。
func TestDeleteSessionRejectsForeignOrigin(t *testing.T) {
	_, base := newTestServer(t)
	sess := createTestSession(t, base, "来源校验")
	req, err := http.NewRequest(http.MethodDelete, base+"/api/v1/sessions/"+sess.SessionID, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Origin", "https://evil.example")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("跨站删除应 403，实际 %d", resp.StatusCode)
	}
	if code, _ := getJSON(t, base+"/api/v1/sessions/"+sess.SessionID); code != 200 {
		t.Fatalf("被拒绝的删除不应改动数据，视图状态 %d", code)
	}
}

func deleteJSON(t *testing.T, url string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}
