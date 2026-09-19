package http

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/application"
	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

func TestServerShutdownDrainsSSEAndReleasesListener(t *testing.T) {
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
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := mustNew(t, Deps{Sessions: sessions, Bus: application.NewEventBus(st), Addr: listener.Addr().String()})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(ctx, listener) }()
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Get("http://" + listener.Addr().String() + "/api/v1/sessions/" + setup.Session.SessionID + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("SSE status %d", response.StatusCode)
	}
	cancel()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SSE kept server alive")
	}
	if _, err := io.ReadAll(response.Body); err != nil {
		t.Fatalf("SSE did not close cleanly: %v", err)
	}
	if conn, err := net.DialTimeout("tcp", listener.Addr().String(), 100*time.Millisecond); err == nil {
		conn.Close()
		t.Fatal("listener is still open")
	}
}

// slowGetSession 让"读取会话"变慢，用来观察请求还在跑时关停的顺序。
type slowGetSession struct {
	ports.Store
	delay time.Duration
	done  chan struct{}
	once  sync.Once
}

func (s *slowGetSession) GetSession(id string) (*domain.Session, error) {
	time.Sleep(s.delay)
	s.once.Do(func() { close(s.done) })
	return s.Store.GetSession(id)
}

// 关停必须等到在途 handler 收尾：Serve 返回之后调用方才会关闭存储，
// 而 handler 里可能正写着数据库（CI 里偶发的 sql: database is closed 就来自这个窗口）。
func TestShutdownWaitsForInFlightHandler(t *testing.T) {
	previousShutdown, previousHandler := shutdownGrace, handlerGrace
	shutdownGrace, handlerGrace = 150*time.Millisecond, 3*time.Second
	defer func() { shutdownGrace, handlerGrace = previousShutdown, previousHandler }()

	inner, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { inner.Close() })
	slow := &slowGetSession{Store: inner, delay: 400 * time.Millisecond, done: make(chan struct{})}

	bus := application.NewEventBus(inner)
	turns := application.NewTurnService(inner, nil, ctxpkg.New(inner, ctxpkg.DefaultOptions()), bus)
	t.Cleanup(turns.Close)
	srv := mustNew(t, Deps{
		Sessions: application.NewSessionService(slow), Turns: turns,
		Branches: application.NewBranchService(inner, turns),
		Memories: application.NewMemoryService(inner),
		Archive:  application.NewArchiveService(inner, "test"),
		Bus:      bus, Addr: "127.0.0.1:0",
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	serveDone := make(chan error, 1)
	go func() { serveDone <- srv.Serve(ctx, listener) }()

	go func() {
		resp, err := http.Get("http://" + listener.Addr().String() + "/api/v1/sessions/session_missing")
		if err == nil {
			resp.Body.Close()
		}
	}()
	time.Sleep(100 * time.Millisecond) // 请求已进入 handler，正在慢读

	cancel()
	select {
	case <-serveDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Serve 未在关停后返回")
	}
	select {
	case <-slow.done:
	default:
		t.Fatal("Serve 在 handler 收尾前返回：调用方紧接着关存储就会写到已关闭的库")
	}
}

// requestDrain 的契约：stop 之后不再接收新请求，因此 wait 的判定不会被后续的
// 计数增加破坏。原先用 sync.WaitGroup 时，新请求的 Add 会与 Wait 并发——那正是
// `go test -race` 在 TestShutdownWaitsForInFlightHandler 上报出来的竞争。
func TestRequestDrainRefusesNewRequestsAfterStop(t *testing.T) {
	d := newRequestDrain()
	if !d.begin() {
		t.Fatal("关停前应接受请求")
	}
	d.stop()
	if d.begin() {
		t.Fatal("stop 之后不应再接受新请求")
	}

	waited := make(chan error, 1)
	go func() { waited <- d.wait(2 * time.Second) }()
	select {
	case err := <-waited:
		t.Fatalf("仍有在途请求时不应立即返回: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	d.end()
	select {
	case err := <-waited:
		if err != nil {
			t.Fatalf("在途请求结束后应正常返回: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("在途请求结束后 wait 仍未返回")
	}
}

// 超时必须返回错误：调用方据此决定是否继续等 handler 收尾（并提醒存储尚未关闭）。
func TestRequestDrainTimesOutWithInFlightRequests(t *testing.T) {
	d := newRequestDrain()
	d.begin()
	d.stop()
	if err := d.wait(20 * time.Millisecond); err == nil {
		t.Fatal("超时应返回错误")
	}
	d.end()
}
