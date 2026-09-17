// tavernagent 是 Agentic Role-Play App（SillyDog）的无头服务入口。
// 默认回环绑定；模型供应商由 data/config 配置驱动（primary/assist/reflection 三槽位），
// 可显式使用 -provider mock 运行离线演示。
package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"tavernagent/internal/adapters/config"
	"tavernagent/internal/adapters/http"
	"tavernagent/internal/adapters/providers/anthropic"
	"tavernagent/internal/adapters/providers/mock"
	"tavernagent/internal/adapters/providers/openai"
	"tavernagent/internal/adapters/providers/responses"
	"tavernagent/internal/adapters/sqlite"
	"tavernagent/internal/application"
	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/ports"
	"tavernagent/internal/util/datalock"
	"tavernagent/web"
)

// appVersion 会写进剧情包清单，用于排查「这个包是哪个版本导出的」。
var appVersion = "tavernagent/dev"
var buildCommit = "unknown"
var buildTime = "unknown"

func main() {
	if err := run(); err != nil {
		log.Printf("服务退出: %v", err)
		os.Exit(1)
	}
}

func run() error {
	addr := flag.String("addr", "127.0.0.1:8890", "监听地址")
	dataDir := flag.String("data", "data", "数据目录")
	fallbackKind := flag.String("provider", "", "未配置时回退的开发供应商（留空表示强制在设置中配置真实模型）")
	pinFlag := flag.String("pin", "", "局域网配对码（留空时自动生成 6 位安全数字码）")
	showVersion := flag.Bool("version", false, "显示构建版本")
	allowedOrigins := flag.String("allow-origin", "", "额外允许的浏览器来源，以逗号分隔（开发代理等）")
	flag.Parse()
	if *showVersion {
		fmt.Printf("%s commit=%s built=%s\n", appVersion, buildCommit, buildTime)
		return nil
	}
	if *fallbackKind != "" && *fallbackKind != "mock" {
		return fmt.Errorf("不支持的开发供应商 %q；可使用 mock 或留空", *fallbackKind)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dataLock, err := datalock.Acquire(*dataDir)
	if err != nil {
		return err
	}
	defer dataLock.Close()

	pin := strings.TrimSpace(*pinFlag)
	if pin == "" {
		number, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
		if err != nil {
			return fmt.Errorf("生成配对码失败: %w", err)
		}
		pin = fmt.Sprintf("%06d", number.Int64())
	}
	log.Printf("🔑 局域网配对码 (LAN PIN): %s", pin)
	log.Printf("📱 局域网设备首次访问请输入该配对码建立安全连接（本机 127.0.0.1 访问自动免检）")

	store, err := sqlite.Open(*dataDir, ports.RealClock{})
	if err != nil {
		return fmt.Errorf("打开存储失败: %w", err)
	}
	defer store.Close()

	// M0 风险项：SQLite FTS5 能力探测。
	info, err := store.SystemInfo()
	if err != nil {
		return fmt.Errorf("读取存储信息失败: %w", err)
	}
	log.Printf("sqlite driver=%s fts5=%v attr_err=%q", info.DriverVersion, info.FTS5Available, info.AttributeError)

	cfgStore, err := config.New(*dataDir)
	if err != nil {
		return fmt.Errorf("配置存储初始化失败: %w", err)
	}
	// v1 配置（槽位内联字段 + 档案）一次性迁移成"模型实例 + 槽位引用"：
	// 迁移前备份为 settings.json.v1.bak，迁移后磁盘只保留 v2。
	if migrated, err := cfgStore.MigrateLegacy(); err != nil {
		log.Printf("模型配置迁移失败（继续使用内存中的迁移结果）: %v", err)
	} else if migrated {
		log.Printf("模型配置已迁移为实例+槽位形态（旧文件备份为 settings.json.v1.bak）")
	}

	fallback := buildFallback(*fallbackKind)
	bus := application.NewEventBus(store)
	sessionSvc := application.NewSessionService(store)
	compiler := ctxpkg.New(store, ctxpkg.DefaultOptions())

	manager := application.NewProviderManager(cfgStore, buildFromConfig, fallback)
	manager.SetUsageStore(store)
	if err := manager.Reload(); err != nil {
		return fmt.Errorf("加载模型配置失败: %w", err)
	}
	turnSvc := application.NewTurnServiceWithManager(store, manager, compiler, bus)
	defer turnSvc.Close()
	if err := turnSvc.Recover(); err != nil {
		return fmt.Errorf("恢复剧情回合失败: %w", err)
	}
	directorSvc := application.NewDirectorService(store, manager, compiler, bus, nil)
	if err := directorSvc.Recover(); err != nil {
		return fmt.Errorf("恢复导演讨论失败: %w", err)
	}
	defer directorSvc.Close()
	branchSvc := application.NewBranchService(store, turnSvc)
	memorySvc := application.NewMemoryService(store)

	archiveSvc := application.NewArchiveService(store, appVersion)

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

	server, err := http.New(http.Deps{
		Director: directorSvc,
		Sessions: sessionSvc, Turns: turnSvc, Branches: branchSvc,
		Memories: memorySvc, Archive: archiveSvc, Manager: manager, Bus: bus, Addr: *addr,
		StaticFS:       staticFS,
		AuthPIN:        pin,
		AllowedOrigins: strings.FieldsFunc(*allowedOrigins, func(r rune) bool { return r == ',' }),
		Metrics:        metrics,
		Usage:          store,
	})
	if err != nil {
		return err
	}
	logMsg := fmt.Sprintf("SillyDog 服务就绪：http://%s （模型：配置驱动）", *addr)
	if *fallbackKind != "" {
		logMsg = fmt.Sprintf("SillyDog 服务就绪：http://%s （模型：配置驱动，兜底 %s）", *addr, *fallbackKind)
	}
	log.Println(logMsg)
	return server.ListenAndServeContext(ctx)
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
