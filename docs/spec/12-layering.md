# 12 三端分层与目录（机器强制）

> 决策：**三端都定义分层并机器强制**。

## 1. Go 后端

依赖方向（单向）：

```
domain ← ports ← context/application/pack ← adapters
```

- `internal/textutil` 是**仅依赖标准库的叶子包**，任何层可 import。
- `domain` 不得 import `application/adapters/context/ports/protocol`。
- `ports` 不得 import `application/adapters/context/protocol`。
- `context`/`pack`/`protocol` 不得 import `application/adapters`。
- 强制：`scripts/layering_check.py`（用 `go list` 的 imports 做 deny-list 检查），已入 CI。

目录职责：

| 目录 | 职责 |
|---|---|
| `cmd/tavernagent`、`cmd/tavernctl` | 入口：服务端 / 离线 CLI |
| `internal/domain` | 纯领域模型与规则，无 IO |
| `internal/ports` | 接口契约（存储、供应商、嵌入等） |
| `internal/application` | 用例编排（回合、导演、记忆、卡片…） |
| `internal/context`、`internal/pack` | 上下文组装 / 归档 |
| `internal/protocol` | 逐行 JSON 帧协议、事件常量 |
| `internal/adapters/*` | HTTP、SQLite、providers、image、tts、embedding、cardassets、mcp |
| `internal/util/*` | 叶子工具（id、datalock、tlscert、endpointguard、httpbody…） |

## 2. Web 前端

层级与允许的 import（由 `web/scripts/check.mjs` + eslint 强制，`pnpm lint`）：

| 层 | 目录 | 允许 import |
|---|---|---|
| 组件层 | `src/components/` | `react`、`zustand/react/shallow`、`../app/`、`../stores/`、`../lib/`、`../ui/`、`../styles`、`./` |
| 原语层（叶子） | `src/ui/` | 仅 `react`、`../lib/`、`./` |
| 其他 | `src/app/`、`src/stores/`、`src/lib/`、`src/styles/` | 按 ESLint 规则 |

- `src/ui/` **不得**依赖 stores/app/components。
- 样式必须走令牌：只能引用 `src/styles/tokens.css` 的变量；禁止 `!important`、颜色/字号字面值、跨文件重复选择器、死类名、`backdrop-filter`/`--glass-*`（六项 CSS 棘轮门禁）。
- 单文件约 800 行上限（Go 与 CSS 一致）。
- `web/dist` 是生成物，不入库；**先 `pnpm build` 再 `go build`**（否则嵌入旧前端或编译失败）。

## 3. Android 客户端

单 Gradle 模块 `:app`，namespace `dev.tavernagent.mobile`，JDK 17 / compileSdk 36。

| 包 | 职责 |
|---|---|
| `data/local/` | 本地持久化 |
| `data/remote/`（`dto/`、`api/`） | 网络 + DTO |
| `data/repo/` | 仓库层（SSE 注册表、导演、配置…） |
| `domain/` | 纯 Kotlin 领域模型/映射（可单测） |
| `playback/`、`service/` | 系统 TTS、前台生成与通知 |
| `ui/`（`theme`、`components`、`nav`、`screens/*`） | Compose UI |
| `media/`、`util/` | 辅助 |

**规则（目标）**：依赖方向 `ui → repo → remote/domain`；`domain` 不得依赖 `data.remote` 的 UI 或 Android 框架；DTO 只在 `data/remote/dto`。

## 4. 差距（必须收敛）

- **Android 无依赖方向检查**：当前只有 CI 的 `:app:testDebugUnitTest :app:assembleDebug`，没有 detekt/ktlint/模块边界检查。目标：至少加一个机器检查（ktlint + 一个禁止 `domain` 反向依赖 `ui`/`service` 的规则）。
- **Web 的 `src/app/`、`src/stores/`、`src/lib/` 之间的依赖方向**未在本文明确；需从 `check.mjs` 与 ESLint 配置中固化并写入本节（补齐后本规范才算完整）。
- **Go 分层 deny-list 之外的包（`application`、`adapters`、`util`、`app`）不受限**，反向依赖由被禁侧捕获；如需更强约束，可改为白名单。
