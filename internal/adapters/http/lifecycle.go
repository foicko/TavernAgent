package http

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// 关停排水窗口。缩短它们可以快速观察关停顺序（测试用），生产值保持宽松。
var (
	// shutdownGrace 是优雅关停的等待上限。
	shutdownGrace = 5 * time.Second
	// handlerGrace 是强制关闭连接后，继续等待 handler 收尾的上限。
	// Shutdown 超时只说明连接被强关，handler 可能仍在写存储——调用方
	// （cmd/tavernagent）会紧接着关闭 SQLite，所以必须先等它们退出。
	handlerGrace = 2 * time.Second
)

// requestDrain 统计在途请求，供关停时等待 handler 收尾。
type requestDrain struct {
	wg sync.WaitGroup
}

func (d *requestDrain) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.wg.Add(1)
		defer d.wg.Done()
		next.ServeHTTP(w, r)
	})
}

func (d *requestDrain) wait(timeout time.Duration) error {
	done := make(chan struct{})
	go func() { d.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return errors.New("关停：仍有在途请求未在 " + timeout.String() + " 内收尾")
	}
}

func (s *Server) ListenAndServeContext(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	return s.Serve(ctx, listener)
}

// Serve drains requests before returning. Cancelling the base context also
// releases long-lived SSE subscribers; short global write deadlines would cut
// valid generation streams, so only header and idle deadlines are imposed.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	requests, cancel := context.WithCancel(ctx)
	defer cancel()
	drain := &requestDrain{}
	server := &http.Server{Handler: drain.wrap(s.Handler()), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second,
		BaseContext: func(net.Listener) context.Context { return requests }}
	served := make(chan struct{})
	stopped := make(chan error, 1)
	go func() {
		select {
		case <-ctx.Done():
			cancel()
			deadline, stop := context.WithTimeout(context.Background(), shutdownGrace)
			defer stop()
			err := server.Shutdown(deadline)
			if err != nil {
				_ = server.Close()
				if waitErr := drain.wait(handlerGrace); waitErr != nil {
					log.Printf("关停：%v（存储尚未关闭，请确认没有 handler 卡在写操作上）", waitErr)
					err = errors.Join(err, waitErr)
				}
			}
			stopped <- err
		case <-served:
			stopped <- nil
		}
	}()
	err := server.Serve(listener)
	close(served)
	shutdownErr := <-stopped
	if errors.Is(err, http.ErrServerClosed) {
		return shutdownErr
	}
	return err
}
