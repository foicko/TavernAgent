// tavernagent 是 Agentic Role-Play App（SillyDog）的无头服务入口。
// 默认回环绑定；模型供应商由 data/config 配置驱动（primary/assist/reflection 三槽位），
// 可显式使用 -provider mock 运行离线演示。
//
// 装配逻辑在 internal/app（组合根），桌面壳 cmd/tavernagent-desktop 与它共用同一后端。
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

	"tavernagent/internal/app"
	ctxpkg "tavernagent/internal/context"
)

// 这三个变量由构建脚本用 -ldflags -X main.* 注入（scripts/build.ps1、scripts/release.py）。
var (
	appVersion  = "tavernagent/dev"
	buildCommit = "unknown"
	buildTime   = "unknown"
)

func main() {
	if err := run(); err != nil {
		log.Printf("服务退出: %v", err)
		os.Exit(1)
	}
}

// cliOptions 是命令行开关的解析结果。
type cliOptions struct {
	addr           string
	dataDir        string
	fallbackKind   string
	pin            string
	allowedOrigins string
	showVersion    bool
	ablation       ctxpkg.Ablation
}

// parseFlags 声明并解析全部命令行开关。
//
// 消融开关用 flag.Func 绑定解析：取值非法时由 flag 包直接报错退出，
// 因此不会出现"解析失败却被忽略、最后跑成生产形态"的静默降级——
// 对基线评估来说，静默降级比启动失败危险得多（ADS-7.8-01）。
func parseFlags() cliOptions {
	addr := flag.String("addr", "127.0.0.1:8890", "监听地址")
	dataDir := flag.String("data", "data", "数据目录")
	fallbackKind := flag.String("provider", "", "未配置时回退的开发供应商（留空表示强制在设置中配置真实模型）")
	pin := flag.String("pin", "", "局域网配对码（留空时自动生成 6 位安全数字码）")
	showVersion := flag.Bool("version", false, "显示构建版本")
	allowedOrigins := flag.String("allow-origin", "", "额外允许的浏览器来源，以逗号分隔（开发代理等）")
	var ablation ctxpkg.Ablation
	flag.Func("ablate", "消融开关（评估用，逗号分隔）：dynamic-context,memory,lorebook,summaries,compaction 或 all；留空为生产形态",
		func(spec string) error {
			parsed, err := ctxpkg.ParseAblation(spec)
			if err != nil {
				return err
			}
			ablation = parsed
			return nil
		})
	flag.Parse()
	return cliOptions{
		addr: *addr, dataDir: *dataDir, fallbackKind: *fallbackKind, pin: *pin,
		allowedOrigins: *allowedOrigins, showVersion: *showVersion, ablation: ablation,
	}
}

func run() error {
	opts := parseFlags()
	if opts.showVersion {
		fmt.Printf("%s commit=%s built=%s prompt=%s\n", appVersion, buildCommit, buildTime, ctxpkg.PromptManifest())
		return nil
	}
	// 纯输入校验放在产生副作用（配对码打印、取锁、开库）之前。
	if opts.fallbackKind != "" && opts.fallbackKind != "mock" {
		return fmt.Errorf("不支持的开发供应商 %q；可使用 mock 或留空", opts.fallbackKind)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pin := strings.TrimSpace(opts.pin)
	if pin == "" {
		number, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
		if err != nil {
			return fmt.Errorf("生成配对码失败: %w", err)
		}
		pin = fmt.Sprintf("%06d", number.Int64())
	}
	log.Printf("🔑 局域网配对码 (LAN PIN): %s", pin)
	log.Printf("📱 局域网设备首次访问请输入该配对码建立安全连接（本机 127.0.0.1 访问自动免检）")

	instance, err := app.Bootstrap(app.Config{
		DataDir:        opts.dataDir,
		Addr:           opts.addr,
		FallbackKind:   opts.fallbackKind,
		PIN:            pin,
		AllowedOrigins: strings.FieldsFunc(opts.allowedOrigins, func(r rune) bool { return r == ',' }),
		Ablation:       opts.ablation,
		AppVersion:     appVersion,
	})
	if err != nil {
		return err
	}
	defer instance.Close()

	logMsg := fmt.Sprintf("SillyDog 服务就绪：http://%s （模型：配置驱动）", opts.addr)
	if opts.fallbackKind != "" {
		logMsg = fmt.Sprintf("SillyDog 服务就绪：http://%s （模型：配置驱动，兜底 %s）", opts.addr, opts.fallbackKind)
	}
	log.Println(logMsg)
	return instance.Server.ListenAndServeContext(ctx)
}
