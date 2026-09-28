package providerutil

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
)

// ReadSSE 把 SSE 响应体拆成事件流。
//
// 三条协议（openai / responses / anthropic）此前各有一份逐行读取 goroutine，
// 语义（多行 data: 以 \n 拼接、空行分帧、EOF 处末行、漏发空行的网关兼容、
// ctx 取消即停）完全一致，差异只在各自如何解释 payload。集中到这里后只维护一处。
//
// 返回值：
//   - events：按到达顺序派发；ctx 结束或读取结束/出错时关闭。
//   - errs：读取错误（缓冲 1 的非阻塞投递），EOF 不算错误。
//
// 调用方在主循环里 select events / errs / 各超时；events 关闭即表示流结束。
func ReadSSE(ctx context.Context, body io.Reader) (<-chan SSEEvent, <-chan error) {
	events := make(chan SSEEvent, 16)
	errs := make(chan error, 1)
	go func() {
		defer close(events)
		br := bufio.NewReader(body)
		var currentEvent string
		// SSE 规范允许一个事件跨多行 data:（以空行结束）：累积到空行再拼接派发。
		var pending [][]byte
		flush := func() bool {
			if len(pending) == 0 {
				return true
			}
			data := bytes.Join(pending, []byte("\n"))
			pending = pending[:0]
			select {
			case events <- SSEEvent{Event: currentEvent, Data: data}:
				return true
			case <-ctx.Done():
				return false
			}
		}
		for {
			line, err := br.ReadBytes('\n')
			// EOF 时 ReadBytes 仍会返回末尾未以换行结束的最后一行：先处理它，
			// 下一轮读取再以 (nil, EOF) 退出，否则网关不补结尾换行时最后一个事件会被丢掉。
			if err != nil && (err != io.EOF || len(line) == 0) {
				if err != io.EOF {
					select {
					case errs <- err:
					default:
					}
				} else {
					flush()
				}
				return
			}
			t := bytes.TrimSpace(line)
			if len(t) == 0 {
				// 空行：一个 SSE 事件结束（多行 data: 在此拼接派发）。
				if !flush() {
					return
				}
				currentEvent = ""
				continue
			}
			if bytes.HasPrefix(t, []byte("event:")) {
				currentEvent = strings.TrimSpace(string(t[len("event:"):]))
				continue
			}
			if !bytes.HasPrefix(t, []byte("data:")) {
				continue
			}
			data := bytes.TrimSpace(t[len("data:"):])
			if len(data) == 0 {
				continue
			}
			// 兼容漏发空行的网关：上一段已是完整 JSON 时先派发，避免把多个
			// 独立事件错误拼成一段。
			if len(pending) > 0 && json.Valid(bytes.Join(pending, []byte("\n"))) {
				if !flush() {
					return
				}
			}
			pending = append(pending, data)
		}
	}()
	return events, errs
}
