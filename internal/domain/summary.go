package domain

import (
	"crypto/sha256"
	"encoding/hex"
)

func SummarySourceHash(nodes []*PlotNode) string {
	h := sha256.New()
	for _, n := range nodes {
		h.Write([]byte(n.NodeID))
		h.Write([]byte(n.ContentJSON))
	}
	return hex.EncodeToString(h.Sum(nil))
}
