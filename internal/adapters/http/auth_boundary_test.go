package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/application"
	"tavernagent/internal/ports"
)

func TestAuthUsesConnectionPeer(t *testing.T) {
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	s := mustNew(t, Deps{Sessions: application.NewSessionService(st), Addr: "0.0.0.0:8890", AuthPIN: "123456", AuthToken: "test-token"})
	for _, tc := range []struct {
		name, peer, host, token string
		want                    int
		required                bool
	}{
		{"local", "127.0.0.1:1234", "localhost:8890", "", 200, false},
		{"local_ipv6", "[::1]:1234", "[::1]:8890", "", 200, false},
		{"local_LAN_host", "127.0.0.1:1234", "192.168.1.10:8890", "", 200, false},
		{"LAN_unpaired", "192.168.1.20:1234", "192.168.1.10:8890", "", 401, true},
		{"LAN_loopback_host", "192.168.1.20:1234", "127.0.0.1:8890", "", 401, true},
		{"LAN_localhost_host", "192.168.1.20:1234", "localhost:8890", "", 401, true},
		{"LAN_ipv6_host", "[fd00::20]:1234", "[::1]:8890", "", 401, true},
		{"LAN_authorized", "192.168.1.20:1234", "127.0.0.1:8890", "test-token", 200, true},
		{"LAN_bad_token", "192.168.1.20:1234", "127.0.0.1:8890", "wrong", 401, true},
		{"unknown_peer", "not-an-address", "127.0.0.1:8890", "", 401, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := func(path string) *httptest.ResponseRecorder {
				r := httptest.NewRequest(http.MethodGet, "http://"+tc.host+path, nil)
				r.RemoteAddr = tc.peer
				r.Header.Set("X-Forwarded-For", "127.0.0.1")
				if tc.token != "" {
					r.Header.Set("Authorization", "Bearer "+tc.token)
				}
				w := httptest.NewRecorder()
				s.Handler().ServeHTTP(w, r)
				return w
			}
			if w := request("/api/v1/sessions"); w.Code != tc.want {
				t.Errorf("peer=%s host=%s status=%d, want %d", tc.peer, tc.host, w.Code, tc.want)
			}
			w := request("/api/v1/auth/status")
			var state map[string]bool
			if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
				t.Fatal(err)
			}
			if state["required"] != tc.required || state["authenticated"] != (tc.want == 200) {
				t.Errorf("authentication status disagrees with peer identity: %v", state)
			}
		})
	}
}

func TestSecureEqual(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b string
		want bool
	}{
		{"identical", "test-token", "test-token", true},
		{"same length different", "test-tokeX", "test-token", false},
		{"different length", "test-token-longer", "test-token", false},
		{"empty both", "", "", true},
		{"empty vs value", "", "test-token", false},
		{"prefix only", "test", "test-token", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := secureEqual(tc.a, tc.b); got != tc.want {
				t.Fatalf("secureEqual(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestLoopbackHostRequiresARealAddress(t *testing.T) {
	for _, host := range []string{"127.evil.example", "127.0.0.1.attacker.example", "0.0.0.0:8890", "[::1", "localhost.attacker.example"} {
		if isLoopbackHost(host) {
			t.Errorf("untrusted host %q treated as loopback", host)
		}
	}
	for _, host := range []string{"127.0.0.2:8890", "[::1]:8890", "::1", "localhost:8890"} {
		if !isLoopbackHost(host) {
			t.Errorf("real loopback host %q rejected", host)
		}
	}
}
