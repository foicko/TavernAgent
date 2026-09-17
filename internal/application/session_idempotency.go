package application

import (
	"encoding/json"
	"errors"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// The root node is an immutable creation receipt, committed with its snapshot.
// This survives process restarts without a second mutable request cache/table.
func (s *SessionService) findSetup(sessionID, requestHash string) (*SetupResult, error) {
	session, err := s.store.GetSession(sessionID)
	if errors.Is(err, ports.ErrNotFound) || errors.Is(err, domain.ErrItemNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, Err("STORAGE_UNAVAILABLE", "读取开局记录失败", 503)
	}
	root, err := s.store.GetNode(session.RootNodeID)
	if err != nil {
		return nil, Err("STORAGE_UNAVAILABLE", "读取开局节点失败", 503)
	}
	var content struct {
		RequestHash string `json:"setupRequestHash"`
		OpeningText string `json:"openingText"`
	}
	if err := json.Unmarshal([]byte(root.ContentJSON), &content); err != nil {
		return nil, Err("STORAGE_UNAVAILABLE", "开局记录损坏", 503)
	}
	if content.RequestHash != requestHash {
		return nil, Err("IDEMPOTENCY_CONFLICT", "此幂等键已用于其他开局请求", 409)
	}
	snapshot, err := s.store.StateAt(root.NodeID)
	if err != nil {
		return nil, Err("STORAGE_UNAVAILABLE", "读取开局状态失败", 503)
	}
	state, err := domain.UnmarshalWorld(snapshot.StateJSON)
	if err != nil {
		return nil, Err("STORAGE_UNAVAILABLE", "开局状态损坏", 503)
	}
	return &SetupResult{Session: session, RootNode: root, State: state, OpeningText: content.OpeningText,
		Branch: &domain.Branch{BranchID: "branch_main_" + sessionID, SessionID: sessionID, Name: "main", HeadNodeID: root.NodeID}}, nil
}
