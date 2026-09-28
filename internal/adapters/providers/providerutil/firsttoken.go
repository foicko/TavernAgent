package providerutil

import (
	"context"
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

// DoWithinFirstTokenBudget 发出请求，并让首字时限从**发出请求**那一刻起算，
// 而不是从收到响应头起算。
//
// 有些网关会把响应头压到模型吐出首个 token 才发；若首字计时器等 client.Do
// 返回后才启动，卡死的上游就只能等到长得多的总时限才被放弃。
//
// 返回值：响应、收到响应头后首字还剩的预算，以及 timedOut——响应头到达之前预算
// 已耗尽时为 true（此时 resp 与 err 均为 nil）。budget <= 0 表示不设时限。
// 返回的响应体在 Close 时一并取消本次请求的上下文。
func DoWithinFirstTokenBudget(client *http.Client, req *http.Request, budget time.Duration) (resp *http.Response, remaining time.Duration, timedOut bool, err error) {
	if budget <= 0 {
		resp, err = client.Do(req)
		return resp, 0, false, err
	}
	ctx, cancel := context.WithCancel(req.Context())
	var fired atomic.Bool
	timer := time.AfterFunc(budget, func() {
		fired.Store(true)
		cancel()
	})
	start := time.Now()
	resp, err = client.Do(req.WithContext(ctx))
	if !timer.Stop() || fired.Load() {
		// 等响应头期间时限已到（或恰与响应头同时到达）：请求上下文已被取消，响应体不可用。
		if resp != nil {
			_ = resp.Body.Close()
		}
		cancel()
		return nil, 0, true, nil
	}
	if err != nil {
		cancel()
		return nil, 0, false, err
	}
	resp.Body = &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
	remaining = budget - time.Since(start)
	if remaining <= 0 {
		// 响应头恰好卡在时限上到达：交给调用方决定下限，别返回 1ns（会立即误报超时）。
		remaining = 0
	}
	return resp, remaining, false, nil
}

// cancelOnClose 在响应体关闭时释放请求上下文。
type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}
