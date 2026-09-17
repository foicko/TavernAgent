package http

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBoundedJSONErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		limit      int64
		status     int
		code       string
	}{
		{"oversized", `{"value":"` + strings.Repeat("x", 64) + `"}`, 32, 413, "REQUEST_TOO_LARGE"},
		{"trailing", `{"value":"ok"} {}`, 100, 400, "BAD_REQUEST"},
		{"invalid", `{"value":`, 100, 400, "BAD_REQUEST"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/", strings.NewReader(tc.body))
			var value struct {
				Value string `json:"value"`
			}
			err := decodeJSON(w, r, &value, tc.limit)
			if err == nil {
				t.Fatal("accepted invalid body")
			}
			writeBodyError(w, err)
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
				t.Fatalf("response: %d %s", w.Code, w.Body.String())
			}
		})
	}
}
