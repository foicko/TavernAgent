package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPairingRateLimitExpiresAndDoesNotTrustQueryToken(t *testing.T) {
	server := mustNew(t, Deps{Addr: "192.168.1.10:8890", AuthPIN: "123456", AuthToken: "secret"})
	now := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
	server.now = func() time.Time { return now }
	pair := func(pin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "http://192.168.1.10:8890/api/v1/auth/pair", strings.NewReader(`{"pin":"`+pin+`"}`))
		r.RemoteAddr = "192.168.1.20:1234"
		w := httptest.NewRecorder()
		server.Handler().ServeHTTP(w, r)
		return w
	}
	for i := 0; i < 10; i++ {
		if got := pair("wrong"); got.Code != 401 {
			t.Fatalf("attempt %d: %d", i, got.Code)
		}
	}
	if got := pair("123456"); got.Code != 429 || got.Header().Get("Retry-After") == "" {
		t.Fatalf("rate limit=%d %v", got.Code, got.Header())
	}
	now = now.Add(5*time.Minute + time.Second)
	if got := pair("123456"); got.Code != 200 {
		t.Fatalf("IP remained permanently locked: %d", got.Code)
	}
	r := httptest.NewRequest(http.MethodGet, "http://192.168.1.10:8890/api/v1/auth/status?token=secret", nil)
	r.RemoteAddr = "192.168.1.20:1234"
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, r)
	if strings.Contains(w.Body.String(), `"authenticated":true`) {
		t.Fatal("URL query token was accepted")
	}
}
