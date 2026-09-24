package domain

import "testing"

func TestVarSetAppliesAndDeletes(t *testing.T) {
	s := NewWorldState()
	if _, err := ApplyEvent(s, &DomainEvent{Type: EventVarSet, PayloadJSON: `{"key":"lang","value":"KO"}`}); err != nil {
		t.Fatalf("apply var_set: %v", err)
	}
	if s.Variables["lang"] != "KO" {
		t.Fatalf("variables = %+v", s.Variables)
	}
	// 空值表示删除该键。
	if _, err := ApplyEvent(s, &DomainEvent{Type: EventVarSet, PayloadJSON: `{"key":"lang","value":""}`}); err != nil {
		t.Fatalf("delete var: %v", err)
	}
	if _, ok := s.Variables["lang"]; ok {
		t.Fatalf("variable not deleted: %+v", s.Variables)
	}
}

func TestVarSetRequiresKey(t *testing.T) {
	s := NewWorldState()
	if _, err := ApplyEvent(s, &DomainEvent{Type: EventVarSet, PayloadJSON: `{"key":"","value":"x"}`}); err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestVariablesSurviveCloneAndHash(t *testing.T) {
	s := NewWorldState()
	s.Variables = map[string]string{"num": "2"}
	c := s.Clone()
	c.Variables["num"] = "3"
	if s.Variables["num"] != "2" {
		t.Fatal("Clone did not deep-copy variables")
	}
	if s.HashID() == c.HashID() {
		t.Fatal("hash must change when variables differ")
	}
}
