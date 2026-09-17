package sqlite

import (
	"database/sql"
	"strings"
	"tavernagent/internal/ports"
	"time"
)

// schemaV11：模型用量台账（T0.1）。
//
// 设计要点：
//  1. 真实值（prompt/completion/cached）与估算值（estimated）**同表共存**，
//     否则无法发现编译侧 token 估算的漂移——这正是此前压缩阈值与预算裁剪
//     只能靠猜的根因。
//  2. reported 显式区分"供应商没回传"与"真实消耗为 0"，避免把缺数当零。
//  3. 台账是**观测数据**，不参与任何业务判定；写入失败不影响回合提交。
const schemaV11 = `
CREATE TABLE turn_usage (
 turn_id TEXT NOT NULL, attempt_id TEXT NOT NULL, model TEXT NOT NULL DEFAULT '', provider TEXT NOT NULL DEFAULT '',
 prompt_tokens INTEGER NOT NULL DEFAULT 0, completion_tokens INTEGER NOT NULL DEFAULT 0,
 cached_tokens INTEGER NOT NULL DEFAULT 0, estimated_tokens INTEGER NOT NULL DEFAULT 0,
 reported INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL,
 PRIMARY KEY(attempt_id)
);
CREATE INDEX idx_turn_usage_turn ON turn_usage(turn_id);
CREATE INDEX idx_turn_usage_created ON turn_usage(created_at);
`

const schemaV15 = `
ALTER TABLE turn_usage ADD COLUMN task TEXT NOT NULL DEFAULT 'generation';
ALTER TABLE turn_usage ADD COLUMN slot TEXT NOT NULL DEFAULT 'primary';
ALTER TABLE turn_usage ADD COLUMN config_fingerprint TEXT NOT NULL DEFAULT '';
ALTER TABLE turn_usage ADD COLUMN latency_ms INTEGER NOT NULL DEFAULT 0;
ALTER TABLE turn_usage ADD COLUMN first_token_ms INTEGER NOT NULL DEFAULT -1;
ALTER TABLE turn_usage ADD COLUMN outcome TEXT NOT NULL DEFAULT 'completed';
`

// RecordTurnUsage 记录一次调用的用量。同一 attempt 以最后一次为准（续写会更新）。
func (s *Store) RecordTurnUsage(rec ports.TurnUsageRecord) error {
	if strings.TrimSpace(rec.AttemptID) == "" {
		return nil
	}
	created := strings.TrimSpace(rec.CreatedAt)
	if created == "" {
		created = time.Now().UTC().Format(time.RFC3339)
	}
	reported := 0
	if rec.Reported {
		reported = 1
	}
	_, err := s.db.Exec(`INSERT INTO turn_usage
		(turn_id, attempt_id, model, provider, prompt_tokens, completion_tokens, cached_tokens, estimated_tokens, reported, created_at,task,slot,config_fingerprint,latency_ms,first_token_ms,outcome)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(attempt_id) DO UPDATE SET
			turn_id=excluded.turn_id, model=excluded.model, provider=excluded.provider,
			prompt_tokens=excluded.prompt_tokens, completion_tokens=excluded.completion_tokens,
			cached_tokens=excluded.cached_tokens, estimated_tokens=excluded.estimated_tokens,
			reported=excluded.reported, created_at=excluded.created_at,task=excluded.task,slot=excluded.slot,config_fingerprint=excluded.config_fingerprint,
			latency_ms=excluded.latency_ms,first_token_ms=excluded.first_token_ms,outcome=excluded.outcome`,
		rec.TurnID, rec.AttemptID, rec.Model, rec.Provider,
		rec.Prompt, rec.Completion, rec.Cached, rec.Estimated, reported, created, rec.Task, rec.Slot, rec.ConfigFingerprint, rec.LatencyMS, rec.FirstTokenMS, rec.Outcome)
	return err
}

// RecentTurnUsage 返回最近 limit 条记录（按创建时间倒序）。limit<=0 时取 20 条。
//
// created_at 是 wall clock 字符串：Windows 默认时钟粒度约 15ms，同一批调用
// （甚至相邻两个任务）极可能拿到同一个时间戳串。此时用 rowid 兜底，
// 让平局按"插入先后"排——台账是"最近调用"视图，插入顺序才是用户想要的顺序。
// 只按 attempt_id 兜底是不够的：它由调用方命名，可能是任务名而非可比较的 id。
func (s *Store) RecentTurnUsage(limit int) ([]ports.TurnUsageRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.rdb().Query(`SELECT turn_id, attempt_id, model, provider,
		prompt_tokens, completion_tokens, cached_tokens, estimated_tokens, reported, created_at,task,slot,config_fingerprint,latency_ms,first_token_ms,outcome
		FROM turn_usage ORDER BY created_at DESC, rowid DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTurnUsage(rows)
}

// TurnUsageTotals 返回累计用量（观测端点使用）。
func (s *Store) TurnUsageTotals() (ports.UsageTotals, error) {
	var t ports.UsageTotals
	var last sql.NullString
	var reported int
	err := s.rdb().QueryRow(`SELECT COUNT(*), COALESCE(SUM(prompt_tokens),0), COALESCE(SUM(completion_tokens),0),
		COALESCE(SUM(cached_tokens),0), COALESCE(SUM(estimated_tokens),0), COALESCE(SUM(reported),0), MAX(created_at)
		FROM turn_usage`).Scan(&t.Calls, &t.Prompt, &t.Completion, &t.Cached, &t.Estimated, &reported, &last)
	if err != nil {
		return t, err
	}
	t.ReportedCalls = reported
	if last.Valid {
		t.LastRecordedAt = last.String
	}
	return t, nil
}

func scanTurnUsage(rows *sql.Rows) ([]ports.TurnUsageRecord, error) {
	out := []ports.TurnUsageRecord{}
	for rows.Next() {
		var r ports.TurnUsageRecord
		var reported int
		if err := rows.Scan(&r.TurnID, &r.AttemptID, &r.Model, &r.Provider,
			&r.Prompt, &r.Completion, &r.Cached, &r.Estimated, &reported, &r.CreatedAt, &r.Task, &r.Slot, &r.ConfigFingerprint, &r.LatencyMS, &r.FirstTokenMS, &r.Outcome); err != nil {
			return nil, err
		}
		r.Reported = reported != 0
		out = append(out, r)
	}
	return out, rows.Err()
}
