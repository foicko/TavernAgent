// Package http 提供 REST 与 SSE 契约（技术契约 §11）。
// 写操作校验请求来源与身份；读操作也校验会话访问范围。
package http

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"tavernagent/internal/adapters/image"
	"tavernagent/internal/adapters/tts"
	"tavernagent/internal/application"
	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// Deps 是 HTTP 适配器的装配依赖（组合根注入）。
// 用具名结构体而不是一长串位置参数：参数变多时调用点仍然可读，
// 也不会因为顺序写错而把两个服务对调。
type Deps struct {
	TTSService     *tts.Service
	ImageService   *image.Service
	Director       *application.DirectorService
	Sessions       *application.SessionService
	Turns          *application.TurnService
	Branches       *application.BranchService
	Memories       *application.MemoryService
	Cards          *application.CardService
	Archive        *application.ArchiveService
	Manager        *application.ProviderManager
	Bus            *application.EventBus
	Addr           string
	StaticFS       fs.FS
	AuthPIN        string // 局域网配对码（非空时，非本机局域网请求必须通过配对鉴权）
	AuthToken      string // 配对成功的授权 Token（留空且 AuthPIN 非空时自动随机生成）
	AllowedOrigins []string
	// SetNativeTheme 是桌面壳注入的"切换原生窗口主题"回调，传 "light"/"dark"。
	// 浏览器部署为空，此时 /api/v1/desktop/theme 返回 404。
	SetNativeTheme func(mode string)
	// Metrics 与 Usage 是运行观测端口（T0.2）。两者都可为空——
	// 缺少读数时端点返回空对象，而不是 500：观测不得影响服务可用性。
	Metrics *application.RuntimeMetrics
	Usage   ports.UsageStore
	// Ablation 是启动期生效的消融开关（ADS-7.8-01）。零值表示生产形态
	// （未关闭任何特性），它随 /api/status 一起暴露，使基线评估的产物
	// 自带"这一轮关了什么"的自证，而不必依赖运行者的记忆。
	Ablation ctxpkg.Ablation
	// Tracer 是追踪端口（T0.2 观测）。留空时退化为 no-op——
	// 观测不该成为启动的前置条件，缺了它服务照常可用，只是没有分段读数。
	Tracer ports.Tracer
	// TLSConfig 非空时监听套接字会被包成 TLS（局域网 HTTPS）。
	// 只影响 ListenAndServeContext 自己创建的监听；桌面壳自带 loopback 监听，不受影响。
	TLSConfig *tls.Config
}

// Server 是 HTTP 适配器。
type Server struct {
	director       *application.DirectorService
	sessions       *application.SessionService
	turns          *application.TurnService
	branches       *application.BranchService
	memories       *application.MemoryService
	cards          *application.CardService
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
	ttsService     *tts.Service
	imageService   *image.Service
	setNativeTheme func(mode string)
	pairFails      map[string]pairFailure
	now            func() time.Time
	tracer         ports.Tracer
	tlsConfig      *tls.Config
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
	tracer := deps.Tracer
	if tracer == nil {
		tracer = ports.NoopTracer{}
	}
	ttsSvc := deps.TTSService
	if ttsSvc == nil {
		ttsSvc = tts.NewService()
	}
	imgSvc := deps.ImageService
	if imgSvc == nil {
		imgSvc, _ = image.NewService("data/images")
	}
	return &Server{
		director: deps.Director,
		sessions: deps.Sessions, turns: deps.Turns,
		branches: deps.Branches, memories: deps.Memories, archive: deps.Archive,
		cards:   deps.Cards,
		manager: deps.Manager, bus: deps.Bus,
		addr: deps.Addr, origin: "http://" + deps.Addr,
		allowedOrigins: append([]string(nil), deps.AllowedOrigins...),
		setNativeTheme: deps.SetNativeTheme,
		metrics:        deps.Metrics,
		usage:          deps.Usage,
		ablation:       deps.Ablation,
		staticFS:       deps.StaticFS,
		authPIN:        pin,
		authToken:      token,
		ttsService:     ttsSvc,
		imageService:   imgSvc,
		tracer:         tracer,
		tlsConfig:      deps.TLSConfig,
		pairFails:      make(map[string]pairFailure), now: time.Now,
	}, nil
}

// Handler 返回 http.Handler（供测试直接使用）。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/tts/voices", s.listTTSVoices)
	mux.HandleFunc("POST /api/v1/tts/synthesize", s.security(s.synthesizeTTS))
	mux.HandleFunc("POST /api/v1/images/generate", s.security(s.generateImage))
	mux.HandleFunc("GET /api/v1/images/{filename}", s.serveImageFile)
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
	s.registerCardRoutes(mux)
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
	s.registerDesktopRoutes(mux)
	mux.HandleFunc("GET /api/status", s.security(s.runtimeStatus))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	if s.staticFS != nil {
		mux.Handle("/", spaHandler(s.staticFS))
	}
	return withLogging(mux, s.tracer)
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

// ---- handlers ----

type createSessionRequest struct {
	IdempotencyKey string `json:"idempotencyKey,omitempty"`
	Title          string `json:"title"`
	// CardID 指卡库中的一张卡（优先，免传 characterJson）；
	// CharacterJSON 为兼容入口（内置预设与旧客户端）。
	CardID           string                `json:"cardId,omitempty"`
	CharacterJSON    string                `json:"characterJson,omitempty"`
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
	// Note 是重生成引导（非叙事要求）：让“演得不对”可以表达成一句可执行的方向，
	// 而不是只能盲重掷或手工改正文。
	Note    string `json:"note,omitempty"`
	Options string `json:"options,omitempty"`
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
		Note: req.Note, Options: req.Options,
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
