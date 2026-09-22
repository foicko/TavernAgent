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

### UI 层与主题

**新导航范式（M7）：舞台优先 · 覆盖式侧栏 · 悬浮 HUD · 命令驱动**
- **窗口**：原生材质（Win11 Mica/Acrylic、macOS vibrancy）→ 不可用时回退不透明；`AmbientBackground` 提供缓慢流动的环境光层。
- **顶栏**：44px 悬浮玻璃条（品牌 + 面包屑 + 居中视图模式 + 命令/模型/主题）；阅读模式或收起时隐藏。其左右内边距由 `WorkspaceViewModel.HeaderMargin` 计算，**HUD 展开时自动让出 344px，避免右侧控件被 HUD 压住**。
- **左侧**：56px 图标轨道（`IconRailView`，统一彩色 emoji + 确定性 emoji 字体栈）；点击展开玻璃**飞出面板**（`LeftRailView`），覆盖在舞台之上，不再挤压中央列。关闭时用 `visited` 类保留「上次所在页签」的弱提示。
- **中央**：全宽 + 居中阅读列（820px）；场景条为吸附玻璃 chip。
- **输入岛**：底部悬浮玻璃岛，聚焦时形态上浮（`Border.composer-island:focus-within`）；控制行统一 30px 高。
- **右侧**：贴边悬浮 HUD（`Border.hud`，滚动条 `AllowAutoHide` 自动隐藏），可收拢为召回按钮。
- **弹窗**：`Modal` 使用 `ExperimentalAcrylicBorder` 模糊 scrim + `anim-pop` 弹簧入场。

- **设计令牌**：`Styles/DesignTokens.axaml`（v2 增加语义 surface/border/text、elevation、material、ambient、z-index、spring 令牌；**仅系统字体栈**，不随包分发字体）。
- **动效库**：`Styles/Motion.axaml`（`anim-rise`/`anim-pop`/`anim-slide-in-*`/`anim-rise-in-bottom`/`anim-pulse`/`anim-spin`/`stagger-*`/`hover-lift`/`press`/`glide`/`soft-brush`）。
  **减少动态**：这些效果全部基于 `Style.Animations`，无法用后置 setter 覆盖，因此 `MotionPreference` 在检测到系统偏好（或 `TAVERNAGENT_REDUCE_MOTION=1`）时**直接不加载 `Motion.axaml`**，改为加载 `Styles/ReducedMotion.axaml` 清空过渡；`AmbientBackground` 的环境光漂移挂在 `.drift` 类上，同样不启用。
- **UI 基元**：`Controls/` 下的 `Pill`/`Field`/`Section`/`Modal`/`Disclosure`/`Switch`/`ListRow`/
  `EmptyState`/`Banner`/`Kbd`/`Skeleton`/`AmbientBackground` 以及 `InlineMarkdownText`（`**粗体**` / `*斜体*`）。
- **舞台**：`Views/Stage/` 下 `StageView`（居中 820px 阅读列 + 吸附场景条 + 悬浮输入岛）、
  `MessageView`（正文/对白/独白、思考链、依据、悬停浮现的回合微操作、抉择卡、插画、朗读）、`ComposerView`
  （注记/检定/宏、上下文甜甜圈、模型速切、思考强度）。
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
| M6.5 | ✅ | Director 导演工作台（状态条 + 双栏：与导演协商 / 阶段编辑器；启用/暂停/恢复/完成/跳过/回退；本地草稿缓存 + 700ms 自动保存；候选建议载入）。心智星图按需延后 |
| M7 | ✅ | 全量 UI 重设（设计系统 v2 + 舞台优先 · 覆盖式侧栏 · 悬浮 HUD · 命令驱动的新范式）、原生窗口材质、动效库与 reduced-motion、自包含打包 + sidecar 同捆、移除 Wails 旧壳 |
| M7.7 | ✅ | UI 走查修复：顶栏避让 HUD + 视图模式居中、图标轨道统一 emoji 字体、心境量表改用 `Meter*` 令牌并按 `Tone` 着色（新增中立刻度线）、HUD 滚动条自动隐藏、行囊空态去彩色 emoji、空态与输入占位文案更新（去掉「左侧栏」）、输入岛控制行统一高度、reduced-motion 改为不加载动效层 |

> 旧 Wails 桌面壳已**移除**（见上）。
