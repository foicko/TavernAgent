# 02 主要功能与三端实现方法

本章说明系统**最主要的功能**，以及三端各自的实现方法。实现必须落在 [`12-layering.md`](12-layering.md) 规定的层次内。

## 功能总览

| 功能 | Go 后端 | Web 前端 | Android |
|---|---|---|---|
| 会话与分支 | `internal/application/session_setup.go`、`branch.go` | `stores/sessionStore`、`app/api.ts` | `data/repo/*`、`ui/screens/*` |
| 回合生成（REST + SSE） | `application/turn_*.go`、`adapters/http/sse.go` | `app/stream.ts`、`stores/slices/turnSlice.ts` | `data/remote/SseClient.kt`、`TurnStreamRegistry.kt` |
| 导演（Director） | `application/director.go`、`adapters/sqlite/director.go` | `stores/director*`、`screens/director` | `data/repo/DirectorRepository.kt`、`ui/screens/director` |
| 记忆（Memory） | `application/memory*.go`、`commitplan.go` | `stores/memory*` | `data/repo/*`、DTO |
| 角色卡（Card） | `application/card*.go`、`cardmarkers`、`cardassets` | `lib/cardMarkers.tsx`、`components/*Card*` | `domain/CardMarker*.kt`（子集） |
| 模型配置 | `application/providers.go`、`adapters/config/file.go` | `stores/settingsStore.ts`、`ModelSettingsPage.tsx` | `data/repo/*Config*` |
| TTS / 图像 / 嵌入 | `adapters/tts`、`adapters/image`、`adapters/embedding` | `app/api.ts`、对应面板 | 部分（播放走系统 TTS） |
| 事件流（SSE） | `adapters/http/sse.go` | `app/stream.ts` | `data/remote/SseClient.kt` |

## 1. 会话、分支与节点（核心数据模型）

- **会话 Session**：一次角色扮演的容器。创建时生成 `root_<sessionId>` 根节点与 `branch_main_<sessionId>` 主分支（`application/session_setup.go`）。带幂等键创建时 `sessionId = "session_" + sha256(key)`，重复创建返回同一会话。
- **分支 Branch**：可从任意节点 fork。`BranchView` 只暴露 `branchId/name/headNodeId/version/activeTurnId`，**不含 `sessionId`**（`application/view.go`）。
- **节点 PlotNode**：不可变的剧情节点，`kind ∈ {root,turn,configuration_change,memory_change,director_event}`，`parentId` 构成 DAG。
- **读取视图**：`GET /api/v1/sessions/{id}` 返回 `SessionView`（`application/view.go`），可带 `viewNodeId` 查看历史时点。

**三端实现方法**：三端都以 `SessionView` 为唯一读取模型；Web/Android 不在本地重建状态机，而是调用后端接口 + 应用 SSE 增量（见 [`05-sse.md`](05-sse.md)）。

## 2. 回合生成（Turn）

回合是核心写路径，涉及并发、幂等、SSE 与原子提交。

- **受理**：`POST /api/v1/sessions/{id}/branches/{branchId}/turns`，**必须带 `idempotencyKey`**（缺省返回 `422 MISSING_IDEMPOTENCY_KEY`）。受理时校验 `expectedHeadId`/`expectedVersion`（乐观并发），返回 `{turnId,status,statusUrl,eventsUrl}`。
- **状态机**：`accepted → queued → preparing → generating → validating → committed`（异常分支 `awaiting_approval`/`awaiting_continuation`/`cancelled`/`failed`/`conflicted`）。定义见 `domain.TurnStatus`。
- **流式**：生成过程中通过 SSE 推送 `turn.started / turn.thinking / block.delta / block.appended`，终态推 `turn.committed`（见 [`05-sse.md`](05-sse.md)）。
- **续写**：`POST /api/v1/turns/{turnId}/continue`；**取消**：`POST /api/v1/turns/{turnId}/cancel`。
- **再生/编辑**：`.../regenerations`、`.../edits`（派生新分支/回合，幂等键规则见 [`09-sync-idempotency.md`](09-sync-idempotency.md)）。
- **提交原子性**：回合结果、状态、outbox 事件在同一 SQLite 事务内落库（`adapters/sqlite/store_turn.go`）。

**三端实现方法**：统一「**POST 受理 → 打开 `eventsUrl` 订阅 → 应用增量 → 终态刷新视图**」的流程。Web 在 `stores/slices/turnSlice.ts`，Android 在 `TurnStreamRegistry.kt`；两端都必须处理断线重连与快照恢复。

## 3. 导演（Director）

导演是一个可选的「叙事编排」子系统：维护 beat 计划、草稿、请求（讨论/命令）。

- 读取：`GET /api/v1/sessions/{id}/branches/{branchId}/director` → `DirectorView`（含 `DirectorSummary`）。
- 草稿：`PUT .../director/draft`；讨论：`POST .../director/messages`；命令：`POST .../director/commands`。
- 请求生命周期：`director.started → director.completed/failed/cancelled/interrupted`，通过 `GET /api/v1/director-requests/{requestId}/events` 订阅。
- 导演命令的提交会写 `session.updated` 事件并原子更新分支头。

**三端实现方法**：Web 支持完整编辑（草稿/命令）；Android 只读查看 + 状态轮询（`DirectorViewModel.pollRequest`）。

## 4. 记忆（Memory）

- 记忆投影从节点派生；`GET /api/v1/sessions/{id}/memories` 返回分页 `MemoryPage`。
- 人工干预：`PATCH .../memories/{memoryId}`（`MemoryPatch`）、`POST .../memories/organize`。
- `MemoryView` 在 `MemoryRecord` 之上附加 `effective`、`protected`（Go 内嵌 `*domain.MemoryRecord` 并扁平化到 wire）。

**三端实现方法**：Web 提供 patch/organize 编辑；Android 只读列表。

## 5. 角色卡（Card）与卡片 UI 标记

- 导入：`POST /api/v1/cards`（兼容别名 `/api/v1/cards/import`），支持 PNG/JSON/charx；charx 与 PNG 内嵌资源落地到 `data/cards/<cardID>/`。
- 资源：`GET /api/v1/cards/{cardId}/assets/{name}`（**hostOnly，无 token**，因为浏览器 `<img>` 无法带 token，见 [`08-auth-security.md`](08-auth-security.md)）。
- 卡片 UI 标记语法（`<img cmd>` / `<img="…">` / `<img_obj>` / `<imgBG>` / `<img src>`、`[HUD]`/`[SCENE]`/`[VAR]`）是**跨语言共享契约**：Go 在 `internal/cardmarkers`，Web 在 `web/src/lib/cardMarkers.tsx`，Kotlin 由生成脚本产出；唯一源是 `testdata/contract/markers.json`。

**三端实现方法**：Go 与 Web 各自解析同一语法并渲染；Kotlin 只做契约测试（Android 不渲染卡片标记）。**改语法必须改 `markers.json`**，生成物由 `--check` 守卫。

## 6. 模型配置（Provider / Model）

见 [`10-provider-config.md`](10-provider-config.md)。核心：**连接信息挂在 model 实例上，slot 只引用 modelId**。

## 7. TTS / 图像 / 嵌入

- TTS：`GET /api/v1/tts/voices`（hostOnly）、`POST /api/v1/tts/synthesize`、`POST /api/v1/tts/plan`。
- 图像：`POST /api/v1/images/plan`、`POST /api/v1/images/generate`、`GET /api/v1/images/{filename}`（hostOnly）。
- 嵌入：`POST /api/v1/embeddings/probe`、`POST /api/v1/embeddings`。

**三端实现方法**：Web 全量；Android 主要用系统 TTS 播放，图像/嵌入按需。

## 8. 事件流（SSE）

三个订阅端点：`/api/v1/turns/{turnId}/events`、`/api/v1/sessions/{id}/events`、`/api/v1/director-requests/{requestId}/events`。契约见 [`05-sse.md`](05-sse.md)。

**三端实现方法**：Go 是唯一的 SSE 产出方；Web 用 fetch + ReadableStream 手写解析（`stream.ts`），Android 用 OkHttp + 自写 SSE 解析（`SseClient.kt`）。两者都必须实现 `Last-Event-ID` 重连、终态关闭、心跳容错。

## 功能变更的判定

新增/修改一个功能时，按以下顺序自问：

1. 它改了对外契约吗？→ 是则走 [`14-docs-sync.md`](14-docs-sync.md)，更新契约源 + 生成物 + 三端 DTO。
2. 它引入新的实体 ID 吗？→ 按 [`07-conventions.md`](07-conventions.md) 给前缀。
3. 它有写操作吗？→ 必须支持幂等键（[`09-sync-idempotency.md`](09-sync-idempotency.md)）。
4. 它有长任务吗？→ 用 SSE 事件表达进度，终态事件收口（[`05-sse.md`](05-sse.md)）。
5. 它属于哪个功能子集？Android 是否要实现？→ 见 [`01-principles.md`](01-principles.md) §4。
