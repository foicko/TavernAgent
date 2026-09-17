package http

// 测试服务夹具：默认走 mock 脚本，需要观察"半行输出/阻塞"时换成给定供应商。
import (
	"net/http/httptest"
	"testing"

	"tavernagent/internal/adapters/providers/mock"
	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/application"
	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/ports"
)

// mustNew 构造测试服务；配对 Token 生成失败（几乎不可能）直接终止用例。
func mustNew(t *testing.T, deps Deps) *Server {
	t.Helper()
	srv, err := New(deps)
	if err != nil {
		t.Fatalf("http.New: %v", err)
	}
	return srv
}

func newTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	return newTestServerWith(t, happyScript())
}

// newTestServerWith 用给定的 mock 脚本起一个测试服务。
func newTestServerWith(t *testing.T, script []mock.Item) (*httptest.Server, string) {
	t.Helper()
	return newTestServerWithProvider(t, mock.New(script))
}

// newTestServerWithProvider 用给定的供应商起一个测试服务（阻塞/半行输出等场景用）。
func newTestServerWithProvider(t *testing.T, provider ports.ModelProvider) (*httptest.Server, string) {
	t.Helper()
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	bus := application.NewEventBus(st)
	sessSvc := application.NewSessionService(st)
	compiler := ctxpkg.New(st, ctxpkg.DefaultOptions())
	turnSvc := application.NewTurnService(st, provider, compiler, bus)
	// 关闭顺序必须是"服务先排空 worker，存储后关"：LIFO 下这里先注册 store 再注册服务。
	t.Cleanup(turnSvc.Close)
	srv := mustNew(t, Deps{
		Sessions: sessSvc, Turns: turnSvc,
		Branches: application.NewBranchService(st, turnSvc),
		Memories: application.NewMemoryService(st),
		Archive:  application.NewArchiveService(st, "test"),
		Bus:      bus, Addr: "127.0.0.1:8890",
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, ts.URL
}
