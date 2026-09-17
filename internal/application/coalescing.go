package application

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type queuedWork struct {
	pending func(context.Context)
	cancel  context.CancelFunc
}

// coalescingQueue keeps one worker and one latest pending task per branch.
// Different branches share a bounded concurrency budget; closing prevents work
// from outliving the database.
//
// cancelRunning 决定"新任务到达时如何对待在途任务"，两种语义都是对的，
// 取决于产物是否可被取代：
//   - true（记忆抽取）：最新状态胜出，在途结果已被取代，取消它省一次费用。
//   - false（摘要维护）：产物按不可变区间键控，晚到一样有效；取消它只会
//     让摘要永远完不成（实测 12 次调用被取消 9 次），因此只合并 pending。
type coalescingQueue struct {
	mu            sync.Mutex
	jobs          map[string]*queuedWork
	ctx           context.Context
	cancel        context.CancelFunc
	sem           chan struct{}
	wg            sync.WaitGroup
	closed        bool
	debounce      time.Duration
	cancelRunning bool
	// metrics 是可选的运行读数：关闭时丢弃的 pending 与关闭后提交都要留痕，
	// 否则"这一轮的记忆抽取没发生"在运维侧完全不可见。
	metrics *RuntimeMetrics
}

func newCoalescingQueue(concurrency int, debounce time.Duration, cancelRunning bool) *coalescingQueue {
	ctx, cancel := context.WithCancel(context.Background())
	return &coalescingQueue{jobs: map[string]*queuedWork{}, ctx: ctx, cancel: cancel, sem: make(chan struct{}, concurrency), debounce: debounce, cancelRunning: cancelRunning}
}

func (q *coalescingQueue) Submit(key string, work func(context.Context)) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		q.metrics.addQueueRejected()
		slog.Warn("background task rejected after shutdown", "key", key)
		return
	}
	if job := q.jobs[key]; job != nil {
		job.pending = work
		if job.cancel != nil && q.cancelRunning {
			job.cancel()
		}
		return
	}
	q.jobs[key] = &queuedWork{pending: work}
	q.wg.Add(1)
	go q.run(key)
}

func (q *coalescingQueue) run(key string) {
	defer q.wg.Done()
	for {
		q.mu.Lock()
		job := q.jobs[key]
		if q.closed || job.pending == nil {
			if q.closed && job.pending != nil {
				q.metrics.addQueueDropped()
				slog.Warn("background task dropped at shutdown", "key", key)
			}
			delete(q.jobs, key)
			q.mu.Unlock()
			return
		}
		work := job.pending
		job.pending = nil
		ctx, cancel := context.WithTimeout(q.ctx, 90*time.Second)
		job.cancel = cancel
		q.mu.Unlock()
		timer := time.NewTimer(q.debounce)
		select {
		case <-ctx.Done():
		case <-timer.C:
		}
		timer.Stop()
		if ctx.Err() == nil {
			select {
			case q.sem <- struct{}{}:
				if ctx.Err() == nil {
					work(ctx)
				}
				<-q.sem
			case <-ctx.Done():
			}
		}
		cancel()
	}
}

func (q *coalescingQueue) Close() {
	if q == nil {
		return
	}
	q.mu.Lock()
	q.closed = true
	q.cancel()
	q.mu.Unlock()
	q.wg.Wait()
}
