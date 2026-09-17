package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"tavernagent/internal/domain"
	"tavernagent/internal/pack"
	"tavernagent/internal/ports"
)

func TestM4MalformedArchiveNeverPartiallyWrites(t *testing.T) {
	st, svc, _, sid, bid, root := newTestServices(t, happyScript())
	tr, err := svc.Accept(context.Background(), sid, bid, &TurnAcceptRequest{IdempotencyKey: "archive-check", ExpectedHeadID: root, Input: domain.TurnInput{Kind: "action", Text: "仔细观察周围的道路", ActionRef: "check.wisdom"}})
	if err != nil {
		t.Fatal(err)
	}
	tr = waitTurn(t, svc, tr.TurnID, domain.TurnCommitted)
	base, err := st.ExportSession(sid, "")
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(base)
	archive := NewArchiveService(st, "test-m4")
	tests := map[string]func(*domain.SessionBundle){
		"overlay cycle": func(b *domain.SessionBundle) {
			b.Memories = append(b.Memories, &domain.MemoryRecord{MemoryID: "cycle-a", SourceNodeID: root, Supersedes: "cycle-b"}, &domain.MemoryRecord{MemoryID: "cycle-b", SourceNodeID: root, Supersedes: "cycle-a"})
		},
		"future evidence": func(b *domain.SessionBundle) {
			b.Memories = append(b.Memories, &domain.MemoryRecord{MemoryID: "future", SourceNodeID: root, Kind: domain.MemoryObserved, Evidence: &domain.MemoryEvidence{SourceNodeID: tr.ResultNodeID, SourceQuote: "仔细观察周围的道路", Confidence: "high"}})
		},
		"quote without source": func(b *domain.SessionBundle) {
			b.Memories = append(b.Memories, &domain.MemoryRecord{MemoryID: "forged", SourceNodeID: root, Kind: domain.MemoryObserved, Evidence: &domain.MemoryEvidence{SourceQuote: "并不存在的引文", Confidence: "high"}})
		},
		"inference without reasoning": func(b *domain.SessionBundle) {
			b.Memories = append(b.Memories, &domain.MemoryRecord{MemoryID: "inference", SourceNodeID: root, Kind: domain.MemoryInferred, Evidence: &domain.MemoryEvidence{Confidence: "high"}})
		},
		"forged consequence": func(b *domain.SessionBundle) {
			r := b.Receipts[0].Receipt
			cr, _ := r.CheckResultOf()
			cr.PermanentEffect = "凭空获得永久能力"
			raw, _ := json.Marshal(cr)
			r.ResultJSON = string(raw)
		},
		"forged DC": func(b *domain.SessionBundle) {
			r := b.Receipts[0].Receipt
			cr, _ := r.CheckResultOf()
			forged, _ := domain.EvaluateCheck(domain.ActionRule{ActionID: cr.ActionID, Attribute: cr.Attribute, DC: 1}, cr.AttributeVal, cr.Natural)
			forged.RollID, forged.RulesetVer = cr.RollID, cr.RulesetVer
			raw, _ := json.Marshal(forged)
			r.ResultJSON = string(raw)
		},
		"checkpoint disagrees with replay": func(b *domain.SessionBundle) {
			sn, _ := st.StateAt(tr.ResultNodeID)
			ws, _ := domain.UnmarshalWorld(sn.StateJSON)
			ws.Relationships["npc_elena"] = domain.RelationValue{Affection: 99}
			raw, _ := json.Marshal(ws)
			replacement := &domain.StateSnapshot{NodeID: tr.ResultNodeID, StateJSON: string(raw), StateHash: ws.HashID()}
			found := false
			for i, s := range b.Snapshots {
				if s.NodeID == tr.ResultNodeID {
					b.Snapshots[i] = replacement
					found = true
				}
			}
			if !found {
				b.Snapshots = append(b.Snapshots, replacement)
			}
		},
	}
	for name, tamper := range tests {
		t.Run(name, func(t *testing.T) {
			var b domain.SessionBundle
			if err := json.Unmarshal(encoded, &b); err != nil {
				t.Fatal(err)
			}
			tamper(&b)
			var buffer bytes.Buffer
			if _, err := pack.Write(&buffer, &b, "test-m4", time.Now()); err != nil {
				t.Fatal(err)
			}
			before, _ := st.ListSessions()
			_, err := archive.Import(context.Background(), buffer.Bytes())
			var ae *APIError
			if !errors.As(err, &ae) || ae.Code != "INVALID_PACK" {
				t.Fatalf("err=%v", err)
			}
			after, _ := st.ListSessions()
			if len(after) != len(before) {
				t.Fatal("invalid archive left a partial session")
			}
		})
	}
}

func TestM4ImportedEvidenceCannotClaimUnsubstantiatedConfidence(t *testing.T) {
	st, _, _, sid, _, root := newTestServices(t, happyScript())
	b, _ := st.ExportSession(sid, "")
	b.Memories = append(b.Memories, &domain.MemoryRecord{MemoryID: "unsupported", SourceNodeID: root, Kind: domain.MemoryObserved, Content: "无原文证据", Confidence: 1, Evidence: &domain.MemoryEvidence{Confidence: "high"}})
	var buffer bytes.Buffer
	if _, err := pack.Write(&buffer, b, "test", time.Now()); err != nil {
		t.Fatal(err)
	}
	res, err := NewArchiveService(st, "test").Import(context.Background(), buffer.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	memories, _ := st.ListMemories(res.Session.SessionID)
	if len(memories) != 1 || memories[0].Evidence.Confidence != "low" || memories[0].Confidence != 0.4 || !memories[0].Evidence.AutoDowngraded {
		t.Fatalf("memory=%+v", memories)
	}
}

type graphReadFailure struct {
	ports.Store
	phase  string
	target string
}

func (s graphReadFailure) GetNode(id string) (*domain.PlotNode, error) {
	if s.phase == "target" && id == s.target {
		return nil, errors.New("injected target read failure")
	}
	if s.phase == "parent" && id != s.target {
		return nil, errors.New("injected parent read failure")
	}
	return s.Store.GetNode(id)
}
func (s graphReadFailure) ChildWindow(sid string, parents []string, limit int, content bool) (map[string][]*domain.PlotNode, bool, error) {
	if s.phase == "children" {
		return nil, false, errors.New("injected child read failure")
	}
	return s.Store.ChildWindow(sid, parents, limit, content)
}
func (s graphReadFailure) ChildCounts(sid string, ids []string) (map[string]int, error) {
	if s.phase == "counts" {
		return nil, errors.New("injected count read failure")
	}
	return s.Store.ChildCounts(sid, ids)
}

func TestM4GraphReadFailuresDoNotLookLikeACompleteGraph(t *testing.T) {
	st, turns, sessions, sid, bid, _ := newTestServices(t, happyScript())
	tr := acceptAndWait(t, turns, st, sid, bid, "graph-fault", "你好")
	for _, phase := range []string{"target", "parent", "children", "counts"} {
		t.Run(phase, func(t *testing.T) {
			svc := NewSessionService(graphReadFailure{Store: st, phase: phase, target: tr.ResultNodeID})
			_, err := svc.Graph(sid, tr.ResultNodeID, 1, 0)
			var ae *APIError
			if !errors.As(err, &ae) || ae.Code != "STORAGE_UNAVAILABLE" {
				t.Fatalf("err=%v", err)
			}
		})
	}
	err := sessions.RequireCharacter("nonexistent", "player")
	var ae *APIError
	if !errors.As(err, &ae) || ae.Code != "NOT_FOUND" {
		t.Fatalf("missing session err=%v", err)
	}
	_, err = sessions.Graph(sid, "nonexistent", 1, 0)
	if !errors.As(err, &ae) || ae.Code != "NOT_FOUND" {
		t.Fatalf("missing graph target err=%v", err)
	}
}
