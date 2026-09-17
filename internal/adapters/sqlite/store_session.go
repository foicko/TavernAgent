package sqlite

import (
	"database/sql"
	"fmt"
	"time"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// ---- sessions & templates ----

func (s *Store) CreateSession(sess *domain.Session, rootNode *domain.PlotNode, branch *domain.Branch, templates []*domain.TemplateVersion) error {
	return s.createSession(sess, rootNode, branch, templates, nil)
}

func (s *Store) CreateSessionWithSnapshot(sess *domain.Session, rootNode *domain.PlotNode, branch *domain.Branch, templates []*domain.TemplateVersion, snapshot *domain.StateSnapshot) error {
	if snapshot == nil || snapshot.NodeID != rootNode.NodeID {
		return fmt.Errorf("initial snapshot must belong to the session root")
	}
	return s.createSession(sess, rootNode, branch, templates, snapshot)
}

func (s *Store) createSession(sess *domain.Session, rootNode *domain.PlotNode, branch *domain.Branch, templates []*domain.TemplateVersion, snapshot *domain.StateSnapshot) error {
	if sess.CharacterID == "" {
		for _, tpl := range templates {
			if tpl.Kind == domain.TemplateCharacter {
				sess.CharacterID = domain.CardIdentity(tpl.Content)
				break
			}
		}
		if sess.CharacterID == "" {
			sess.CharacterID = "legacy_" + sess.SessionID
		}
	}
	if sess.UpdatedAt.IsZero() {
		sess.UpdatedAt = sess.CreatedAt
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO sessions(session_id, root_node_id, title, created_at, ruleset_version, character_id, updated_at) VALUES(?,?,?,?,?,?,?)`,
		sess.SessionID, sess.RootNodeID, sess.Title, sess.CreatedAt.UTC().Format(time.RFC3339Nano), sess.RulesetVersion, sess.CharacterID, sess.UpdatedAt.UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	for _, tpl := range templates {
		if _, err := tx.Exec(`INSERT INTO template_versions(template_version_id, kind, schema_version, content, content_hash, created_at) VALUES(?,?,?,?,?,?)`,
			tpl.TemplateVersionID, string(tpl.Kind), tpl.SchemaVersion, tpl.Content, tpl.ContentHash, s.now()); err != nil {
			return err
		}
		if err := indexLorebookTx(tx, tpl); err != nil {
			return err
		}
	}
	if err := insertNodeTx(tx, rootNode, s.now()); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO branches(branch_id, session_id, name, head_node_id, version, active_turn_id, created_at) VALUES(?,?,?,?,?,?,?)`,
		branch.BranchID, branch.SessionID, branch.Name, branch.HeadNodeID, branch.Version, "", s.now()); err != nil {
		return err
	}
	if snapshot != nil {
		if _, err := tx.Exec(`INSERT INTO state_snapshots(node_id, snapshot_version, ruleset_version, state_json, state_hash) VALUES(?,?,?,?,?)`,
			snapshot.NodeID, snapshot.SnapshotVersion, snapshot.RulesetVersion, snapshot.StateJSON, snapshot.StateHash); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) GetSession(sessionID string) (*domain.Session, error) {
	row := s.rdb().QueryRow(`SELECT session_id, root_node_id, title, created_at, ruleset_version, character_id, updated_at FROM sessions WHERE session_id=?`, sessionID)
	var sess domain.Session
	var at, updated string
	var ruleset sql.NullString
	if err := row.Scan(&sess.SessionID, &sess.RootNodeID, &sess.Title, &at, &ruleset, &sess.CharacterID, &updated); err != nil {
		return nil, mapErr(err)
	}
	sess.CreatedAt = s.parseTime(at)
	sess.UpdatedAt = s.parseTime(updated)
	sess.RulesetVersion = ruleset.String
	return &sess, nil
}

func (s *Store) ListSessions() ([]*domain.Session, error) {
	rows, err := s.rdb().Query(`SELECT session_id, root_node_id, title, created_at, ruleset_version, character_id, updated_at FROM sessions ORDER BY updated_at DESC, session_id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*domain.Session{}
	for rows.Next() {
		var sess domain.Session
		var at, updated string
		var ruleset sql.NullString
		if err := rows.Scan(&sess.SessionID, &sess.RootNodeID, &sess.Title, &at, &ruleset, &sess.CharacterID, &updated); err != nil {
			return nil, err
		}
		sess.CreatedAt = s.parseTime(at)
		sess.UpdatedAt = s.parseTime(updated)
		sess.RulesetVersion = ruleset.String
		out = append(out, &sess)
	}
	return out, rows.Err()
}

// SetSessionRuleset 见端口注释：规则升级的显式入口。
func (s *Store) SetSessionRuleset(sessionID, version string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`UPDATE sessions SET ruleset_version=? WHERE session_id=?`, version, sessionID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func (s *Store) GetTemplateVersion(templateVersionID string) (*domain.TemplateVersion, error) {
	row := s.rdb().QueryRow(`SELECT template_version_id, kind, schema_version, content, content_hash FROM template_versions WHERE template_version_id=?`, templateVersionID)
	var t domain.TemplateVersion
	var kind string
	if err := row.Scan(&t.TemplateVersionID, &kind, &t.SchemaVersion, &t.Content, &t.ContentHash); err != nil {
		return nil, mapErr(err)
	}
	t.Kind = domain.TemplateKind(kind)
	return &t, nil
}
