package sqlite

import (
	"encoding/json"
	"tavernagent/internal/domain"
)

type memoryMetadata struct {
	Evidence   *domain.MemoryEvidence `json:"evidence,omitempty"`
	MergedFrom []string               `json:"mergedFrom,omitempty"`
	SecretID   string                 `json:"secretId,omitempty"`
}

func memoryMetadataJSON(m *domain.MemoryRecord) string {
	b, _ := json.Marshal(memoryMetadata{m.Evidence, m.MergedFrom, m.SecretID})
	return string(b)
}

func decodeMemoryMetadata(m *domain.MemoryRecord, raw string) error {
	var metadata memoryMetadata
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return err
	}
	m.Evidence, m.MergedFrom, m.SecretID = metadata.Evidence, metadata.MergedFrom, metadata.SecretID
	return nil
}
