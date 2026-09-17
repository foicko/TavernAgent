package sqlite

import (
	"encoding/json"
	"fmt"
	"strings"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"tavernagent/internal/search"
)

// Worldbook search is a derived index of immutable templates. It never indexes
// character secrets, and never decides which keyword-triggered entries the
// compiler injects. Template scope is applied before ranking and pagination.
const lorebookSchema = `
CREATE TABLE IF NOT EXISTS lorebook_sources (
 template_version_id TEXT PRIMARY KEY REFERENCES template_versions(template_version_id) ON DELETE CASCADE,
 content_hash TEXT NOT NULL, entry_count INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS lorebook_entries (
 id INTEGER PRIMARY KEY,
 template_version_id TEXT NOT NULL REFERENCES template_versions(template_version_id) ON DELETE CASCADE,
 book_index INTEGER NOT NULL, entry_index INTEGER NOT NULL,
 book_name TEXT NOT NULL, entry_json TEXT NOT NULL, search_text TEXT NOT NULL,
 title_tokens TEXT NOT NULL, keys_tokens TEXT NOT NULL, content_tokens TEXT NOT NULL,
 UNIQUE(template_version_id, book_index, entry_index)
);`

const lorebookTriggers = `
CREATE TRIGGER IF NOT EXISTS lorebook_entries_ai AFTER INSERT ON lorebook_entries BEGIN
 INSERT INTO lorebook_fts(rowid,title,keys,content) VALUES(new.id,new.title_tokens,new.keys_tokens,new.content_tokens);
END;
CREATE TRIGGER IF NOT EXISTS lorebook_entries_ad AFTER DELETE ON lorebook_entries BEGIN
 DELETE FROM lorebook_fts WHERE rowid=old.id;
END;
CREATE TRIGGER IF NOT EXISTS lorebook_entries_au AFTER UPDATE ON lorebook_entries BEGIN
 DELETE FROM lorebook_fts WHERE rowid=old.id;
 INSERT INTO lorebook_fts(rowid,title,keys,content) VALUES(new.id,new.title_tokens,new.keys_tokens,new.content_tokens);
END;`

func (s *Store) ensureLorebookIndex() error {
	if _, err := s.db.Exec(lorebookSchema); err != nil {
		return err
	}
	_, err := s.db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS lorebook_fts USING fts5(title,keys,content,tokenize='porter unicode61 remove_diacritics 2')`)
	s.loreFTS = err == nil
	if s.loreFTS {
		if _, err := s.db.Exec(lorebookTriggers); err != nil {
			return err
		}
	}
	// Repair missing template rows as well as partial derived-index loss.
	rows, err := s.db.Query(`SELECT t.template_version_id,t.content,t.content_hash FROM template_versions t
 LEFT JOIN lorebook_sources x ON x.template_version_id=t.template_version_id
 WHERE t.kind='lorebook' AND (x.template_version_id IS NULL OR x.content_hash<>t.content_hash
 OR x.entry_count<>(SELECT COUNT(*) FROM lorebook_entries e WHERE e.template_version_id=t.template_version_id))`)
	if err != nil {
		return err
	}
	var pending []*domain.TemplateVersion
	for rows.Next() {
		t := &domain.TemplateVersion{Kind: domain.TemplateLorebook}
		if err := rows.Scan(&t.TemplateVersionID, &t.Content, &t.ContentHash); err != nil {
			rows.Close()
			return err
		}
		pending = append(pending, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, t := range pending {
		if err := indexLorebookTx(tx, t); err != nil {
			return err
		}
	}
	if s.loreFTS {
		var total, indexed, aligned int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM lorebook_entries`).Scan(&total); err != nil {
			return err
		}
		if err := tx.QueryRow(`SELECT COUNT(*) FROM lorebook_fts`).Scan(&indexed); err != nil {
			return err
		}
		if err := tx.QueryRow(`SELECT COUNT(*) FROM lorebook_fts f JOIN lorebook_entries e ON e.id=f.rowid`).Scan(&aligned); err != nil {
			return err
		}
		if total != indexed || total != aligned {
			if _, err := tx.Exec(`DELETE FROM lorebook_fts;
 INSERT INTO lorebook_fts(rowid,title,keys,content) SELECT id,title_tokens,keys_tokens,content_tokens FROM lorebook_entries`); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func indexLorebookTx(tx execer, template *domain.TemplateVersion) error {
	if template.Kind != domain.TemplateLorebook {
		return nil
	}
	var books []domain.Lorebook
	if err := json.Unmarshal([]byte(template.Content), &books); err != nil {
		return fmt.Errorf("invalid lorebook template: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM lorebook_entries WHERE template_version_id=?`, template.TemplateVersionID); err != nil {
		return err
	}
	count := 0
	for bi, book := range books {
		for ei, entry := range book.Entries {
			raw, err := json.Marshal(entry)
			if err != nil {
				return err
			}
			keys := strings.Join(entry.Keys, " ") + " " + strings.Join(entry.SecondaryKeys, " ")
			if _, err := tx.Exec(`INSERT INTO lorebook_entries(template_version_id,book_index,entry_index,book_name,entry_json,search_text,title_tokens,keys_tokens,content_tokens)
 VALUES(?,?,?,?,?,?,?,?,?)`, template.TemplateVersionID, bi, ei, book.Name, string(raw),
				strings.ToLower(entry.Title+"\n"+keys+"\n"+entry.Content),
				strings.Join(search.Tokenize(entry.Title), " "), strings.Join(search.Tokenize(keys), " "), strings.Join(search.Tokenize(entry.Content), " ")); err != nil {
				return err
			}
			count++
		}
	}
	_, err := tx.Exec(`INSERT INTO lorebook_sources(template_version_id,content_hash,entry_count) VALUES(?,?,?)
 ON CONFLICT(template_version_id) DO UPDATE SET content_hash=excluded.content_hash,entry_count=excluded.entry_count`, template.TemplateVersionID, template.ContentHash, count)
	return err
}

func (s *Store) SearchLorebook(templateVersionID, text string, offset, limit int) ([]ports.LorebookMatch, error) {
	if limit <= 0 {
		limit = 40
	}
	limit = min(limit, 101) // one extra row permits a bounded hasMore response
	offset = max(offset, 0)
	text = strings.TrimSpace(text)
	expr := search.MatchExpr(text)
	query := `SELECT e.book_index,e.entry_index,e.book_name,e.entry_json FROM lorebook_entries e WHERE e.template_version_id=?`
	args := []any{templateVersionID}
	if text != "" && s.loreFTS && expr != "" {
		query = `SELECT e.book_index,e.entry_index,e.book_name,e.entry_json FROM lorebook_fts CROSS JOIN lorebook_entries e ON e.id=lorebook_fts.rowid
 WHERE lorebook_fts MATCH ? AND e.template_version_id=? ORDER BY bm25(lorebook_fts,4,2,1),e.book_index,e.entry_index`
		args = []any{expr, templateVersionID}
	} else {
		if text != "" {
			query += ` AND instr(e.search_text,?)>0`
			args = append(args, strings.ToLower(text))
		}
		query += ` ORDER BY e.book_index,e.entry_index`
	}
	query += ` LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := s.rdb().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ports.LorebookMatch, 0)
	for rows.Next() {
		var m ports.LorebookMatch
		var raw string
		if err := rows.Scan(&m.BookIndex, &m.EntryIndex, &m.BookName, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &m.Entry); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
