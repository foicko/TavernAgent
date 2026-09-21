// Package mcp implements the Model Context Protocol (MCP) server for TavernAgent.
// It allows AI tools (such as Claude Desktop, Cursor, and IDE extensions) to inspect
// sessions, query memories, list character cards, and preview compiled prompts.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"

	"tavernagent/internal/application"
	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// Server implements an MCP JSON-RPC 2.0 server over stdio.
type Server struct {
	store    ports.Store
	sessions *application.SessionService
	turns    *application.TurnService
	memories *application.MemoryService
	cards    *application.CardService
	compiler *ctxpkg.Compiler

	mu sync.Mutex
}

// Config holds dependencies for the MCP server.
type Config struct {
	Store    ports.Store
	Sessions *application.SessionService
	Turns    *application.TurnService
	Memories *application.MemoryService
	Cards    *application.CardService
	Compiler *ctxpkg.Compiler
}

// New creates a new MCP Server instance.
func New(cfg Config) *Server {
	return &Server{
		store:    cfg.Store,
		sessions: cfg.Sessions,
		turns:    cfg.Turns,
		memories: cfg.Memories,
		cards:    cfg.Cards,
		compiler: cfg.Compiler,
	}
}

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string  `json:"jsonrpc"`
	ID      any     `json:"id"`
	Result  any     `json:"result,omitempty"`
	Error   *rpcErr `json:"error,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ServeStdio reads newline-delimited JSON-RPC from in and writes responses to out.
func (s *Server) ServeStdio(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	// Support payloads up to 16MB for large card contents and histories
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 16*1024*1024)

	encoder := json.NewEncoder(out)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var req jsonRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			s.mu.Lock()
			_ = encoder.Encode(jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      nil,
				Error:   &rpcErr{Code: -32700, Message: "Parse error"},
			})
			s.mu.Unlock()
			continue
		}

		// Handle notifications (no ID)
		if req.ID == nil {
			s.handleNotification(ctx, req)
			continue
		}

		resp := s.handleRequest(ctx, req)
		s.mu.Lock()
		if err := encoder.Encode(resp); err != nil {
			log.Printf("mcp: encode response error: %v", err)
		}
		s.mu.Unlock()
	}

	return scanner.Err()
}

func (s *Server) handleNotification(_ context.Context, req jsonRPCRequest) {
	// Notifications such as "notifications/initialized"
	switch req.Method {
	case "notifications/initialized":
		// client acknowledged initialization
	default:
		// ignored
	}
}

func (s *Server) handleRequest(ctx context.Context, req jsonRPCRequest) jsonRPCResponse {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
	}

	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]any{
				"tools":     map[string]any{},
				"resources": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "tavernagent",
				"version": "1.0.0",
			},
		}

	case "ping":
		resp.Result = map[string]any{}

	case "tools/list":
		resp.Result = map[string]any{
			"tools": s.listTools(),
		}

	case "tools/call":
		res, err := s.callTool(ctx, req.Params)
		if err != nil {
			resp.Result = map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": fmt.Sprintf("Error: %v", err)},
				},
				"isError": true,
			}
		} else {
			resp.Result = map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": res},
				},
				"isError": false,
			}
		}

	case "resources/list":
		resp.Result = map[string]any{
			"resources": []map[string]any{
				{
					"uri":         "tavernagent://sessions",
					"name":        "Story Sessions",
					"description": "List of all active TavernAgent roleplay sessions",
					"mimeType":    "application/json",
				},
				{
					"uri":         "tavernagent://cards",
					"name":        "Character Cards",
					"description": "Installed character cards in library",
					"mimeType":    "application/json",
				},
			},
		}

	case "resources/read":
		var p struct {
			URI string `json:"uri"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			resp.Error = &rpcErr{Code: -32602, Message: "Invalid params"}
			return resp
		}
		content, err := s.readResource(ctx, p.URI)
		if err != nil {
			resp.Error = &rpcErr{Code: -32000, Message: err.Error()}
			return resp
		}
		resp.Result = map[string]any{
			"contents": []map[string]any{
				{
					"uri":      p.URI,
					"mimeType": "application/json",
					"text":     content,
				},
			},
		}

	default:
		resp.Error = &rpcErr{Code: -32601, Message: fmt.Sprintf("Method not found: %s", req.Method)}
	}

	return resp
}

func (s *Server) listTools() []map[string]any {
	return []map[string]any{
		{
			"name":        "list_sessions",
			"description": "List all story roleplay sessions in TavernAgent with ID, title, character, and turn count",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			"name":        "get_session_state",
			"description": "Get detailed story state, world variables, inventory, active turn, and character info for a session",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_id": map[string]any{
						"type":        "string",
						"description": "ID of the session to inspect",
					},
				},
				"required": []string{"session_id"},
			},
		},
		{
			"name":        "query_memories",
			"description": "Retrieve episodic, semantic and active memories recorded for a character in the session",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_id": map[string]any{
						"type":        "string",
						"description": "Session ID",
					},
				},
				"required": []string{"session_id"},
			},
		},
		{
			"name":        "compile_prompt",
			"description": "Compile the complete LLM prompt (system prompt + worldbook entries + epistemic memories + dialogue history) that TavernAgent sends for the next turn",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_id": map[string]any{
						"type":        "string",
						"description": "Session ID",
					},
					"input_text": map[string]any{
						"type":        "string",
						"description": "Optional user input to simulate in prompt compilation",
					},
				},
				"required": []string{"session_id"},
			},
		},
		{
			"name":        "list_cards",
			"description": "List all character cards installed in TavernAgent's card library",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	}
}

func (s *Server) callTool(ctx context.Context, params json.RawMessage) (string, error) {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return "", fmt.Errorf("invalid tool call format: %w", err)
	}

	switch call.Name {
	case "list_sessions":
		sessions, err := s.store.ListSessions()
		if err != nil {
			return "", fmt.Errorf("list sessions: %w", err)
		}
		data, _ := json.MarshalIndent(sessions, "", "  ")
		return string(data), nil

	case "get_session_state":
		var args struct {
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil || args.SessionID == "" {
			return "", fmt.Errorf("session_id is required")
		}

		view, err := s.sessions.View(args.SessionID, application.ViewQuery{Limit: 20})
		if err != nil {
			return "", fmt.Errorf("build session view: %w", err)
		}
		data, _ := json.MarshalIndent(view, "", "  ")
		return string(data), nil

	case "query_memories":
		var args struct {
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil || args.SessionID == "" {
			return "", fmt.Errorf("session_id is required")
		}

		memories, err := s.store.ListMemories(args.SessionID)
		if err != nil {
			return "", fmt.Errorf("list memories: %w", err)
		}
		data, _ := json.MarshalIndent(memories, "", "  ")
		return string(data), nil

	case "compile_prompt":
		var args struct {
			SessionID string `json:"session_id"`
			InputText string `json:"input_text"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil || args.SessionID == "" {
			return "", fmt.Errorf("session_id is required")
		}

		branches, err := s.sessions.Branches(args.SessionID)
		if err != nil || len(branches) == 0 {
			return "", fmt.Errorf("no branch found for session %s: %v", args.SessionID, err)
		}
		branch := branches[0]

		state := domain.NewWorldState()
		if s.store != nil {
			if snap, err := s.store.StateAt(branch.HeadNodeID); err == nil && snap != nil {
				if ws, err := domain.UnmarshalWorld(snap.StateJSON); err == nil && ws != nil {
					state = ws
				}
			}
		}

		if s.compiler == nil {
			return "", fmt.Errorf("prompt compiler not initialized")
		}

		input := args.InputText
		if input == "" {
			input = "（继续推演）"
		}

		req, err := s.compiler.Compile(ctx, args.SessionID, branch.HeadNodeID, input, ctxpkg.TurnDirectives{}, state, nil)
		if err != nil {
			return "", fmt.Errorf("compile prompt: %w", err)
		}

		data, _ := json.MarshalIndent(req, "", "  ")
		return string(data), nil

	case "list_cards":
		if s.cards == nil {
			return "[]", nil
		}
		cards, err := s.cards.List()
		if err != nil {
			return "", fmt.Errorf("list cards: %w", err)
		}
		data, _ := json.MarshalIndent(cards, "", "  ")
		return string(data), nil

	default:
		return "", fmt.Errorf("unknown tool: %s", call.Name)
	}
}

func (s *Server) readResource(ctx context.Context, uri string) (string, error) {
	switch uri {
	case "tavernagent://sessions":
		sessions, err := s.store.ListSessions()
		if err != nil {
			return "", err
		}
		data, _ := json.MarshalIndent(sessions, "", "  ")
		return string(data), nil

	case "tavernagent://cards":
		if s.cards == nil {
			return "[]", nil
		}
		cards, err := s.cards.List()
		if err != nil {
			return "", err
		}
		data, _ := json.MarshalIndent(cards, "", "  ")
		return string(data), nil

	default:
		return "", fmt.Errorf("unknown resource URI: %s", uri)
	}
}

// RunStdioServer is a helper that runs the MCP server on os.Stdin and os.Stdout.
func (s *Server) RunStdioServer(ctx context.Context) error {
	return s.ServeStdio(ctx, os.Stdin, os.Stdout)
}
