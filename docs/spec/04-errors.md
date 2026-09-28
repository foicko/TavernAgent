# 04 统一错误模型

## 1. 错误信封（强制）

所有 `/api/` 错误响应必须是**扁平 JSON 信封**：

```json
{
  "code": "IDEMPOTENCY_CONFLICT",
  "message": "人类可读的中文说明",
  "retryable": false,
  "turnId": "",
  "traceId": "01H..."
}
```

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `code` | string | 是 | 稳定错误码（见 §2），客户端据此分支，**不要解析 `message`** |
| `message` | string | 是 | 面向用户/日志的说明；语言与措辞可变 |
| `retryable` | bool | 是 | 客户端是否应重试（配合退避） |
| `turnId` | string | 是 | 与回合相关的错误带回合 ID，否则空串 |
| `traceId` | string | 否 | 等于 `X-Request-ID`；仅在 `withLogging` 生效时存在 |

- 实现：`internal/adapters/http/respond.go` 的 `writeError` / `writeAPIError`。
- 序列化失败时的兜底也必须保持同一形状（仅缺 `traceId`）。
- **禁止**：嵌套 `{"error":{...}}`；对 `/api/` 错误使用 `http.Error` / 纯文本。

## 2. 错误码枚举（跨端共享）

错误码是**跨端契约**。当前 Go 侧映射在 `internal/application/errors.go` 的 `Err`（`*APIError{Code,Message,Retryable,StatusCode}`）。规范要求这份枚举成为**唯一源**，并生成 TS/Kotlin 常量（待补，见 §4）。

### 领域错误码与 HTTP 状态

| 错误码 | HTTP | retryable | 含义 |
|---|---|---|---|
| `IDEMPOTENCY_CONFLICT` | 409 | false | 同幂等键但载荷/基线不匹配 |
| `HEAD_CONFLICT` | 409 | false | 期望分支头与实际不一致（乐观并发） |
| `STALE_OPTION` | 409 | false | 选项已过期/不适用 |
| `MISSING_IDEMPOTENCY_KEY` | 422 | false | 写请求缺少幂等键 |
| `PROTOCOL_INVALID` | 422 | false | 模型输出不符合协议 |
| `OPERATION_REJECTED` | 422 | false | 操作在当前状态不可执行 |
| `QUEUE_FULL` | 429 | true | 生成队列已满 |
| `PROVIDER_UNAVAILABLE` | 502 | true | 模型供应商不可用 |
| `STORAGE_UNAVAILABLE` | 503 | true | 存储不可用 |
| `BUDGET_EXCEEDED` | 507 | false | 超出预算/配额 |
| `MODEL_IN_USE` | 409 | false | 模型被 slot 引用，不能删 |
| `MODEL_NOT_FOUND` | 404 | false | 模型不存在 |
| `SLOT_TAKES_REFERENCE_ONLY` | 400 | false | slot 只能引用 modelId，不能内联连接信息 |

### 传输/框架错误码

| 错误码 | HTTP | 触发 |
|---|---|---|
| `AUTH_REQUIRED` | 401 | 非本机 LAN 访问且未带合法 token |
| `INVALID_PIN` | 401 | 配对码错误 |
| `FORBIDDEN` | 403 | Host 非私有地址，或跨来源写请求 |
| `TOO_MANY_ATTEMPTS` | 429 | 配对尝试过多（带 `Retry-After`） |
| `NOT_FOUND` | 404 | 资源不存在 |
| `BAD_REQUEST` | 400 | 请求体解析失败 |
| `REQUEST_TOO_LARGE` | 413 | 请求体超限 |
| `INTERNAL` | 500 | 未预期错误（细节只进日志） |
| `SSE_UNSUPPORTED` | 500 | 连接不支持流式（无 Flusher） |
| `SHUTTING_DOWN` | 503 | 服务关停中拒绝新请求 |

> 未知/未映射的 Go 错误统一降级为 `500 INTERNAL` + 通用中文消息，真实错误只写日志（不泄漏内部细节）。

## 3. 客户端处理规则（三端一致）

1. 读 `code` 决定行为，**不解析 `message`**。
2. `retryable == true`：按退避重试（SSE 见 [`05-sse.md`](05-sse.md)；普通请求建议 1/2/4/8/16s 上限 5 次）。
3. `401`：清除本地 token，跳转配对（Web/Android）。
4. `409 IDEMPOTENCY_CONFLICT`：**不要盲目重试**；重新拉取最新状态后再决定。
5. `4xx`（除 `429`）：终态失败，向用户提示；`5xx`/`429`/网络错误：可重试。
6. 把 `traceId` 附到错误提示/日志，便于对账。

## 4. 差距（必须收敛）

- **无跨端共享的错误码枚举**：目前只有 Go 侧 `APIError` 与 TS/Kotlin 各自的字符串。目标：在 `testdata/contract/` 增加错误码源（或并入 `api.json`），生成三端常量，并加契约测试。
- **stdlib 纯文本错误**：`/api/` 下未知路径与 `404/405` 未走信封。目标：用自定义 `NotFoundHandler` / `MethodNotAllowedHandler` 包装为信封。
- **`message` 目前是中文**：规范允许其可变；若未来需要多语言，只改 `message`，`code` 不变。
