# 09 同步、重放与幂等

> 决策：**快照 + 重放 + 幂等键**。

## 1. 写幂等（强制）

**所有写接口必须支持幂等键**，重复提交同一键返回同一结果，绝不产生重复副作用。

### 回合受理（主路径）

- 请求必须带 `idempotencyKey`；缺失 → `422 MISSING_IDEMPOTENCY_KEY`。
- 唯一约束：`UNIQUE(session_id, idempotency_key)`。
- 重放校验（`checkReplayMatches`）：分支、基线（容忍 maintenance 节点）、载荷哈希、模式一致 → 返回既有回合；不一致 → `409 IDEMPOTENCY_CONFLICT`。
- 队列满时创建的回合被标记失败并**绑定该幂等键**，同键重试返回该失败回合（不新建）。

### 再生 / 编辑

- 键缺省为 `derive_<uuid>`；同键返回既有分支/回合，或 `IDEMPOTENCY_CONFLICT`。

### 导演命令与请求

- 命令：键非空且 ≤200 字符；比对整请求哈希；存储主键 `(session_id, idempotency_key)` + `payload_hash` → 命中返回 `AlreadyDone`，哈希不符 `409`。
- 讨论请求：`UNIQUE(session_id, idempotency_key)`；命中返回旧请求，哈希/分支不符 `409`。

### 会话创建

- 带幂等键创建时 `sessionId = "session_" + sha256(idempotencyKey)`；重复创建返回同一会话。

### 客户端要求

| 要求 | Web | Android |
|---|---|---|
| 每个写操作生成稳定幂等键 | ✅ | ✅ |
| 重试复用同一键（网络错误/超时） | ✅ | ✅ |
| 收到 `409 IDEMPOTENCY_CONFLICT` | 重新拉状态，不盲目重试 | 同 |
| 键的作用域 | 按 session/分支隔离 | 同 |

**规则**：幂等键必须在**用户可见的一次操作**内稳定（例如「发送回合」按钮触发一次操作 → 一个键）；不得每次重试生成新键，否则幂等失效。

## 2. 事件重放

- 持久事件写入 outbox，`sequence` 每聚合单调递增，可重放（见 [`05-sse.md`](05-sse.md)）。
- 重连时带 `Last-Event-ID: <aggregateId>:<sequence>`，服务端从该序号之后重放。
- **瞬时事件**（`turn.thinking`/`block.delta`）不持久、不重放。

## 3. 快照恢复

断线期间丢失的瞬时增量通过「快照」补齐：

1. 持久事件：`Last-Event-ID` 重放。
2. 进行中的当前块：`GET /api/v1/turns/{turnId}` → `draft.inFlight`（来自 `TurnService.PartialDraft`），客户端合并进本地草稿。

- **没有 `snapshot` SSE 事件名**；快照是 REST 拉取。Android 内部把它建模为 `TurnEvent.Snapshot(status,inFlight)`，但这不是 wire 事件。
- Web：`mergeInFlightDraft`（仅在能延长当前块时合并）。
- Android：`TurnStreamRegistry` 仅在 SSE 未在投递时合并 `draft.inFlight`。

## 4. 轮询对账（fallback）

SSE 之外，三端都要有轮询兜底：

- Web：`GET /turns/{id}` 每 5s。
- Android：每 3s。
- 收到 `turn.started/turn.truncated/turn.awaiting_*` → 触发一次对账；`turn.committed` → 拉取完整视图。

**规则**：SSE 是主通道，轮询是对账；两者都必须幂等（重复应用同一状态不得产生副作用）。

## 5. 并发（乐观锁）

- 写回合用 `expectedHeadId` + `expectedVersion` 做乐观并发控制。
- 不匹配 → `409 HEAD_CONFLICT`；选项过期 → `409 STALE_OPTION`。
- 客户端收到 `409` 必须刷新分支头/视图后再让用户重试，不得自动用旧基线重放。

## 6. 差距

- Web 与 Android 的对账间隔不同（5s vs 3s），不构成契约差异，但规范建议统一到 **3s**（或都改为「事件驱动 + 长间隔兜底」），减少实现分歧。
- Android 的终态集合只含回合事件，导演终态靠状态轮询；Web 把 `director.*` 也算终态。规范建议统一：**导演请求的终态由 `director.completed/failed/cancelled/interrupted` 决定**，轮询仅作兜底。
