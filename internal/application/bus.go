package application

import (
	"encoding/json"
	"sync"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"tavernagent/internal/util/id"
)

// Subscription 是 SSE 订阅者通道。
type Subscription struct {
	Events chan *domain.OutboxEvent
	Stop   chan struct{}
	done   chan struct{}
}

// EventBus 在内存中向回合订阅者广播已持久化的 outbox 事件。
// 持久写在 store；广播只是投递通知（Last-Event-ID 从存储重放，不依赖内存）。
type EventBus struct {
	store ports.OutboxStore
	mu    sync.RWMutex
	subs  map[string]map[*Subscription]struct{} // key: aggregateID(turnId)
}

// Poll 读取某聚合的出站事件（SSE 断线恢复用）。
func (b *EventBus) Poll(aggregateID string, afterSequence int64, limit int) ([]*domain.OutboxEvent, error) {
	return b.store.PollOutbox(aggregateID, afterSequence, limit)
}

func NewEventBus(store ports.Store) *EventBus {
	return &EventBus{store: store, subs: map[string]map[*Subscription]struct{}{}}
}

// Publish 持久化 outbox 事件并广播给订阅者。
func (b *EventBus) Publish(aggregateID, typ string, payload any) (*domain.OutboxEvent, error) {
	pb, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	ev := &domain.OutboxEvent{EventID: id.New(), AggregateID: aggregateID, Type: typ, PayloadJSON: string(pb)}
	if err := b.store.AppendOutbox([]*domain.OutboxEvent{ev}); err != nil {
		return nil, err
	}
	b.BroadcastOnly(ev)
	return ev, nil
}

// BroadcastOnly 只向内存订阅者投递（用于 commit 事件——其行已在提交事务内写入）。
func (b *EventBus) BroadcastOnly(ev *domain.OutboxEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	set, ok := b.subs[ev.AggregateID]
	if !ok {
		return
	}
	for sub := range set {
		select {
		case sub.Events <- ev:
		case <-sub.Stop:
		default: // 订阅者慢，跳过该事件（可通过重放补回）
		}
	}
}

// Subscribe 订阅某 turn 的事件；返回订阅与关闭函数。
// 调用方应先订阅，再通过 Poll 重放持久事件，避免两者之间的投递空隙。
// 广播只用于唤醒读取；持久事件的顺序和序号以 Poll 结果为准。
func (b *EventBus) Subscribe(aggregateID string, buffer int) (*Subscription, func()) {
	sub := &Subscription{Events: make(chan *domain.OutboxEvent, buffer), Stop: make(chan struct{}), done: make(chan struct{})}
	b.mu.Lock()
	if b.subs[aggregateID] == nil {
		b.subs[aggregateID] = map[*Subscription]struct{}{}
	}
	b.subs[aggregateID][sub] = struct{}{}
	b.mu.Unlock()

	var once sync.Once
	closeFn := func() {
		once.Do(func() {
			b.mu.Lock()
			if set, ok := b.subs[aggregateID]; ok {
				delete(set, sub)
				if len(set) == 0 {
					delete(b.subs, aggregateID)
				}
			}
			b.mu.Unlock()
			close(sub.Stop)
			close(sub.done)
		})
	}
	return sub, closeFn
}
