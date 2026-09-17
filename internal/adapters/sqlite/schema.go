package sqlite

// migrations 是 schema 变更序列，以 PRAGMA user_version 追踪。
var migrations = []string{
	schemaV1,
	schemaV2,
	schemaV3,
	schemaV4,
	schemaV5,
	schemaV6,
	schemaV7,
	schemaV8,
	schemaV9,
	schemaV10,
	schemaV11,
	schemaV12,
	schemaV13,
	schemaV14,
	schemaV15,
}

// schemaV8：会话归属角色卡（M4l 双向硬绑定，契约 §11.2.1）。
//
// character_id 让"角色 ⇄ 会话"在持久层成为强绑定：会话创建时由角色卡
// 的稳定 ID 填充；回合受理方据此做双向校验（CHAR_SESSION_MISMATCH）。
// 旧会话迁移后为空串——校验方对空值放行（向后兼容，不阻断既有存档）。
// 索引支撑"按角色列会话"（前端切换角色卡时自愈装载专属时间线）。
const schemaV8 = `
ALTER TABLE sessions ADD COLUMN character_id TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_sessions_character ON sessions(character_id);
`

// schemaV3：记忆修订改为 copy-on-write。
// 新增 supersedes（新记录指向被取代的旧记录），去掉 superseded_by
// （旧模型把链接写在共享的旧记录上，会让修订泄漏到其它分支，违反 T17）。
const schemaV3 = `
ALTER TABLE memory_records ADD COLUMN supersedes TEXT NOT NULL DEFAULT '';
ALTER TABLE memory_records DROP COLUMN superseded_by;
CREATE INDEX idx_memory_supersedes ON memory_records(supersedes);
`

// schemaV2：branches 补 created_at（ListBranches 排序依赖；旧库经迁移补齐）。
const schemaV2 = `
ALTER TABLE branches ADD COLUMN created_at TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_branches_created ON branches(created_at);
`

const schemaV1 = `
CREATE TABLE sessions (
  session_id   TEXT PRIMARY KEY,
  root_node_id TEXT NOT NULL,
  title        TEXT NOT NULL DEFAULT '',
  created_at   TEXT NOT NULL
);

CREATE TABLE template_versions (
  template_version_id TEXT PRIMARY KEY,
  kind                TEXT NOT NULL,
  schema_version      INTEGER NOT NULL,
  content             TEXT NOT NULL,
  content_hash        TEXT NOT NULL,
  created_at          TEXT NOT NULL
);

CREATE TABLE plot_nodes (
  node_id        TEXT PRIMARY KEY,
  session_id     TEXT NOT NULL REFERENCES sessions(session_id),
  parent_id      TEXT,
  kind           TEXT NOT NULL,
  depth          INTEGER NOT NULL,
  turn_number    INTEGER NOT NULL,
  schema_version INTEGER NOT NULL,
  content_json   TEXT NOT NULL,
  created_at     TEXT NOT NULL
);
CREATE INDEX idx_nodes_parent ON plot_nodes(session_id, parent_id);

CREATE TABLE branches (
  branch_id      TEXT PRIMARY KEY,
  session_id     TEXT NOT NULL REFERENCES sessions(session_id),
  name           TEXT NOT NULL,
  head_node_id   TEXT NOT NULL REFERENCES plot_nodes(node_id),
  version        INTEGER NOT NULL,
  active_turn_id TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_branches_session ON branches(session_id);

CREATE TABLE domain_events (
  event_id        TEXT PRIMARY KEY,
  node_id         TEXT NOT NULL REFERENCES plot_nodes(node_id),
  event_index     INTEGER NOT NULL,
  type            TEXT NOT NULL,
  payload_json    TEXT NOT NULL,
  ruleset_version TEXT NOT NULL DEFAULT '',
  UNIQUE(node_id, event_index)
);
CREATE INDEX idx_events_node ON domain_events(node_id, event_index);

CREATE TABLE state_snapshots (
  node_id          TEXT PRIMARY KEY REFERENCES plot_nodes(node_id),
  snapshot_version INTEGER NOT NULL,
  ruleset_version  TEXT NOT NULL,
  state_json       TEXT NOT NULL,
  state_hash       TEXT NOT NULL
);

CREATE TABLE turn_requests (
  turn_id           TEXT PRIMARY KEY,
  session_id        TEXT NOT NULL,
  branch_id         TEXT NOT NULL,
  idempotency_key   TEXT NOT NULL,
  payload_hash      TEXT NOT NULL,
  expected_head_id  TEXT NOT NULL,
  expected_version  INTEGER NOT NULL,
  after_turn_id     TEXT NOT NULL DEFAULT '',
  status            TEXT NOT NULL,
  mode              TEXT NOT NULL,
  input_json        TEXT NOT NULL DEFAULT '',
  result_node_id    TEXT NOT NULL DEFAULT '',
  failure_code      TEXT NOT NULL DEFAULT '',
  failure_message   TEXT NOT NULL DEFAULT '',
  cancel_requested  INTEGER NOT NULL DEFAULT 0,
  created_at        TEXT NOT NULL,
  updated_at        TEXT NOT NULL,
  UNIQUE(session_id, idempotency_key)
);
CREATE INDEX idx_turns_session ON turn_requests(session_id);

CREATE TABLE turn_attempts (
  attempt_id     TEXT PRIMARY KEY,
  turn_id        TEXT NOT NULL REFERENCES turn_requests(turn_id),
  attempt_no     INTEGER NOT NULL,
  base_head_id   TEXT NOT NULL,
  base_version   INTEGER NOT NULL,
  config_version TEXT NOT NULL DEFAULT '',
  created_at     TEXT NOT NULL
);

CREATE TABLE draft_frames (
  attempt_id   TEXT NOT NULL,
  frame_seq    INTEGER NOT NULL,
  payload      TEXT NOT NULL,
  payload_hash TEXT NOT NULL,
  PRIMARY KEY (attempt_id, frame_seq)
);

CREATE TABLE action_receipts (
  receipt_id      TEXT PRIMARY KEY,
  turn_id         TEXT NOT NULL,
  action_id       TEXT NOT NULL,
  base_head_id    TEXT NOT NULL,
  ruleset_version TEXT NOT NULL,
  result_json     TEXT NOT NULL,
  status          TEXT NOT NULL
);

CREATE TABLE memory_records (
  memory_id      TEXT PRIMARY KEY,
  source_node_id TEXT NOT NULL REFERENCES plot_nodes(node_id),
  kind           TEXT NOT NULL,
  owner_ids      TEXT NOT NULL DEFAULT '[]',
  content        TEXT NOT NULL,
  entity_ids     TEXT NOT NULL DEFAULT '[]',
  confidence     REAL NOT NULL DEFAULT 0,
  pinned         INTEGER NOT NULL DEFAULT 0,
  superseded_by  TEXT NOT NULL DEFAULT '',
  hidden         INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_memory_source ON memory_records(source_node_id);

CREATE TABLE summary_artifacts (
  summary_id       TEXT PRIMARY KEY,
  from_node_id     TEXT NOT NULL,
  to_node_id       TEXT NOT NULL,
  source_hash      TEXT NOT NULL,
  visibility_scope TEXT NOT NULL DEFAULT '',
  text             TEXT NOT NULL
);

CREATE TABLE bookmarks (
  bookmark_id TEXT PRIMARY KEY,
  session_id  TEXT NOT NULL,
  node_id     TEXT NOT NULL,
  title       TEXT NOT NULL DEFAULT ''
);

CREATE TABLE outbox_events (
  event_id     TEXT PRIMARY KEY,
  aggregate_id TEXT NOT NULL,
  sequence     INTEGER NOT NULL,
  type         TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  created_at   TEXT NOT NULL,
  UNIQUE(aggregate_id, sequence)
);
CREATE INDEX idx_outbox_agg ON outbox_events(aggregate_id, sequence);
`

// schemaV4：规则版本下沉到会话与回合（M4 前置）。
//
// 此前所有提交都写编译期常量 RulesetVersion，规则升级无法区分新旧数据。
// 空串表示迁移前的旧数据，读取方按"未记录"处理并回退到当前常量。
const schemaV4 = `
ALTER TABLE sessions ADD COLUMN ruleset_version TEXT NOT NULL DEFAULT '';
ALTER TABLE turn_requests ADD COLUMN ruleset_version TEXT NOT NULL DEFAULT '';
`

// schemaV5：记忆检索（M4a）。
//
//   - importance 是 1~10 的重要度，参与评分（契约 §8.1 的 importance01）
//     与 recency 半衰期分档；默认 5（中档）。
//   - memory_mentions 记录「某个已提交节点明确提到了某条记忆」。
//     存在这里而不是回写 memory_records.last_mention_turn：记忆记录是共享的，
//     回写会让 A 分支的提及抬高 B 分支的 recency，泄漏分支隔离（T16/T17）。
//     lastMention 是**路径**的属性，所以锚在节点上。
const schemaV5 = `
ALTER TABLE memory_records ADD COLUMN importance INTEGER NOT NULL DEFAULT 5;

CREATE TABLE memory_mentions (
  memory_id   TEXT NOT NULL,
  node_id     TEXT NOT NULL REFERENCES plot_nodes(node_id),
  turn_number INTEGER NOT NULL,
  PRIMARY KEY (memory_id, node_id)
);
CREATE INDEX idx_memory_mentions_node ON memory_mentions(node_id);
`

// schemaV6：动作收据带行动实例标识（M4c）。
//
// roll_id 让"同一个行动实例"可被多个回合引用（重生成会为新的 turnId 建
// 独立收据，但结果只掷一次）。它不是 receipt_id 的替代品：receipt_id 标识
// 一条收据行，roll_id 标识一次掷骰。
const schemaV6 = `
ALTER TABLE action_receipts ADD COLUMN roll_id TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_receipts_roll ON action_receipts(roll_id, base_head_id);
`

// schemaV7：摘要产物补模型配置与版本（M4b，契约 §9.2 要求记录）。
const schemaV7 = `
ALTER TABLE summary_artifacts ADD COLUMN model_config_version TEXT NOT NULL DEFAULT '';
ALTER TABLE summary_artifacts ADD COLUMN summary_version INTEGER NOT NULL DEFAULT 1;
`

// schemaV12：记忆主体唯一值约束与时间轴（T3.1 / T3.2）。
//
//   - subject_key 是小写点号分隔的主体键（如 character.clara.affiliation），
//     用于在分支路径上保证同一主体只有一条生效记录。
//   - created_turn / valid_from_turn / valid_until_turn 提供记忆生命周期时间轴，
//     valid_until_turn > 0 表示在指定回合后失效（0 表示永久有效）。
const schemaV12 = `
ALTER TABLE memory_records ADD COLUMN subject_key TEXT NOT NULL DEFAULT '';
ALTER TABLE memory_records ADD COLUMN created_turn INTEGER NOT NULL DEFAULT 0;
ALTER TABLE memory_records ADD COLUMN valid_from_turn INTEGER NOT NULL DEFAULT 0;
ALTER TABLE memory_records ADD COLUMN valid_until_turn INTEGER NOT NULL DEFAULT 0;
CREATE INDEX idx_memory_subject ON memory_records(subject_key);
`
