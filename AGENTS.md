# AGENTS.md

TavernAgent（SillyDog）：本地优先的 AI 角色扮演 / 交互叙事引擎。Go 1.26 单二进制服务 + SQLite + React 19 SPA，前端经 `//go:embed all:dist`（`web/embed.go`）在编译期嵌入；另有原生 Android 客户端（`android/`，Kotlin/Compose，独立构建）。

`docs/PROJECT_STATUS.md` 是**唯一权威文档**（架构、已交付能力、工程门禁、技术债）。做较大改动前先读它，与本文冲突时以它为准。

## 构建顺序（最容易踩的坑）
`web/dist` 是生成物、不入库，却被 `//go:embed` 嵌入：**在 `web/` 跑 `pnpm build` 之前，不要执行任何 `go build` / `go run` / `go test ./...`**——否则嵌入旧前端，或在新克隆上直接编译失败（没有 `dist`）。

- 完整构建：`powershell -ExecutionPolicy Bypass -File scripts/build.ps1 -Version dev` → `build/tavernagent.exe` + `BUILD-INFO.json`
- 热更新：`powershell -ExecutionPolicy Bypass -File scripts/dev.ps1`（后端 8890 + Vite 5173；代理目标用 `VITE_API_TARGET` 覆盖）
- 离线运行：`go run ./cmd/tavernagent -provider mock`
- 发布打包：`python scripts/release.py --target <os/arch> --smoke`

## 常用命令
前端（在 `web/`；Node 24 / pnpm 11.9.0）：
```
pnpm install --frozen-lockfile
pnpm typecheck   # tsc --noEmit
pnpm lint        # node scripts/check.mjs && eslint src --max-warnings 0
pnpm test        # vitest run src
pnpm build       # tsc --noEmit && vite build
pnpm exec vitest run src/lib/__tests__/foo.test.ts   # 单个测试
```
浏览器 e2e（必须先构建嵌入前端的二进制）：
```
go build -o build/browser/tavernagent ./cmd/tavernagent
cd web; pnpm exec playwright install chromium; pnpm test:browser   # 自动起 scripts/browser_server.py（18891）
```

Android（原生客户端，`android/`；JDK 17+ 与 Android SDK platform 36，不参与单二进制嵌入）：
```
cd android
./gradlew :app:assembleDebug          # 产物 app/build/outputs/apk/debug/app-debug.apk
./gradlew :app:testDebugUnitTest      # JVM 单元/契约测试
```

Go（仓库根；Go 1.26.6）：
```
go test -count=1 ./...
go vet ./...
go test ./internal/application -run TestName   # 单个测试
golangci-lint run                              # 版本锁定见 .golangci-version（v2.12.2）
```
CI（`.github/workflows/quality.yml`）额外要求：`gofmt -l internal cmd` 为空、Linux 上 `go test -race`、`govulncheck@v1.1.4 ./...`、`python -m unittest discover -s scripts/tests -v`、`python scripts/release_audit.py --strict`。

## 硬约束（不了解就会写错）
- **仓库只放矢量**：`release_audit.py --strict` 会把任何入库的 `.png/.jpg/.webp/.gif`、以及混进源码清单的 `data/`、`output/`、`build/`、`chars/` 判为违规。架构图入库为 SVG（`docs/artifacts/architecture/`）；本地 PNG 生成物已被 gitignore。
- **前端分层**由 `pnpm lint` 强制：`src/components/` 只能 import `app|stores|lib|ui|react`；`src/ui/` 是叶子层（仅 `react` + `../lib`）。
- **样式必须走令牌**：组件样式只能引用 `src/styles/tokens.css` 的变量；禁止新增 `!important`、颜色字面值、跨文件重复选择器、死类名、`backdrop-filter`/`--glass-*`（六项 CSS 棘轮门禁）。纯结构改动需 `pnpm build:parity && pnpm css:parity` 证明声明集合未变；有意改视觉后 `pnpm css:snapshot`；收紧基线用 `node scripts/check.mjs --update-css-baseline`（放宽项会被打印，要求解释）。
- **单文件约 800 行上限**（Go 与 CSS 一致）：有若干超限文件被基线豁免，往里加行会让门禁失败——应拆分而非继续加。
- **Go 分层禁止反向依赖**：`domain` ← `ports` ← `context`/`application`/`pack`/`search` ← `adapters`（例如 `domain` 不得 import `application`）。
- **运行时数据在 `data/`（gitignore）**：SQLite `storage.db`、模型密钥 `data/config/secrets.json`、charx 资源 `data/cards/<cardID>/`。测试一律用临时目录，绝不动日常故事库；不要把模型凭据写进仓库。

## 关键入口与同步点
- `cmd/tavernagent`：服务端（flag：`-addr -data -provider -pin -tls -allow-origin -ablate`）。`cmd/tavernctl`：离线 CLI（`audit` 数据库自检 / `prompt` 提示词 / `mcp` stdio 服务 / `pack` 归档）。
- `internal/adapters/http`：REST + SSE 契约；`internal/adapters/providers`：chat / responses / anthropic / mock；`internal/adapters/sqlite`：存储与迁移。
- 卡片 UI 标记（`<img cmd>`/`<img="…">`/`<img_obj>`/`<imgBG>`/`<img src>`、`[HUD]`/`[SCENE]`/`[VAR]`）在 **`internal/cardmarkers`（Go）与 `web/src/lib/cardMarkers.tsx` 各有一份语法，改一处必须同步另一处**；详见 `docs/CARD_UI_MARKERS.md`。
- charx / PNG 内嵌资源在导入时落地到 `data/cards/<cardID>/`，经 `/api/v1/cards/{id}/assets/{ref}` 提供（该路由不加鉴权，因为浏览器 `<img>` 无法带 token）。
- `showcase/`（独立 Vite 展示站，`scripts/dev-showcase.ps1`，5174）与 `reference-components/` 是设计参考，不属于主二进制。
- `android/`：原生 Android 客户端（Kotlin/Compose + OkHttp/SSE），与 Web 端共用同一 REST + SSE 契约（`android/docs/API_CONTRACT.md`）；改动接口时需同步该契约文档与客户端 DTO。
