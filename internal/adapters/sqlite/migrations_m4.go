package sqlite

import (
	"encoding/json"
	"tavernagent/internal/domain"
)

const schemaV9 = `
ALTER TABLE sessions ADD COLUMN updated_at TEXT NOT NULL DEFAULT '';
UPDATE sessions SET updated_at = created_at;
CREATE INDEX idx_sessions_character_updated ON sessions(character_id, updated_at DESC);
ALTER TABLE memory_records ADD COLUMN metadata_json TEXT NOT NULL DEFAULT '{}';
CREATE TABLE memory_batches (
 batch_id TEXT PRIMARY KEY,
 session_id TEXT NOT NULL REFERENCES sessions(session_id),
 branch_id TEXT NOT NULL REFERENCES branches(branch_id),
 source_turn_id TEXT NOT NULL DEFAULT '',
 node_id TEXT NOT NULL REFERENCES plot_nodes(node_id),
 payload_hash TEXT NOT NULL,
 created_at TEXT NOT NULL
);
CREATE UNIQUE INDEX idx_cognitive_source ON memory_batches(branch_id, source_turn_id)
 WHERE source_turn_id <> '';
CREATE INDEX idx_turns_result ON turn_requests(result_node_id);
`

// Recover old bindings from the immutable root template, not session titles.
// Unresolvable legacy data gets an isolated identity rather than bypassing checks.
func (s *Store) backfillSessionIdentities() error {
	rows, err := s.db.Query(`SELECT s.session_id, n.content_json FROM sessions s
	 JOIN plot_nodes n ON n.node_id=s.root_node_id WHERE s.character_id=''`)
	if err != nil {
		return err
	}
	type pending struct{ id, content string }
	var items []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.content); err != nil {
			rows.Close()
			return err
		}
		items = append(items, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, p := range items {
		var root struct {
			CardRef string `json:"cardRef"`
		}
		_ = json.Unmarshal([]byte(p.content), &root)
		var raw string
		_ = s.db.QueryRow(`SELECT content FROM template_versions WHERE template_version_id=?`, root.CardRef).Scan(&raw)
		identity := domain.CardIdentity(raw)
		if identity == "" {
			identity = "legacy_" + p.id
		}
		if _, err := s.db.Exec(`UPDATE sessions SET character_id=? WHERE session_id=? AND character_id=''`, identity, p.id); err != nil {
			return err
		}
	}
	return nil
}
