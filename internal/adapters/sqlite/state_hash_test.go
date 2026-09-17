package sqlite

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"tavernagent/internal/domain"
)

// 指纹只有一种格式（"v2:<sha256>"）。旧格式（无前缀的裸 hex）不再兼容：
// 它由历史版本的 String() 渲染算出，用当前代码复算不出可信结果，
// 与其"猜着放过"，不如明确拒绝——调用方应重新导入或重建状态。
func TestLegacyFingerprintIsRejected(t *testing.T) {
	st := newTestStore(t)
	_, root, _ := seedSession(t, st)
	state := domain.NewWorldState()
	state.Characters["guide"] = domain.CharacterInfo{CharacterID: "guide", Name: "Guide", Attributes: map[string]int{"strength": 12}}
	raw, _ := state.Marshal()

	legacy := sha256.Sum256([]byte("rendered by an older WorldState.String()"))
	if err := st.SaveSnapshot(&domain.StateSnapshot{NodeID: root.NodeID, StateJSON: raw, StateHash: hex.EncodeToString(legacy[:]), SnapshotVersion: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.StateAt(root.NodeID); err == nil {
		t.Fatal("旧格式指纹必须被拒绝，不能静默通过验证")
	}
}

func TestCheckpointDetectsCharacterAttributeCorruption(t *testing.T) {
	st := newTestStore(t)
	_, root, _ := seedSession(t, st)
	state := domain.NewWorldState()
	state.Characters["guide"] = domain.CharacterInfo{CharacterID: "guide", Attributes: map[string]int{"strength": 12}}
	hash := state.HashID()
	state.Characters["guide"].Attributes["strength"] = 99
	raw, _ := state.Marshal()
	if err := st.SaveSnapshot(&domain.StateSnapshot{NodeID: root.NodeID, StateJSON: raw, StateHash: hash, SnapshotVersion: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.StateAt(root.NodeID); err == nil {
		t.Fatal("corrupt attributes escaped checkpoint verification")
	}
}
