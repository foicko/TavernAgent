# 03 REST API 契约

## 1. 通用约定

- **前缀**：所有业务接口在 `/api/v1/` 下（`/api/v1` 是命名空间，不是兼容承诺，见 [`01-principles.md`](01-principles.md) §3）。
- **方法限定**：路由用 Go 1.22+ 的 `METHOD /path` 模式注册。**已知路径 + 错误方法**由 stdlib 返回 `405`（带 `Allow` 头，纯文本，非 JSON 信封）。
- **JSON**：请求与响应均为 `application/json; charset=utf-8`；编码前先序列化再写头（`respond.go` 的 `writeJSON`），并追加换行。
- **请求体**：超限返回 `413 REQUEST_TOO_LARGE`，解析失败返回 `400 BAD_REQUEST`（`request_body.go`）。
- **请求 ID**：`withLogging` 为每个请求分配/回显 `X-Request-ID`；错误信封中的 `traceId` 即此值。
- **字段命名 / 时间 / ID**：见 [`07-conventions.md`](07-conventions.md)。
- **幂等**：所有写接口必须支持幂等键，见 [`09-sync-idempotency.md`](09-sync-idempotency.md)。

## 2. 路由表

### 健康 / 状态 / 静态

| 方法 | 路径 | 说明 | 中间件 |
|---|---|---|---|
| GET | `/healthz` | 存活探针，返回 `{"ok":true}` | 无（豁免 host 防线） |
| GET | `/api/status` | 运行时状态 | security |
| GET | `/` 及其余非 `/api/` GET/HEAD | 嵌入 SPA 静态资源（index.html 回退） | 无 |

### 鉴权 / 配对

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v1/auth/pair` | 用配对码换取 token；限流 10 次/5 分钟，超限 `429 TOO_MANY_ATTEMPTS` + `Retry-After` |
| GET | `/api/v1/auth/status` | 返回 `{authenticated, required}` |

### 会话 / 分支 / 节点 / 图谱

| 方法 | 路径 |
|---|---|
| POST / GET | `/api/v1/sessions` |
| GET / DELETE | `/api/v1/sessions/{id}` |
| GET / POST | `/api/v1/sessions/{id}/branches` |
| GET | `/api/v1/sessions/{id}/export` |
| POST | `/api/v1/sessions/import` |
| GET | `/api/v1/sessions/{id}/graph` |
| GET | `/api/v1/sessions/{id}/lorebook` |
| GET | `/api/v1/nodes/{nodeId}` |

### 回合 / 再生 / 编辑

| 方法 | 路径 |
|---|---|
| POST | `/api/v1/sessions/{id}/branches/{branchId}/turns`（受理，需幂等键） |
| POST | `/api/v1/sessions/{id}/branches/{branchId}/regenerations` |
| POST | `/api/v1/sessions/{id}/branches/{branchId}/edits` |
| GET | `/api/v1/turns/{turnId}` |
| POST | `/api/v1/turns/{turnId}/cancel` |
| POST | `/api/v1/turns/{turnId}/continue` |

### 记忆

| 方法 | 路径 |
|---|---|
| GET | `/api/v1/sessions/{id}/memories` |
| PATCH | `/api/v1/sessions/{id}/branches/{branchId}/memories/{memoryId}` |
| POST | `/api/v1/sessions/{id}/branches/{branchId}/memories/organize` |

### 导演

| 方法 | 路径 |
|---|---|
| GET | `/api/v1/sessions/{id}/branches/{branchId}/director` |
| PUT | `/api/v1/sessions/{id}/branches/{branchId}/director/draft` |
| POST | `/api/v1/sessions/{id}/branches/{branchId}/director/messages` |
| POST | `/api/v1/sessions/{id}/branches/{branchId}/director/commands` |
| GET | `/api/v1/director-requests/{requestId}` |
| GET | `/api/v1/director-requests/{requestId}/events`（SSE） |
| POST | `/api/v1/director-requests/{requestId}/cancel` |

### 模型 / 供应商配置

| 方法 | 路径 |
|---|---|
| GET / POST | `/api/v1/config/models` |
| PUT / DELETE | `/api/v1/config/models/{modelId}` |
| POST | `/api/v1/config/models/{modelId}/probe` |
| GET / PUT | `/api/v1/config/provider` |
| POST | `/api/v1/config/provider/probe` |

### 卡片

| 方法 | 路径 | 中间件 |
|---|---|---|
| POST | `/api/v1/cards` | security |
| POST | `/api/v1/cards/import`（兼容别名） | security |
| GET | `/api/v1/cards` | security |
| GET | `/api/v1/cards/{cardId}` | security |
| DELETE | `/api/v1/cards/{cardId}` | security |
| GET | `/api/v1/cards/{cardId}/assets/{name}` | **hostOnly（无 token）** |

### TTS / 图像 / 嵌入 / 目录

| 方法 | 路径 | 中间件 |
|---|---|---|
| GET | `/api/v1/tts/voices` | **hostOnly** |
| POST | `/api/v1/tts/synthesize` | security |
| POST | `/api/v1/tts/plan` | security |
| POST | `/api/v1/images/plan` | security |
| POST | `/api/v1/images/generate` | security |
| GET | `/api/v1/images/{filename}` | **hostOnly** |
| POST | `/api/v1/embeddings/probe` | security |
| POST | `/api/v1/embeddings` | security |
| GET | `/api/v1/catalog` | security |

### SSE

| 方法 | 路径 |
|---|---|
| GET | `/api/v1/turns/{turnId}/events` |
| GET | `/api/v1/sessions/{id}/events` |
| GET | `/api/v1/director-requests/{requestId}/events` |

## 3. 响应约定

- **成功**：直接返回资源对象或 `{ok:true}`；集合型响应用具名数组字段（如 `{"sessions":[...]}`、`{"cards":[...]}`），**不裸返数组**。
- **受理型**：回合受理返回 `{turnId,status,statusUrl,eventsUrl}`；客户端据此订阅 SSE。
- **删除**：返回成功信封（`{ok:true}` 或空体），幂等（重复删除不报错）。
- **分页**：记忆等集合用 `hasMore` + 游标（`oldestTurnId` 等）表达，**不用页码**。
- **错误**：见 [`04-errors.md`](04-errors.md)。

## 4. 中间件顺序（Go）

```
requestDrain.wrap → withLogging → apiHostGuard(mux) → [per-route] security / hostOnly / directorRoute(security)
```

- `requestDrain`：关停期间新请求返回 `503 SHUTTING_DOWN`。
- `withLogging`：`X-Request-ID`、panic 恢复、访问日志；`/healthz` 跳过日志。
- `apiHostGuard`：**任何 `/api/` 路径**都要求 Host 是 loopback/私有地址，否则 `403 FORBIDDEN`（与 token 无关）。`/healthz` 与静态资源豁免。
- `security`：Host +（非 loopback 且启用 PIN 时）token +（写请求带 Origin 时）同源校验。详见 [`08-auth-security.md`](08-auth-security.md)。
- `hostOnly`：仅 Host 校验，无 token，用于浏览器无法附加 token 的资源读取路由。

**三端实现方法**：客户端必须对 `/api/` 请求使用 loopback/私有 Host，并（非本机时）在 `Authorization: Bearer` 或 `X-Auth-Token` 中带 token；写请求不要带不匹配的 `Origin`。

## 5. 差距（必须收敛）

- 未知 `/api/*` 路径与 stdlib `404/405` 返回**纯文本**，不符合 [`04-errors.md`](04-errors.md) 的统一信封。目标：`/api/` 下的所有错误都返回 JSON 信封。
- 路由表目前**无机器校验**（仅 `android/docs/API_CONTRACT.md` 散文 + playwright e2e）。目标：纳入契约门禁（见 [`06-dto.md`](06-dto.md) 与 [`13-testing-gates.md`](13-testing-gates.md)）。
