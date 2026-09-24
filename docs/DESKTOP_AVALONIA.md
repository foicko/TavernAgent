# 桌面端（跨平台 · Avalonia + Sidecar）

`desktop/` 是下一代桌面端：**Avalonia 12（.NET 10）+ Sidecar 子进程守护**。它不复刻
WebView，而是用原生 XAML 重新实现 Web 前端的界面与交互，后端仍是同一个 Go 无头服务
——以**子进程（sidecar）**方式拉起，只监听 loopback。

> 与已移除的旧 Wails 壳的区别：Avalonia 端是真正的原生渲染、跨平台
> （Windows / macOS / Linux），不依赖 WebView2/WebKit。旧壳（`cmd/tavernagent-desktop`、
> `web/src/lib/desktopShell.ts`、`scripts/build.ps1 -Desktop`）已在 M7 功能对等后删除。
> 无头服务中的 `/api/v1/desktop` 与 `/api/v1/desktop/theme` 端点保留（无害且仍有测试覆盖），
> 仅不再有 Wails 壳消费它们。

## 运行

```powershell
# 开发：构建 Go sidecar 到 build/ 并启动 Avalonia（自动发现 build/tavernagent.exe）
powershell -ExecutionPolicy Bypass -File scripts/dev-desktop.ps1 -Mock   # 离线 mock
powershell -ExecutionPolicy Bypass -File scripts/dev-desktop.ps1 -Rebuild # 强制重建 sidecar

# 仅构建/测试
dotnet build desktop/TavernAgent.Desktop.slnx
dotnet test  desktop/tests/TavernAgent.Core.Tests/TavernAgent.Core.Tests.csproj
dotnet test  desktop/tests/TavernAgent.Desktop.Tests/TavernAgent.Desktop.Tests.csproj

# 打包（自包含 + sidecar 同捆；win-x64 先行，mac/linux 脚本已就绪）
powershell -ExecutionPolicy Bypass -File scripts/publish-desktop.ps1 -Version 0.7.0 -Zip
powershell -ExecutionPolicy Bypass -File scripts/publish-desktop.ps1 -Rid osx-arm64
# 重新生成应用图标（项目自绘，MIT）
powershell -ExecutionPolicy Bypass -File scripts/make-icon.ps1
```

Sidecar 可执行文件的发现顺序：
1. 环境变量 `TAVERNAGENT_SIDECAR`（开发直指路径）
2. 桌面程序同目录 `tavernagent(.exe)` 或 `sidecar/` 子目录
3. 从程序目录向上最多 8 级，查找 `<dir>/build/tavernagent(.exe)` 或 `<dir>/tavernagent(.exe)`
   （覆盖仓库根的 `build/` 产物）
4. 环境变量 `TAVERNAGENT_PROVIDER=mock` 可强制离线 mock provider

## 解决方案结构

```
desktop/
├── TavernAgent.Desktop.slnx
├── src/
│   ├── TavernAgent.Sidecar/     # 子进程守护：拉起/健康检查/优雅停机/日志/单实例/Job Object
│   ├── TavernAgent.Core/        # 与 Web 对齐的数据层：REST + SSE 客户端、DTO、Zustand 端口
│   └── TavernAgent.Desktop/     # Avalonia UI：ViewModels + Views + Controls + Styles
└── tests/
    ├── TavernAgent.Core.Tests/     # 数据层单测 + SSE 解析 + sidecar 集成 + 端到端剧情流
    └── TavernAgent.Desktop.Tests/  # Avalonia.Headless 视图/VM 测试
```

### Sidecar 守护（`TavernAgent.Sidecar`）

- **启动**：`PortAllocator` 选一个空闲 loopback 端口 → 用 `-addr/-data[-provider]` 拉起
  `tavernagent` → 轮询 `/healthz` 直到就绪（默认 15s）。
- **停止**：Windows 走 `CTRL_BREAK`（FreeConsole→AttachConsole→GenerateConsoleCtrlEvent），
  Unix 走 `SIGTERM`；超过宽限期（默认 5s）强杀。
- **兜底**：Windows 用 **Job Object（KILL_ON_JOB_CLOSE）** 保证父进程崩溃时子进程随之退出。
- **单实例**：Windows 命名互斥体，其它平台数据目录锁文件；先于 sidecar 获取，避免
  “数据目录被占用”的误导性报错。
- **日志**：持续泵出 sidecar stdout/stderr 到 `<dataDir>/logs/sidecar.log`（超限滚动）。

### 数据层（`TavernAgent.Core`）

与 `web/src` 一一对应：`TavernApiClient` 对应 `api.ts`（鉴权、30s 超时、二进制路径），
`SseClient` 对应 SSE 订阅（Last-Event-ID 续传、`aggregateId:seq` 去重、停顿看门狗、退避重连），
`StoryStore`/`SettingsStore`/`UiStore` 对应各自的 Zustand store（session/turn/branch/memory/director）。

### 稳定性与线程模型

- **崩溃兜底**：`Program.cs` 订阅 `Dispatcher.UIThread.UnhandledException`（另有 `AppDomain` /
  `TaskScheduler`），异常写入 `<dataDir>/logs/desktop-error.log` 并以提示条呈现，不再静默退出进程。
- **线程模型**：Core 的 store 通知（SSE / 轮询定时器 / HTTP 续体）统一经 `Core.Stores.StoreDispatcher.Post`
  投递到 UI 线程（宿主在 `Program.Main` 注入 `UiDispatcher.Post`）；无宿主（单测）时同步触发。
  ViewModel 侧的订阅走 `UiEvents.PropertyChanged/Event`（返回可退订令牌）或 `UiDispatcher.Post`，
  杜绝后台线程改绑定的 `ObservableCollection`（Avalonia `Collection was modified` 崩溃）。
- **后台任务**：`Task.SafeFireAndForget()` 取代裸 `_ = XxxAsync()`，异常不再逃逸到 dispatcher/AppDomain。
- **窗口 chrome**：`WindowDecorations="BorderOnly"` + 自绘顶栏（显式 `BeginMoveDrag`），
  **不使用** `ExtendClientAreaToDecorationsHint` / `WindowDrawnDecorations`（Avalonia 12 会抛
  `The property HasTitleBar is readonly` 启动崩溃）。退出时同步等待 sidecar 停机，避免孤儿进程。
- **资源**：`ImageLoader` 使用有上限的缓存且失败不永久缓存；`TavernApiClient` 的响应体读取纳入 30s 超时。
- **原生互操作**：`WindowsProcess` 命令行转义按 `CommandLineToArgvW` 规则、管道创建原子化；
  `SidecarChild` 释放后置空句柄、`kill(-1)` 守卫；`GracefulStopWindows` 恢复 `SetConsoleCtrlHandler`。

### UI 层与主题

**导航范式（v4）：扁平纸墨 · 三栏内嵌 · 命令驱动**（对标 beautifului.dev，与 Web 同构）
- **窗口**：完全不透明扁平外壳（已移除 Win11 Mica/Acrylic、macOS vibrancy 与 `AmbientBackground` 氛围光层）。
- **外壳**：`WorkspaceView` 为 `Grid RowDefinitions="Auto,*" ColumnDefinitions="Auto,*,Auto"`——第 0 行顶栏横跨三列，第 1 行左栏 / 舞台 / 右栏全部内嵌，不再有悬浮顶栏与覆盖式 HUD。
- **顶栏**：内嵌 `56px` 扁平条（`Height={DynamicResource HeaderHeightPx}`，表面 + 底部发丝线；品牌 + 面包屑 + 居中视图模式 + 命令/模型/主题）；阅读模式保留顶栏、仅隐藏左右栏。
- **左侧**：常驻可折叠侧栏 `224px`（展开，`LeftRailView`）/ `52px`（收起，`IconRailView`），宽度走 `DoubleTransition` 240ms；同页签再次点击收拢为图标轨，收拢后用 `visited` 类保留「上次所在页签」的弱提示。
- **中央**：全宽 + 居中阅读列（820px）；场景条为内嵌扁平表面卡（`Border.surface`）。
- **输入岛**：底部扁平 surface 卡 + 发丝线（`Border.composer-island`），聚焦时以 `RingBrush` 描边；控制行统一 30px 高。
- **右侧**：内嵌 `304px` 栏（`RightRailWidth`，收起为 0 并显示舞台右上角的扁平召回 pill），滚动条 `AllowAutoHide` 自动隐藏；含立绘/神态、名牌、三维心境、行囊、**会话变量**（卡片 `[VAR]`/`defaultVariables`，只读）、前情回顾。卡片自带情绪立绘（`assets[type=emotion]`）与主题 token 分别用于立绘换图与卡片表面配色（见 [CARD_UI_MARKERS](CARD_UI_MARKERS.md)）。
- **弹窗**：`Modal` 使用扁平 scrim（`ScrimBrush`）+ `anim-pop` 弹簧入场；已无 `ExperimentalAcrylicBorder`。

- **设计令牌**：`Styles/DesignTokens.axaml`（v4 全面对齐 beautifului.dev：page/canvas/surface/inset 中性面、ink 三级文字、单一蓝色强调、发丝线描边与低透明阴影，无毛玻璃/无装饰渐变；`HeaderHeightPx=56`、`ScrimBrush`、`RingThickness`；**仅系统字体栈**，不随包分发字体）。
- **表面与按钮**：`Styles/Surfaces.axaml`（原 `Glass.axaml`）定义 `Border.surface`/`surface-inset`/`surface-elevated`/`panel`；`Button.ui-btn` 变体矩阵 primary / secondary / outline / quiet / ghost / danger / link × xs/sm/md/lg + `icon`（旧 `Button.pill/.primary/.danger` 别名已删除）。
- **主题**：默认跟随系统（`Application.PlatformSettings.GetColorValues().ThemeVariant`，`App.axaml` 的 `RequestedThemeVariant="Default"`）；用户手动切换后经 `DesktopSettings.Theme` 持久化到本地 JSON。
- **动效库**：`Styles/Motion.axaml`（`anim-rise`/`anim-pop`/`anim-slide-in-*`/`anim-rise-in-bottom`/`anim-pulse`/`anim-spin`/`stagger-*`/`hover-lift`/`press`/`glide`/`soft-brush`）。
  **减少动态**：这些效果全部基于 `Style.Animations`，无法用后置 setter 覆盖，因此 `MotionPreference` 在检测到系统偏好（或 `TAVERNAGENT_REDUCE_MOTION=1`）时**直接不加载 `Motion.axaml`**，改为加载 `Styles/ReducedMotion.axaml` 清空过渡。
- **UI 基元**：`Controls/` 下的 `Pill`/`Field`/`Section`/`Modal`/`Disclosure`/`Switch`/`ListRow`/
  `EmptyState`/`Banner`/`Kbd`/`Skeleton` 以及 `InlineMarkdownText`（`**粗体**` / `*斜体*`）。
- **图标**：`Styles/Icons.axaml` 提供 24×24 描边几何（`StreamGeometry`）与 `Path.icon` 样式（`.accent/.primary/.danger/.warn` 变体），
  与 Web 端 `web/src/ui/Icon.tsx` 一一对应；外壳/导航/工具条的装饰性 emoji 已统一替换为线性图标。
- **舞台**：`Views/Stage/` 下 `StageView`（居中 820px 阅读列 + 吸附场景条 + 悬浮输入岛）、
  `MessageView`（正文/对白/独白、思考链、依据、悬停浮现的回合微操作、抉择卡、插画、朗读）、`ComposerView`
  （导演模式/注记/宏、上下文甜甜圈、模型速切、思考强度）。检定由 LLM 在剧情节点给出的选项触发，输入台不再提供手动检定选择器。
- **设置与弹窗**：`Views/Modals/Settings/` 四页签设置中心；角色导入 / 世界书 / 生图弹窗；
  命令面板（`Views/CommandPaletteView`，Ctrl+K）。
- **导演模式**：`Views/Stage/DirectorWorkspaceView`（双栏工作台）+ 输入岛「🎬 导演」入口，
  Core `StoryStore.Director` 提供 load/save/command/discuss + 持久轮询+SSE 看门狗与幂等键。
- **音频**：TTS 播放走 **SoundFlow**（miniaudio 后端，跨平台、内置 MP3 解码），
  合成仍由 sidecar `/api/v1/tts/synthesize` 完成，客户端按文本键缓存。
- **动效**：轨道宽度 240ms spring、模态淡入缩放、决策卡 hover 抬升、打字光标/骨架 shimmer。

## 打包与分发

`scripts/publish-desktop.ps1` 产出**自包含**桌面端并同捆 Go sidecar：

- `dotnet publish -c Release -r <rid> --self-contained`（`PublishTrimmed=false`、`InvariantGlobalization=false`）。
- 按 RID 交叉编译 sidecar（`CGO_ENABLED=0`）到发布目录 `sidecar/tavernagent(.exe)`，
  与 `DesktopPaths` 的查找顺序（同目录 → `sidecar/` → 向上找 `build/`）一致。
- 应用图标为项目自绘（`scripts/make-icon.ps1` → `Assets/app.ico`，MIT）；
  Windows 清单 `app.manifest` 声明 PerMonitorV2 DPI 与 longPathAware。
- 已验证：`win-x64`（约 77 MB zip，含 `miniaudio.dll`）。`osx-arm64` / `linux-x64` 分支与
  `.icns`/`.desktop` 模板已就绪，待在对应平台验证。
- 原生音频库（SoundFlow → `miniaudio`）随包分发，属第三方运行库（见 `THIRD_PARTY_NOTICES.md`）。

## 里程碑

| 里程碑 | 状态 | 内容 |
|---|---|---|
| M0–M2 | ✅ | 解决方案脚手架、Sidecar 守护、Core 数据层、启动闪屏与主题 |
| M3 | ✅ | 设计令牌全量对齐、UI 基元、三栏外壳（顶栏/侧栏折叠/纯净编年/Toast） |
| M4 | ✅ | 中央舞台：叙事流、思考链、回合依据与微操作、抉择卡、悬浮输入岛、上下文甜甜圈、模型/强度速切、会话引导 |
| M5 | ✅ | 左栏（因果树 + 快照探针 + 剧情分支图 + 大纲 / 角色卡库 / 世界书 / 会话 + 剧情包导入导出）与右栏（立绘 + 神态 + 名牌 + 三维心境 + 浮动增减 + 行囊动作气泡 + 前情回顾）、栏内弹窗（档案/誓约/记忆库）、回合候选版本切换 |
| M6 | ✅ | 设置中心（模型与连接 CRUD/槽位指派/探测 + 交互偏好；语音 / 图像 / 记忆向量四页签，本地 JSON 持久化）、角色导入弹窗（拖放/选择 → 服务端解析 → 开新会话）、世界书示例、命令面板（Ctrl+K，跳转/剧情/视图/面板）、回合插画（提示词提取 + 生图弹窗 + 灯箱）、TTS 朗读（SoundFlow/miniaudio 播放 + MiMo 情境演播） |
| M6.5 | ✅ | Director 导演工作台（状态条 + 双栏：与导演协商 / 阶段编辑器；启用/暂停/恢复/完成/跳过/回退；本地草稿缓存 + 700ms 自动保存；候选建议载入） |
| M7 | ✅ | 全量 UI 重设（舞台优先 · 悬浮 HUD · 命令驱动）、动效库与 reduced-motion、自包含打包 + sidecar 同捆、移除 Wails 旧壳 |
| M7.7 | ✅ | UI 走查修复：顶栏避让 HUD + 视图模式居中、图标轨道统一 emoji 字体、心境量表改用 `Meter*` 令牌并按 `Tone` 着色（新增中立刻度线）、HUD 滚动条自动隐藏、行囊空态去彩色 emoji、空态与输入占位文案更新（去掉「左侧栏」）、输入岛控制行统一高度、reduced-motion 改为不加载动效层 |
| M8 | ✅ | UI 重构对标 beautifului.dev：设计系统 v3 扁平发丝线（page/canvas/surface/inset + 单一蓝 accent）、移除毛玻璃/亚克力/氛围光、常驻折叠侧栏（224/52）与 68px 顶栏、阅读列 820、右栏 304；两端同步移除「心智星图」功能（Web 的 MindGraphModal 与桌面端遗留 `UiStore.MindGraphOpen` 死状态） |
| M9 | ✅ | 两端对齐「三栏内嵌」范式：桌面端顶栏 56px 内嵌 + 左/右栏并排（右栏 304/0，240ms 过渡，删除悬浮 HUD/`HeaderMargin`）；`Glass.axaml` → `Surfaces.axaml` 并统一 `surface*` 类名、补齐 `Button.quiet`、删除旧按钮别名；`Modal` 去亚克力改扁平 `ScrimBrush`；`EmptyState` 改 `IconGeometry`；主题默认跟随系统并持久化；Web 顶栏去展厅入口、主题持久化/跟随系统、`--glass-*` 兼容层整体删除、六项 CSS 门禁收紧（`!important` 14→4、重复选择器 18→0、颜色字面值与玻璃残留归零） |
| M10 | ✅ | 桌面端稳定性与布局对齐：修复 Avalonia 12 启动崩溃（去 `WindowDrawnDecorations`）、跨线程集合崩溃（`StoreDispatcher`/`UiEvents`/`SafeFireAndForget` + UI 异常兜底）、重试泄漏与资源泄漏；响应式轨道对称可再展开（1280/1024，auto 标志与手动偏好分离）；输入岛宽度对齐 820、控制行统一 30px；Modal 单一滚动 + 焦点 + 88% 上限 + 18 圆角；补 `SceneBarView` 接线、缺失画刷、`.fork` 样式与 token 化；补回归测试 |
| charx UI | ✅ | 卡片 UI 映射（M2–M4）：`<img cmd>`/`[HUD]`/`[SCENE]`/`[VAR]` 标记原生渲染（含 HUD 进度条、常驻场景条）、右栏会话变量面板、卡片主题 token（仅 `CardHudView`/`SceneBarView`）、情绪立绘（`assets[type=emotion]` → 神态键）、`sdData` 生图基底、`{{getvar::键}}` 只读宏；见 [CARD_UI_MARKERS](CARD_UI_MARKERS.md) |

> 旧 Wails 桌面壳已**移除**（见上）。
