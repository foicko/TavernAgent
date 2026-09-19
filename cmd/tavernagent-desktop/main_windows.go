//go:build windows

// tavernagent-desktop 是 Windows 桌面壳（Wails v2 + WebView2）。
//
// 它不是"另一个后端"：internal/app 装配出的后端与命令行版完全一致，桌面壳只做
// 三件事——起一个 loopback 监听、把窗口指过去、把窗口配色接回前端。
//
// 为什么窗口不能停在 Wails 的资源服务主机名上：WebView2 的资源响应必须**整包交付**
// （wails 的 responsewriter_windows.go 把响应体攒进 bytes.Buffer，等 handler 返回才
// PutByteContent），SSE 在那里无法流式送达——fetch 连响应头都收不到，既不报错也不
// 重连，于是"生成中的增量"与"后台认知已更新"的通知全部静默丢失，读者下一次提交
// 就会带着过期的分支头撞上 409。loopback 上是真正的 TCP 连接，与浏览器端行为一致，
// 界面与 /api/* 同源，不需要 CORS。
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailswindows "github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"

	"tavernagent/internal/app"
)

// appVersion 由构建脚本用 -ldflags -X main.appVersion=... 注入。
var appVersion = "tavernagent/dev"

// defaultAddr 是桌面端的固定监听地址。
//
// 必须固定：浏览器存储（localStorage）按**源**隔离，而源包含端口。端口一变，
// 上次打开的会话、卡片缓存、导演草稿备份就全部读不到了。8891 与命令行版的
// 默认端口 8890 相邻，便于记忆，也避免与无头实例抢同一个端口。
const defaultAddr = "127.0.0.1:8891"

func main() {
	if err := run(); err != nil {
		log.Printf("桌面端退出: %v", err)
		fatalDialog(err.Error() + logHint())
		os.Exit(1)
	}
}

// logRotateBytes 是日志文件的最大体积。桌面壳把每个 HTTP 请求都记一行，
// 长期运行会一直长，所以到量就滚动一次（只保留上一份）。
const logRotateBytes = 2 << 20

// logFile 记录当前日志文件路径，用于在失败时告诉用户去哪儿看详情。
var logFile string

func logHint() string {
	if logFile == "" {
		return ""
	}
	return "\n\n详细日志：" + logFile
}

// setupLogging 把日志接到数据目录下的文件，同时保留 stderr。
//
// 桌面壳是 GUI 子系统程序（构建时 -H windowsgui）：进程不分配控制台，双击启动时
// stderr 无处可去，于是"端口被占""数据目录已被另一个实例锁住"这类启动期信息
// 就彻底看不到了。所以日志必须落盘；从终端启动或重定向时 stderr 仍然有效，
// 两边都写，开发时照样能直接看。
func setupLogging(dataDir string) {
	dir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("提示：无法创建日志目录 %s（%v），本次只写 stderr", dir, err)
		return
	}
	path := filepath.Join(dir, "tavernagent-desktop.log")
	if info, err := os.Stat(path); err == nil && info.Size() >= logRotateBytes {
		_ = os.Remove(path + ".1")
		if err := os.Rename(path, path+".1"); err != nil {
			_ = os.Truncate(path, 0)
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Printf("提示：无法打开日志文件 %s（%v），本次只写 stderr", path, err)
		return
	}
	logFile = path
	// 句柄故意不关：日志要活到进程退出，退出时由内核回收。
	log.SetOutput(io.MultiWriter(os.Stderr, f))
}

// fatalDialog 用原生消息框报告启动失败。
//
// 没有控制台时，"双击后什么都没发生"是最难排查的故障形态：启动期错误（数据目录
// 被另一个实例锁住、端口无法监听等）必须在窗口出现之前就能被看见。
// 窗口起来之后的错误走前端提示，不走这里。
func fatalDialog(text string) {
	caption, err := windows.UTF16PtrFromString("SillyDog · TavernAgent 启动失败")
	if err != nil {
		return
	}
	message, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	const mbOK, mbIconError, mbSetForeground = 0x0, 0x10, 0x10000
	_, _ = windows.MessageBox(0, message, caption, mbOK|mbIconError|mbSetForeground)
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

// appCtx 在 OnStartup 时写入，之后会被 HTTP 处理协程（切换主题）读取，必须加锁。
var (
	appCtxMu sync.Mutex
	appCtx   context.Context
)

// setNativeTheme 由 HTTP 适配层回调（POST /api/v1/desktop/theme）。
//
// 页面跑在 loopback 源上，拿不到 Wails 注入的 window.runtime，因此窗口配色这条桥
// 只能反向走我们的同源接口。窗口还没起来时静默忽略：这只是显示偏好。
func setNativeTheme(mode string) {
	appCtxMu.Lock()
	ctx := appCtx
	appCtxMu.Unlock()
	if ctx == nil {
		return
	}
	if mode == "dark" {
		runtime.WindowSetDarkTheme(ctx)
		return
	}
	runtime.WindowSetLightTheme(ctx)
}

// listen 占用桌面端端口。固定端口被占时降级到随机端口，但要大声说出来——
// 换了端口就等于换了源，本地偏好会读不到，不能静默发生。
func listen(addr string) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err == nil {
		return ln, nil
	}
	if addr != defaultAddr {
		return nil, fmt.Errorf("监听 %s 失败: %w", addr, err)
	}
	fallback, ferr := net.Listen("tcp", "127.0.0.1:0")
	if ferr != nil {
		return nil, fmt.Errorf("监听 %s 失败（%v），回退随机端口也失败: %w", addr, err, ferr)
	}
	log.Printf("⚠️  %s 已被占用，本次改用 %s：本地偏好（上次会话、卡片缓存等）不会保留；服务端数据不受影响。",
		addr, fallback.Addr())
	return fallback, nil
}

func run() error {
	dataDir := flag.String("data", defaultDataDir(), "数据目录")
	addr := flag.String("addr", defaultAddr, "监听地址（本地偏好按源隔离，请保持端口稳定）")
	fallbackKind := flag.String("provider", "", "未配置时回退的开发供应商（mock）")
	flag.Parse()
	if *fallbackKind != "" && *fallbackKind != "mock" {
		return fmt.Errorf("不支持的开发供应商 %q；可使用 mock 或留空", *fallbackKind)
	}
	setupLogging(*dataDir)

	// 先占端口再装配：Addr 要写进 Config（服务端据此判断同源写请求），
	// 而"端口被占用"这类失败应该在打开窗口之前就报出来。
	ln, err := listen(*addr)
	if err != nil {
		return err
	}
	networkAddr := ln.Addr().String()
	origin := "http://" + networkAddr

	instance, err := app.Bootstrap(app.Config{
		DataDir:      *dataDir,
		Addr:         networkAddr,
		FallbackKind: *fallbackKind,
		// 只监听 loopback，没有第二屏需要配对：配对码留空即关闭鉴权（与无头模式的
		// 本机免检一致）。将来若要开放局域网共享，必须同时启用 PIN。
		PIN:            "",
		SetNativeTheme: setNativeTheme,
		AppVersion:     appVersion,
	})
	if err != nil {
		_ = ln.Close()
		return err
	}
	// wails.Run 返回（窗口关闭）后按依赖逆序释放：监听 → worker → 存储 → 数据目录锁。
	defer instance.Close()

	httpSrv := &http.Server{Handler: instance.Server.Handler()}
	go func() {
		if err := httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP 服务退出: %v", err)
		}
	}()
	defer func() { _ = httpSrv.Close() }()

	log.Printf("桌面端服务就绪：%s（数据目录 %s）", origin, *dataDir)

	return wails.Run(&options.App{
		Title:     "SillyDog · TavernAgent",
		Width:     1440,
		Height:    900,
		MinWidth:  1024,
		MinHeight: 640,
		AssetServer: &assetserver.Options{
			// 资源服务只做一件事：把窗口重定向到真正的 loopback 服务（见文件头注释）。
			// 到达之后，静态资源、/api/* 与 SSE 全部由 loopback 服务承载。
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, origin, http.StatusFound)
			}),
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
			// （web/src/lib/desktopShell.ts -> POST /api/v1/desktop/theme）。
			// 否则切到墨黑幽夜时，深色界面顶上会挂着一根突兀的浅色标题栏。
			Theme:       wailswindows.Light,
			CustomTheme: titleBarTheme(),
		},
	})
}
