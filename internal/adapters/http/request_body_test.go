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

// M20：并发上传预算必须封顶，且释放后可再次获取（不能泄漏名额）。
func TestUploadSlotBudget(t *testing.T) {
	acquired := 0
	for acquireUploadSlot() {
		acquired++
		if acquired > maxConcurrentUploads {
			t.Fatalf("acquired more than budget: %d", acquired)
		}
	}
	if acquired != maxConcurrentUploads {
		t.Fatalf("acquired %d slots, want %d", acquired, maxConcurrentUploads)
	}
	releaseUploadSlot()
	if !acquireUploadSlot() {
		t.Fatal("slot should be available again after release")
	}
	// 归还全部名额，避免影响同包其他用例。
	for i := 0; i < maxConcurrentUploads; i++ {
		releaseUploadSlot()
	}
}
