package application

import (
	"errors"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

func requireCharacter(store ports.SessionStore, sessionID, expected string) (*domain.Session, error) {
	sess, err := store.GetSession(sessionID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) || errors.Is(err, domain.ErrItemNotFound) {
			return nil, Err("NOT_FOUND", "会话不存在", 404)
		}
		return nil, Err("STORAGE_UNAVAILABLE", "读取会话归属失败", 503)
	}
	if expected != "" && expected != sess.CharacterID {
		return nil, Err("CHAR_SESSION_MISMATCH", "角色与会话归属不一致，请重新选择该角色的会话", 400)
	}
	return sess, nil
}

func (s *SessionService) RequireCharacter(sessionID, expected string) error {
	_, err := requireCharacter(s.store, sessionID, expected)
	return err
}
