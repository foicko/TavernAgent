package domain

import "testing"

func TestDirectorCompletionEvidence(t *testing.T) {
	quote := "她清楚地喊出了旅人的旧称呼"
	for _, tc := range []struct {
		name, kind, body string
		valid            bool
	}{
		{"actual narration", "narration", quote + "。", true},
		{"actual dialogue", "dialogue", "我听见" + quote + "。", true},
		{"thought", "inner_monologue", quote + "。", false},
		{"unsupported block", "proposal", quote + "。", false},
		{"cherry picked conditional", "narration", "如果" + quote + "，一切就会不同。", false},
		{"imagined narration", "narration", "你想象着" + quote + "的情景。", false},
		{"not present", "narration", "她还没有说话。", false},
		{"actual after hypothetical", "narration", "你曾希望她记得过去。" + quote + "。", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report := DirectorReport{Status: "completed", Evidence: []DirectorEvidence{{BlockSeq: 1, Quote: quote}}}
			err := ValidateDirectorEvidence(report, []TextBlock{{Kind: tc.kind, Text: tc.body}})
			if (err == nil) != tc.valid {
				t.Fatalf("evidence validity=%v, want %v: %v", err == nil, tc.valid, err)
			}
		})
	}
}
