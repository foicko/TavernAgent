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
//
// 为什么不是 sync.WaitGroup：WaitGroup 的契约是“计数器为零时开始的 Add 必须发生在
// Wait 之前”。而这里的新请求恰恰可能在 Wait 已经开始时抵达，于是 Add 与内部计数器的
// 读取并发——这正是 `go test -race` 报出来的那种竞争（TestShutdownWaitsForInFlightHandler）。
// 换成 互斥锁 + 条件变量：stop() 一旦置位就再也不会新增计数，wait() 因此变得可靠。
type requestDrain struct {
	mu     sync.Mutex
	cond   *sync.Cond
	active int
	closed bool
}

func newRequestDrain() *requestDrain {
	d := &requestDrain{}
	d.cond = sync.NewCond(&d.mu)
	return d
}

// begin 登记一个在途请求；已进入关停时返回 false（不再接受新请求）。
func (d *requestDrain) begin() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return false
	}
	d.active++
	return true
}

func (d *requestDrain) end() {
	d.mu.Lock()
	d.active--
	if d.active == 0 {
		d.cond.Broadcast()
	}
	d.mu.Unlock()
}

// stop 关闭入口：调用之后 begin 一律失败，wait 的判定才不会被后续 Add 破坏。
func (d *requestDrain) stop() {
	d.mu.Lock()
	d.closed = true
	d.mu.Unlock()
}

func (d *requestDrain) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !d.begin() {
			// http.Server.Shutdown 已停止接受新连接，这里只是兵底：
			// 宁可明确拒绘，也不要在存储已经关闭后让 handler 跑起来。
			http.Error(w, "服务正在关停", http.StatusServiceUnavailable)
			return
		}
		defer d.end()
		next.ServeHTTP(w, r)
	})
}

func (d *requestDrain) wait(timeout time.Duration) error {
	done := make(chan struct{})
	go func() {
		d.mu.Lock()
		for d.active > 0 {
			d.cond.Wait()
		}
		d.mu.Unlock()
		close(done)
	}()
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
	drain := newRequestDrain()
	server := &http.Server{Handler: drain.wrap(s.Handler()), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second,
		BaseContext: func(net.Listener) context.Context { return requests }}
	served := make(chan struct{})
	stopped := make(chan error, 1)
	go func() {
		select {
		case <-ctx.Done():
			cancel()
			// 先关入口再等：之后不可能再有新的 begin，wait 的判定才成立。
			drain.stop()
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
