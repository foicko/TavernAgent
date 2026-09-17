package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/ports"
)

func TestObservedProviderRecordsEveryTaskAndRedactsConfiguration(t *testing.T) {
	store, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	config := ports.ProviderConfig{Slot: "reflection", Kind: "mock", Model: "fixture", APIKey: "never-persist-this-key", HasAPIKey: true}
	provider := &observedProvider{store: store, config: config, ModelProvider: cognitiveProviderFunc(func(_ context.Context, r ports.ChatRequest, sink ports.StreamSink) error {
		rows, err := store.RecentTurnUsage(20)
		if err != nil {
			t.Fatalf("read usage ledger: %v", err)
		}
		// 预留行必须在调用模型之前就已落库（进程中断也留得下痕迹）。
		// 这里按 attempt_id 精确查自己那一行：不能假设 rows[0] 就是本次调用——
		// Windows 时钟粒度约 15ms，相邻任务会拿到同一个 created_at，
		// 平局时按写入先后排序，而"本次调用"与"上一个任务"可能挤在同一毫秒里。
		found := false
		for _, row := range rows {
			if row.AttemptID == r.AttemptID {
				if row.Outcome != "in_flight" {
					t.Fatalf("尝试 %s 的预留行状态应为 in_flight，实际 %q", r.AttemptID, row.Outcome)
				}
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("缺少 %s 的预留行（应在其调用模型前写入）", r.AttemptID)
		}
		if err := sink.Chunk([]byte("fixture")); err != nil {
			return err
		}
		sink.Usage(ports.TokenUsage{Prompt: 17, Completion: 9, Reported: true})
		if r.Task == "failed" {
			return errors.New("provider unavailable")
		}
		if r.Task == "cancelled" {
			return context.Canceled
		}
		return nil
	})}
	for _, task := range []string{"primary", "continuation", "director", "reflection", "summary", "probe", "failed", "cancelled"} {
		err := provider.Stream(context.Background(), ports.ChatRequest{Task: task, AttemptID: task, EstimatedInputTokens: 20}, ports.StreamSink{})
		if (task == "failed" || task == "cancelled") != (err != nil) {
			t.Fatalf("task=%s err=%v", task, err)
		}
	}
	rows, err := store.RecentTurnUsage(20)
	if err != nil || len(rows) != 8 {
		t.Fatalf("expected one row per call, got %d: %v", len(rows), err)
	}
	for _, row := range rows {
		if row.ConfigFingerprint == "" || strings.Contains(row.ConfigFingerprint, config.APIKey) || row.Slot != "reflection" || !row.Reported || row.Prompt != 17 || row.Completion != 9 || row.FirstTokenMS < 0 {
			t.Fatalf("invalid observation: %+v", row)
		}
		want := "completed"
		if row.Task == "failed" || row.Task == "cancelled" {
			want = row.Task
		}
		if row.Outcome != want {
			t.Fatalf("task=%s outcome=%s", row.Task, row.Outcome)
		}
	}
	original := rows[0].ConfigFingerprint
	provider.config.APIKey = "a-different-credential"
	if err := provider.Stream(context.Background(), ports.ChatRequest{Task: "probe", AttemptID: "new-key"}, ports.StreamSink{}); err != nil {
		t.Fatal(err)
	}
	rows, _ = store.RecentTurnUsage(1)
	if rows[0].ConfigFingerprint != original {
		t.Fatal("credential leaked into configuration fingerprint")
	}
}

// 台账写入失败必须留下读数（不参与业务判定，但不能只剩一行日志）。
type failingUsageStore struct{}

func (failingUsageStore) RecordTurnUsage(ports.TurnUsageRecord) error { return errors.New("disk full") }
func (failingUsageStore) RecentTurnUsage(int) ([]ports.TurnUsageRecord, error) {
	return nil, nil
}
func (failingUsageStore) TurnUsageTotals() (ports.UsageTotals, error) {
	return ports.UsageTotals{}, nil
}

func TestUsageWriteFailureIsCounted(t *testing.T) {
	metrics := &RuntimeMetrics{}
	provider := &observedProvider{
		config:         ports.ProviderConfig{Slot: "primary", Kind: "mock", Model: "fixture"},
		store:          failingUsageStore{},
		onWriteFailure: metrics.UsageWriteFailed,
		ModelProvider: cognitiveProviderFunc(func(context.Context, ports.ChatRequest, ports.StreamSink) error {
			return nil
		}),
	}
	if err := provider.Stream(context.Background(), ports.ChatRequest{Task: "primary", AttemptID: "att_fail"}, ports.StreamSink{}); err != nil {
		t.Fatalf("台账写失败不应影响调用结果: %v", err)
	}
	// 预留与收尾各写一次，两次都失败。
	if got := metrics.Snapshot().UsageWriteFailures; got != 2 {
		t.Fatalf("usageWriteFailures = %d, 期望 2", got)
	}
}
