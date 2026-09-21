// SSE 事件流：回合与会话两个入口共用同一套「订阅 + 对账 + 心跳」循环。
//
// 从 server.go 抽出来：这里的截止时间、Flusher 断言与水位推进是弱网可靠性的
// 关键路径，独立成文件后改动面一目了然。
package http

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"tavernagent/internal/domain"
)

// ---- SSE ----

func (s *Server) turnEvents(w http.ResponseWriter, r *http.Request) {
	turnID := r.PathValue("turnId")
	if _, err := s.turns.Get(turnID); err != nil {
		writeError(w, 404, "NOT_FOUND", "回合不存在", false, "")
		return
	}
	s.eventStream(w, r, turnID)
}

func (s *Server) sessionEvents(w http.ResponseWriter, r *http.Request) {
	if err := s.sessions.RequireCharacter(r.PathValue("id"), ""); err != nil {
		writeAPIError(w, err)
		return
	}
	s.eventStream(w, r, r.PathValue("id"))
}

func (s *Server) eventStream(w http.ResponseWriter, r *http.Request, aggregateID string) {
	controller := http.NewResponseController(w)
	_ = controller.SetWriteDeadline(time.Now().Add(30 * time.Second))
	defer func() { _ = controller.SetWriteDeadline(time.Time{}) }()
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "SSE_UNSUPPORTED", "当前连接不支持流式", false, "")
		return
	}
	after := int64(0)
	if lei := r.Header.Get("Last-Event-ID"); strings.HasPrefix(lei, aggregateID+":") {
		if n, err := strconv.ParseInt(strings.TrimPrefix(lei, aggregateID+":"), 10, 64); err == nil && n > 0 {
			after = n
		}
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(200)
	flusher.Flush()

	// Subscribe before reading. Notifications are only wakeups: every delivery
	// comes from the durable outbox and advances one monotonic watermark.
	sub, closeFn := s.bus.Subscribe(aggregateID, 64)
	defer closeFn()
	drain := func() bool {
		for {
			if r.Context().Err() != nil {
				return false
			}
			events, err := s.bus.Poll(aggregateID, after, 256)
			if err != nil {
				return false
			}
			for _, ev := range events {
				if ev.Sequence <= after {
					continue
				}
				if !s.writeSSE(w, flusher, ev) {
					return false
				}
				after = ev.Sequence
			}
			if len(events) < 256 {
				return true
			}
		}
	}
	if !drain() {
		return
	}
	reconcile := time.NewTicker(time.Second)
	defer reconcile.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-sub.Events:
			// Partial text/thinking is best-effort and has no durable event ID.
			// A completed block still replaces any partial draft after reconnect.
			if ev != nil && ev.Sequence == 0 && (ev.Type == "turn.thinking" || ev.Type == "block.delta") {
				_ = controller.SetWriteDeadline(time.Now().Add(30 * time.Second))
				if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, ev.PayloadJSON); err != nil {
					return
				}
				flusher.Flush()
				continue
			}
			if !drain() {
				return
			}
		case <-reconcile.C:
			if !drain() {
				return
			}
		case <-heartbeat.C:
			_ = controller.SetWriteDeadline(time.Now().Add(30 * time.Second))
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
		}
	}
}

func (s *Server) writeSSE(w http.ResponseWriter, flusher http.Flusher, ev *domain.OutboxEvent) bool {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(30 * time.Second))
	_, err := fmt.Fprintf(w, "event: %s\nid: %s:%d\ndata: %s\n\n", ev.Type, ev.AggregateID, ev.Sequence, ev.PayloadJSON)
	if err != nil {
		return false
	}
	flusher.Flush()
	return true
}
