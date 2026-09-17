package datalock

import "testing"

func TestExclusiveOwnershipAndRelease(t *testing.T) {
	dir := t.TempDir()
	first, err := Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate, err := Acquire(dir); err == nil {
		duplicate.Close()
		t.Fatal("second process ownership accepted")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	next.Close()
}
