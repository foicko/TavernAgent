package sqlite

import (
	"database/sql"
	"errors"
	"time"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// maxCards 是卡库硬上限。卡是用户资产，超过时显式拒绝（而不是像浏览器缓存
// 那样静默淘汰），让用户自己决定移除哪些。
const maxCards = 256

// ---- 角色卡库（schemaV16）----
//
// 卡库与会话模板分表：本表的行可以被用户删除，而 template_versions 一旦被
// 会话引用就不可变。删除卡库行不级联、不触碰模板——已有故事继续可玩。

// ListCards 返回全部卡（最近更新在前）。为避免把每张卡的兆级 JSON 一次性
// 装进内存并下发给前端，这里显式不取 character_json。
func (s *Store) ListCards() ([]*domain.CharacterCardEntry, error) {
	rows, err := s.rdb().Query(`SELECT card_id, name, short_name, avatar, format, role, content_hash, created_at, updated_at, last_used_at
		FROM character_cards ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.CharacterCardEntry
	for rows.Next() {
		var c domain.CharacterCardEntry
		var created, updated, used string
		if err := rows.Scan(&c.CardID, &c.Name, &c.ShortName, &c.Avatar, &c.Format, &c.Role, &c.ContentHash, &created, &updated, &used); err != nil {
			return nil, err
		}
		c.CreatedAt = s.parseTime(created)
		c.UpdatedAt = s.parseTime(updated)
		if used != "" {
			if t := s.parseTime(used); !t.IsZero() {
				c.LastUsedAt = &t
			}
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

// GetCard 返回单张卡的完整内容；不存在时返回 ports.ErrNotFound。
func (s *Store) GetCard(cardID string) (*domain.CharacterCardEntry, error) {
	row := s.rdb().QueryRow(`SELECT card_id, name, short_name, avatar, format, role, character_json, content_hash, created_at, updated_at, last_used_at
		FROM character_cards WHERE card_id=?`, cardID)
	c, err := s.scanCard(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ports.ErrNotFound
	}
	return c, err
}

// SaveCard 以 card_id 为准 upsert：重复导入同一张卡是覆盖，不产生副本。
// created_at 保留首次导入时间，只更新内容与 updated_at。
func (s *Store) SaveCard(card *domain.CharacterCardEntry) error {
	if card == nil || card.CardID == "" {
		return errors.New("card: empty card id")
	}
	created, updated := s.now(), s.now()
	if !card.CreatedAt.IsZero() {
		created = card.CreatedAt.UTC().Format(time.RFC3339Nano)
	}
	if !card.UpdatedAt.IsZero() {
		updated = card.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	used := ""
	if card.LastUsedAt != nil {
		used = card.LastUsedAt.UTC().Format(time.RFC3339Nano)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// 新卡先查上限；已存在的卡（同 card_id）是更新，不受上限影响。
	var existing int
	err := s.db.QueryRow(`SELECT 1 FROM character_cards WHERE card_id=?`, card.CardID).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		var count int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM character_cards`).Scan(&count); err != nil {
			return err
		}
		if count >= maxCards {
			return ports.ErrCardLibraryFull
		}
	} else if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO character_cards
		(card_id, name, short_name, avatar, format, role, character_json, content_hash, created_at, updated_at, last_used_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(card_id) DO UPDATE SET
			name=excluded.name, short_name=excluded.short_name, avatar=excluded.avatar,
			format=excluded.format, role=excluded.role, character_json=excluded.character_json,
			content_hash=excluded.content_hash, updated_at=excluded.updated_at,
			last_used_at=excluded.last_used_at`,
		card.CardID, card.Name, card.ShortName, card.Avatar, card.Format, card.Role, card.CharacterJSON,
		card.ContentHash, created, updated, used)
	return err
}

// DeleteCard 移除一张卡；不存在时返回 ports.ErrNotFound（调用方按幂等处理）。
func (s *Store) DeleteCard(cardID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`DELETE FROM character_cards WHERE card_id=?`, cardID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ports.ErrNotFound
	}
	return nil
}

// TouchCard 记录最近一次用于创建会话的时间。
// 卡不存在时返回 ErrNotFound，但调用方通常把它当作 best-effort（不影响建会话）。
func (s *Store) TouchCard(cardID string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`UPDATE character_cards SET last_used_at=? WHERE card_id=?`,
		at.UTC().Format(time.RFC3339Nano), cardID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func (s *Store) scanCard(row *sql.Row) (*domain.CharacterCardEntry, error) {
	var c domain.CharacterCardEntry
	var created, updated, used string
	if err := row.Scan(&c.CardID, &c.Name, &c.ShortName, &c.Avatar, &c.Format, &c.Role, &c.CharacterJSON,
		&c.ContentHash, &created, &updated, &used); err != nil {
		return nil, err
	}
	c.CreatedAt = s.parseTime(created)
	c.UpdatedAt = s.parseTime(updated)
	if used != "" {
		if t := s.parseTime(used); !t.IsZero() {
			c.LastUsedAt = &t
		}
	}
	return &c, nil
}
