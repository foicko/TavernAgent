package sqlite

import "testing"

func TestPlaceholdersEmptyAndFilled(t *testing.T) {
	cases := map[int]string{0: "", -1: "", 1: "?", 3: "?,?,?"}
	for n, want := range cases {
		if got := placeholders(n); got != want {
			t.Errorf("placeholders(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestInClauseEmptyReturnsNil(t *testing.T) {
	if s, a := inClause(nil); s != "" || a != nil {
		t.Fatalf("inClause(nil) = (%q, %v), want empty", s, a)
	}
	s, a := inClause([]string{"a", "b"})
	if s != "?,?" || len(a) != 2 || a[0] != "a" || a[1] != "b" {
		t.Fatalf("inClause = (%q, %v)", s, a)
	}
}

func TestInClauseAnyGeneric(t *testing.T) {
	s, a := inClauseAny([]int{1, 2, 3})
	if s != "?,?,?" || len(a) != 3 || a[2] != 3 {
		t.Fatalf("inClauseAny = (%q, %v)", s, a)
	}
	if s, a := inClauseAny([]int(nil)); s != "" || a != nil {
		t.Fatalf("inClauseAny(nil) = (%q, %v), want empty", s, a)
	}
}
