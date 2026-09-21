// Package app 是组合根：把存储、配置、供应商与 HTTP 适配器装配成一个可运行实例。
//
// 无头服务入口（cmd/tavernagent）与桌面壳（cmd/tavernagent-desktop）共用它，
// 这样"桌面端"与"命令行端"必定是同一个后端——桌面模式不是另写一套装配，
// 而是同一个 http.Handler 承载在同一个 loopback 监听上，只是多开了一个原生窗口。
//
// 分层说明：组合根是唯一允许同时依赖全部适配器的地方；应用层/领域层依旧不知道
// 数据库、网络与模型 SDK 的存在。
package app

import (
	"crypto/tls"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"tavernagent/internal/adapters/config"
	httpadapter "tavernagent/internal/adapters/http"
	"tavernagent/internal/adapters/image"
	"tavernagent/internal/adapters/providers/anthropic"
	"tavernagent/internal/adapters/providers/mock"
	"tavernagent/internal/adapters/providers/openai"
	"tavernagent/internal/adapters/providers/responses"
	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/application"
	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/ports"
	"tavernagent/internal/util/datalock"
	"tavernagent/internal/util/trace"
	"tavernagent/web"
)

// Config 是装配参数。
type Config struct {
	// DataDir 是数据目录（存储、配置、密钥、锁都在这里）。
	DataDir string
	// Addr 是网络监听地址；桌面模式不监听 TCP 时可只用于来源回退，不参与监听。
	Addr string
	// FallbackKind 是"未配置任何模型时"的回退供应商，目前只支持 "mock"。
	FallbackKind string
	// PIN 是局域网配对码；留空表示不启用配对鉴权（仅适合不开放 TCP 的单机桌面模式）。
	PIN string
	// SetNativeTheme 是桌面壳注入的原生窗口配色回调（浏览器部署留空）。
	SetNativeTheme func(mode string)
	// AllowedOrigins 是额外放行的浏览器来源（开发代理等）。
	AllowedOrigins []string
	// Ablation 是启动期生效的消融开关（评估用）。
	Ablation ctxpkg.Ablation
	// AppVersion 会写进剧情包清单，用于排查"这个包是哪个版本导出的"。
	AppVersion string
	// TLSConfig 非空时，无头监听会包成 HTTPS（局域网加密访问）。
	// 桌面壳自带 loopback 监听，不受此项影响。
	TLSConfig *tls.Config
}

// App 是一个已装配、可运行的实例。
type App struct {
	// Server 是需要被承载的 HTTP 适配器：无头模式用 TCP 监听，桌面模式交给资源服务。
	Server   *httpadapter.Server
	Addr     string
	PIN      string
	DataDir  string
	Store    ports.Store
	Sessions *application.SessionService
	Turns    *application.TurnService
	Memories *application.MemoryService
	Cards    *application.CardService
	Archive  *application.ArchiveService
	Compiler *ctxpkg.Compiler
	Manager  *application.ProviderManager

	closers []func()
}

// Close 按"后创建先关闭"释放资源（与原入口的 defer 链一致）：
// 导演/回合 worker → 存储 → 数据目录锁。可重复调用。
func (a *App) Close() {
	for i := len(a.closers) - 1; i >= 0; i-- {
		a.closers[i]()
	}
	a.closers = nil
}

// Bootstrap 装配一个实例。任何一步失败都不留下半开状态：已获取的资源按逆序释放。
func Bootstrap(cfg Config) (*App, error) {
	if strings.TrimSpace(cfg.DataDir) == "" {
		return nil, fmt.Errorf("数据目录不能为空")
	}
	if cfg.FallbackKind != "" && cfg.FallbackKind != "mock" {
		return nil, fmt.Errorf("不支持的开发供应商 %q；可使用 mock 或留空", cfg.FallbackKind)
	}

	out := &App{Addr: cfg.Addr, PIN: cfg.PIN}
	assembled := false
	defer func() {
		if !assembled {
			out.Close()
		}
	}()

	dataLock, err := datalock.Acquire(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	out.closers = append(out.closers, func() { _ = dataLock.Close() })

	store, err := sqlite.Open(cfg.DataDir, ports.RealClock{})
	if err != nil {
		return nil, fmt.Errorf("打开存储失败: %w", err)
	}
	out.closers = append(out.closers, func() { _ = store.Close() })

	// M0 风险项：SQLite FTS5 能力探测。
	info, err := store.SystemInfo()
	if err != nil {
		return nil, fmt.Errorf("读取存储信息失败: %w", err)
	}
	log.Printf("sqlite driver=%s fts5=%v attr_err=%q", info.DriverVersion, info.FTS5Available, info.AttributeError)

	cfgStore, err := config.New(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("配置存储初始化失败: %w", err)
	}
	// v1 配置（槽位内联字段 + 档案）一次性迁移成"模型实例 + 槽位引用"：
	// 迁移前备份为 settings.json.v1.bak，迁移后磁盘只保留 v2。
	if migrated, err := cfgStore.MigrateLegacy(); err != nil {
		log.Printf("模型配置迁移失败（继续使用内存中的迁移结果）: %v", err)
	} else if migrated {
		log.Printf("模型配置已迁移为实例+槽位形态（旧文件备份为 settings.json.v1.bak）")
	}

	bus := application.NewEventBus(store)
	sessionSvc := application.NewSessionService(store)
	compiler, err := setupCompiler(store, cfg.Ablation)
	if err != nil {
		return nil, err
	}

	manager := application.NewProviderManager(cfgStore, buildFromConfig, buildFallback(cfg.FallbackKind))
	manager.SetUsageStore(store)
	if err := manager.Reload(); err != nil {
		return nil, fmt.Errorf("加载模型配置失败: %w", err)
	}
	turnSvc := application.NewTurnServiceWithManager(store, manager, compiler, bus)
	out.closers = append(out.closers, turnSvc.Close)
	if err := turnSvc.Recover(); err != nil {
		return nil, fmt.Errorf("恢复剧情回合失败: %w", err)
	}
	directorSvc := application.NewDirectorService(store, manager, compiler, bus, nil)
	if err := directorSvc.Recover(); err != nil {
		return nil, fmt.Errorf("恢复导演讨论失败: %w", err)
	}
	out.closers = append(out.closers, directorSvc.Close)

	branchSvc := application.NewBranchService(store, turnSvc)
	memorySvc := application.NewMemoryService(store)
	archiveSvc := application.NewArchiveService(store, cfg.AppVersion)
	cardSvc := application.NewCardService(store)

	// http 层不再直接持有 Store：读取视图由 application 组装，
	// 适配器只做协议转换（I2）。
	staticFS, err := web.DistFS()
	if err != nil {
		log.Printf("提示：前端静态资源未加载 (%v)，仅提供 API 服务", err)
	}

	// 运行读数（观测）：计数器与用量台账都在这里装配，
	// 缺任一项服务仍可运行，只是 /api/status 少一段数据。
	metrics := &application.RuntimeMetrics{}
	turnSvc.SetMetrics(metrics)
	// 台账写失败要能被读到（不再只剩一行日志）：接进同一组计数器。
	manager.SetUsageFailureSink(metrics.UsageWriteFailed)

	// 进程内追踪记录器：只保留最近若干次请求的耗时与分段打点，随 /api/status 暴露。
	// 之所以不是 OTel SDK：见 internal/ports/trace.go 的包注释。
	tracer := trace.NewRecorder(trace.DefaultCapacity)

	imgSvc, err := image.NewService(filepath.Join(cfg.DataDir, "images"))
	if err != nil {
		log.Printf("生图服务初始化失败: %v", err)
	}

	server, err := httpadapter.New(httpadapter.Deps{
		Director:     directorSvc,
		ImageService: imgSvc,
		Sessions:     sessionSvc, Turns: turnSvc, Branches: branchSvc,
		Memories: memorySvc, Archive: archiveSvc, Manager: manager, Bus: bus, Addr: cfg.Addr,
		Cards:          cardSvc,
		StaticFS:       staticFS,
		AuthPIN:        cfg.PIN,
		SetNativeTheme: cfg.SetNativeTheme,
		AllowedOrigins: cfg.AllowedOrigins,
		Metrics:        metrics,
		Usage:          store,
		Ablation:       cfg.Ablation,
		Tracer:         tracer,
		TLSConfig:      cfg.TLSConfig,
	})
	if err != nil {
		return nil, err
	}
	out.Server = server
	out.DataDir = cfg.DataDir
	out.Store = store
	out.Sessions = sessionSvc
	out.Turns = turnSvc
	out.Memories = memorySvc
	out.Cards = cardSvc
	out.Archive = archiveSvc
	out.Compiler = compiler
	out.Manager = manager
	assembled = true
	return out, nil
}

// setupCompiler 解析消融开关并构造**唯一**的编译器（ADS-7.8-01）。
//
// 幂等性来自构造顺序：开关在编译器之前落定，编译器是所有上下文编译的唯一入口，
// 因此不存在"某个模块已经捕获了未消融的配置"这种半开状态。
func setupCompiler(store ports.Store, ablation ctxpkg.Ablation) (*ctxpkg.Compiler, error) {
	opts := ctxpkg.DefaultOptions()
	ablation.Apply(&opts)
	if ablation.Enabled() {
		log.Printf("⚠️  消融模式已启用，仅用于评估对照，已关闭特性：%s", ablation.String())
	}
	log.Printf("提示词版本 %s", ctxpkg.PromptManifest())
	return ctxpkg.New(store, opts), nil
}

// buildFromConfig 按槽位配置构建供应商（组合根注入，适配层在此收敛）。
func buildFromConfig(cfg ports.ProviderConfig) (ports.ModelProvider, error) {
	switch cfg.Kind {
	case "openai-chat", "openai-compatible", "openai":
		return openai.New(openai.Config{
			BaseURL: cfg.BaseURL, Model: cfg.Model, APIKey: cfg.APIKey,
			Temperature: cfg.Temperature, MaxTokens: cfg.MaxTokens,
			ReasoningEffort: cfg.ReasoningEffort,
		}), nil
	case "openai-responses", "responses":
		return responses.New(responses.Config{
			BaseURL: cfg.BaseURL, Model: cfg.Model, APIKey: cfg.APIKey,
			Temperature: cfg.Temperature, MaxTokens: cfg.MaxTokens,
			ReasoningEffort: cfg.ReasoningEffort,
		}), nil
	case "anthropic-messages", "anthropic":
		return anthropic.New(anthropic.Config{
			BaseURL: cfg.BaseURL, Model: cfg.Model, APIKey: cfg.APIKey,
			Temperature: cfg.Temperature, MaxTokens: cfg.MaxTokens,
			ReasoningEffort: cfg.ReasoningEffort,
		}), nil
	default:
		return nil, &unsupportedProvider{name: cfg.Kind}
	}
}

func buildFallback(kind string) ports.ModelProvider {
	if kind == "mock" {
		return mock.NewDemo()
	}
	return nil
}

type unsupportedProvider struct{ name string }

func (u *unsupportedProvider) Error() string {
	return "不支持的供应商协议: " + u.name + "；当前支持 openai-chat (Chat Completions)、openai-responses (Responses API) 与 anthropic-messages (Anthropic Messages API)"
}
