package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"tavernagent/internal/adapters/mcp"
	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/application"
	"tavernagent/internal/ports"
)

func setupTestMCP(t *testing.T) (*mcp.Server, ports.Store) {
	t.Helper()
	dir := t.TempDir()
	store, err := sqlite.Open(dir, ports.RealClock{})
	if err != nil {
		t.Fatalf("sqlite open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	sessionSvc := application.NewSessionService(store)
	cardSvc := application.NewCardService(store)

	srv := mcp.New(mcp.Config{
		Store:    store,
		Sessions: sessionSvc,
		Cards:    cardSvc,
	})
	return srv, store
}

func TestMCPInitialize(t *testing.T) {
	srv, _ := setupTestMCP(t)

	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n")
	var out bytes.Buffer

	if err := srv.ServeStdio(context.Background(), in, &out); err != nil {
		t.Fatalf("ServeStdio: %v", err)
	}

	var resp struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Result  struct {
			ProtocolVersion string `json:"protocolVersion"`
			ServerInfo      struct {
				Name string `json:"name"`
			} `json:"serverInfo"`
		} `json:"result"`
	}

	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nOutput: %s", err, out.String())
	}

	if resp.Result.ServerInfo.Name != "tavernagent" {
		t.Errorf("expected server name 'tavernagent', got %q", resp.Result.ServerInfo.Name)
	}
	if resp.Result.ProtocolVersion != "2024-11-05" {
		t.Errorf("expected protocolVersion '2024-11-05', got %q", resp.Result.ProtocolVersion)
	}
}

func TestMCPToolsListAndCall(t *testing.T) {
	srv, _ := setupTestMCP(t)

	requests := []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_sessions","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"resources/list","params":{}}`,
		`{"jsonrpc":"2.0","id":4,"method":"ping"}`,
	}

	in := strings.NewReader(strings.Join(requests, "\n") + "\n")
	var out bytes.Buffer

	if err := srv.ServeStdio(context.Background(), in, &out); err != nil {
		t.Fatalf("ServeStdio: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != len(requests) {
		t.Fatalf("expected %d responses, got %d:\n%s", len(requests), len(lines), out.String())
	}

	// 1: tools/list
	var listResp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &listResp); err != nil {
		t.Fatalf("parse tools/list: %v", err)
	}
	if len(listResp.Result.Tools) == 0 {
		t.Errorf("expected tools list to not be empty")
	}

	// 2: tools/call list_sessions
	var callResp struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &callResp); err != nil {
		t.Fatalf("parse tools/call: %v", err)
	}
	if callResp.Result.IsError {
		t.Errorf("tools/call reported error: %v", callResp.Result)
	}
	if len(callResp.Result.Content) == 0 || callResp.Result.Content[0].Text == "" {
		t.Errorf("expected non-empty content in tools/call result")
	}
}
