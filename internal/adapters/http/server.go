// Package http 提供 REST 与 SSE 契约（技术契约 §11）。
// 写操作校验请求来源与身份；读操作也校验会话访问范围。
package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"tavernagent/internal/application"
	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// Deps 是 HTTP 适配器的装配依赖（组合根注入）。
// 用具名结构体而不是一长串位置参数：参数变多时调用点仍然可读，
// 也不会因为顺序写错而把两个服务对调。
type Deps struct {
	Director       *application.DirectorService
	Sessions       *application.SessionService
	Turns          *application.TurnService
	Branches       *application.BranchService
	Memories       *application.MemoryService
	Archive        *application.ArchiveService
	Manager        *application.ProviderManager
	Bus            *application.EventBus
	Addr           string
	StaticFS       fs.FS
	AuthPIN        string // 局域网配对码（非空时，非本机局域网请求必须通过配对鉴权）
	AuthToken      string // 配对成功的授权 Token（留空且 AuthPIN 非空时自动随机生成）
	AllowedOrigins []string
	// Metrics 与 Usage 是运行观测端口（T0.2）。两者都可为空——
	// 缺少读数时端点返回空对象，而不是 500：观测不得影响服务可用性。
	Metrics *application.RuntimeMetrics
	Usage   ports.UsageStore
	// Ablation 是启动期生效的消融开关（ADS-7.8-01）。零值表示生产形态
	// （未关闭任何特性），它随 /api/status 一起暴露，使基线评估的产物
	// 自带"这一轮关了什么"的自证，而不必依赖运行者的记忆。
	Ablation ctxpkg.Ablation
}

// Server 是 HTTP 适配器。
type Server struct {
	director       *application.DirectorService
	sessions       *application.SessionService
	turns          *application.TurnService
	branches       *application.BranchService
	memories       *application.MemoryService
	archive        *application.ArchiveService
	manager        *application.ProviderManager
	bus            *application.EventBus
	metrics        *application.RuntimeMetrics
	usage          ports.UsageStore
	ablation       ctxpkg.Ablation
	addr           string
	origin         string
	allowedOrigins []string
	staticFS       fs.FS
	authPIN        string
	authToken      string
	pairFails      map[string]pairFailure
	now            func() time.Time
	mu             sync.Mutex
}

// New 创建 HTTP 服务。
func New(deps Deps) (*Server, error) {
	pin := strings.TrimSpace(deps.AuthPIN)
	token := strings.TrimSpace(deps.AuthToken)
	if pin != "" && token == "" {
		generated, err := generateRandomToken(24)
		if err != nil {
			return nil, fmt.Errorf("生成配对 Token 失败: %w", err)
		}
		token = generated
	}
	return &Server{
		director: deps.Director,
		sessions: deps.Sessions, turns: deps.Turns,
		branches: deps.Branches, memories: deps.Memories, archive: deps.Archive,
		manager: deps.Manager, bus: deps.Bus,
		addr: deps.Addr, origin: "http://" + deps.Addr,
		allowedOrigins: append([]string(nil), deps.AllowedOrigins...),
		metrics:        deps.Metrics,
		usage:          deps.Usage,
		ablation:       deps.Ablation,
		staticFS:       deps.StaticFS,
		authPIN:        pin,
		authToken:      token,
		pairFails:      make(map[string]pairFailure), now: time.Now,
	}, nil
}

// Handler 返回 http.Handler（供测试直接使用）。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/sessions/{id}/branches/{branchId}/director", s.directorRoute(s.getDirector))
	mux.HandleFunc("PUT /api/v1/sessions/{id}/branches/{branchId}/director/draft", s.directorRoute(s.saveDirectorDraft))
	mux.HandleFunc("POST /api/v1/sessions/{id}/branches/{branchId}/director/messages", s.directorRoute(s.directorMessage))
	mux.HandleFunc("POST /api/v1/sessions/{id}/branches/{branchId}/director/commands", s.directorRoute(s.directorCommand))
	mux.HandleFunc("GET /api/v1/director-requests/{requestId}", s.directorRoute(s.getDirectorRequest))
	mux.HandleFunc("GET /api/v1/director-requests/{requestId}/events", s.directorRoute(s.directorEvents))
	mux.HandleFunc("POST /api/v1/director-requests/{requestId}/cancel", s.directorRoute(s.cancelDirectorRequest))
	mux.HandleFunc("POST /api/v1/sessions", s.security(s.createSession))
	mux.HandleFunc("GET /api/v1/sessions", s.security(s.listSessions))
	mux.HandleFunc("GET /api/v1/sessions/{id}", s.security(s.getSession))
	mux.HandleFunc("DELETE /api/v1/sessions/{id}", s.security(s.deleteSession))
	mux.HandleFunc("GET /api/v1/sessions/{id}/branches", s.security(s.listBranches))
	mux.HandleFunc("POST /api/v1/sessions/{id}/branches", s.security(s.forkBranch))
	mux.HandleFunc("GET /api/v1/sessions/{id}/export", s.security(s.exportSession))
	mux.HandleFunc("POST /api/v1/sessions/import", s.security(s.importSession))
	mux.HandleFunc("POST /api/v1/cards/import", s.security(s.importCard))
	mux.HandleFunc("GET /api/v1/nodes/{nodeId}", s.security(s.getNode))
	mux.HandleFunc("POST /api/v1/sessions/{id}/branches/{branchId}/turns", s.security(s.acceptTurn))
	mux.HandleFunc("POST /api/v1/sessions/{id}/branches/{branchId}/regenerations", s.security(s.regenerateTurn))
	mux.HandleFunc("POST /api/v1/sessions/{id}/branches/{branchId}/edits", s.security(s.editTurn))
	mux.HandleFunc("GET /api/v1/sessions/{id}/graph", s.security(s.getGraph))
	mux.HandleFunc("GET /api/v1/sessions/{id}/lorebook", s.security(s.getLorebook))
	mux.HandleFunc("GET /api/v1/sessions/{id}/events", s.security(s.sessionEvents))
	mux.HandleFunc("GET /api/v1/turns/{turnId}", s.security(s.getTurn))
	mux.HandleFunc("GET /api/v1/turns/{turnId}/events", s.security(s.turnEvents))
	mux.HandleFunc("POST /api/v1/turns/{turnId}/cancel", s.security(s.cancelTurn))
	mux.HandleFunc("POST /api/v1/turns/{turnId}/continue", s.security(s.continueTurn))
	// M3：记忆管理（列表按分支标记有效性；修订走 copy-on-write 覆盖记录）
	mux.HandleFunc("GET /api/v1/sessions/{id}/memories", s.security(s.listMemories))
	mux.HandleFunc("PATCH /api/v1/sessions/{id}/branches/{branchId}/memories/{memoryId}", s.security(s.patchMemory))
	mux.HandleFunc("POST /api/v1/sessions/{id}/branches/{branchId}/memories/organize", s.security(s.organizeMemories))
	// 模型配置 API：模型实例 CRUD + 槽位指派 + 探测
	mux.HandleFunc("GET /api/v1/config/models", s.security(s.listModels))
	mux.HandleFunc("POST /api/v1/config/models", s.security(s.saveModel))
	mux.HandleFunc("PUT /api/v1/config/models/{modelId}", s.security(s.saveModel))
	mux.HandleFunc("DELETE /api/v1/config/models/{modelId}", s.security(s.deleteModel))
	mux.HandleFunc("POST /api/v1/config/models/{modelId}/probe", s.security(s.probeModel))
	mux.HandleFunc("GET /api/v1/config/provider", s.security(s.listProviderConfigs))
	mux.HandleFunc("PUT /api/v1/config/provider", s.security(s.saveProviderConfig))
	mux.HandleFunc("POST /api/v1/config/provider/probe", s.security(s.probeProvider))
	// 局域网配对鉴权 API（SEC-01）
	mux.HandleFunc("POST /api/v1/auth/pair", s.pairAuth)
	mux.HandleFunc("GET /api/v1/auth/status", s.authStatus)
	mux.HandleFunc("GET /api/status", s.security(s.runtimeStatus))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	if s.staticFS != nil {
		mux.Handle("/", spaHandler(s.staticFS))
	}
	return withLogging(mux)
}

func spaHandler(staticFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(staticFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/healthz" {
			http.NotFound(w, r)
			return
		}
		cleanPath := strings.TrimPrefix(r.URL.Path, "/")
		if cleanPath == "" {
			cleanPath = "index.html"
		}
		if f, err := staticFS.Open(cleanPath); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		// SPA 单页应用路由兜底
		r2 := new(http.Request)
		*r2 = *r
		r2.URL = new(url.URL)
		*r2.URL = *r.URL
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}

// ListenAndServe 启动监听（默认回环绑定）。
func (s *Server) ListenAndServe() error {
	return s.ListenAndServeContext(context.Background())
}

// ---- 安全：回环与局域网私有地址绑定 + 来源校验（T28 相关，支持 LAN 访问）----

func (s *Server) security(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		// 允许本机及私有局域网 Host 访问
		if !isLANHost(host) {
			writeError(w, 403, "FORBIDDEN", "只允许本机或局域网私有网络访问", false, "")
			return
		}

		// 局域网鉴权（SEC-01）：本机免检；非本机 LAN 访问若启用了 AuthPIN，则必须携带合法授权 Token
		if !isLoopbackPeer(r) && s.authPIN != "" {
			token := extractToken(r)
			if token == "" || !secureEqual(token, s.authToken) {
				writeError(w, 401, "AUTH_REQUIRED", "局域网访问需要配对码授权", false, "")
				return
			}
		}

		if isWrite(r) {
			o := r.Header.Get("Origin")
			if o != "" && !s.allowedOrigin(r) {
				writeError(w, 403, "FORBIDDEN", "跨来源写请求被拒绝", false, "")
				return
			}
		}
		h(w, r)
	}
}

func extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	if tok := r.Header.Get("X-Auth-Token"); tok != "" {
		return tok
	}
	return ""
}

func (s *Server) pairAuth(w http.ResponseWriter, r *http.Request) {
	if !isLANHost(r.Host) || (isWrite(r) && !s.allowedOrigin(r)) {
		writeError(w, 403, "FORBIDDEN", "请求来源不受支持", false, "")
		return
	}
	if s.authPIN == "" {
		writeJSON(w, 200, map[string]any{"ok": true, "token": ""})
		return
	}
	ip := clientIP(r)

	var req struct {
		PIN string `json:"pin"`
	}
	if err := decodeJSON(w, r, &req, 1024); err != nil {
		writeBodyError(w, err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for address, failure := range s.pairFails {
		if !now.Before(failure.Expires) {
			delete(s.pairFails, address)
		}
	}
	failure := s.pairFails[ip]
	if failure.Count >= 10 {
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(failure.Expires.Sub(now).Seconds()))))
		writeError(w, 429, "TOO_MANY_ATTEMPTS", "尝试次数过多，请五分钟后重试", false, "")
		return
	}
	if !secureEqual(strings.TrimSpace(req.PIN), s.authPIN) {
		if failure.Count == 0 {
			failure.Expires = now.Add(5 * time.Minute)
		}
		failure.Count++
		s.pairFails[ip] = failure
		writeError(w, 401, "INVALID_PIN", "配对码错误", false, "")
		return
	}

	delete(s.pairFails, ip)

	writeJSON(w, 200, map[string]any{"ok": true, "token": s.authToken})
}

type pairFailure struct {
	Count   int
	Expires time.Time
}

func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) {
	if !isLANHost(r.Host) {
		writeError(w, 403, "FORBIDDEN", "只允许本机或局域网私有网络访问", false, "")
		return
	}
	if isLoopbackPeer(r) || s.authPIN == "" {
		writeJSON(w, 200, map[string]any{"authenticated": true, "required": false})
		return
	}
	token := extractToken(r)
	authed := token != "" && secureEqual(token, s.authToken)
	writeJSON(w, 200, map[string]any{"authenticated": authed, "required": true})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func isWrite(r *http.Request) bool {
	switch r.Method {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	}
	return false
}

func isLoopbackHost(host string) bool {
	h := hostname(host)
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func hostname(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		return host[1 : len(host)-1]
	}
	return host
}

// Only the transport peer can grant the local exemption. Host and forwarded
// headers are supplied by the caller and must never establish its identity.
func isLoopbackPeer(r *http.Request) bool {
	ip := net.ParseIP(clientIP(r))
	return ip != nil && ip.IsLoopback()
}

func isLANHost(host string) bool {
	h := hostname(host)
	if isLoopbackHost(h) {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && (ip.IsPrivate() || ip.IsLoopback())
}

func sameOrigin(a, b string) bool { return strings.TrimSuffix(a, "/") == strings.TrimSuffix(b, "/") }

// ---- handlers ----

type createSessionRequest struct {
	IdempotencyKey   string                `json:"idempotencyKey,omitempty"`
	Title            string                `json:"title"`
	CharacterJSON    string                `json:"characterJson"`
	PlayerName       string                `json:"playerName"`
	PlayerRole       string                `json:"playerRole,omitempty"`
	PlayerBackpack   []domain.ItemInstance `json:"playerBackpack,omitempty"`
	OpeningVariantID string                `json:"openingVariantId,omitempty"`
	OpeningText      string                `json:"openingText,omitempty"`
}

func (s *Server) exportSession(w http.ResponseWriter, r *http.Request) {
	if s.archive == nil {
		writeError(w, 503, "STORAGE_UNAVAILABLE", "剧情包服务不可用", true, "")
		return
	}
	res, err := s.archive.Export(r.Context(), r.PathValue("id"), r.URL.Query().Get("branchId"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	name := fmt.Sprintf("%s-%s.tavernpack",
		sanitizeFileName(res.Manifest.Session.Title), time.Now().Format("20060102-150405"))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", contentDisposition(name))
	w.Header().Set("Content-Length", strconv.Itoa(len(res.Data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(res.Data)
}
func (s *Server) getNode(w http.ResponseWriter, r *http.Request) {
	view, err := s.sessions.NodeView(r.PathValue("nodeId"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 200, view)
}

type forkBranchRequest struct {
	FromNodeID          string `json:"fromNodeId"`
	Name                string `json:"name,omitempty"`
	ExpectedCharacterID string `json:"expectedCharacterId,omitempty"`
}

// forkBranch 从既有节点建立新分支（写入位置），本身不生成内容。
func (s *Server) forkBranch(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	var req forkBranchRequest
	if err := decodeJSON(w, r, &req, maxRequestBytes); err != nil {
		writeBodyError(w, err)
		return
	}
	b, err := s.branches.Fork(r.Context(), sessionID, application.ForkRequest{
		FromNodeID: req.FromNodeID, Name: req.Name,
		ExpectedCharacterID: req.ExpectedCharacterID,
	})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"branch": b})
}

type regenerateRequest struct {
	NodeID         string `json:"nodeId"`
	Recheck        bool   `json:"recheck,omitempty"`
	IdempotencyKey string `json:"idempotencyKey,omitempty"`
	Name           string `json:"name,omitempty"`
	// ExpectedCharacterID 角色归属校验（M4l，契约 §11.2）。空表示不校验。
	ExpectedCharacterID string `json:"expectedCharacterId,omitempty"`
}

// regenerateTurn 为指定回合生成候选版本：从 parent(N) 建候选分支并受理一次生成。
// 原节点 N 不被改写，新回复与它是兄弟节点（技术契约 §7）。
func (s *Server) regenerateTurn(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if err := s.sessions.RequireBranch(sessionID, r.PathValue("branchId")); err != nil {
		writeAPIError(w, err)
		return
	}
	var req regenerateRequest
	if err := decodeJSON(w, r, &req, maxRequestBytes); err != nil {
		writeBodyError(w, err)
		return
	}
	res, err := s.branches.DeriveTurn(r.Context(), sessionID, application.DeriveRequest{
		NodeID: req.NodeID, IdempotencyKey: req.IdempotencyKey, Label: req.Name, Recheck: req.Recheck,
		ExpectedCharacterID: req.ExpectedCharacterID,
	})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 202, map[string]any{
		"branch":     res.Branch,
		"branchId":   res.Branch.BranchID,
		"turnId":     res.Turn.TurnID,
		"status":     res.Turn.Status,
		"statusUrl":  "/api/v1/turns/" + res.Turn.TurnID,
		"eventsUrl":  "/api/v1/turns/" + res.Turn.TurnID + "/events",
		"headNodeId": res.Branch.HeadNodeID,
	})
}

type editTurnRequest struct {
	NodeID              string             `json:"nodeId"`
	Input               *domain.TurnInput  `json:"input,omitempty"`
	Blocks              []domain.TextBlock `json:"blocks,omitempty"`
	IdempotencyKey      string             `json:"idempotencyKey,omitempty"`
	Name                string             `json:"name,omitempty"`
	ExpectedCharacterID string             `json:"expectedCharacterId,omitempty"` // M4l 角色归属校验
}

// editTurn 处理两类编辑（技术契约 §7）：
//
//	带 input    → 编辑玩家输入：从 parent(N) 建分支重新生成，旧后续不复用
//	带 blocks   → 编辑助手正文：落为**纯叙事候选**（零状态变化，同步返回节点）
//
// 两者都不改写原节点 N。编辑正文之所以降级为纯叙事：当前没有规则检定，
// 无法验证新文本是否仍支持原来的状态变化，保留原提议会造成叙述与状态矛盾。
func (s *Server) editTurn(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if err := s.sessions.RequireBranch(sessionID, r.PathValue("branchId")); err != nil {
		writeAPIError(w, err)
		return
	}
	var req editTurnRequest
	if err := decodeJSON(w, r, &req, maxRequestBytes); err != nil {
		writeBodyError(w, err)
		return
	}
	switch {
	case req.Input != nil && len(req.Blocks) > 0:
		writeError(w, 400, "EDIT_AMBIGUOUS", "不能同时编辑玩家输入与助手正文", false, "")
		return
	case req.Input != nil:
		res, err := s.branches.DeriveTurn(r.Context(), sessionID, application.DeriveRequest{
			NodeID: req.NodeID, Input: req.Input, IdempotencyKey: req.IdempotencyKey, Label: req.Name,
			ExpectedCharacterID: req.ExpectedCharacterID,
		})
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, 202, map[string]any{
			"branch":    res.Branch,
			"branchId":  res.Branch.BranchID,
			"turnId":    res.Turn.TurnID,
			"status":    res.Turn.Status,
			"statusUrl": "/api/v1/turns/" + res.Turn.TurnID,
			"eventsUrl": "/api/v1/turns/" + res.Turn.TurnID + "/events",
		})
	case len(req.Blocks) > 0:
		res, err := s.branches.EditAssistantText(r.Context(), sessionID, application.AssistantEditRequest{
			NodeID: req.NodeID, Blocks: req.Blocks, Label: req.Name,
			ExpectedCharacterID: req.ExpectedCharacterID,
		})
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, 201, map[string]any{
			"branch":   res.Branch,
			"branchId": res.Branch.BranchID,
			"nodeId":   res.Node.NodeID,
			"mode":     "narrative",
		})
	default:
		writeError(w, 400, "EDIT_EMPTY", "需要 input（编辑玩家输入）或 blocks（编辑助手正文）", false, "")
	}
}

type acceptTurnRequest struct {
	IdempotencyKey  string           `json:"idempotencyKey"`
	ExpectedHeadID  string           `json:"expectedHeadId"`
	ExpectedVersion int64            `json:"expectedVersion"`
	AfterTurnID     string           `json:"afterTurnId,omitempty"`
	Mode            string           `json:"mode,omitempty"`
	Input           domain.TurnInput `json:"input"`
	// ExpectedCharacterID 角色归属校验（M4l，契约 §11.2）。空表示不校验。
	ExpectedCharacterID string `json:"expectedCharacterId,omitempty"`
}

func (s *Server) acceptTurn(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	branchID := r.PathValue("branchId")
	var req acceptTurnRequest
	if err := decodeJSON(w, r, &req, maxRequestBytes); err != nil {
		writeBodyError(w, err)
		return
	}
	if req.IdempotencyKey == "" {
		writeError(w, 422, "MISSING_IDEMPOTENCY_KEY", "缺少幂等键", false, "")
		return
	}
	if req.Input.Kind == "" {
		req.Input.Kind = "text"
	}
	if req.Input.Kind == "option" && req.Input.OptionRef == nil {
		writeError(w, 422, "STALE_OPTION", "选项引用缺失", false, "")
		return
	}
	turn, err := s.turns.Accept(r.Context(), sessionID, branchID, &application.TurnAcceptRequest{
		IdempotencyKey: req.IdempotencyKey, ExpectedHeadID: req.ExpectedHeadID,
		ExpectedVersion: req.ExpectedVersion, AfterTurnID: req.AfterTurnID, Mode: req.Mode,
		Input: req.Input, ExpectedCharacterID: req.ExpectedCharacterID,
	})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 202, map[string]any{
		"turnId":    turn.TurnID,
		"status":    turn.Status,
		"statusUrl": "/api/v1/turns/" + turn.TurnID,
		"eventsUrl": "/api/v1/turns/" + turn.TurnID + "/events",
	})
}

func (s *Server) cancelTurn(w http.ResponseWriter, r *http.Request) {
	if !s.requireTurnCharacter(w, r) {
		return
	}
	turn, err := s.turns.Cancel(r.Context(), r.PathValue("turnId"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"turnId": turn.TurnID, "status": turn.Status})
}

func (s *Server) continueTurn(w http.ResponseWriter, r *http.Request) {
	if !s.requireTurnCharacter(w, r) {
		return
	}
	turn, err := s.turns.Continue(r.Context(), r.PathValue("turnId"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 202, map[string]any{
		"turnId":    turn.TurnID,
		"status":    turn.Status,
		"eventsUrl": "/api/v1/turns/" + turn.TurnID + "/events",
	})
}

func (s *Server) requireTurnCharacter(w http.ResponseWriter, r *http.Request) bool {
	var req struct {
		ExpectedCharacterID string `json:"expectedCharacterId"`
	}
	if err := decodeJSON(w, r, &req, 4096); err != nil && err != io.EOF {
		writeBodyError(w, err)
		return false
	}
	turn, err := s.turns.Get(r.PathValue("turnId"))
	if err != nil {
		writeError(w, 404, "NOT_FOUND", "回合不存在", false, "")
		return false
	}
	if err := s.sessions.RequireCharacter(turn.SessionID, req.ExpectedCharacterID); err != nil {
		writeAPIError(w, err)
		return false
	}
	return true
}

// ---- 记忆管理（M3）----

func (s *Server) listMemories(w http.ResponseWriter, r *http.Request) {
	if s.memories == nil {
		writeError(w, 501, "NOT_IMPLEMENTED", "未启用记忆服务", false, "")
		return
	}
	query := r.URL.Query()
	limit := 40
	if query.Has("limit") {
		var err error
		limit, err = strconv.Atoi(query.Get("limit"))
		if err != nil || limit < 1 || limit > 200 {
			writeAPIError(w, application.Err("BAD_REQUEST", "limit 须为 1～200", 400))
			return
		}
	}
	page, err := s.memories.ListPageAt(r.PathValue("id"), query.Get("branchId"), query.Get("nodeId"), application.MemoryQuery{Search: query.Get("search"), Kind: query.Get("kind"), Limit: limit, Cursor: query.Get("cursor")})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 200, page)
}

func (s *Server) patchMemory(w http.ResponseWriter, r *http.Request) {
	if s.memories == nil {
		writeError(w, 501, "NOT_IMPLEMENTED", "未启用记忆服务", false, "")
		return
	}
	var patch application.MemoryPatch
	if err := decodeJSON(w, r, &patch, maxRequestBytes); err != nil {
		writeBodyError(w, err)
		return
	}
	updated, err := s.memories.OverlayContext(r.Context(),
		r.PathValue("id"), r.PathValue("branchId"), r.PathValue("memoryId"), patch)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 200, updated)
}

func validSlot(slot string) bool {
	switch slot {
	case string(ports.SlotPrimary), string(ports.SlotAssist), string(ports.SlotReflection):
		return true
	}
	return false
}

// ---- SSE ----

func (s *Server) turnEvents(w http.ResponseWriter, r *http.Request) {
	turnID := r.PathValue("turnId")
	if _, err := s.turns.Get(turnID); err != nil {
		writeError(w, 404, "NOT_FOUND", "回合不存在", false, "")
		return
	}
	s.eventStream(w, r, turnID)
}

func (s *Server) sessionEvents(w http.ResponseWriter, r *http.Request) {
	if err := s.sessions.RequireCharacter(r.PathValue("id"), ""); err != nil {
		writeAPIError(w, err)
		return
	}
	s.eventStream(w, r, r.PathValue("id"))
}

func (s *Server) eventStream(w http.ResponseWriter, r *http.Request, aggregateID string) {
	controller := http.NewResponseController(w)
	_ = controller.SetWriteDeadline(time.Now().Add(30 * time.Second))
	defer func() { _ = controller.SetWriteDeadline(time.Time{}) }()
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "SSE_UNSUPPORTED", "当前连接不支持流式", false, "")
		return
	}
	after := int64(0)
	if lei := r.Header.Get("Last-Event-ID"); strings.HasPrefix(lei, aggregateID+":") {
		if n, err := strconv.ParseInt(strings.TrimPrefix(lei, aggregateID+":"), 10, 64); err == nil && n > 0 {
			after = n
		}
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(200)
	flusher.Flush()

	// Subscribe before reading. Notifications are only wakeups: every delivery
	// comes from the durable outbox and advances one monotonic watermark.
	sub, closeFn := s.bus.Subscribe(aggregateID, 64)
	defer closeFn()
	drain := func() bool {
		for {
			if r.Context().Err() != nil {
				return false
			}
			events, err := s.bus.Poll(aggregateID, after, 256)
			if err != nil {
				return false
			}
			for _, ev := range events {
				if ev.Sequence <= after {
					continue
				}
				if !s.writeSSE(w, flusher, ev) {
					return false
				}
				after = ev.Sequence
			}
			if len(events) < 256 {
				return true
			}
		}
	}
	if !drain() {
		return
	}
	reconcile := time.NewTicker(time.Second)
	defer reconcile.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-sub.Events:
			// Partial text/thinking is best-effort and has no durable event ID.
			// A completed block still replaces any partial draft after reconnect.
			if ev != nil && ev.Sequence == 0 && (ev.Type == "turn.thinking" || ev.Type == "block.delta") {
				_ = controller.SetWriteDeadline(time.Now().Add(30 * time.Second))
				if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, ev.PayloadJSON); err != nil {
					return
				}
				flusher.Flush()
				continue
			}
			if !drain() {
				return
			}
		case <-reconcile.C:
			if !drain() {
				return
			}
		case <-heartbeat.C:
			_ = controller.SetWriteDeadline(time.Now().Add(30 * time.Second))
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
		}
	}
}

func (s *Server) organizeMemories(w http.ResponseWriter, r *http.Request) {
	if s.memories == nil {
		writeError(w, 501, "NOT_IMPLEMENTED", "未启用记忆服务", false, "")
		return
	}
	var req struct {
		ExpectedCharacterID string `json:"expectedCharacterId"`
	}
	if err := decodeJSON(w, r, &req, maxRequestBytes); err != nil {
		writeBodyError(w, err)
		return
	}
	plan, err := s.memories.Organize(r.Context(), r.PathValue("id"), r.PathValue("branchId"), req.ExpectedCharacterID)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, 200, plan)
}

func (s *Server) writeSSE(w http.ResponseWriter, flusher http.Flusher, ev *domain.OutboxEvent) bool {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(30 * time.Second))
	_, err := fmt.Fprintf(w, "event: %s\nid: %s:%d\ndata: %s\n\n", ev.Type, ev.AggregateID, ev.Sequence, ev.PayloadJSON)
	if err != nil {
		return false
	}
	flusher.Flush()
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string, retryable bool, turnID string) {
	writeJSON(w, status, map[string]any{
		"code": code, "message": message, "retryable": retryable, "turnId": turnID,
	})
}

func writeAPIError(w http.ResponseWriter, err error) {
	if appErr, ok := err.(*application.APIError); ok {
		writeError(w, appErr.StatusCode, appErr.Code, appErr.Message, appErr.Retryable, "")
		return
	}
	writeError(w, 500, "INTERNAL", err.Error(), true, "")
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 健康检查是高频探活，跳过避免日志噪音。
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	})
}

// statusRecorder 包装 ResponseWriter 捕获状态码（访问日志用）。
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Flush 透传：SSE 路径依赖 w.(http.Flusher) 断言，包装层必须保持该能力。
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
