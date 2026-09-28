package httpbody

import (
	"strings"
	"testing"
)

func TestSnippetTrimsAndHandlesEmpty(t *testing.T) {
	if got := Snippet([]byte("  upstream bad gateway  "), SnippetLimit); got != "upstream bad gateway" {
		t.Fatalf("got %q", got)
	}
	if got := Snippet(nil, SnippetLimit); got != "" {
		t.Fatalf("empty body should yield empty snippet, got %q", got)
	}
	if got := Snippet([]byte("   \n\t "), SnippetLimit); got != "" {
		t.Fatalf("whitespace-only body should yield empty snippet, got %q", got)
	}
}

func TestSnippetRuneSafeTruncation(t *testing.T) {
	got := Snippet([]byte(strings.Repeat("很", 600)), 512)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("truncated snippet should end with ellipsis: %q", got)
	}
	if n := len([]rune(got)); n != 513 {
		t.Fatalf("rune length = %d, want 512 + ellipsis", n)
	}
	if strings.Contains(got, "\uFFFD") {
		t.Fatalf("truncation must not break UTF-8: %q", got)
	}
}

func TestSnippetReplacesHTMLErrorPage(t *testing.T) {
	for _, body := range []string{"<!DOCTYPE html><html><body>502</body></html>", "  <HTML>oops</HTML>", "x<html>y"} {
		if got := Snippet([]byte(body), SnippetLimit); got != HTMLBodyNotice {
			t.Fatalf("body %q -> %q, want HTML notice", body, got)
		}
	}
}

func TestSnippetKeepsShortJSON(t *testing.T) {
	if got := Snippet([]byte(`{"error":{"message":"nope"}}`), SnippetLimit); got != `{"error":{"message":"nope"}}` {
		t.Fatalf("got %q", got)
	}
}
