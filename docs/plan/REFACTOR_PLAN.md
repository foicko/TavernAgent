# 重构计划 B1–B5（降低维护负担）

> 目标：**降低"安全改动一个功能需要触碰多少文件/多少处概念"**，LOC 只是副产品。
> 一次成体系推进，但**分阶段提交、每阶段单独跑门禁**；禁止大爆炸式重写。
> 关联：`docs/PROJECT_STATUS.md`、`AGENTS.md`（测试策略/分层约束）、MODEL-EVAL 中的长期质量闭环。

## 0. 原则与总验收

- 每个阶段独立成 commit，提交信息 `refactor(scope): 主题`；每阶段结束必须全绿。
- 总验收（每阶段都跑）：`go vet ./...`、`go test -count=1 ./...`、`golangci-lint run`（0 issues）、
  前端 `pnpm typecheck && pnpm lint && pnpm test && pnpm build`、`python -m unittest discover -s scripts/tests`、
  `python scripts/release_audit.py --strict`。
- 阶段内若触及对外契约，必须同步 Android `android/docs/API_CONTRACT.md`。
- **不做**：为降 LOC 删校验/安全/降级分支；合并分层；删除测试安全网；一次移动超过一个子域。

## 1. 分支与提交策略

- 分支：`refactor/maintenance-surface`（或直接 main + 顺序提交，视你的发布节奏）。
- 提交序列：`B2 → B5 → B1(收尾) → B3 → B4`（先低风险、后高爆炸半径；B5 为 B3/B4 清障）。
- 每阶段提交前保留可回滚点：阶段粒度越细越好，B4 内部再拆 4 个子提交。

## 2. B1 砍不发货 surface（✅ 已完成）

**已完成**：`showcase/` → `archive/showcase/`；`FIX_PLAN.md`/`OPTIMIZATION_PLAN.md` → `docs/plan/`；
路径引用（`.gitignore`、`dev-showcase.ps1`、`AGENTS.md`）已同步。

**收尾项**：
- [ ] 复核 `archive/` 是否长期保留；若确定不再维护，直接删除（历史可找回）并去掉 `dev-showcase.ps1`。
- [ ] 盘点 `scripts/` 33 个跟踪脚本，标注"发布/评测必需"与"一次性转换"；仅保留必需项（当前看全部有用途，预计无需删）。
- [ ] 复核 `docs/` 内是否还有过程性内容可并入 `docs/plan/`。

**验收**：顶层目录仅 `internal web android scripts docs archive cmd LICENSES .github testdata`；无脚本引用失效路径。

## 3. B2 死码与未接线能力清算（✅ 已完成）

权威信号：`golangci-lint unused`（0）已清；`deadcode` 候选 41 个需逐条裁决。

### B2.1 向量/RRF 子系统（最大的未接线块）
- 涉及：`internal/search/hybrid.go`（179 行）、`internal/ports/vector.go`、`internal/adapters/embedding/openai.go`、
  以及 `internal/adapters/http/embedding_handler.go` 的接线。
- 裁决二选一（**建议：删除**，因为 `PROJECT_STATUS` 的 P3-B 只承诺"接口能力"，未承诺接线）：
  - 删除：移除 hybrid/vector/embedding 及相关 handler 与测试，更新 `PROJECT_STATUS` §3/§7 措辞；
  - 接线：把向量检索并入 `context/memory_*` 召回，作为独立 feature 提交（超出本次重构范围）。
- 验收：`deadcode ./...` 中 search/vector 相关 0 条；记忆召回相关测试仍绿。

### B2.2 `internal/util/autostart`（入口已移除）
- `-autostart` 便捷入口在 `PROJECT_STATUS` §6 记为"回退未补"。整包（4 符号 + 测试）删除，
  或恢复 CLI 入口（建议删除，保持"无壳层便捷入口"）。
- 验收：无 import；`PROJECT_STATUS` 相应条目更新。

### B2.3 未使用/测试专用包装收敛
逐条确认生产不可达且非接口/反射目标后处理：
| 符号 | 位置 | 处置 |
| --- | --- | --- |
| `NewArchiveService` | application/archive.go | 生产用 `NewArchiveServiceWithAssets`；把测试改用后者后删除 |
| `Server.ListenAndServe` / `ListenAndServeContext` | http/server.go,lifecycle.go | 生产用 `ListenAndServeContextOnReady`；删除未用包装 |
| `Server.RunStdioServer` | mcp/server.go | tavernctl 用 `mcp.New`；删除 |
| `pack.Write` | pack/pack.go | 导出走 ArchiveService；删除或改为内部 |
| `StreamParser.Draft` | protocol/parser.go | 未用；删除 |
| `SettlePromise` / `OptionsModes` | domain | 未用；删除 |
| `ExtractBlocks` | cardmarkers | 未用；删除 |
| `Service.SetMock` | image/tts | 测试专用；若仅测试用，移到 _test 或删除 |
| `Fault` / `BuildLine` | providers/mock | 测试专用；移到 _test |
| `HashState`/`MarshalState`/`UnmarshalState` | sqlite/store.go | 若仅测试用，评估保留为受支持 API 或移入内部 |

- 验收：`deadcode ./...` 输出降为 0（或全部写入 allowlist 并注明"仅测试用/未来接线"）。

**已删除（零引用）**：`domain.SettlePromise`、`domain.OptionsModes`、`cardmarkers.ExtractBlocks`、
`image.SetMock`、`tts.SetMock`、`mock.Fault`、`mock.BuildLine`、`sqlite.UnmarshalState`、
`http.Server.ListenAndServe`、`http.Server.ListenAndServeContext`、`mcp.Server.RunStdioServer`、
`protocol.StreamParser.Draft`。

**保留（allowlist，理由见括号，待复核）**：
| 符号 | 理由 |
| --- | --- |
| `sqlite.HashState` / `MarshalState` | 测试夹具（`helpers_test`/`store_turn_m23`）使用 |
| `pack.Write` | 测试（`archive_state_integrity`）使用 |
| `application.NewArchiveService` | 测试用便捷构造器（生产用 `NewArchiveServiceWithAssets`） |
| `application.MemoryService.{List,Overlay,Usage,branchChain}`、`TurnService.{SetCompactor,SetRuleset,SetRandom,Metrics}`、`CompactorService.RunOnce`、`ProviderManager.Configs` | 疑似保留的公共 API；可能经接口/未来入口使用，需逐条复核后再删 |
| `context.PromptTemplateNames`、`tts.Service.Synthesize`、`embedding.OpenAIProvider.Endpoint` | 同上，待复核（`Synthesize` 可能经 TTS 端点间接使用） |

## 4. B3 跨语言契约单源化（收益最大）

### B3.1 卡片标记语法（Go / TS / Kotlin 三份）
- 现状：`internal/cardmarkers/cardmarkers.go`、`web/src/lib/cardMarkers.tsx`、`android/.../CardMarkerParser.kt`。
- 方案（分两步）：
  1. **Golden 契约用例**（低风险、先做）：新增 `testdata/contract/markers.json`，含
     `<img cmd>`/`<img=>`/`<img_obj>`/`<imgBG>`/`<img src>`（双引号）/`[IMG:]`/`[HUD]`/`[SCENE]`/`[VAR]`
     与"未加引号不解析"等边界；三端各加一个测试读取同一文件断言一致。
  2. **单一实现源**（后续可选）：由 Go 生成 TS/Kotlin 正则/常量，或把语法编译成一份共享数据。
- 验收：三端 golden 测试同时绿；`AGENTS.md` 的"两处必须同步"改为"由 golden 用例保证一致"。

### B3.2 API DTO
- 现状：Go 请求/响应结构 ↔ `web/src/app/types.ts`(483) ↔ Kotlin DTO。
- 方案：以 Go 的 HTTP 契约为准，产出 `docs/API_CONTRACT.md`（或已有 android 版）为规范，
  再生成/校验 TS 与 Kotlin DTO；先做"契约夹具 + 三端解码测试"，再评估 codegen。
- 验收：任一 DTO 字段改动会同时触发三端测试失败；契约文档与实现有 CI 校验。

### B3.3 SSE 事件
- 现状：Go 发射事件 ↔ `web/src/app/stream.ts` ↔ Kotlin `SseLineParser`。
- 方案：把事件名/负载集中为一份枚举/清单（Go 常量 + 生成或 golden），三端解析测试覆盖同一批样本。
- 验收：新增/改名事件必然导致三端测试更新。

## 5. B4 拆 `internal/application`（54 文件，最大爆炸半径）

**目标**：从"1 个 54 文件大包"降为"1 个门面包 + 若干子包"，**保持 `application` 对外 API 不变**（facade + 类型别名），
避免全仓 import 改动。

建议分包（先做前三档，`media/session` 视收益再定）：
| 子包 | 从现有文件吸收 |
| --- | --- |
| `application/turn` | turnlifecycle, turn_accept, turn_run, turn_commit, turn_continue, live, commitplan, idempotency, headstate, coalescing, check, ruleeffects |
| `application/memory` | memory, memory_page, memoryquota, cognitive, secrets, calibration |
| `application/director` | director, director_progress, director_archive |
| `application/archive` | archive, archive_validation, cardarchive |
| `application/card` | card, cardlibrary, cardui, charx, tacard, pngcard, avatar, avatar_resolve, capabilities, lorebook, macros, inline |
| `application`（门面） | service, errors, bus, identity, view, metrics, providers, provider_usage, catalog, session_*, branch |
| 待定 | ttsplan, imageplan（`media`） |

执行步骤（每步一个提交）：
1. 建门面：把将被移动的类型在 `application` 中以 `type X = sub.X` 别名再导出；先不动实现。
2. 逐子包迁移（顺序：director → memory → archive → card → turn），每次只动一个子包并跑全门禁。
3. 迁完后收紧门面：只导出真正需要的 API；其余降为子包内部。
4. 更新 `AGENTS.md` 分层图与 `PROJECT_STATUS`。

**验收**：`application` 文件数显著下降；对外 import 路径不变（除新子包）；全门禁绿；无循环依赖。

## 6. B5 分层修正 + 门禁（✅ 已完成）

- 现状：`internal/domain/organizer.go` 与 `internal/ports/vector.go` import `internal/search`（反向依赖）。
- 方案：
  1. 抽叶子包 `internal/text`（或 `internal/util/tokenize`）：迁 `search/tokenize.go` 的 `Tokens` 与
     纯分词逻辑；`search`、`domain`、`ports`、`context`、`sqlite` 统一 import 叶子包。
  2. 若 B2 删除 `ports/vector.go`，`ports → search` 自然消失。
  3. 新增 **import 方向门禁**（golangci `depguard` 或 `scripts/layering_check.py`）：
     禁止 `domain → {application,adapters,context,pack,search}`、`ports → {adapters,application,context}` 等，
     接入 CI，防止再次长歪。
- 验收：`domain`、`ports` 不再 import 任何上层包；新门禁在 CI 生效且本地可跑。

## 7. 一次性执行顺序（依赖）

```
B1(收尾) ─┐
B2(死码) ──┼─> B5(分层+门禁) ─> B3(契约单源化) ─> B4(拆 application)
           ┘
```
- B2/B5 先行：清掉未接线与反向依赖，减少 B3/B4 的噪声。
- B3 与 B4 相互独立，但都在 B5 之后（新分层门禁可即时拦住 B4 的越界）。

## 8. 每阶段命令

```powershell
# Go
gofmt -l internal cmd; go vet ./...; go test -count=1 ./...; golangci-lint run
go run golang.org/x/tools/cmd/deadcode@latest ./...   # B2 收尾核对
# 前端
Set-Location web; pnpm typecheck; pnpm lint; pnpm test; pnpm build; Set-Location ..
# 脚本/合规
python -m unittest discover -s scripts/tests; python scripts/release_audit.py --strict
# 契约（B3 新增）
python scripts/contract_check.py    # 或对应 golden 测试入口
# 分层（B5 新增）
python scripts/layering_check.py
```

## 9. 风险与回滚

| 风险 | 缓解 |
| --- | --- |
| B4 大范围移动导致编译地狱 | facade + 类型别名，逐子包迁移；每步独立提交 |
| B3 codegen 引入构建复杂度 | 先 golden 夹具 + 三端测试，codegen 作为可选后续 |
| 误删"未接线但计划接线"的能力 | B2 逐条裁决并写明理由；向量/RRF 若确定要做则转 feature 而非删除 |
| 门禁红导致节奏受阻 | 每阶段先本地全门禁；CI 保持一致 |

## 10. 工作量预估（粗）

| 阶段 | 规模 | 风险 |
| --- | --- | --- |
| B1 收尾 | 小 | 极低 |
| B2 死码清算 | 小–中（删 ~600–1000 行） | 低 |
| B5 分层 + 门禁 | 中 | 低 |
| B3 契约单源化 | 大（三端 + 夹具） | 中 |
| B4 拆 application | 大（54 文件重组） | 中–高 |
