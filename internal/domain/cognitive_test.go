package domain

import "testing"

func TestEvidenceContract(t *testing.T) {
	excerpts := []EvidenceExcerpt{{NodeID: "n1", Text: "玩家拔出了腰间的佩剑"}, {NodeID: "n2", Text: "她说你好"}}
	for _, tc := range []struct {
		name                            string
		kind                            MemoryKind
		quote, reason, confidence, want string
		downgraded, bad                 bool
	}{
		{"quoted high", MemoryObserved, "拔出了腰间的佩剑", "", "high", "high", false, false},
		{"surrounding quotes high", MemoryObserved, "“拔出了腰间的佩剑”", "", "high", "high", false, false},
		{"trailing punctuation high", MemoryObserved, "拔出了腰间的佩剑，", "", "high", "high", false, false},
		{"short quote", MemoryObserved, "你好", "", "high", "medium", true, false},
		{"empty high cascades", MemoryObserved, "", "", "high", "low", true, false},
		{"empty medium", MemoryObserved, "", "", "medium", "low", true, false},
		{"inferred reason", MemoryInferred, "腰间的佩剑", "表明仍有戒心", "medium", "medium", false, false},
		{"inferred no reason", MemoryInferred, "拔出了腰间的佩剑", "", "high", "", false, true},
		{"fabricated quote", MemoryObserved, "她已经把怀表交给了我", "", "high", "", false, true},
		{"cross-block quote", MemoryObserved, "佩剑她说", "", "high", "", false, true},
		{"unknown confidence", MemoryObserved, "你好", "", "certain", "", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ev, score, err := ValidateMemoryEvidence(tc.kind, MemoryEvidence{SourceQuote: tc.quote, Reasoning: tc.reason, Confidence: tc.confidence, SourceNodeID: "forged"}, excerpts)
			if (err != nil) != tc.bad {
				t.Fatalf("error=%v", err)
			}
			if tc.bad {
				return
			}
			if ev.Confidence != tc.want || ev.AutoDowngraded != tc.downgraded || score <= 0 || score >= 1 {
				t.Fatalf("evidence=%+v score=%v", ev, score)
			}
			if ev.SourceNodeID == "forged" {
				t.Fatal("trusted model source node")
			}
		})
	}
}

func TestCognitiveMemoryRejectsForeignEntities(t *testing.T) {
	s := NewWorldState()
	s.Characters["npc"] = CharacterInfo{CharacterID: "npc", Participant: true}
	_, err := ValidateCognitiveMemory(CognitiveMemory{Content: "看见陌生人", EntityIDs: []string{"other_session_npc"}}, MemoryObserved, s, nil)
	if err == nil {
		t.Fatal("accepted foreign entity")
	}
}

func TestResolveEntityID(t *testing.T) {
	s := NewWorldState()
	s.Characters["player"] = CharacterInfo{CharacterID: "player", Name: "林舟", Aliases: []string{"旅人", "异乡人"}}
	s.Characters["npc_liel"] = CharacterInfo{CharacterID: "npc_liel", Name: "莉尔", Aliases: []string{"小女仆"}}
	s.Items["item_sword"] = ItemInstance{InstanceID: "item_sword", Name: "铁剑", Aliases: []string{"佩剑"}}

	cases := []struct {
		input string
		want  string
	}{
		{"player", "player"},
		{"林舟", "player"},
		{"旅人", "player"},
		{"玩家", "player"},
		{"主角", "player"},
		{"user", "player"},
		{"self", "player"},
		{"npc_liel", "npc_liel"},
		{"liel", "npc_liel"},
		{"莉尔", "npc_liel"},
		{"小女仆", "npc_liel"},
		{"item_sword", "item_sword"},
		{"铁剑", "item_sword"},
		{"unknown_entity", "unknown_entity"},
	}

	for _, c := range cases {
		got := ResolveEntityID(c.input, s)
		if got != c.want {
			t.Errorf("ResolveEntityID(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestValidateMemoryEvidenceRobustness(t *testing.T) {
	excerpts := []EvidenceExcerpt{
		{NodeID: "root", Text: "你推开酒馆的厚重木门，风雪被关在了身后。壁炉的火光温暖着空气。"},
		{NodeID: "n1", Text: "莉尔：「我、我明白了……如果您希望的话，我会试着去准备的，大人。」"},
	}

	for _, tc := range []struct {
		name      string
		quote     string
		wantNode  string
		wantError bool
	}{
		{"opening scene quote", "风雪被关在了身后", "root", false},
		{"direct quote", "我会试着去准备的", "n1", false},
		{"bracket wrapped", "「我会试着去准备的」", "n1", false},
		{"speaker prefix", "莉尔：「我会试着去准备的」", "n1", false},
		{"ellipsis normalization", "我、我明白了...如果您希望的话", "n1", false},
		{"caesura comma variation", "我，我明白了……如果您希望的话", "n1", false},
		{"markdown wrapped", "*我会试着去准备的*", "n1", false},
		{"fabricated quote fails", "从未出现的逐字引文", "", true},
		{"cross block fails", "空气莉尔", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ev, _, err := ValidateMemoryEvidence(MemoryObserved, MemoryEvidence{SourceQuote: tc.quote, Confidence: "high"}, excerpts)
			if (err != nil) != tc.wantError {
				t.Fatalf("quote %q err=%v, wantError=%v", tc.quote, err, tc.wantError)
			}
			if !tc.wantError && ev.SourceNodeID != tc.wantNode {
				t.Fatalf("quote %q matched node %q, want %q", tc.quote, ev.SourceNodeID, tc.wantNode)
			}
		})
	}
}

func TestTier5CleanRunesLengthThreshold(t *testing.T) {
	excerpts := []EvidenceExcerpt{
		{NodeID: "root", Text: "你 推 开 酒 馆 的 厚 重 木 门，风雪被关在身后。"},
	}
	// 5 clean runes should fail Tier 5 (< 8)
	quote5 := "你#推#开#酒#馆"
	_, _, err5 := ValidateMemoryEvidence(MemoryObserved, MemoryEvidence{SourceQuote: quote5, Confidence: "high"}, excerpts)
	if err5 == nil {
		t.Fatalf("Tier 5 with 5 clean runes should fail, but got nil error")
	}

	// 8 clean runes should pass Tier 5 (>= 8)
	quote8 := "你#推#开#酒#馆#的#厚#重"
	ev8, _, err8 := ValidateMemoryEvidence(MemoryObserved, MemoryEvidence{SourceQuote: quote8, Confidence: "high"}, excerpts)
	if err8 != nil {
		t.Fatalf("Tier 5 with 8 clean runes should pass, but got: %v", err8)
	}
	if ev8.SourceNodeID != "root" {
		t.Fatalf("matched node %q, want 'root'", ev8.SourceNodeID)
	}
}
