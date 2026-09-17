package sqlite

// 用量台账的两个不变量：
//  1. 同进程"写后立读"必须看得见（只读池不得返回陈旧快照）；
//  2. 时间戳平局时按写入先后排序（墙钟粒度粗，平局是常态而非例外）。
//
// 这两条都是回归性质：前者保护双连接池的可见性，后者保护
// "最近调用"视图与调用方对 [0] 的假设。

import (
	"fmt"
	"testing"

	"tavernagent/internal/ports"
)

// 写后立读：走 RecentTurnUsage（只读池）必须立刻看到刚写入的行。
func TestUsageReaderSeesCommittedWrites(t *testing.T) {
	store := newTestStore(t)
	const iterations = 300
	for i := 0; i < iterations; i++ {
		rec := ports.TurnUsageRecord{
			AttemptID: fmt.Sprintf("att_%03d", i), Task: "generation", Outcome: "in_flight",
			// 定宽递增时间戳：保证"最新一条"就是本行，测试才只考验可见性。
			CreatedAt: fmt.Sprintf("2026-01-01T00:00:00.%09dZ", i),
		}
		if err := store.RecordTurnUsage(rec); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
		rows, err := store.RecentTurnUsage(1)
		if err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
		if len(rows) == 0 || rows[0].AttemptID != rec.AttemptID {
			t.Fatalf("第 %d 次写后立读看不到自己：rows=%d", i, len(rows))
		}
	}
}

// created_at 全是同一个串时（墙钟粗粒度下的常态），顺序按写入先后倒序。
// 注意 attempt_id 刻意取成"字母序与写入序相反"的值：
// 若排序回退到按 attempt_id 兜底，这条测试会失败。
func TestUsageTiesOrderByInsertion(t *testing.T) {
	store := newTestStore(t)
	same := "2026-09-15T10:00:00.1234567Z"
	writes := []string{"aaa", "mmm", "zzz", "bbb"}
	for _, attemptID := range writes {
		if err := store.RecordTurnUsage(ports.TurnUsageRecord{
			AttemptID: attemptID, Task: "generation", Outcome: "completed", CreatedAt: same,
		}); err != nil {
			t.Fatalf("write %s: %v", attemptID, err)
		}
	}
	rows, err := store.RecentTurnUsage(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(writes) {
		t.Fatalf("行数 = %d, 期望 %d", len(rows), len(writes))
	}
	for i, want := range []string{"bbb", "zzz", "mmm", "aaa"} {
		if rows[i].AttemptID != want {
			t.Fatalf("第 %d 行 = %s, 期望 %s（应按写入先后倒序，而不是 attempt_id 字母序）",
				i, rows[i].AttemptID, want)
		}
	}
}

// 覆盖写（UPSERT）不改变行的写入位次：预留行先写，完成态更新后仍应排在
// 之后写入的其他行之前。这正是"最近调用"视图要的语义。
func TestUsageUpsertKeepsInsertionOrder(t *testing.T) {
	store := newTestStore(t)
	same := "2026-09-15T10:00:00.5000000Z"
	first := ports.TurnUsageRecord{AttemptID: "first", Task: "generation", Outcome: "in_flight", CreatedAt: same}
	if err := store.RecordTurnUsage(first); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordTurnUsage(ports.TurnUsageRecord{AttemptID: "second", Task: "generation", Outcome: "in_flight", CreatedAt: same}); err != nil {
		t.Fatal(err)
	}
	first.Outcome = "completed"
	first.LatencyMS = 12
	if err := store.RecordTurnUsage(first); err != nil {
		t.Fatal(err)
	}
	rows, err := store.RecentTurnUsage(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].AttemptID != "second" || rows[1].AttemptID != "first" {
		t.Fatalf("覆盖写后顺序 = %+v, 期望 [second first]", rows)
	}
	if rows[1].Outcome != "completed" {
		t.Fatalf("覆盖写未生效: %+v", rows[1])
	}
}
