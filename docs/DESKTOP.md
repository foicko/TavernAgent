# 桌面端（Windows）

`cmd/tavernagent-desktop` 是 Windows 桌面壳：**Wails v2 + WebView2**，界面仍是现有的
React 应用，后端仍是 `internal/app` 装配出的同一个服务。桌面端不是"另一套后端"，
它只是换了个方式承载同一个 `http.Handler`。

## 运行

```powershell
# 从源码直接跑（开发标签；无标签会弹 “correct build tags” 报错框）
go run -tags "desktop,dev" ./cmd/tavernagent-desktop

# 构建（连同无头服务一起；桌面用 production 标签）
powershell -ExecutionPolicy Bypass -File scripts/build.ps1 -Version dev -Desktop
# 产物：build/tavernagent.exe（无头）与 build/tavernagent-desktop.exe（桌面）
```

> **构建标签是必须的**：Wails v2 在 `!dev && !production` 时会编译成
> `internal/app/app_default_windows.go`——一个只弹
> “Wails applications will not build without the correct build tags” 对话框、
> 根本不创建窗口的变体。因此桌面端必须带 `desktop,production`（发布）或
> `desktop,dev`（开发）。`scripts/build.ps1 -Desktop` 已经内置发布标签。
> 注意：不带标签的 `go build ./...`、`go vet ./...` 仍能通过（它只是编译了那个
> 弹框变体），所以这类检查**不能**当作“桌面端可运行”的证据。

数据目录默认在 `%APPDATA%\TavernAgent`（可用 `-data` 覆盖）；WebView2 的用户数据
（含 localStorage 里的角色库缓存）落在该目录的 `webview` 子目录，便于整体备份。

## 图标与窗口栏

- **应用图标**：源图为 `packaging/windows/icon.ico`（由 `packaging/windows/make_icon.py`
  绘制，母题取自现有 `custom.svg`：板岩玻璃底 + 浅色剪影）。改图标后重跑该脚本即可；
  `python packaging/windows/make_icon.py --preview` 会额外输出 `output/icon-preview.png`
  （多尺寸对照，`output/` 不入库）。
- **exe 资源**：`cmd/winresgen` 把图标、DPI 清单（`packaging/windows/app.manifest`，
  `permonitorv2`）与 VERSIONINFO 编成 `rsrc_windows_<arch>.syso`。Go 构建会自动拾取包目录下的
  `.syso`，**无需 `wails build` 或 `rc.exe`**。`scripts/build.ps1 -Desktop` 已内置这一步。
  `.syso` 是生成物（已 gitignore）。
- **窗口栏配色**：`windows.Options.CustomTheme` 把原生标题栏/边框对齐到应用设计令牌
  （浅色 #f9f8f5 / 深色 #111215 等）；前端 `web/src/lib/desktopShell.ts` 在主题变化时
  调用 `window.runtime.WindowSetLightTheme/DarkTheme`，避免深色界面顶着一根浅色标题栏。

## 架构要点

- **同一后端**：`internal/app.Bootstrap` 被 `cmd/tavernagent`（无头）与
  `cmd/tavernagent-desktop`（桌面）共用，避免两端行为漂移。
- **窗口地址是 `http://wails.localhost/`**：Wails 的 `AssetServer.Handler` 把
  整个后端（SPA 资源 + `/api/*`）交给进程内资源服务。因此界面与 API **同源**：
  不需要 TCP 端口、不需要 CORS，也不会与用户已运行的命令行实例抢 `8890`。
- **嵌入式主机白名单**：`http.Deps.EmbeddedHost`（桌面端设为 `wails.localhost`）
  是唯一的额外放行来源，见 `internal/adapters/http/server.go` 的
  `embeddedHostAllowed`。它只对进程内请求生效；一旦将来开放 TCP，必须同时启用
  配对码（`PIN`），否则伪造同名 Host 的局域网请求会绕过来源校验。
- **单实例**：`options.SingleInstanceLock` 让第二次启动唤起已有窗口，而不是去抢
  数据目录锁（那会直接报错）。
- **退出顺序**：`wails.Run` 返回后 `App.Close()` 按依赖逆序释放
  （worker → 存储 → 数据目录锁），与无头模式的 `defer` 链一致。

## 为什么只做 Windows

Wails v2 的 cgo 只出现在 macOS/Linux 前端（系统 WebKit/Obj-C）。Windows 前端基于
WebView2，**不需要 cgo**，因此能保持本项目 `CGO_ENABLED=0` 的构建矩阵不被污染。
`cmd/tavernagent-desktop` 用 `//go:build windows` 隔离，非 Windows 平台由一个
stub 文件兜底，保证 `go build ./...`、`go vet ./...` 仍可通过。

## 已知待办

- **导出改用原生保存对话框**：`web/src/lib/packFile.ts` 的 `downloadBlob()` 依赖
  `a.download`；WebView2 能下载，但不如原生对话框可控（`.tavernpack` 导出建议接
  Wails dialog）。
- **托盘常驻与开机自启**：尚未接入。
- **代码签名**：未签名时 Windows SmartScreen 会拦截首次运行。
- **局域网第二屏**：桌面端当前不开放 TCP。若要保留"手机/平板连 PC 游玩"，需加一个
  显式开关：开启时监听局域网端口并**强制启用配对码**。
- **完全无边框的自定义标题栏**：当前用的是原生标题栏 + 配色对齐；若想彻底统一视觉，
  可改 `Frameless: true` 并在前端自绘标题栏（Wails 已支持无边框拖拽与边缘缩放）。
