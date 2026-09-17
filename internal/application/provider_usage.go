package application

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"tavernagent/internal/ports"
	"tavernagent/internal/util/id"
)

// observedProvider covers every configured inference path, including probes,
// director discussion, reflection and summary tasks. It never records prompts
// or credentials. A started row survives an interrupted process.
type observedProvider struct {
	ports.ModelProvider
	config ports.ProviderConfig
	store  ports.UsageStore
	// onWriteFailure 在台账写入失败时被调用（计入 RuntimeMetrics）。
	// 台账是观测数据，写失败不阻断回合，但必须留得下读数。
	onWriteFailure func()
}

func (p *observedProvider) Stream(ctx context.Context, req ports.ChatRequest, sink ports.StreamSink) (streamErr error) {
	if p.store == nil {
		return p.ModelProvider.Stream(ctx, req, sink)
	}
	config := p.config
	config.APIKey = ""
	config.HasAPIKey = false
	raw, _ := json.Marshal(config)
	callID := req.AttemptID
	if callID == "" {
		callID = "call_" + id.New()
	}
	task := req.Task
	if task == "" {
		task = "generation"
	}
	started := time.Now()
	record := ports.TurnUsageRecord{TurnID: req.TurnID, AttemptID: callID, Task: task, Slot: config.Slot, Model: config.Model, Provider: config.Kind,
		Estimated: req.EstimatedInputTokens, ConfigFingerprint: hashString(string(raw)), CreatedAt: started.UTC().Format(time.RFC3339Nano), FirstTokenMS: -1, Outcome: "in_flight"}
	write := func() {
		if err := p.store.RecordTurnUsage(record); err != nil {
			slog.Warn("model usage recording failed", "callId", callID, "error", err)
			if p.onWriteFailure != nil {
				p.onWriteFailure()
			}
		}
	}
	write()
	var mu sync.Mutex
	defer func() {
		mu.Lock()
		defer mu.Unlock()
		record.LatencyMS = time.Since(started).Milliseconds()
		record.Outcome = "completed"
		if streamErr != nil {
			record.Outcome = "failed"
			if errors.Is(streamErr, context.Canceled) {
				record.Outcome = "cancelled"
			}
		}
		write()
	}()
	return p.ModelProvider.Stream(ctx, req, ports.StreamSink{
		OnChunk: func(chunk []byte) error {
			mu.Lock()
			if len(chunk) > 0 && record.FirstTokenMS < 0 {
				record.FirstTokenMS = time.Since(started).Milliseconds()
			}
			mu.Unlock()
			return sink.Chunk(chunk)
		},
		OnUsage: func(usage ports.TokenUsage) {
			mu.Lock()
			record.Prompt = usage.Prompt
			record.Completion = usage.Completion
			record.Cached = usage.Cached
			record.Reported = usage.Reported
			mu.Unlock()
			sink.Usage(usage)
		},
	})
}

func (m *ProviderManager) SetUsageStore(store ports.UsageStore) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.usage = store
	m.clients = map[string]ports.ModelProvider{}
}

// SetUsageFailureSink 注册"台账写入失败"的接收器（组合根接 RuntimeMetrics）。
// 与 SetUsageStore 一样作废已构建客户端，确保新的观测包装带上该回调。
func (m *ProviderManager) SetUsageFailureSink(sink func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.usageFailureSink = sink
	m.clients = map[string]ports.ModelProvider{}
}
