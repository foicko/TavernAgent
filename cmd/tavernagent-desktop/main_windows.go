//go:build windows

// tavernagent-desktop 是 Windows 桌面壳（Wails v2 + WebView2）。
//
// 它不是"另一个后端"：internal/app 装配出的后端与命令行版完全一致，这里只是把
// 它的 http.Handler 交给进程内的资源服务承载。于是窗口地址是
// http://wails.localhost/，界面与 /api/* 同源——不需要 TCP 端口、不需要 CORS，
// 也不会与用户已经运行的命令行实例抢 8890。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailswindows "github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"tavernagent/internal/app"
)

// appVersion 由构建脚本用 -ldflags -X main.appVersion=... 注入。
var appVersion = "tavernagent/dev"

func main() {
	if err := run(); err != nil {
		log.Printf("桌面端退出: %v", err)
		os.Exit(1)
	}
}

// defaultDataDir 用 Windows 约定的应用数据目录，而不是相对 ./data：
// 从开始菜单/快捷方式启动时工作目录不确定，相对路径会写出"幽灵数据目录"。
func defaultDataDir() string {
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, "TavernAgent")
	}
	return "data"
}

// colorRef 把 0xRRGGBB 转成 Windows 的 COLORREF（0x00BBGGRR）。
func colorRef(rgb uint32) int32 {
	return int32((rgb>>16)&0xFF | rgb&0x00FF00 | (rgb&0xFF)<<16)
}

// titleBarTheme 取应用设计令牌里的一组颜色（web/src/styles/tokens.css）。
// 目标不是“好看”，而是让原生标题栏与应用的浅色/深色底、边框、文字连续。
func titleBarTheme() *wailswindows.ThemeSettings {
	return &wailswindows.ThemeSettings{
		// 浅色：乳白纸墨（--bg-canvas #f9f8f5 / --text-main #141414）
		LightModeTitleBar:          colorRef(0xF9F8F5),
		LightModeTitleBarInactive:  colorRef(0xF3F2EF),
		LightModeTitleText:         colorRef(0x141414),
		LightModeTitleTextInactive: colorRef(0x8A8A86),
		LightModeBorder:            colorRef(0xE4E2DC),
		LightModeBorderInactive:    colorRef(0xECE9E3),
		// 深色：墨黑幽夜（--bg-canvas #111215 / --text-main #f0f0ed）
		DarkModeTitleBar:          colorRef(0x111215),
		DarkModeTitleBarInactive:  colorRef(0x17181C),
		DarkModeTitleText:         colorRef(0xF0F0ED),
		DarkModeTitleTextInactive: colorRef(0x71737A),
		DarkModeBorder:            colorRef(0x2A2D33),
		DarkModeBorderInactive:    colorRef(0x232529),
	}
}

// appCtx 在 OnStartup 时写入、可能被第二实例回调跨 goroutine 读取，必须加锁。
var (
	appCtxMu sync.Mutex
	appCtx   context.Context
)

func run() error {
	dataDir := flag.String("data", defaultDataDir(), "数据目录")
	fallbackKind := flag.String("provider", "", "未配置时回退的开发供应商（mock）")
	flag.Parse()
	if *fallbackKind != "" && *fallbackKind != "mock" {
		return fmt.Errorf("不支持的开发供应商 %q；可使用 mock 或留空", *fallbackKind)
	}

	instance, err := app.Bootstrap(app.Config{
		DataDir:      *dataDir,
		Addr:         "wails.localhost",
		FallbackKind: *fallbackKind,
		// 桌面端不开放任何 TCP 监听，没有第二屏需要配对：配对码留空即关闭鉴权。
		// 一旦将来为桌面端加"共享到局域网"开关，必须同时启用 PIN（见 http 的 embeddedHostAllowed 注释）。
		PIN:          "",
		EmbeddedHost: "wails.localhost",
		AppVersion:   appVersion,
	})
	if err != nil {
		return err
	}
	// wails.Run 返回（窗口关闭）后按依赖逆序释放：worker → 存储 → 数据目录锁。
	defer instance.Close()

	return wails.Run(&options.App{
		Title:     "SillyDog · TavernAgent",
		Width:     1440,
		Height:    900,
		MinWidth:  1024,
		MinHeight: 640,
		AssetServer: &assetserver.Options{
			// Assets 留空：所有请求（SPA 资源与 /api/*）都转给我们自己的 handler。
			Handler: instance.Server.Handler(),
		},
		OnStartup: func(ctx context.Context) {
			appCtxMu.Lock()
			appCtx = ctx
			appCtxMu.Unlock()
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			// 第二个实例不再去抢数据目录锁（那会报错），而是唤起已有窗口。
			UniqueId: "tavernagent-desktop",
			OnSecondInstanceLaunch: func(options.SecondInstanceData) {
				appCtxMu.Lock()
				ctx := appCtx
				appCtxMu.Unlock()
				if ctx == nil {
					return
				}
				runtime.WindowUnminimise(ctx)
				runtime.WindowShow(ctx)
			},
		},
		Windows: &wailswindows.Options{
			// WebView2 的 localStorage（角色库缓存等）与数据目录放在一起，便于整体备份/迁移。
			WebviewUserDataPath: filepath.Join(*dataDir, "webview"),
			// 标题栏与边框贴合应用配色（默认浅色启动），运行时由前端按应用主题切换
			// （web/src/lib/desktopShell.ts -> WindowSetLightTheme/DarkTheme）。
			// 否则切到墨黑幽夜时，深色界面顶上会挂着一根突兀的浅色标题栏。
			Theme:       wailswindows.Light,
			CustomTheme: titleBarTheme(),
		},
	})
}
