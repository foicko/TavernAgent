package http

// 模型配置 API 的用例：实例 CRUD（脱敏/校验/删除保护）+ 槽位只收引用。
import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tavernagent/internal/adapters/config"
	"tavernagent/internal/adapters/providers/mock"
	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/application"
	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/ports"
)

// 模型配置 API 测试的公共夹具：一个真实的文件配置存储 + 管理器 + 路由。
func newModelConfigServer(t *testing.T) (*httptest.Server, *application.ProviderManager) {
	t.Helper()
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cfgStore, err := config.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mgr := application.NewProviderManager(cfgStore, func(cfg ports.ProviderConfig) (ports.ModelProvider, error) {
		return mock.New(happyScript()), nil
	}, mock.New(happyScript()))
	if err := mgr.Reload(); err != nil {
		t.Fatal(err)
	}
	bus := application.NewEventBus(st)
	sessSvc := application.NewSessionService(st)
	turnSvc := application.NewTurnServiceWithManager(st, mgr, ctxpkg.New(st, ctxpkg.DefaultOptions()), bus)
	srv := mustNew(t, Deps{
		Sessions: sessSvc, Turns: turnSvc,
		Branches: application.NewBranchService(st, turnSvc),
		Memories: application.NewMemoryService(st),
		Archive:  application.NewArchiveService(st, "test"),
		Manager:  mgr, Bus: bus, Addr: "127.0.0.1:8890",
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, mgr
}

// 实例 CRUD：脱敏返回、必填校验、级联删除保护、实例级探测、旧的档案路由已移除。
func TestModelInstanceAPI(t *testing.T) {
	ts, _ := newModelConfigServer(t)

	createBody := `{"name":"DeepSeek 官方","kind":"openai-compatible","baseUrl":"https://api.example.com","model":"deepseek-chat","apiKey":"sk-1234567890abcd","reasoningEffort":"high"}`
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/config/models", strings.NewReader(createBody))
	r := post(t, req)
	if r.status != 200 {
		t.Fatalf("create status = %d body=%s", r.status, r.body)
	}
	if strings.Contains(r.body, "sk-1234567890abcd") || !strings.Contains(r.body, "sk-1…abcd") {
		t.Fatalf("密钥必须只回脱敏值: %s", r.body)
	}
	// 思考强度的线上字段名要与前端类型一致（契约靠这条锁住）。
	if !strings.Contains(r.body, `"reasoningEffort":"high"`) {
		t.Fatalf("思考强度未按 reasoningEffort 回传: %s", r.body)
	}
	var created ports.ModelInstance
	if err := json.Unmarshal([]byte(r.body), &created); err != nil || created.ID == "" {
		t.Fatalf("缺少实例 ID: %s (%v)", r.body, err)
	}

	// 缺模型名 → 422 MISSING_MODEL。
	req6, _ := http.NewRequest("POST", ts.URL+"/api/v1/config/models", strings.NewReader(`{"name":"没有模型","kind":"openai-chat"}`))
	if r6 := post(t, req6); r6.status != 422 || !strings.Contains(r6.body, "MISSING_MODEL") {
		t.Fatalf("missing model status = %d %s", r6.status, r6.body)
	}

	// 实例级探测：mock 构建器恒可用 → ok=true。
	req9, _ := http.NewRequest("POST", ts.URL+"/api/v1/config/models/"+created.ID+"/probe", strings.NewReader(`{"format":false}`))
	r9 := post(t, req9)
	if r9.status != 200 || !strings.Contains(r9.body, `"ok":true`) {
		t.Fatalf("probe status = %d body=%s", r9.status, r9.body)
	}

	// GET 列表脱敏。
	req2, _ := http.NewRequest("GET", ts.URL+"/api/v1/config/models", nil)
	if r2 := post(t, req2); r2.status != 200 || strings.Contains(r2.body, "sk-1234567890abcd") {
		t.Fatalf("GET 泄漏密钥: %d %s", r2.status, r2.body)
	}

	// 未被引用 → 可以删除；再删 → 404。
	req12, _ := http.NewRequest("DELETE", ts.URL+"/api/v1/config/models/"+created.ID, nil)
	if r12 := post(t, req12); r12.status != 200 {
		t.Fatalf("delete status = %d %s", r12.status, r12.body)
	}
	req14, _ := http.NewRequest("DELETE", ts.URL+"/api/v1/config/models/"+created.ID, nil)
	if r14 := post(t, req14); r14.status != 404 {
		t.Fatalf("重复删除应当 404: %d", r14.status)
	}

	// 旧的档案路由已删除。
	req13, _ := http.NewRequest("GET", ts.URL+"/api/v1/config/profiles", nil)
	if r13 := post(t, req13); r13.status != 404 {
		t.Fatalf("旧档案路由应当已移除: %d", r13.status)
	}
}

// 槽位只存引用：接受 enabled+modelId，拒绝连接信息字段，拒绝未知槽位与不存在的实例，
// 被引用的实例删除时 409 并指出槽位。
func TestSlotAssignmentAPI(t *testing.T) {
	ts, _ := newModelConfigServer(t)
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/config/models", strings.NewReader(`{"name":"A","kind":"openai-chat","baseUrl":"https://api.example.com","model":"deepseek-chat","apiKey":"sk-1234567890abcd"}`))
	created := struct {
		ID string `json:"id"`
	}{}
	json.Unmarshal([]byte(post(t, req).body), &created)
	if created.ID == "" {
		t.Fatal("create 未返回 ID")
	}

	assign := `{"slot":"primary","enabled":true,"modelId":"` + created.ID + `"}`
	req2, _ := http.NewRequest("PUT", ts.URL+"/api/v1/config/provider", strings.NewReader(assign))
	r2 := post(t, req2)
	if r2.status != 200 || !strings.Contains(r2.body, `"model":"deepseek-chat"`) {
		t.Fatalf("指派失败: %d %s", r2.status, r2.body)
	}

	// GET 槽位带解析结果。
	req8, _ := http.NewRequest("GET", ts.URL+"/api/v1/config/provider", nil)
	r8 := post(t, req8)
	if r8.status != 200 || strings.Contains(r8.body, "sk-1234567890abcd") || !strings.Contains(r8.body, `"resolvedModelId":"`+created.ID+`"`) {
		t.Fatalf("槽位视图不对: %d %s", r8.status, r8.body)
	}

	// 槽位不接受连接信息：指路到实例 API。
	legacy := `{"slot":"assist","enabled":true,"kind":"openai-chat","baseUrl":"https://x","model":"m"}`
	req3, _ := http.NewRequest("PUT", ts.URL+"/api/v1/config/provider", strings.NewReader(legacy))
	if r3 := post(t, req3); r3.status != 422 || !strings.Contains(r3.body, "SLOT_TAKES_REFERENCE_ONLY") {
		t.Fatalf("旧字段应当被拒绝并指路: %d %s", r3.status, r3.body)
	}

	// 非法槽位 → 422；引用不存在的实例 → 404。
	req4, _ := http.NewRequest("PUT", ts.URL+"/api/v1/config/provider", strings.NewReader(`{"slot":"evil","enabled":true}`))
	if r4 := post(t, req4); r4.status != 422 {
		t.Fatalf("bad slot status = %d", r4.status)
	}
	req5, _ := http.NewRequest("PUT", ts.URL+"/api/v1/config/provider", strings.NewReader(`{"slot":"assist","enabled":true,"modelId":"m_missing"}`))
	if r5 := post(t, req5); r5.status != 404 {
		t.Fatalf("missing instance status = %d", r5.status)
	}

	// 删除被主引用的实例 → 409 且指出槽位；槽位级探测保留。
	req7, _ := http.NewRequest("DELETE", ts.URL+"/api/v1/config/models/"+created.ID, nil)
	if r7 := post(t, req7); r7.status != 409 || !strings.Contains(r7.body, "primary") {
		t.Fatalf("应当 409 并指出占用槽位: %d %s", r7.status, r7.body)
	}
	req10, _ := http.NewRequest("POST", ts.URL+"/api/v1/config/provider/probe", strings.NewReader(`{"slot":"primary"}`))
	if r10 := post(t, req10); r10.status != 200 || !strings.Contains(r10.body, `"ok":true`) {
		t.Fatalf("slot probe status = %d body=%s", r10.status, r10.body)
	}

	// 解除引用后可以删除。
	req11, _ := http.NewRequest("PUT", ts.URL+"/api/v1/config/provider", strings.NewReader(`{"slot":"primary","enabled":false,"modelId":""}`))
	if r11 := post(t, req11); r11.status != 200 {
		t.Fatalf("unbind status = %d %s", r11.status, r11.body)
	}
	req12, _ := http.NewRequest("DELETE", ts.URL+"/api/v1/config/models/"+created.ID, nil)
	if r12 := post(t, req12); r12.status != 200 {
		t.Fatalf("delete status = %d %s", r12.status, r12.body)
	}
}
