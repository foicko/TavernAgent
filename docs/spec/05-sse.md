# 05 SSE 事件流契约

## 1. 端点与传输

三个订阅端点（均为 `security` 包装，token 规则见 [`08-auth-security.md`](08-auth-security.md)）：

| 路径 | 聚合 ID（aggregateId） |
|---|---|
| `GET /api/v1/turns/{turnId}/events` | `turnId` |
| `GET /api/v1/sessions/{id}/events` | 会话 ID |
| `GET /api/v1/director-requests/{requestId}/events` | 请求 ID |

- 响应头：`Content-Type: text/event-stream`、`Cache-Control: no-cache`、`Connection: keep-alive`。
- 无查询参数；重连状态**只**来自请求头 `Last-Event-ID`（格式 `<aggregateId>:<sequence>`）。
- 心跳：每 15s 一条 SSE 注释 `: ping`；服务端每 1s 对账一次 outbox；30s 写超时在每次 flush 前重设。

## 2. 帧格式

**持久事件**（来自 outbox，可重放）：

```
event: <type>
id: <aggregateId>:<sequence>
data: <payload JSON>

```

**瞬时事件**（`turn.thinking`、`block.delta`，仅 `Sequence == 0`，不持久、不重放，无 `id:`）：

```
event: <type>
data: <payload JSON>

```

**心跳**：`: ping`。**轮询失败**：`event: error` + `data: {"message":"event stream poll failed"}`（无 `id:`）。

## 3. 事件名与负载字段（强制）

事件名唯一源：`testdata/contract/sse-events.json`；Go 常量 `internal/protocol/events.go`；TS `sseEvents.generated.ts`；Kotlin `SseEventNames.kt`。

### 回合事件

| 事件 | 持久 | 负载字段 |
|---|---|---|
| `turn.started` | 是 | `turnId`, `branchId` |
| `turn.thinking` | 否 | `attemptId`, `thinking` |
| `block.delta` | 否 | `attemptId`, `seq`(int), `kind`, `speakerId`(nullable), `delta` |
| `block.appended` | 是 | `attemptId`, `frameSeq`(int), `frame` = `{v,seq,type,kind,speakerId,text}` |
| `turn.truncated` | 是 | `turnId`, `attemptId`, `code`, `message`, `reason`, `retryable` |
| `turn.committed` | 是 | `nodeId`, `branchId`, `branchVersion` |
| `turn.cancelled` | 是 | `turnId`, `attemptId`, `code`, `message`, `reason`, `retryable` |
| `turn.failed` | 是 | 同 `turn.cancelled` |
| `turn.conflicted` | 是 | 同 `turn.cancelled` |

### 导演事件

| 事件 | 持久 | 负载字段 |
|---|---|---|
| `director.started` | 是 | `requestId` |
| `director.completed` | 是 | `requestId` |
| `director.failed` | 是 | `requestId` |
| `director.cancelled` | 是 | `requestId` |
| `director.interrupted` | 是 | `requestId` |

### 内部事件（不作为 SSE 契约暴露）

| 事件 | 说明 |
|---|---|
| `turn.state.updated` | 仅用于唤醒轮询，`Sequence==0`，空负载，不会写到 wire |

### 终态事件

`turn.committed`、`turn.cancelled`、`turn.failed`、`turn.conflicted`（`terminalTurnEvents`）。到达终态后客户端关闭订阅。`turn.truncated` 与两个 `awaiting_*` **不是终态**。

## 4. 顺序、去重与重连

- **顺序权威是 outbox 的 `sequence`**（每聚合单调递增）。通知只是唤醒信号；客户端必须以 `sequence` 去重/排序。
- **重连**：客户端带 `Last-Event-ID: <aggregateId>:<sequence>`；服务端从该序号之后重放持久事件。序号前缀不匹配的 `Last-Event-ID` 被忽略（从 0 开始）。
- **瞬时事件不重放**：断线期间丢失的 `turn.thinking`/`block.delta` 由「快照」恢复——见 [`09-sync-idempotency.md`](09-sync-idempotency.md)。**没有 `snapshot` 事件名**：快照走 REST（`GET /api/v1/turns/{turnId}` → `draft.inFlight`）。
- **退避**：建议 1/2/4/8/16s，上限 5 次；`4xx`（除 `429`）视为终态失败，`5xx`/`429`/网络错误可重试。Android 额外遵守服务端 `retry:` 作为退避下限。
- **停滞看门狗**：Web 45s 无数据即重连（心跳 15s，故 45s 足以判定异常）。

## 5. 三端实现要求

| 要求 | Go | Web | Android |
|---|---|---|---|
| 产出事件名/负载与契约一致 | 契约测试 + 源码扫描 | 类型级断言 | 解码测试 |
| 解析 `event:`/`id:`/`data:` | — | `app/stream.ts` | `data/remote/SseClient.kt` |
| `Last-Event-ID` 重连 + 去重 | 服务端实现 | `stream.ts` | `TurnStreamRegistry.kt` |
| 终态关闭订阅 | — | `turnSlice.ts` | `TurnStreamRegistry.kt` |
| 断线快照恢复 | `GET /turns/{id}` 提供 `draft.inFlight` | `mergeInFlightDraft` | `TurnEvent.Snapshot` |

> 注意：浏览器原生 `EventSource` 不能设置请求头，而 token 是**仅请求头**的（[`08-auth-security.md`](08-auth-security.md)）。因此非本机 LAN 场景下 Web/Android 都必须用**可设头的流式客户端**（Web 用 `fetch` + ReadableStream），不能用原生 `EventSource`。

## 6. 差距（必须收敛）

- **续写信号统一为 `turn.truncated`**：状态 `awaiting_continuation` 存在，但**没有** `turn.awaiting_continuation` / `turn.awaiting_approval` 事件名（后端从不产出，已从契约与三端移除）。三端一律以 `turn.truncated` 作为「需续写」信号。
- **负载字段未机器化**：目前只有事件名单源化，负载字段靠散文（本章）与各端代码。目标：在 `sse-events.json` 中为每个事件声明必需字段，三端加解析断言（B3.3 扩展）。
- **`turn.committed` 的 live 唤醒负载是 `{}`**：只有持久事件带真实负载；客户端必须依赖 `sequence>0` 的持久事件读取 `nodeId`，不要读 live 唤醒的负载。
