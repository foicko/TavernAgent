# WEB_CLIENT_NOTES — TavernAgent Web 客户端语义与视觉令牌（Android 移植对照）

> 来源：`D:\TavernAgent-src\web\src`（React 19 + Zustand + fetch/SSE）以及少量只读的后端契约文件
> （`internal/adapters/http/{server.go,security.go,sse.go}`、`web/src/app/__tests__/contract_fixtures.json`）。
> 本文只描述**现状语义**，不修改任何既有文件。移植时如与后端源码冲突，以后端源码为准。
>
> 阅读顺序建议：§1（连上）→ §2/§3（能对话）→ §4~§7（高级能力）→ §11（长什么样）。

---

## 0. 速查表

### 0.1 后端调用层源码地图

| 文件 | 职责 |
| --- | --- |
| `web/src/app/api.ts` | 唯一 REST + SSE 客户端。`BASE = ""`（同源），所有请求在此统一加 token/超时/401。 |
| `web/src/app/types.ts` | 全部契约类型（camelCase，与 Go 端 JSON 一致）。 |
| `web/src/stores/storyStore.ts` | Zustand 组合根（session/turn/branch/memory/director 五切片）。 |
| `web/src/stores/slices/turnSlice.ts` | 回合状态机、SSE 路由 `routeEvent`、在途回合恢复 `attachTurnStream`。 |
| `web/src/stores/slices/sessionSlice.ts` | 会话列表/打开/删除、开新局、剧情包导入导出。 |
| `web/src/stores/slices/branchSlice.ts` | 切分支/回溯/分叉/重生成/改正文/改输入/历史翻页。 |
| `web/src/stores/slices/memorySlice.ts` | 记忆列表分页、CoW 修订、整理。 |
| `web/src/stores/slices/directorSlice.ts` | 导演工作区读取/草稿保存/命令/讨论/取消（含 SSE+轮询对账）。 |
| `web/src/stores/storyHelpers.ts` | SWR 视图缓存、节点解析、`watchSession` 会话级 SSE、导航 epoch/归属守卫。 |
| `web/src/stores/settingsStore.ts` | 模型实例 + 三槽位 + 写作偏好（**不持久化**）。 |
| `web/src/stores/uiStore.ts` | 主题、双模视口、侧栏、弹窗、Toast。 |
| `web/src/lib/ttsPlayer.ts` | TTS 播放器（全局单例、LRU 音频缓存、Web Speech 降级）。 |

### 0.2 localStorage / 持久化键一览（Android 必须逐字对齐）

| 键 | 写入方 | 内容 |
| --- | --- | --- |
| `tavernagent_auth_token` | `app/api.ts` | 局域网配对 Token（Bearer）。 |
| `tavernagent.theme` | `stores/uiStore.ts` | `"light"` / `"dark"`；无值时跟随系统 `prefers-color-scheme`。 |
| `tavernagent.lastSessionId` | `stores/sessionMemory.ts` | 上次打开的故事 ID（启动恢复用）。 |
| `tavernagent_imported_character_cards` | `lib/characterCardStore.ts` | 导入卡库缓存（服务端为真相，本地仅缓存）。 |
| `tavernagent_session_avatar_map` | `lib/characterCardStore.ts` | sessionId → data URL 头像关联。 |
| `tavernagent_tts_settings` | `stores/ttsSettingsStore.ts`（zustand persist） | TTS 引擎/音色/端点/语速/偏好。 |
| `tavernagent_image_settings` | `stores/imageSettingsStore.ts`（zustand persist） | 生图引擎/尺寸/风格 + `turnIllustrations`（回合→图片结果）。 |
| `tavernagent_embedding_settings` | `stores/embeddingSettingsStore.ts`（zustand persist） | 向量检索配置。 |
| `tavernagent_director_draft_<sessionId>_<branchId>` | `components/DirectorWorkspace.tsx` | 导演草稿的浏览器侧防丢备份 `{plan,version,baseRevisionId}`。 |
| `tavernagent.onboarding.model.dismissed` | `components/FirstRunBanner.tsx` | `"1"` 表示已忽略「未配置模型」提示。 |

**没有持久化**（刷新即回默认）：`optionMode` / `optionsFrequency` / `autoContinue`（在 `settingsStore` 内存里）、`viewMode` 双模、侧栏折叠、`leftTab`、`activeCharKey`、分支/节点指针（`activeBranchId`/`viewNodeId`）、`lastTurnId`/`turnCursor`（SSE Last-Event-ID）。分支与节点指针不落地：刷新后总是回到服务端默认分支头。

### 0.3 同源与代理

- 生产：单二进制同源提供 SPA + API（`BASE=""`，即相对路径 `/api/v1/...`）。
- 开发：Vite 5173 代理 `/api` → `http://127.0.0.1:8890`（可用 `VITE_API_TARGET` 覆盖）。
- Web 端**没有**可配置的后端 Base URL 输入框；Android 端必须自己提供（首次配置里存 host:port）。

---

## 1. 连接、配对与持久化

### 1.1 Token 存取

- 键：`tavernagent_auth_token`。
- `getStoredAuthToken()`：任何异常（隐私模式）返回 `""`；`setStoredAuthToken(token)`：空串 = `removeItem`。
- 每个 HTTP 请求（含 SSE、上传、下载）都带 `Authorization: Bearer <token>`（token 非空时）。
- 后端 `internal/adapters/http/security.go` 也接受 `X-Auth-Token`，但 Web 客户端只用 `Authorization`。
- 本机回环访问免鉴权（按 TCP 对端 IP 判断）；启用 `-pin` 后，非回环的局域网访问必须带 Token，否则任何受保护路由返回 `401 {"code":"AUTH_REQUIRED"}`。

### 1.2 启动健康探测 / 授权状态

App 挂载时（`App.tsx`）调用一次：

```
GET /api/v1/auth/status
→ { authenticated: boolean, required: boolean }
```

- 回环对端或服务端未启用 PIN 时：`{authenticated:true, required:false}`（后端直接返回）。
- `required && !authenticated` → 打开配对弹窗。
- 请求失败（离线）被静默吞掉：应用进入"离线/未连接"状态，不阻塞渲染。Web 客户端**不调用** `/healthz`；Android 可用未鉴权的 `GET /healthz → {"ok":true}` 做连通性探测（本仓库后端存在该路由，Web 不用它）。

### 1.3 配对码流程（AuthPairingModal）

1. 弹窗出现（启动状态检查或任意请求触发 `onAuthRequired`）。文案要求输入服务端控制台打印的 **6 位数字配对码**；输入框 `inputMode=numeric pattern=[0-9]* maxLength=12`，提交前 `pin.trim()` 只校验非空。
2. `POST /api/v1/auth/pair`，body `{ "pin": "<code>" }`（JSON）。
3. 成功 `{ok:true, token}`：`setStoredAuthToken(token)` → `clearSessionCache()`（身份变了，旧缓存视图作废）→ `onSuccess`（App 重新 `loadSessions()`）。
4. 失败：
   - `401 {"code":"INVALID_PIN"}` → "配对码错误，请核对控制台输出的 6 位 PIN 码"。
   - `429 {"code":"TOO_MANY_ATTEMPTS"}`（带 `Retry-After`，后端每 IP 失败 10 次后锁定 5 分钟）→ "尝试次数过多，已被暂时限流，请稍后再试"。
   - 其它 → 使用响应 `message`。
5. 自动弹出时是**阻塞式门禁**（`dismissible=false`：无关闭按钮、Esc/遮罩不能关）；但 Web 的 `App` 始终传了 `onClose`，因此实际可取消并留在离线态。

### 1.4 401 的用户可见行为

两条通道：

- 普通请求：`http()` 里 401 且响应体 JSON `code === "AUTH_REQUIRED"` 时触发 `notifyAuthRequired()`；其它 401 只是抛 `ApiError`（带后端 message）。
- `binaryRequest()`（导出/导入/卡片上传/TTS 合成）：**任何** 401 都触发授权弹窗。
- SSE：连接 401 时触发授权弹窗，并作为**不可重试**的终态错误（4xx 分支）。

`onAuthRequired` 监听者（`App.tsx`）唯一动作：打开配对弹窗。**不会**自动清 token，也不会重试原请求；配对成功只刷新会话列表，用户需自己重做刚才的操作（例如重新发送）。

### 1.5 超时 / 离线 / 重连

- 所有非 SSE 请求：`AbortSignal.timeout(30_000)`。超时（及 Safari 旧版 AbortError）→ `ApiError(status=0, "请求超时，请检查网络或稍后重试")`。
- 错误消息解包：`ApiError.message` 优先取响应 JSON 的 `message` 字段，否则用原始文本。
- 启动时 `loadSessions()` 失败静默忽略（离线降级，不弹错）。
- 会话级 SSE 断线耗尽后在 `focus`/`visibilitychange` 时"复活"并补一次 `reloadView()`。
- 在途回合的同步失败会写入 `error` 并每 5s 重试（见 §2.4）。

### 1.6 http/https 处理

Web 客户端本身不做 scheme 处理（同源）。唯一和 scheme 相关的是"模型连接"表单校验：`baseUrl` 必须是带 host 的 `http(s)://`，否则报"请填写完整地址"。Android 端连接本服务建议：局域网 `http://<host>:8890`（`-tls` 时 `https://`）；`cleartext` 需在 Android 网络配置里放行。

---

## 2. SSE 事件流

### 2.1 端点

| 订阅 | 端点 | 用途 |
| --- | --- | --- |
| 回合流 | `GET /api/v1/turns/{turnId}/events` | 一次生成的全过程（增量、思考、终态）。 |
| 会话流 | `GET /api/v1/sessions/{sessionId}/events` | `session.updated`（例如另一标签页提交了新回合）。 |
| 导演流 | `GET /api/v1/director-requests/{requestId}/events` | 导演讨论请求的终态。 |

### 2.2 传输实现（`subscribeEvents`）

- **不是** `EventSource`，而是 `fetch` + `ReadableStream`，以便带 `Authorization` 与 `Last-Event-ID`。
- 请求头：`Accept: text/event-stream`；有 token 时 `Authorization: Bearer …`；续传时 `Last-Event-ID: <aggregateId>:<seq>`。
- 分帧：块之间以空行分隔（`\r?\n\r?\n`）；支持多行 `data:`（以 `\n` join）；`:` 开头是注释（心跳 `: ping`）；`event:` 前可有可无空白（解析用 `line.slice(6).trim()`）。
- `data` 尝试 `JSON.parse`，失败时按字符串下发（例如纯文本）。没有 `data:` 的块返回 `null`，不派发。
- 累计缓冲 > 1 MiB 抛 `SSE event too large` 并按断线处理。
- 事件 id 契约：持久事件为 `id: <aggregateId>:<seq>`（`seq` 单调）；**部分增量（`turn.thinking` / `block.delta`）没有 id**（后端 `sse.go` 中 `Sequence == 0` 的 best-effort 帧）。客户端只对带 `id:` 的行做去重与游标推进：id 前缀必须等于 `aggregateId + ":"`，`seq <= maxSeq` 丢弃；无 id 的增量继承上一事件 id 但**不做去重**（避免把不同 delta 当重复丢掉）。
- 服务端行为（`internal/adapters/http/sse.go`）：写前先按 `Last-Event-ID` 回放 outbox → 订阅实时；每 1s 对账 drain、每 15s 发 `: ping`；单次写超时 30s。

### 2.3 重连 / 心跳 / 终态

- 心跳看门狗 `SSE_STALL_MS = 45_000`：初始连接与每次 `reader.read()` 都有 45s 截止；超时即断开重连。服务端 15s 心跳留了 3 个周期余量。
- 退避 `SSE_BACKOFF_MS = [1000, 2000, 4000, 8000, 16000]`，最多 5 次；**一旦真的收到数据 `retries = 0`**（长时间连接不会因为历史断线次数被卡死）。
- 4xx（429 除外）视为不可重试：401 触发配对弹窗，其它直接 `onFinalError(ApiError)`。5xx / 网络错误 / 超时进入退避。
- 成功重连（`retries > 0`）回调 `onReconnect(attempt)`。
- 退避耗尽 → `onFinalError(new Error("SSE reconnect exhausted"))`。
- 终态事件立即 `return` 并关闭订阅：
  - 回合：`turn.committed` / `turn.cancelled` / `turn.failed` / `turn.conflicted`
  - 导演：`director.completed` / `director.failed` / `director.cancelled` / `director.interrupted`
- 取消/中止：`subscribe*` 返回同步 `stop()`；内部 `AbortController` 同时监听调用方传入的 `signal`。切换会话/分支、删除会话、开始新导航都会 detach。

### 2.4 回合事件派发（`routeEvent`）

前置守卫：若当前 `view.sessionId` 或 `lastTurnId` 已变化，直接 `onEnd()` 丢弃。

| 事件 | payload | 处理 |
| --- | --- | --- |
| `turn.started` | `{}` | `activeTurnSub.synchronize()`（拉一次 `GET /api/v1/turns/{id}`） |
| `turn.thinking` | `{thinking, attemptId}` | `thinkingText += thinking`；同时写入 `turnThinking[turnId]`。首个 thinking 记录时间戳，用于 settle 后显示"思考了 N 秒"。 |
| `block.delta` | `{seq, kind, speakerId, delta}` | 追加到 `draftBlocks[seq-1].text`；缺位用 `{kind:"narration", text:""}` 填充；`kind` 缺省 `narration`。 |
| `block.appended` | `{frameSeq, frame:{seq,kind,speakerId,text}}` | 用完整 frame **替换** `draftBlocks[seq-1]`（或 push）。 |
| `turn.truncated` | `{}` | `synchronize()` → phase 映射 `truncated` |
| `turn.committed` | `{}` | `synchronize(true)`：停止传输、`phase="committed"`、`reloadView()` 拉权威历史 |
| `turn.cancelled` | `{}` | detach；`phase="idle"`；清空 draft |
| `turn.failed` | `{code,message}` | detach；`phase="failed"`；`error=message`（缺省"生成失败"） |
| `turn.conflicted` | `{}` | detach；`phase="failed"`；`error="分支已变化，请刷新后再试"` |

### 2.5 轮询对账（SSE 的兜底）

`attachTurnStream(sessionId, turnId, alreadyCommitted=false, snapshotFirst=false)`：

- 打开 SSE（非 committed 时），并同时起一个状态轮询：首轮延迟 = `snapshotFirst ? 0 : 5000ms`，之后 `TURN_STATUS_INTERVAL_MS = 5000`。
- 轮询 `GET /api/v1/turns/{turnId}` → `{status, input, failureMessage, draft}`：
  - `committed` → 停流、`reloadView()`；
  - `awaiting_continuation` → `truncated`；若设置里 `autoContinue=true` 且相位刚变，自动调用 `continueTurn()`；
  - `awaiting_approval` → `awaiting_confirm`；
  - `failed/cancelled/conflicted` → 走 `routeEvent` 合成事件；
  - 其它 → `mergeInFlightDraft(turn.draft)` + `phase="generating"`。
- **在途草稿合并规则（`mergeInFlightDraft`）**：只允许把当前块变长（`existing.text.length >= inFlight.text.length` 则忽略）；`seq > 0` 时按 seq 定位，`seq = 0` 视为"最后一块之后的新块"；只补文本/kind/speakerId。断线期间行内增量不重放，靠这个快照补齐。
- `onReconnect` / `onFinalError` 都只是请求一次同步（不弹错误）。
- `window.focus` / `document.visibilitychange` 也会请求一次同步（回到前台立刻对齐）。
- 会话级订阅（`watchSession`）：只处理 `session.updated`，且 `data.branchId` 必须等于当前激活分支，否则忽略。若当前回合忙 → 同步在途回合；否则 100ms 去抖后 `reloadView()`。终态失败标记 `stalled`，等 `focus/visibilitychange` 重建订阅并 `reloadView()`。

---

## 3. 回合生命周期

### 3.1 相位机（`TurnPhase`）

```
idle ──send/choose/regenerate/edit/continue/retry──▶ generating
generating ──truncated/awaiting_continuation──▶ truncated ──continue──▶ generating
generating ──awaiting_approval──▶ awaiting_confirm
generating ──turn.failed/conflicted──▶ failed ──clearError──▶ idle
generating ──turn.committed──▶ committed ──reloadView 完成──▶ idle
```

- `isTurnBusy = generating | truncated | awaiting_confirm | committed`。这四个相位（加 `directorCommanding`、`branchBusy`）就是"输入台/写操作禁用"的判定（`storyBusy`）。
- `historyReadOnly = !!viewNodeId`：浏览历史快照时输入台只读，只能"返回最新进度"或"从此处开辟新分支"。

### 3.2 精确请求体

**发送（send）** `POST /api/v1/sessions/{sessionId}/branches/{branchId}/turns`

```json
{
  "idempotencyKey": "k_<Date.now()>_<sendSeq>",
  "expectedHeadId": "<view.branch.headNodeId>",
  "expectedVersion": "<view.branch.version>",
  "expectedCharacterId": "<view.characterId>",
  "input": {
    "kind": "text | action",
    "text": "<原文>",
    "actionRef": "<仅 actionRef 存在时>",
    "note": "<玩家注记，trim 后非空才带>",
    "options": "always | never"        // 仅当偏好 ≠ auto 时携带；auto 由服务端默认
  }
}
```

- 发送前置守卫：空文本 / busy / `directorCommanding` → 静默返回；`viewNodeId` → `error="历史快照为只读，请先返回最新进度或从此处新建分支"`；无 view 时先 `createSessionFromPreset(activeCharKey)`；角色归属不一致 → 可见错误（便于输入台恢复文字）。
- 输入台在调用前已清空；失败（`phase="failed"`）时输入台会把正文与注记一起还原。
- **幂等与重放**：`pendingSubmission` 记住 `{owner, view, key, input}`。若受理响应丢失（超时/5xx），`retry()` 会**原样重放同一个 key 与同一份 view/input**；收到 4xx 明确拒绝时清除 pending（服务端已把该次受理写成失败终态，重试同 key 只会拿回失败回合）。
- 成功后：`lastTurnId = result.turnId`，清空 thinking/draft，`attachTurnStream`。

**选项（chooseOption）**

- "直接推进"模式：同一 `/turns` 端点，`input = {kind:"option", text:<选项文本>, optionRef:{nodeId, optionId}}`。
  - `nodeId` = `view.nodes` 中最后一个 `kind==="turn"` 的节点，找不到则 `branch.headNodeId`。
- "填入编辑（fill-edit）"模式：不请求，只 `set({pendingInput: option.text})`，ComposerShelf 把它填进 textarea 并聚焦。

**取消（cancel）** `POST /api/v1/turns/{turnId}/cancel`，body `{expectedCharacterId}`。
- 返回 `{turnId,status}`：`committed` → 以已完成方式 attach；`cancelled` → detach + `idle` + `refreshView()`。

**继续（continue）** 仅 `phase==="truncated"` 可调用。`POST /api/v1/turns/{turnId}/continue`，body `{expectedCharacterId}`；随后重新 attach SSE。失败回落 `truncated` 并继续 attach。

**重试（retry）**
1. 若有 `pendingSubmission`（同属主）→ 原 key 重放；
2. 否则若 `lastTurnId` 存在：先 `POST cancel`（失败再 `GET turn`），若 `committed` → 已完成方式 attach；若是 `cancelled/failed/conflicted` → 继续；其它状态 → `error="上一回合尚未结束，请等待取消完成后重试"` 并 attach；
3. 新 key `retry_<Date.now()>_<seq>` 重新发送 `lastInput`。

**重生成（regenerate）** `POST /api/v1/sessions/{id}/branches/{branchId}/regenerations`

```json
{ "nodeId": "<目标回合节点>", "expectedCharacterId": "...",
  "idempotencyKey": "rg_<ts>_<seq>", "recheck": false,
  "note": "<引导，trim 后非空才带>",
  "options": "always|never" }
→ { branch, branchId, turnId, status, statusUrl, eventsUrl }
```

`recheck=true` 重新掷骰。成功后 `openDerivedBranch(sessionId, res.branchId, res.turnId)`：切到新分支、`phase="generating"`、attach 新回合流。

**改玩家输入（editPlayerInput）** `POST .../edits`

```json
{ "nodeId": "...", "expectedCharacterId": "...",
  "input": { "kind": "text", "text": "<trim>" }, "idempotencyKey": "ed_<ts>_<seq>" }
→ DeriveResult { branchId, turnId, ... }
```

同样 `openDerivedBranch(branchId, turnId)`（派生分支 + 生成）。

**改正文（editAssistantText）** 同一 `/edits`，body 带 `blocks: TextBlock[]`、key `ea_<ts>_<seq>`；返回 `AssistantEditResult {branchId, nodeId}`。服务端同步落为"纯叙事候选"（**零状态变化**），客户端 `openDerivedBranch(branchId)`（无新回合）。`branchBusy` 锁在 `finally` 无条件释放（切分支会让归属守卫失效）。

**分叉（fork）** `POST /api/v1/sessions/{id}/branches`，body `{fromNodeId, name?, expectedCharacterId}` → `{branch}`；只建分支不生成内容，随后 `openDerivedBranch(branch.branchId)`。

**开新局（createSession）** `POST /api/v1/sessions`

```json
{ "idempotencyKey": "<uuid>", "title": "...", "cardId": "...", "characterJson": "...",
  "playerName": "...", "playerRole": "...", "playerBackpack": [], "openingVariantId": "...", "openingText": "..." }
→ { sessionId, branchId, rootNodeId, branchVersion, openingText, state }
```

- 幂等键在 `pendingSessionSetup` 内按 `(epoch,payload)` 记忆：同一次提交失败重试复用同一 key（避免生成两个会话）。
- 返回后 `openSession(sessionId)` + `loadSessions()`；列表刷新失败只 Toast，不报全局错。

**删除（deleteSession）** `DELETE /api/v1/sessions/{id}` → `{ok}`；进行中回合时后端 409 `SESSION_BUSY`。删当前会话时：detach 流 → 清空视图与 `lastSessionId` → 清 SWR 缓存 → 刷新列表 → 自动打开列表第一条（打开失败要提示，不能假报成功）。

### 3.3 选项展示

- 只渲染**最后一条消息**的 `options`，且 `phase==="idle"`、`viewMode==="studio"`、`viewNodeId` 为空。
- 意图徽标（`OptionView.intent`）：`aggressive` 强势威慑/swords/`--color-danger-text`；`clever` 机智试探/lightbulb/`--color-info`；`emotional` 共情抚慰/handshake/`--color-success`；`chaotic` 出其不意/dice/`--accent-amber`。未知 intent 回退 `sparkles` + `--border-mid`。
- 卡片为 2 列网格（≤768px 单列），左侧 3px 意图色竖条，整卡可点/可 Enter/Space。
- `suppressedOptions>0`（服务端按"只在关键节点"收起）时，在回合依据条里显示"未摆出选项 N 条（本轮承接上一轮抉择）"。

### 3.4 玩家注记（note）

- ComposerShelf 的"注记"行：`input maxLength=2000`，Enter 不提交（Escape 清空关闭）。
- 发送时 `trim` 后作为 `input.note` **独立字段**发送（绝不混进正文 text）。
- 历史回看：`TurnContent.inputNote` → `StoryMessage.inputNote` → `TurnEvidence` 的常驻 chip「本轮注记」；带要求的重生成也把引导写进同一字段。

### 3.5 流式草稿与已提交结果的衔接

- 流式中：`draftBlocks`（`kind: narration|dialogue|inner_monologue`）逐块渲染 + 打字机光标；`dialogue` 显示为「…」、`inner_monologue` 显示为（…）（两端引号会被去除后再包）。
- `turn.committed` 到达后先进入 `committed` 相位（卡片标题变"（回复已完成，正在同步选项…）"），`reloadView()` 完成后：
  - `messages = messagesFromNodes(fresh, turnThinking, turnThinkingSeconds)`；
  - 把本回合 thinking 从 `turnId` 迁到 `nodeId`（`fresh.headNode.nodeId`）并记录"思考了 N 秒"；
  - 清空 `thinkingText`/draft、相位回 `idle`、`detachTurnStream`。
- 历史节点里 `thinking` 由 `turnThinking[nodeId] || turnThinking[turnId]` 取（`TurnContent.provenance.turnId` 关联）。
- 刷新/断线恢复：`view.branch.activeTurnId` 非空 → `recoverActiveTurn`：清 `turnCursor`、`phase="generating"`、`draftBlocks=[]`、立即 attach（`snapshotFirst=true`，立刻拉一次转状态补齐在途文本）。

### 3.6 刷新后的恢复链路

1. App 启动：`loadSessions()` → `chooseStartupSession(sessions, lastSessionId(), 角色匹配)` → `openSession(id)`。
2. `openSession`：
   - 命中内存 SWR 缓存（最多 12 条）→ 立即用缓存渲染，同时后台 `GET /api/v1/sessions/{id}` 校验；
   - 未命中 → `sessionLoading=true` + 骨架屏，等请求返回；
   - 成功后记录 `sessionViewCache`、`rememberSession(id)`、`recoverActiveTurn(view)`、`loadMemories()`。
   - 失败：若有在途回合（generating/truncated/awaiting_confirm）则不把相位打回 failed（等流收敛）；否则 `phase="failed"` + error。
3. `SessionView` 由 `GET /api/v1/sessions/{id}?branchId=&viewNodeId=&limit=&before=` 提供；`viewNodeId` 只用于**浏览**历史，不推进分支指针；写入永远落在 `branchId` 对应的 `branch.headNodeId`（`expectedHeadId/expectedVersion` 乐观并发）。
4. 更早历史：`hasMore` + `oldestTurnId` 向上翻页（`before=<oldestTurnId>`），合并时按 `id` 去重并丢弃重复的 opening 消息。

---

## 4. 分支树

### 4.1 数据来源

- 左栏"脉络"树：直接用 `SessionView.nodes`（当前窗口的祖先链，root 在前；`messagesFromNodes` 会按 `depth` 再按 `turnNumber` 排序，但树面板按服务端顺序渲染）。
- 每个节点：`tag`（root=命运起点；其余=回合 #turnNumber）、`desc`（root=`openingText`；其余优先 `玩家: 「inputText 截 36 字」`，否则首个 block 截 40 字，兜底"文学演义片段"）、`isActive`（=`branch.headNodeId`）、`isViewing`（=`viewNodeId`）、`canRegenerate`（`kind==="turn"`）。迷你量表展示的是**当前** `hud.relationships` 的对角色值（并非每个节点的历史值）。
- 图谱弹窗（StoryMapModal）：`GET /api/v1/sessions/{id}/graph?nodeId=<target>&up=6&down=2` → `{nodes:[{nodeId,parentId,kind,turnNumber,depth,childCount,isCandidate}], targetNodeId, truncated?}`（服务端最多 300 节点，只发目标周围一圈）。
- 节点详情：`GET /api/v1/nodes/{nodeId}` → `{node, state, candidates}`。
- 分支列表：`SessionView.branches`（含 `branchId/name/headNodeId/version/activeTurnId?`）。
- 候选版本：`SessionView.candidateGroups: Record<parentNodeId, PlotNode[]>`，`TurnActions` 用 `msg.parentId` 查组，`>1` 时显示"版本 i/n"与左右切换（内部实际调用 `viewAt` 切到兄弟节点）。

### 4.2 渲染与布局

- 左栏树：纵向卡片栈 + 左侧 2px 竖线（`--border-light`）；节点卡含标题行（tag + 状态药丸"活跃中/已封存分支/快照"）与摘要行。
- 图谱：按 `depth` 分列布局，`x = 24 + (depth-firstDepth)*196`，`y = 24 + 行内序号*88`；SVG 三次贝塞尔从左节点右缘连到右节点左缘；节点按钮显示标签（`故事起点`/`记忆整理 / 修订`(memory_change)/`导演安排 / 进度调整`(director_event)/`回合 N`）、`当前分支头 · N 个子节点 · 候选`；点击选中，右侧详情面板显示节点正文摘要。
- 画布自动滚动使 `target` 居中。

### 4.3 四个动作的语义

| 动作 | 调用 | 效果 |
| --- | --- | --- |
| 回溯定位（jump/viewAt） | 本地 `activeBranchId` 不变、`viewNodeId=nodeId` 后 `reloadView()` | 只读浏览历史；输入台与写操作锁定；失败回滚落点并提示"查看历史节点失败，已还原到原位置" |
| 返回最新进度（backToHead） | `viewNodeId=null` 后 `reloadView()` | 回到分支头 |
| 开辟分支（fork） | `POST /branches {fromNodeId}` | 只建分支（可命名，默认 UI 传"平行分支_XXXX"），**不生成内容**，随后切到新分支 |
| 重生变体（regenerate） | `POST .../regenerations {nodeId, recheck?}` | 从 `nodeId` 的父状态派生**候选分支**并生成新回复；"重新掷骰"= `recheck:true`；"带要求重演"= 带 `note` |

- 切分支（switchBranch）会重置导演工作区、记忆、draft，并 `recoverActiveTurn`（新分支可能已有在途回合）。
- 所有分支写操作都带 `expectedCharacterId`（跨角色串写防护）；写操作在 `storyBusy` 时禁用。

---

## 5. 面板数据映射（characters / relationships / items / promises / scene / milestones）

面板分两类：**左栏**（树/角色/世界书/会话）与**右栏**（立绘与 HUD + 三个弹窗）。以下为"哪个响应字段喂给哪个面板"。

### 5.1 角色（characters）

- 数据：`SessionView.state.characters`（`Record<characterId, CharacterInfo>`），排除 `characterId === "player"`；优先 `participant:true`，否则第一个。
- 展示解析（`characterPresentation(activeCharKey, view)`）：
  - 名称：`CharacterInfo.name` → 短名 `nickname` → `cleanShortName(name)`；
  - 身份：`CharacterInfo.description`（清洗后截短，展开 `{{char}}/{{user}}`）→ 卡库 `role` → 预设 `role`；
  - 铭牌标签：内置预设用 `stagePill`；导入卡用 `format`（如 `CHARX 角色卡`）或 `tags` 前 2 个，兜底"角色档案 · 动态叙事"；
  - 头像：`CharacterInfo.avatar` → 本地卡 `avatar` → 会话头像映射 → 预设 `avatar`；
  - 档案弹窗（DossierModal）字段：`description + personality`（人设）、`scenario`（世界与场景）、`mes_example`（对话范例）、`system_prompt + post_history_instructions`（系统规则）、`creator / character_version / creator_notes`（创作者说明）。注意这些字段用**线上 V2 命名**（蛇形），不是 camelCase。
- 无服务端会话时用 `CHARACTER_PRESETS.custom` 中性皮肤兜底（`SELECTABLE_PRESET_KEYS` 为空，不内置第三方卡）。

### 5.2 关系 / 心境（relationships / moods）

- 数据：`view.state.relationships[characterId] = {affection, trust, alertness}`；角色 ID 由 `primaryCharacterId(state, activeCharKey)` 解析：`npc_<key>` → `<key>` → 第一个 `participant` → 第一个非 player。
- 显示：
  - 好感 `affection`：显示带符号整数（`+12`），进度条宽度 `affectionPercent = clamp((v+100)/2, 0..100)%`；
  - 信任 `trust`：显示 `v%`，条宽 `clamp(0..100)%`；
  - 戒备 `alertness`：显示 `v%`，条宽 `clamp(0..100)%`。
- 心境：`view.state.moods[characterId] = {moodCode, text}`。`moodCode` 关键词映射立绘神情：alert/angry/guard/shock→`alert`；smile/happy/joy/relieved→`smile`；whisper/blush/soft/moved/sad→`whisper`；calm/neutral/quiet→`calm`（默认 `alert`）。
- 数值变化的浮动提示：`TurnContent.changes[]`（服务端从领域事件推导）渲染成 chips，`delta>0` 上色"涨"、`<0`"跌"；关系条的瞬时浮动由 `uiStore.spawnMeterDelta` 做 1.4s 动画。
- 会话级键值：`view.state.variables` 按 key 排序展示（`[VAR]` 标记写入）。

### 5.3 物品（items）

- 数据：`view.state.items`（`Record<instanceId, ItemInstance>`）。
- 过滤：`quantity > 0 && ownerId === "player"`。
- 展示：名称 `name`；状态 `keepsake ? "核心信物" : "数量: N"`；图标 `keepsake ? clock : package`。
- 点击物品弹出动作气泡（`InventoryActionBubble`），两个固定动作把文本注入输入台：
  - 端详："我将【name】握在手心仔细端详，思索其背后的渊源与线索..."
  - 出示："我将【name】轻轻置于桌面推向对方眼前，观察其神色变化。"

### 5.4 承诺 / 契约（promises）

- 数据：`view.state.promises`（`Record<promiseId, PromiseItem>`）。
- 展示：`content` 正文；来源 `sourceNodeId` 前 8 位；状态药丸：`proposed` 口头提议(warning)、`active` 誓约生效中(success)、`fulfilled` 已践行达成(neutral)、`broken` 已违背(danger)、`cancelled` 已解除(neutral)。
- 右栏磁贴副标题 = 生效中的条数（`state === "active"`）。
- 无服务端会话时回退到角色预设的 `pledge`。

### 5.5 场景（scene）

三处来源、优先级不同：
1. **常驻场景条（SceneBar）**：从 `messages` 里**倒序**找最近一条 `uiHints` 中 `kind==="scene"` 的 `text`（`[SCENE:...]` 标记，服务端已归一化）。实时流正文里的标记由 `cardMarkers` 直接渲染。
2. 剧情大纲（左栏"剧情进展"区块）：`view.outline.scene`。
3. TTS 导演上下文：`view.state.scene.title` → `view.outline.scene`。

### 5.6 里程碑 / 目标（milestones / goals）

- 数据：`view.outline = {milestones: [{milestoneId, description}], goals: [{goalId, characterId, text}], scene?}`（左侧"剧情进展"区块）。
- 展示：里程碑列表直接显示 `description`；目标显示 `view.state.characters[goal.characterId]?.name || goal.characterId` + `text`；都没有时显示"尚无已记录的里程碑或目标"。
- 另有"前情回顾与交接"区块（`view.activeSummary.text`，XML 文本 `story_checkpoint` 解析）：
  - `<narrative_arc>` 剧情推进；
  - `<character_dynamics><mindset character="...">` 心境条目 + `<hidden_tension>` 暗流；
  - `<open_loops>` 待解悬念与约定；
  - `<milestones>` 阶段里程碑。
  - 用 `DOMParser` 解析，只取 textContent，不渲染任何标记。

### 5.7 其他右栏/HUD 元素

- 立绘：`tachieExpression ∈ {alert, calm, smile, whisper}`；优先卡内 `type=emotion` 资源映射（`emotionMapFromAssets`），否则角色头像；CSS filter 随神情变化。
- 检定（checks）：`TurnEvidence` 展示 `natural/attributeModifier/total/dc/outcome`，outcome 映射 4 档（crit 带符号），公式拆解 d20 + 修正 = 总值 vs DC。
- 本轮参考记忆：`TurnContent.injectedMemories[{memoryId,text,kind}]`，kind 中文映射 见证/转述/推测/秘密。
- 上下文指标（ContextMetricsPopover）：本地估算（`lib/tokenEstimator`），显示人设/记忆/历史/草稿占比与百分比；不是服务端数据。
- 世界书（LorebookPanel）：`GET /api/v1/sessions/{id}/lorebook?q=&offset=&limit=40` → `{entries:[{bookIndex,entryIndex,bookName,entry:{title,keys,secondaryKeys,content,enabled}}], hasMore, nextOffset}`；另有 `view.secrets[{secretId,title,order,content?,revealed}]`（未揭示只给标题）。

---

## 6. 记忆库 UI

### 6.1 数据

`GET /api/v1/sessions/{id}/memories?branchId=&nodeId=&search=&kind=&cursor=&limit=40`
→ `{memories: MemoryView[], usage?: MemoryUsage, nodeId?, nextCursor?, total?, counts?}`

- `MemoryView`：`{memoryId, sourceNodeId, kind: observed|reported|inferred|secret, ownerIds?, content, entityIds?, confidence?, pinned, supersedes?, hidden, effective, protected?, importance?, mergedFrom?, evidence?:{sourceQuote?,reasoning?,confidence,autoDowngraded?}, subjectKey?, createdTurn?, validFromTurn?, validUntilTurn?}`。
- `MemoryUsage`：`{used, protected, limit, reclaimable, headroom, tier: Normal|Notice|Degraded|Critical}`；UI 显示"普通记忆 used/limit · 受保护 N · tier 文案（充足/留意容量/建议整理/容量紧张）"+`<progress>`。
- 客户端 kind → 分类映射：`observed/reported → episodic`（亲历/转述）、`inferred → inference`（推测）、`secret → secret`（秘密）。

### 6.2 筛选 / 搜索 / 分页

- 过滤按钮：`all`（生效）/`episodic`/`inference`/`secret`/`hidden`（隐藏），括号显示计数（优先 `counts[kind]`，否则本地统计）。
- 搜索框：**250ms 去抖**后调用 `loadMemories({search, kind})`；同时局部再过滤（content + entities 小写包含）。
- 分页：`limit=40`，游标数组保存每一页的 cursor，`nextCursor` 翻页；`moving` 与 `anchored`（同查询下保持当前页）语义见 `memorySlice.loadMemories`。
- 隐藏条目：`effective` 表示"在当前查看路径上生效"（CoW 覆盖后原记录为 false）；`hidden` 的条目只在"隐藏"筛选里出现，且不进上下文。

### 6.3 写操作（全部是分支级 CoW）

`PATCH /api/v1/sessions/{id}/branches/{branchId}/memories/{memoryId}`

```json
{ "content": "...",        // 修订（可选）
  "pinned": true,          // 置顶强化召回（可选）
  "hidden": false,         // 隐藏 / 恢复（可选）
  "expectedCharacterId": "...", "expectedHeadId": "...", "expectedVersion": 3,
  "idempotencyKey": "<uuid>" }
→ MemoryView
```

- 修订是 copy-on-write：后端新建覆盖记录，不改原记录、不影响其它分支；`supersedes` 非空 → UI 标"已修订"。
- 三按钮语义：**置顶**（`pinned` 取反）、**纠正**（内联 textarea 保存 `content`）、**隐藏/恢复**（`hidden` 取反）。
- 幂等键 `pendingMemoryPatch` 按 payload 记忆（重试同一次修订复用 key）。
- 只读守卫（全部命中则按钮禁用）：`viewNodeId`（历史视图）、`storyBusy`、`memoryLoading`、`memoryError`。
- 整理：`POST /api/v1/sessions/{id}/branches/{branchId}/memories/organize`，body `{expectedCharacterId}` → 成功后 `reloadView()`；失败用 Toast（不写 story 级 error，因为弹窗可能看不到）。
- 修订/整理成功后重置分页到第一页并 `reloadView()`。

---

## 7. 导演模式

### 7.1 入口与整体流程

1. **状态条**（ComposerShelf 顶部，仅当 `view.director` 存在时显示）：标题 + 状态（`active` 执行中 / `paused` 已暂停 / `completed` 已完成）、`currentBeatTitle`、`completed/total`、warning；按钮"暂停大纲/恢复大纲"直接发命令。
2. **工作区**（DirectorWorkspace，portal 全屏浮层；从状态条或"导演模式"按钮打开）：
   - 左列：讨论（`requests` 列表 + 输入框，maxLength 8000；生成中可"停止讨论"）。
   - 右列：大纲草稿编辑（标题、全局叙事要求、阶段列表；阶段可上移/下移/删除/新增，最多 32 个；每阶段有名称/剧情安排/完成条件）。已结束/当前阶段锁定编辑（`settled/current`）。
   - 底部："确认启用大纲"；活跃时提供"暂停/恢复、标记当前阶段完成、跳过当前阶段、回退到某阶段"。
3. 未打开任何故事时点"导演模式"：先用当前角色建会话（`createSessionFromPreset`），再打开工作区。

### 7.2 API

| 用途 | 调用 | 备注 |
| --- | --- | --- |
| 读取工作区 | `GET /api/v1/sessions/{id}/branches/{branchId}/director?viewNodeId=` | 返回 `{state: DirectorState\|null, draft: DirectorDraft\|null, requests: DirectorRequest[], readOnly, viewNodeId}`；`readOnly` 在历史视图为 true |
| 保存草稿 | `PUT .../director/draft` | body `{expectedCharacterId, expectedDraftVersion, baseRevisionId, plan}`；500ms 去抖自动保存 + 串行化（inFlight 排队）；`version` 是并发令牌 |
| 讨论 | `POST .../director/messages` | body `{expectedCharacterId, expectedHeadId, expectedDraftVersion, idempotencyKey, text}` → `{requestId, status, request, statusUrl, eventsUrl}` |
| 命令 | `POST .../director/commands` | body `{expectedCharacterId, expectedHeadId, expectedVersion, idempotencyKey, action, beatId?, replace?, draftVersion?}`；`action ∈ activate\|pause\|resume\|complete\|skip\|rewind`（开始=activate；回滚=rewind 到指定 beatId） |
| 查询讨论 | `GET /api/v1/director-requests/{requestId}` | 终态对账 |
| 取消讨论 | `POST /api/v1/director-requests/{requestId}/cancel` | body `{expectedCharacterId}` |

### 7.3 关键语义

- `DirectorPlan = {planId?, revisionId?, title, guidance, beats:[{beatId, title, instruction, completionCriteria}]}`；有效性要求标题非空、至少 1 个 beat 且每 beat 三字段非空。
- `DirectorState = {plan, status: active|paused|completed, currentBeatId, progress: Record<beatId,{status: completed|skipped, sourceNodeId, evidence?:[{blockSeq,quote}]}>, warning?}`。
- 草稿冲突处理：`draft.version > 服务端版本` 时保留本地；保存失败保留本地备份（key 见 §0.2）并提供"保留我的编辑并保存 / 载入已保存草稿 / 刷新状态"。
- 讨论跟踪不依赖组件：`attachDirectorRequest` 用导演 SSE + 3s 轮询对账；终态后刷新工作区。任何 `director.*` 事件（`director.started` 除外）都触发一次对账；`director.completed/failed/cancelled/interrupted` 是 SSE 终态。
- `replace:true` 表示整体替换大纲、从首阶段重新开始（启用时 UI 勾选确认）；启用后从**下一回合**生效。
- 历史视图（`viewNodeId`）只读：不能保存/命令/讨论。

---

## 8. 模型设置

### 8.1 数据模型

- `ModelInstance`（唯一携带连接信息）字段：`id, name, kind, baseUrl?, model?, apiKey?(只写，读取为脱敏值), hasApiKey?, temperature?, maxTokens?, contextWindow?, reasoningEffort?("" | low | medium | high)`。
- `ProviderConfig`（槽位解析视图）：`slot, enabled, modelId?, resolvedModelId?, kind?, baseUrl?, model?, apiKey?(脱敏), hasApiKey?, temperature?, maxTokens?, contextWindow?, reasoningEffort?`；`modelId` 为空 = 跟随 `primary`。
- 三槽位（UI 名 / slot / 用途）：
  - **对话生成** `primary` — 推进剧情；
  - **导演协商** `assist` — 导演模式讨论大纲；
  - **后台记忆** `reflection` — 抽取记忆、维护摘要；只有它有整体开关（`enabled=false` 显示"已关闭"，其它槽位 false 表示回落主线）。
- 接口：`GET /api/v1/config/models` → `{models, slots}`；`POST/PUT/DELETE /api/v1/config/models[/{id}]`；`PUT /api/v1/config/provider` `{slot, enabled, modelId}`；`POST /api/v1/config/provider/probe` `{slot, format}`、`POST /api/v1/config/models/{id}/probe` `{format}`。

### 8.2 连接表单字段与校验（ConnectionForm）

顺序：服务商预设（DeepSeek/OpenAI/Anthropic/本地 Ollama/OneAPI/自定义）→ **模型名称** `model` → **接口地址** `baseUrl` → **API Key**（密码框+眼睛；提示"已保存密钥 sk-1…abcd（留空则不修改）"；回传空串=保留原密钥；含"…"的脱敏值会被拦下）→ **名称** `name` → **协议** `kind`（openai-chat / openai-responses / anthropic-messages）→ **生成参数**：上下文窗口、最大输出、温度(0~2)、思考强度（默认/低/中/高）。

- 校验：名称非空；模型名非空；baseUrl 若非空必须是完整 http(s) URL；温度 0~2；窗口 ≤ 2,000,000 且 `窗口 − 最大输出 ≥ 2048`。
- 派生提示："输入预算约 N tokens（窗口 − 最大输出 − 安全余量）"；公式与后端一致：`window − output − max(512, floor(window/20))`（output 空按 2048）。
- 新连接默认值：`kind=openai-chat, temperature=0.8, maxTokens=4096, contextWindow=131072, reasoningEffort=""`。
- 删除保护：被槽位引用时禁用删除并列出"正在被 X / Y 使用"。

### 8.3 探测（probe）

- 连接表单的"测试连接"：先保存过才能探测（无 id 报"先保存连接，再测试它"）；`probeModel(id, format=false)` → `ProbeResult {slot, ok, kind?, model?, connectMsg?, formatTested?, formatOk?, formatMsg?, latencyMs?}`；`format=false` 只做连通性检查（不请求格式帧，**不产生生成费用**）。
- 成功显示绿色 Banner：`连接正常 · 延迟 {latencyMs} ms · 模型 {model}`；失败显示 `connectMsg` 或错误消息。
- `settingsStore.probes` 缓存每个模型最近结果、`probing` 标记在途；均只在内存。
- 槽位级别另有 `probeProvider(slot, format)`（当前 UI 未直接使用）。

### 8.4 输入台快捷控件

- `ModelQuickSwitch`：显示当前主线实例标题（`connectionTitle`），菜单选择即 `assignSlot("primary", enabled, modelId)`，Toast"已切换到「X」，从下一回合开始生效"。
- `ThinkingEffortControl`：修改**主线连接本身**的 `reasoningEffort`（`saveModel({...current, apiKey:""})`；空串=默认不发思考参数）；同样只影响下一回合。
- 设置页"写作偏好"：选项"填入编辑"模式开关、截断自动续写开关、剧情选项频率（auto/always/never）——这三项**不持久化**（见 §0.2）。

---

## 9. TTS 与图像

### 9.1 朗读（TTS）

- 引擎（`tavernagent_tts_settings`）：`edge`（默认，zh-CN-XiaoxiaoNeural，免配置）/ `mimo`（小米 MiMo，电影级导演控制）/ `openai`（OpenAI 兼容本地模型 GPT-SoVITS/CosyVoice/Kokoro）/ `web-speech`（浏览器原生，离线）。
- 文本抽取（`dialogueOnly` 默认 **true**）：优先按结构化 blocks 只取 `kind==="dialogue"`；纯文本回退用引号正则 `[「“『]...」”』|"..."`；无对白时按钮显示"仅旁白"并 2.2s 提示，不发请求。
- 音色：`GET /api/v1/tts/voices` → `TTSVoice[] {id,name,gender,locale,description}`（拉取失败静默、使用内置回退列表）；Edge 用 `edgeVoice`；openai/mimo 用自由文本 `customVoice`。
- 合成：`POST /api/v1/tts/synthesize`，body `{engine, voice, text, instruction?, baseUrl?, apiKey?, model?, speed?}`（openai/mimo 才带 baseUrl/apiKey/model）；返回音频 Blob。
- 导演模式（MiMo 专属增强）：用角色人设、场景（`state.scene.title`→`outline.scene`）、当前导演阶段、心境文本/码、关系量表、旁白打包成 `instruction`，并给台词注入 `[深呼吸][停顿]` 类音频标签；非 MiMo 引擎自动 `stripAudioTags`，避免念出标记词。心情来源链路：回合 `changes` 里的 mood → `hud.moods[char]` → `state.moods[char]`。
- 播放器（`lib/ttsPlayer.ts`，全局单例）：
  - 点击同段 = 停止（toggle）；点新段 = 先 `stop()` 再播；
  - 引擎返回的 Blob → `URL.createObjectURL` → `new Audio`；LRU 缓存最多 20 条（key = `engine:voice:speed:baseUrl:instruction:text`），淘汰时 revoke；
  - 播放失败（离线/后端不可用）自动降级 `window.speechSynthesis`（`utter.lang="zh-CN"`, `rate=speed`）；
  - 播放状态通过订阅通知所有 TTS 按钮（`is-playing` 高亮）；
  - 语速滑块 0.5–2.0（默认 1.0）；"试听发音"播放固定样例"你好！很高兴能与你一同开启这段旅程。"。
- 每回合/开场白卡片右上角有"朗读"按钮；打开生成时工具栏含"停止"。

### 9.2 图像生成

- 设置（`tavernagent_image_settings`）：引擎 `comfyui`（默认，`http://127.0.0.1:8188`，model `image_qwen_image_2_1_t2i`）/ `openai`（SiliconFlow Flux / DALL·E 3）/ `sd-webui`（`http://127.0.0.1:7860`）；字段 baseUrl/apiKey/model；默认画幅 `1024x576`（另有 `1024x1024`、`576x1024`）；风格 `anime|fantasy|cinematic|realistic`（各带正/负向提示词）。
- **不是自动生成**：由回合工具栏的"生图"按钮打开 `IllustrationModal`。
  - prompt 自动草拟：取该回合全部 block 文本 → 去括号注释（中英文）/Markdown 符号 → 截前 180 个字符 → `Character: <名>, <摘要>, <风格正向词>`；若卡片带 `sdData`（Stable Diffusion 模板），前置拼接为基底。负向词仅本地生成，不提交给后端（后端接口有 `negativePrompt?` 但当前 UI 不传）。
  - 用户可改 prompt、换风格（换风格会重算 prompt）与画幅。
  - 请求：`POST /api/v1/images/generate`，body `{prompt, engine, baseUrl, apiKey, model, size, style}` → `{id, url, prompt, mimeType, createdAt}`。
  - 成功后 `attachIllustration(turnId, res)` 写入持久化映射 `turnIllustrations`（key 为消息 id：回合节点=nodeId，开场=`<nodeId>:opening`），并内联预览。
  - 回合下渲染 `TurnIllustration`：缩略图 + "放大"（全屏 lightbox，Esc/点击遮罩关闭）+ "重绘"；图片 URL 由后端 `GET /api/v1/images/{filename}` 提供（**该路由不加鉴权**，浏览器 `<img>` 可直接加载）。
- 卡内 `<img cmd>` 等标记（`lib/cardMarkers.tsx` + `lib/risuImages.tsx`）：从 `/api/v1/cards/{cardId}` 的 `assets[{name,type,url}]` 建立 name→url 映射（仅内存缓存），把正文标记渲染为 `<img class="story-inline-image" loading="lazy">`；`[HUD:]`/`[SCENE:]` 渲染为原生状态面板/场景芯片，`[VAR:]` 不展示；`[[` 转义为字面 `[`。

---

## 10. 剧情包归档（.tavernpack）

- 导出：`GET /api/v1/sessions/{sessionId}/export`（**不带 branchId**，即整个会话）→ 二进制 Blob。
  - 文件名 `packFileName(title)`：`<标题清洗后截 40 字>-YYYYMMDD-HHmmss.tavernpack`（非法字符 `/\:*?"<>|` → `_`）。
  - 下载实现：`URL.createObjectURL` + 临时 `<a download>`，10s 后 revoke。
  - 成功后 Toast"已导出 .tavernpack 剧情包"；失败写 `packError`（面板常驻 Banner"导出失败：…"）。
- 导入：`<input type="file" accept=".tavernpack,.zip">` → `file.arrayBuffer()` → `POST /api/v1/sessions/import`，`Content-Type: application/zip`，body 原始字节 → `ImportResult {sessionId, title, rootNodeId, scope?, counts?, assets?}`。
  - 成功后：`clearSessionCache()`（导入会重写 ID/引用）→ `loadSessions()` → `openSession(res.sessionId)` → Toast"已导入剧本包：<文件名>"。
  - 失败写 `packError`（"导入失败：…"）并重新抛出；Banner 可手动关闭。
- 两个请求都走 `binaryRequest`：30s 超时、401 触发配对、非 2xx 抛带后端文本的 `ApiError`。
- **没有任何进度条**：导出/导入都是单次同步请求，只有"进行中"禁用与最终成功/失败提示。Android 若要显示进度，需要自行加（后端也未提供进度端点）。

---

## 11. 视觉设计令牌与移动端成像

令牌唯一来源：`web/src/styles/tokens.css`（`styles.css` 通过 `@import` 先引入）。组件样式只允许引用变量，不得写颜色字面值。主题切换 = `<html data-theme="dark">`。

### 11.1 颜色令牌（light 默认 / `[data-theme="dark"]`）

| 语义 | 令牌 | Light | Dark |
| --- | --- | --- | --- |
| 页面底 | `--bg-base` | `#fafafc` | `#101114` |
| 下沉画布 | `--bg-canvas` | `#f3f4f6` | `#141518` |
| 卡片面 | `--bg-surface` | `#ffffff` | `#1a1b1f` |
| 内嵌井 | `--bg-subtle` | `#f8f9fa` | `#16171a` |
| 浮起面 | `--bg-elevated` | `#ffffff` | `#212227` |
| 输入/字段 | `--bg-muted` | `#f3f4f6` | `#212227` |
| 面板底 | `--bg-panel` | `#fafafc` | `#141518` |
| 发丝线 | `--border-light` | `#e5e7eb` | `#2a2b30` |
| 强线 | `--border-mid` | `#d1d5db` | `#383940` |
| 墨线 | `--border-dark` | `#111827` | `#f4f4f6` |
| 悬停/最弱线 | `--border-subtle` | `#f3f4f6` | `#1f2024` |
| 主文本 | `--text-main` | `#111827` | `#f4f4f6` |
| 次文本 | `--text-muted` | `#4b5563` | `#a1a1aa` |
| 弱文本 | `--text-dim` | `#9ca3af` | `#71717a` |
| 强调 | `--accent-solid` | `#2563eb` | `#2563eb` |
| 强调对比 | `--accent-contrast` | `#ffffff` | `#ffffff` |
| 强调淡底 | `--accent-subtle` | `#eff6ff` | `rgba(37,99,235,.15)` |
| 强调深 | `--accent-glow` | `#1d4ed8` | `#3b82f6` |
| 琥珀（导演/警示） | `--accent-amber` | `#d97706` | `#f59e0b` |
| 绿（成功） | `--accent-green` / `--color-success` | `#059669` | `#10b981` |
| 危险（兼容别名） | `--danger` | `#e3474c` | 同（未覆盖） |
| 危险文本 | `--color-danger-text` | `#dc2626` | `#ef4444` |
| 危险底 | `--color-danger-bg` | `rgba(220,38,38,.08)` | `rgba(239,68,68,.14)` |
| 警告 | `--color-warning` | `#d97706` | `#f59e0b` |
| 信息 | `--color-info` | `#2563eb` | `#2563eb` |
| 焦点环 | `--ring` | `rgba(37,99,235,.35)` | `rgba(37,99,235,.40)` |
| 遮罩 | `--scrim` | `rgba(15,23,42,.45)` | 同（未覆盖） |
| 量表槽 | `--meter-track` | `#e5e7eb` | `#2a2b30` |
| 心境三色 | `--meter-aff` / `--meter-trust` / `--meter-alert` | `#e11d48` / `#2563eb` / `#d97706` | 未覆盖（沿用 light） |
| 记忆分类线 | `--memory-episodic/-inference/-secret` | `#2563eb` / `#d97706` / `#9333ea` | 未覆盖（沿用 light） |
| 成功底 | `--color-success-bg` | `rgba(5,150,105,.09)` | `rgba(16,185,129,.14)` |
| 警告底 | `--color-warn-bg` | `rgba(217,119,6,.08)` | `rgba(245,158,11,.14)` |

阴影：light `--shadow-sm/md/float = 0 0 0 1px #e5e7eb + 1px/3px/12px 黑色低透明`；dark 变成白色发丝环 + 更重的黑层（`0 0 0 1px rgba(255,255,255,.08~.12)`）。
`index.html` 的 `theme-color` 是 `#f9f8f5`(light) / `#111215`(dark)，与 `--bg-base` 略有差异——按系统偏好而非应用内主题切换，移植时二者可各自取值。

### 11.2 圆角 / 间距 / 字体 / 骨架

- 圆角：`--radius-xs 4 / sm 6 / md 8 / lg 10 / xl 14 / full 9999`（chip 6 / control 8 / card 10 / window 14 / pill 999）。
- 间距：`--space-1..6 = 4 / 8 / 12 / 16 / 24 / 32`。
- 字号：`--fs-xs 10.5 / sm 11.5 / md 13 / lg 15 / xl 18`；行高 `--lh-tight 1.35 / normal 1.6 / narrative 1.78`；字重 400/500/600。
- 字体：
  - `--font-display` = `--font-body` = `Inter, ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif`；
  - `--font-narrative` = `'Noto Serif SC', Georgia, "Songti SC", serif`（**叙事正文专用衬线**）；
  - `--font-mono` = `"JetBrains Mono", ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace`（思考正文、ticker）。
- 骨架尺寸：`--header-h 56px`（≤768px → 48px）、左栏 `224px`/折叠 `52px`、右栏 `304px`、阅读列 `--reading-w 820px`。
- 动效：`--duration-micro 80 / quick 150 / fast 250 / medium 350 / slow 450`；缓动 `--ease-smooth-out cubic-bezier(.23,1,.32,1)`、`--ease-bounce (.34,1.36,.64,1)`、`--ease-spring (.16,1,.3,1)`；`prefers-reduced-motion` 时全部归零。
- 断点约定：**480 / 768 / 1024 / 1280** 四档，全站一致。

### 11.3 叙事排版（移动端要镜像的字号）

| 元素 | 桌面 | ≤768px | 纯净编年模式 |
| --- | --- | --- | --- |
| `.dialogue-content-text` 正文 | 15px / 1.75，`--font-narrative` | 15px / 1.8 | 16.5px / 1.9 |
| `.block-narration` 旁白 | 15.5px / 1.85，左侧 2px `--border-dark` 竖线 + 16px 缩进 | 15px / 1.8 | 16.5px / 1.95，缩进 20px |
| `.player-speech-text` 玩家文本 | 15px / 1.6 | 15px / 1.8 | 16.5px / 1.9 |
| `.composer-textarea` | 13.5px | **16px**（防 iOS 聚焦缩放） | 13.5px |

`.dialogue-turn-wrap`：桌面白卡 + 1px 发丝线 + `--radius-lg(10)` + `16px 20px` 内边距 + `--shadow-sm`；≤768px 圆角降为 `--radius-sm(6)`、内边距 `12px 14px`。悬停边框变 `--border-mid`、阴影升 `--shadow-md`；流式中边框变 `--accent-solid`。

### 11.4 独特版式/视觉身份（移动端应保持的识别点）

1. **文学流，不是聊天气泡**：角色回复是**整宽卡片**，内部是连续散文（旁白/对白/心声三种段落样式）；只有玩家输入是**右对齐、max-width 85%（手机 95%）的气泡**（`--bg-surface` + `--border-mid`）。助手侧没有头像气泡布局。
2. **三种正文段落语言**：旁白 = 衬线 + 左侧 2px 墨色竖线；对白 = `「…」` 包裹；心声 = `（…）` 包裹并弱化颜色。`*动作*`/`**强调**` 走极简内联 Markdown（仅粗体/斜体）。
3. **思考块（Reasoning）**：生成中是一条"呼吸"状态行（图标随 `--accent-solid` 脉动 + 单行 ticker 滚动 + "展开/收起"），settle 后变成可展开的折叠区，标题"思考过程 (Reasoning)"+"思考了 N 秒"；内容等宽 12px、行高 1.7、`--bg-subtle` 底、最高 300px 可滚。这是与通用聊天 App 拉开差距的核心组件。
4. **抉择卡**：2 列网格（手机 1 列），每张卡左侧 3px 意图色竖条 + 顶部意图徽章（强势/机智/共情/出其不意）+ 正文，点击即推进；只在最新回合且 idle 时出现。
5. **底部输入中枢**：一张圆角卡片（`--radius-xl` + 发丝线 + focus 时 `--accent-solid` 边框 + focus ring），顶部一行工具芯片（导演模式/注记/对白/动作/心声 + 快捷宏药丸），中间朴素 textarea，底部控制行（左：+ 圆钮、模型胶囊、导演状态徽章；右：上下文仪表、思考强度、圆形**强调蓝发送键**，忙碌时按钮展开成"演义中/同步中"文字）。
6. **顶栏**：左品牌 + 场景面包屑（书名 + 分支药丸），中间"工作台 ⇄ 纯净编年"分段 Tabs，右侧命令面板入口、当前主线模型胶囊、明暗主题切换。手机端 48px、只留品牌 + 模式 + 图标按钮（文字隐藏）。
7. **左右栏身份**：左栏 4 页签（脉络/角色/世界/会话），树是"竖线 + 节点卡"；右栏是"立绘舞台"（290px 高、顶部表情药丸、底部表情切换器、底部溶墨渐变蒙版）+ 铭牌 + 三个导航磁贴 + 三维量表（细条）+ 物品格 + 会话变量 + 前情回顾。手机端左右栏变成**贴底抽屉**（≤480px，带抓取条、圆角顶、最高 75dvh），481–768px 为左右侧滑抽屉，顶栏触控目标 ≥44px。
8. **材质纪律**：扁平 surface + 1px 发丝线 + 低透明柔和阴影，**无毛玻璃、无装饰渐变、无 `!important`**；唯一渐隐用于立绘底部与叙事流底部蒙版（`mask-image`）。
9. **纯净编年模式（reading）**：隐藏两侧栏与场景条，去卡片化（透明底、无边框），正文放大到 16.5px/1.9，顶部给出书名题头、字数、预估读毕分钟、当前轮次，底部输入台极简。
10. **安静 Toast**：单行居中通知 2.2s 自动消失（右上/底部皆可，Web 是 fixed 底部居中），用于"已展开/已收起/已切换"等低强度反馈；错误另有叙事流内的错误卡（红色描边，含重试/模型设置/关闭）。

---

## 12. Android 移植对照清单（易踩的语义点）

1. **token 与 401**：token 存本地；401 `AUTH_REQUIRED` 要能拉起配对界面；配对成功后旧数据缓存要整体作废（Web 是 `clearSessionCache()`）。
2. **幂等键必须复用**：`send/regenerate/edit/command/message/createSession/memory patch` 在"结果未知"的重试里必须带**同一个 key**；明确 4xx 后不能复用（后端的失败终态会被重放）。
3. **SSE 分帧与去重**：只有带 `id:` 的持久事件参与去重；`turn.thinking`/`block.delta` 无 id，断线后靠 `turn.draft` 快照补齐且只能让文本变长。Last-Event-ID 需 `aggregateId:seq` 格式。
4. **相位/只读锁定**：`generating/truncated/awaiting_confirm/committed` 都视为忙碌；`viewNodeId != null` 视为只读历史（输入、编辑、记忆、导演全部禁用）。
5. **选项双模式**：direct 直接请求；fill-edit 只填入输入框。选项只在最新回合且 idle 时展示。
6. **注记**：必须作为独立 `input.note` 发送并在回看时展示（`TurnContent.inputNote`）。
7. **分支写操作 4 种语义**：fork 只建分支；regenerate/regen-recheck/guidance 生成候选分支；edit-input 派生并生成；edit-assistant 同步纯叙事候选。都不要改老节点。
8. **刷新恢复**：只恢复"上次会话"，不恢复分支/节点；在途回合用 `branch.activeTurnId` + `GET /turns/{id}` 恢复；思考/草稿是内存态，断连后服务端权威。
9. **导演讨论不绑 UI**：讨论请求一旦发起，跟踪由 store/后台负责（SSE + 3s 轮询），面板关掉也要走到终态。
10. **TTS/生图凭据**：TTS 与生图的自定义 baseUrl/apiKey/model 由客户端**直传后端**，密钥存在客户端本地存储里（明文 JSON）——Android 端应至少用 EncryptedSharedPreferences，且不要把它当安全边界。
