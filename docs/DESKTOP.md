# 桌面端（Windows）

`cmd/tavernagent-desktop` 是 Windows 桌面壳：**Wails v2 + WebView2**，界面仍是现有的
React 应用，后端仍是 `internal/app` 装配出的同一个服务。桌面端不是“另一套后端”：
它把同一个 `http.Handler` 架在 loopback 上，只是额外开了一个原生窗口。

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

### 控制台窗口与日志

桌面端 exe 用 `-H windowsgui` 构建，是 **GUI 子系统**程序，双击启动不会弹出控制台窗口。
`scripts/build.ps1` 构建完会读 PE 头的 Subsystem 字段自检（2 = GUI，3 = 控制台），
漏掉该标志直接报错——这个参数很容易在改动中被顺手丢掉，而丢掉的后果是每次启动
都多一个空白终端窗口。

> `go run ./cmd/tavernagent-desktop` 仍会占用当前终端，那是 `go run` 自身的行为
> （它会新建一个临时可执行文件并前台运行），与最终产物无关。

没有控制台之后，日志不能只写 stderr：

- 日志同时写 `<dataDir>/logs/tavernagent-desktop.log`（超过 2 MiB 滚动一次，保留上一份）
  与 stderr，因此从终端启动或重定向时照样能直接看。两个目标用 `logSink` 而不是
  `io.MultiWriter` 串起来：后者遇到第一个写失败的 writer 就 return，而无控制台启动时
  stderr 是无效句柄——那样会让日志文件那份副本也被吞掉，文件恒为空。
- **启动期失败**（数据目录被另一个实例独占锁住、端口无法监听等）会弹原生消息框说明
  原因并指向日志文件。少了这一步，“双击后什么都没发生”是最难排查的故障形态。
  窗口起来之后的错误仍走前端提示。

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
  调 `POST /api/v1/desktop/theme`，由桌面壳转成
  `runtime.WindowSetLightTheme/DarkTheme`，避免深色界面顶着一根浅色标题栏。
  （页面跑在 loopback 源上，拿不到 Wails 注入的 `window.runtime`，所以这条桥必须
  走我们自己的同源接口；纯浏览器部署下该接口 404，前端安静降级。）

## 架构要点

- **同一后端**：`internal/app.Bootstrap` 被 `cmd/tavernagent`（无头）与
  `cmd/tavernagent-desktop`（桌面）共用，避免两端行为漂移。
- **窗口加载的是真正的 loopback 服务**（默认 `http://127.0.0.1:8891/`）：
  Wails 的资源服务只把一个 302 重定向交给窗口，到达之后静态资源、`/api/*` 与
  SSE 全部由 `net/http` 承载。界面与接口**同源**，不需要 CORS。

  > **为什么不能把 API 放在资源服务里**：WebView2 的资源响应必须**整包交付**
  > （`wails/v2/pkg/assetserver/webview/responsewriter_windows.go` 把响应体攒进
  > `bytes.Buffer`，等 handler 返回才 `PutByteContent`），因此 SSE 在那里根本无法
  > 流式送达：`fetch` 连响应头都收不到，既不报错也不重连。后果是“生成中的增量”
  > 与“后台已更新”的通知全部静默丢失——读者会拿着过期的分支头去提交，于是撞上
  > `HEAD_CONFLICT`（“分支已变化，请刷新当前节点”），而正文只能靠 5 秒轮询刷新。
  > 这是 SSE 必须走真实 TCP 连接的原因，不是可选的优化。
- **端口固定**（默认 `127.0.0.1:8891`，可用 `-addr` 覆盖）：浏览器存储
  （localStorage）按**源**隔离，而源包含端口。换端口就等于换了源——上次打开的会话、
  卡片缓存、导演草稿备份会全部读不到。因此固定端口被占用时会大声降级到随机端口
  并写明后果，而不是静默换一个。
- **单实例**：由 `run()` 开头的 `acquireSingleInstance`（见 `single_instance_windows.go`）
  用命名互斥体判定，检测到已有实例就按窗口标题唤起它并安静退出。
  **这条判定必须排在 `app.Bootstrap` 之前**：Bootstrap 会抢数据目录的独占锁，而
  `wails.Run` 里的 `options.SingleInstanceLock` 在那之后才生效——放在后面的话，第二个
  实例会先撞上"数据目录已被其他进程使用"并弹错误框，`SingleInstanceLock` 永远轮不到
  执行。所以这里刻意**不再设** `options.SingleInstanceLock`（同一判断的重复实现，且位置
  过晚）。`-autostart on/off` 是脚本化入口，排在单实例判定之前，不受影响。
- **退出顺序**：`wails.Run` 返回后 `App.Close()` 按依赖逆序释放
  （监听 → worker → 存储 → 数据目录锁），与无头模式的 `defer` 链一致。

## 为什么只做 Windows

Wails v2 的 cgo 只出现在 macOS/Linux 前端（系统 WebKit/Obj-C）。Windows 前端基于
WebView2，**不需要 cgo**，因此能保持本项目 `CGO_ENABLED=0` 的构建矩阵不被污染。
`cmd/tavernagent-desktop` 用 `//go:build windows` 隔离，非 Windows 平台由一个
stub 文件兜底，保证 `go build ./...`、`go vet ./...` 仍可通过。

## 开机自启与局域网第二屏

两者都不需要打开窗口，直接执行后退出（便于脚本化与断言）：

```powershell
build\tavernagent-desktop.exe -autostart on    # 写 HKCU\...\Run（任务管理器可见、可关）
build\tavernagent-desktop.exe -autostart off
# 局域网第二屏：另起一个监听，强制配对码；-tls=auto 时叠加自签证书
build\tavernagent-desktop.exe -lan -tls auto
```

- 自启实现见 `internal/util/autostart`：HKCU 而非 HKLM（不需要提权），命令带引号
  （安装路径常含空格，不带引号会被拆成“程序 + 参数”，表现为开机什么都没发生）。
- 第二屏见 `cmd/tavernagent-desktop/lan_windows.go`：**独立端口**（默认 8892）而不是
  复用窗口那个——窗口的源必须稳定在 `127.0.0.1`（localStorage 按源隔离），而且自签
  证书会让 WebView 弹证书警告；分开之后加密只作用在第二屏，桌面窗口完全不受影响。
- 开启第二屏时配对码是强制的：没有配对码的局域网暴露等于把故事库交给同网段所有设备。
  日志会打印配对码与可直接在手机上输入的地址。

## 已知待办

- **系统托盘常驻**：未实现。托盘需要在 Wails 的消息循环之外自建一个 Win32 隐藏窗口 +
  `Shell_NotifyIcon` 消息循环（社区 systray 库在 Windows 侧带 cgo，会破坏
  `CGO_ENABLED=0`），而这类代码**必须**在真机 GUI 会话里验证过才能合入——只编译通过
  不足以证明它不与 Wails 的循环打架。目前"常驻"需求由 `-autostart` 覆盖，
  开关由 `-lan` / `-autostart` 两个命令行入口承担。
- **导出改用原生保存对话框**：`web/src/lib/packFile.ts` 的 `downloadBlob()` 依赖
  `a.download`；WebView2 能下载，但不如原生对话框可控（`.tavernpack` 导出建议接
  Wails dialog）。
- **代码签名**：未签名时 Windows SmartScreen 会拦截首次运行。
- **完全无边框的自定义标题栏**：当前用的是原生标题栏 + 配色对齐；若想彻底统一视觉，
  可改 `Frameless: true` 并在前端自绘标题栏（Wails 已支持无边框拖拽与边缘缩放）。
