# TavernAgent HTTP API Contract (Android 客户端对接)

> 唯一事实来源：`internal/adapters/http/*.go`（`*_test.go` 除外），并向下追踪
> `internal/application`、`internal/domain`、`internal/ports`、`internal/protocol`、`internal/pack`、
> `internal/adapters/{tts,image,embedding}`、`cmd/tavernagent`。与 Web 端消费点（`web/src/app/api.ts`、
> `web/src/app/types.ts`）交叉校验。与任何散文文档冲突时，以本文件描述的源码行为为准。
>
> 约定：所有 JSON 字段名都是 Go struct 的 `json` tag（外部一律 camelCase，除少数历史字段例外，
> 文中逐一标注）。时间字段是 RFC3339 字符串。ID 是不透明字符串，客户端不要解析其结构。

---

## 目录

- [1. 连接基础](#1-连接基础)
  - [1.1 Base URL / 端口](#11-base-url--端口)
  - [1.2 TLS 模式](#12-tls-模式)
  - [1.3 通用请求/响应约定](#13-通用请求响应约定)
- [2. 鉴权、配对与来源校验](#2-鉴权配对与来源校验)
  - [2.1 网络边界（Host 校验）](#21-网络边界host-校验)
  - [2.2 配对 `POST /api/v1/auth/pair`](#22-配对-post-apiv1authpair)
  - [2.3 鉴权状态 `GET /api/v1/auth/status`](#23-鉴权状态-get-apiv1authstatus)
  - [2.4 Token 的呈现方式](#24-token-的呈现方式)
  - [2.5 Origin 校验](#25-origin-校验)
  - [2.6 免鉴权路由清单](#26-免鉴权路由清单)
  - [2.7 401 / 403 精确响应体](#27-401--403-精确响应体)
- [3. 错误信封](#3-错误信封)
- [4. 健康检查与运行状态](#4-健康检查与运行状态)
- [5. 角色卡](#5-角色卡)
- [6. 会话](#6-会话)
- [7. 回合（生成 / 重生成 / 编辑 / 取消 / 续写）](#7-回合生成--重生成--编辑--取消--续写)
- [8. SSE 事件流](#8-sse-事件流)
- [9. 节点、图谱、世界书](#9-节点图谱世界书)
- [10. 状态与面板数据](#10-状态与面板数据)
- [11. 记忆](#11-记忆)
- [12. 导演](#12-导演)
- [13. 模型配置](#13-模型配置)
- [14. TTS / 图像 / 向量](#14-tts--图像--向量)
- [15. 展示目录](#15-展示目录)
- [16. 附录](#16-附录)
  - [16.1 错误码总表](#161-错误码总表)
  - [16.2 并发、锁与幂等语义](#162-并发锁与幂等语义)
  - [16.3 端到端最短路径](#163-端到端最短路径)

---

## 1. 连接基础

### 1.1 Base URL / 端口

- 默认监听：`-addr 127.0.0.1:8890` → `http://127.0.0.1:8890`（仅本机）。
- 局域网：`tavernagent -addr 0.0.0.0:8890`（或指定某个私网 IP）→ Android 用
  `http://<局域网IP>:8890` 直连。
- 所有路由都在同一端口；`/api/v1/...` 是 REST，`/healthz` 是无鉴权探活；
  若二进制嵌入了前端 `dist`，根路径 `/` 提供 SPA（Android 不需要）。
- 服务端超时：`ReadHeaderTimeout=5s`、`ReadTimeout=120s`（覆盖整个请求体读取）、
  `IdleTimeout=60s`；SSE 每次写入的写截止时间为 30s。
- 服务关停期间新请求会拿到 `503 {"code":"SHUTTING_DOWN","retryable":true}`。

### 1.2 TLS 模式

启动开关 `-tls`（`cmd/tavernagent/tls.go` + `internal/util/tlscert`）：

| 取值 | 含义 |
| --- | --- |
| `off`（默认） | 明文 HTTP |
| `auto` | 自动生成自签证书（数据目录内），日志打印 SHA-256 指纹 |
| `files` | 使用 `-tls-cert` / `-tls-key` 指定的 PEM |

安全约束：绑定非回环地址且 `-tls != off` 时，必须显式提供 `-pin`，否则启动失败。
TLS 只解决"传输被看"，不解决"谁能进"——配对鉴权在两种模式下都存在。
Android 连接自签 HTTPS 需要自行信任证书（或使用网络配置例外）；明文 HTTP 需要在
network security config 中放行 cleartext。

#### 1.2.1 Android 明文流量策略（M57）

客户端当前在 `AndroidManifest.xml` 用 `android:usesCleartextTraffic="true"` **全局**
放行明文，而不是用 `networkSecurityConfig` 按域放行。这是有意为之：

- 目标主机由用户在运行时填写（任意局域网 IP / `*.local` / 任意端口），而
  `networkSecurityConfig` 只能静态列举**字面域名**，无法表达"任意私有网段"。
  静态配置要么把 LAN 明文全部挡死（功能不可用），要么退化成与全局放行等价。
- 因此静态 `networkSecurityConfig` 在本项目**不可实施**。残留下限：配对令牌是局域网
  完整授权，若用户把地址填成恶意/错误主机，明文请求会把令牌发往该主机。

已实施的缓解：`allowBackup=false`（云 / `adb backup` 不外带存储）、配对令牌经
Keystore 包装存储、服务端对所有 `/api/` 请求做 Host 白名单（拒绝非本机/非私有网段，
防 DNS rebinding），客户端在保存地址时校验 `http(s)://host:port` 形式。

后续可选加固（尚未实施）：发送令牌前在客户端校验目标主机属于回环/私有网段，
命中公网地址时拒绝明文并提示改用 HTTPS。

### 1.3 通用请求/响应约定

- 请求体：JSON（除卡片上传、剧情包导入、SSE 外）。默认 JSON 上限 `1 MiB`
  （`maxRequestBytes`）；超出返回 `413 REQUEST_TOO_LARGE`。请求体必须是**恰好一个**
  JSON 对象，尾随内容返回 `400 BAD_REQUEST`。
- 成功响应：`Content-Type: application/json; charset=utf-8`，UTF-8 编码。
- 每个请求都会回显响应头 `X-Request-ID`（客户端可用该头传入，最长 64 字符的可打印
  ASCII，否则服务端重新生成；`/healthz` 不参与）。
- 错误体见 §3；错误体中的 `traceId` 与 `X-Request-ID` 同值。
- 路由用 Go 1.22 `ServeMux` 的方法+路径模式注册。方法不匹配会得到 **405**（纯文本
  `Method Not Allowed`，带 `Allow` 头），不存在路径会得到 **404**（纯文本），
  二者**不是** §3 的 JSON 错误体——客户端解析错误体前先判 `Content-Type`。

---

## 2. 鉴权、配对与来源校验

`internal/adapters/http/security.go`。所谓"受保护路由"= 套了 `s.security(...)` 的路由，
它依次做三件事：

1. **Host 校验**：`Host` 头的主机名必须是 `localhost` 或可解析的**回环/私网 IP**，
   否则 `403 FORBIDDEN`（明文域名如 `myhost.local` 会失败，除非它就是 `localhost`）。
2. **局域网鉴权**：当且仅当**TCP 对端 IP 不是回环**且服务端配置了 PIN 时，要求携带
   合法 token（见 §2.4），否则 `401 AUTH_REQUIRED`。
   - 判定依据是对端 IP，不是 `Host`、不是 `X-Forwarded-For`（后者被完全忽略）。
   - 本机进程（对端 `127.0.0.1` / `::1`）访问一律免 token。
3. **写请求 Origin 校验**：请求方法 ∈ {POST, PUT, PATCH, DELETE} 且带 `Origin` 头时，
   Origin 必须同源或被 `-allow-origin` 白名单允许，否则 `403 FORBIDDEN`。

> 注意：以上顺序中，非私网/非回环 Host 在第一关就 403，即使 token 合法。

### 2.1 网络边界（Host 校验）

- 允许：`localhost[:port]`、`127.0.0.0/8`、`::1`、RFC1918 私网 IP（`10/8`、`172.16/12`、
  `192.168/16`）、IPv6 私网 `fc00::/7` 等 `net.IP.IsPrivate()` 为真的地址。
- 拒绝：公网 IP、任意域名（除 `localhost`）、`0.0.0.0`。
- 卡片资源与图片读取路由不套 security，所以 `<img>` 加载不受影响。

### 2.2 配对 `POST /api/v1/auth/pair`

- 鉴权：公开（自身做 Host 校验 + 写请求 Origin 校验，不要求 token）。
- 配对码规则：服务端启动时取 `-pin`，留空则生成 **6 位数字**随机码并打印在控制台日志
  （仅当绑定非回环地址时打印）。比较前对请求值 `trim`，再恒时比较。
- 限流：**按对端 IP**，窗口内首次失败起 5 分钟有效；同一 IP 失败 **10 次**后返回 429，
  带 `Retry-After`（秒，最少 1）；5 分钟后自动解锁。
- 请求：

```json
{ "pin": "123456" }
```

- 成功（200）：

```json
{ "ok": true, "token": "3f2c…（48 位十六进制）" }
```

- 若服务端未配置 PIN（理论上仅在嵌入式/测试装配下）：`200 {"ok":true,"token":""}`。
- 失败：

| 状态 | body.code | 说明 |
| --- | --- | --- |
| 401 | `INVALID_PIN` | 配对码错误（计入失败次数） |
| 429 | `TOO_MANY_ATTEMPTS` | 失败 ≥10 次，带 `Retry-After` 响应头 |
| 400 | `BAD_REQUEST` | body 不是合法 JSON / 请求体 >1024 字节 |
| 403 | `FORBIDDEN` | Host 不受支持，或跨来源写请求 |

- Token 生命周期：**进程生命周期**。服务启动时若未显式注入固定 token（CLI 无此开关），
  会随机生成 24 字节（48 hex 字符）；重启后 token 与（未固定 `-pin` 时的）PIN 都会变化。
  客户端必须持久化 token，并在收到 `401 AUTH_REQUIRED` 时重新走配对。

### 2.3 鉴权状态 `GET /api/v1/auth/status`

- 鉴权：公开（Host 校验仍生效）。
- 响应（200）：

```json
{ "authenticated": true, "required": false }
```

| 场景 | authenticated | required |
| --- | --- | --- |
| 回环对端 | `true` | `false` |
| 非回环对端且服务端未配置 PIN | `true` | `false` |
| 非回环对端 + 有 PIN + 无/错 token | `false` | `true` |
| 非回环对端 + 有 PIN + 正确 token | `true` | `true` |

- token **只能**从请求头读，`?token=` 查询参数无效。

### 2.4 Token 的呈现方式

后续请求在以下任一请求头携带 token（`extractToken`）：

```
Authorization: Bearer <token>
```

或（Web 不使用，Android 可用）：

```
X-Auth-Token: <token>
```

适用对象包括 REST、上传、下载与 SSE（SSE 用 `fetch` 流式读，不能带自定义头的场合请用
`Authorization`）。**所有**受保护路由都适用；公开路由即使带了 token 也不校验。

### 2.5 Origin 校验

- 只对写方法（POST/PUT/PATCH/DELETE）生效；**GET/HEAD 不校验**。
- `Origin` 为空：放行（原生 Android 客户端默认不发 Origin）。
- Origin 解析后要求：scheme ∈ {http, https}、有 host、无 userinfo、路径为空或 `/`、
  无 query/fragment；然后与 `(当前连接 scheme, Host)` 同源，或在 `-allow-origin`
  白名单中（逗号分隔，比较时忽略尾部 `/`）。
- 不通过：`403 FORBIDDEN`（`"跨来源写请求被拒绝"`）。

### 2.6 免鉴权路由清单

完全不经过 `security()`（无需 token、不做 Host 校验）：

| 方法+路径 | 备注 |
| --- | --- |
| `GET /healthz` | 探活 |
| `GET /api/v1/tts/voices` | 音色目录（只读公开） |
| `GET /api/v1/images/{filename}` | 生成图片文件（`<img>` 加载） |
| `GET /api/v1/cards/{cardId}/assets/{name}` | 卡内嵌资源（`<img>` 加载） |

自带独立校验（Host + Origin，但不需要 token）：

| 方法+路径 | 备注 |
| --- | --- |
| `POST /api/v1/auth/pair` | 配对 |
| `GET /api/v1/auth/status` | 鉴权状态 |

其余全部受保护。注意：如果 Android 用域名（如 `http://tavern.local:8890`）访问，
受保护路由会因 Host 校验失败返回 403；请直接使用 IP。

### 2.7 401 / 403 精确响应体

两者都是标准错误信封（§3）：

```json
{
  "code": "AUTH_REQUIRED",
  "message": "局域网访问需要配对码授权",
  "retryable": false,
  "turnId": "",
  "traceId": "…（与 X-Request-ID 同值）"
}
```

```json
{
  "code": "FORBIDDEN",
  "message": "跨来源写请求被拒绝",
  "retryable": false,
  "turnId": "",
  "traceId": "…"
}
```

`FORBIDDEN` 的 message 有三种（同码不同文案）：
`"只允许本机或局域网私有网络访问"`（Host 关）、`"跨来源写请求被拒绝"`（Origin 关）、
`"请求来源不受支持"`（pair 路由的 Host/Origin 关）。

---

## 3. 错误信封

`internal/adapters/http/respond.go` 的 `writeError`。**所有** handler 产生的错误
（含 4xx/5xx）都是同一形状：

```json
{
  "code": "HEAD_CONFLICT",
  "message": "分支已变化，请刷新当前节点",
  "retryable": true,
  "turnId": "",
  "traceId": "AbC123xY"
}
```

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `code` | string | 是 | 稳定错误码（见 §16.1） |
| `message` | string | 是 | 面向用户的中文文案，客户端可直接展示 |
| `retryable` | bool | 是 | 服务端建议是否可原样重试 |
| `turnId` | string | 是 | 回合相关错误可携带；普通错误为空串 |
| `traceId` | string | 否 | 仅当响应头 `X-Request-ID` 存在时出现（正常请求都有） |

状态码语义（`application.Err` 的默认覆盖，多数调用点显式传状态）：

| code | 默认状态（未显式指定时） |
| --- | --- |
| `IDEMPOTENCY_CONFLICT` | 409 |
| `HEAD_CONFLICT`, `STALE_OPTION` | 409 |
| `QUEUE_FULL` | 429 |
| `PROVIDER_UNAVAILABLE` | 502 |
| `PROTOCOL_INVALID`, `OPERATION_REJECTED` | 422 |
| `BUDGET_EXCEEDED` | 507 |
| `STORAGE_UNAVAILABLE` | 503 |
| 其它 | 400 |

未知异常统一 `500 INTERNAL / "服务内部错误，请稍后重试；若持续出现请查看服务端日志"`，
`retryable=true`；handler panic 也会被中间件兜底成同一形状（前提是响应头还没写出）。

---

## 4. 健康检查与运行状态

### 4.1 `GET /healthz`

- 鉴权：公开；不写访问日志、不分配 `X-Request-ID`。
- 200：

```json
{ "ok": true }
```

### 4.2 `GET /api/status`

- 鉴权：受保护。
- 响应 200（观测端点，任何一段取不到就降级为空/缺省，**不会** 5xx）：

```json
{
  "counters": {
    "turnsAccepted": 3,
    "turnsCommitted": 2,
    "turnsFailed": 0,
    "turnsCancelled": 1,
    "continuations": 0,
    "budgetExceeded": 0,
    "memoriesInjected": 12,
    "budgetDroppedHistory": 0,
    "budgetDroppedSummaries": 0,
    "budgetUnfit": 0,
    "summaryBlocked": 0,
    "summaryNormalized": 0,
    "queueDropped": 0,
    "queueRejected": 0,
    "usageWriteFailures": 0,
    "compilePhases": {
      "lorebook": { "calls": 2, "totalMs": 8, "avgMs": 4.0 }
    }
  },
  "prompt": "v6/c1a2b3c4d5e6",
  "ablation": "none",
  "trace": {
    "total": 12,
    "slow": 1,
    "slowThresholdMs": 1500,
    "recent":  [ { "requestId": "…", "name": "http.request", "method": "GET", "path": "/api/v1/sessions", "status": 200, "ms": 12.3, "marks": {"compile.lorebook": 4.1} } ],
    "slowest": [ ]
  },
  "usage": {
    "Calls": 1,
    "Prompt": 100,
    "Completion": 20,
    "Cached": 80,
    "Estimated": 130,
    "ReportedCalls": 1,
    "LastRecordedAt": "2026-09-13T00:00:00Z"
  },
  "recent": [
    {
      "TurnID": "…", "AttemptID": "…", "Model": "…", "Provider": "openai-chat",
      "Prompt": 100, "Completion": 20, "Cached": 80, "Estimated": 130,
      "Reported": true, "CreatedAt": "2026-09-13T00:00:00Z",
      "Task": "generation", "Slot": "primary", "ConfigFingerprint": "…",
      "LatencyMS": 1200, "FirstTokenMS": 300, "Outcome": ""
    }
  ]
}
```

**重要**：`usage` 与 `recent` 的字段**没有** `json` tag，所以 JSON 键是大写 Go 字段名
（`Calls`、`TurnID`…），不要按 camelCase 解析。`compilePhases` 只包含发生过的阶段；
`trace` 仅在进程内 tracer 支持快照时出现。

---

## 5. 角色卡

路由（`internal/adapters/http/cards.go`）。`POST /api/v1/cards` 与
`POST /api/v1/cards/import` 是**同一个 handler**（别名）。

| 方法+路径 | 鉴权 | 说明 |
| --- | --- | --- |
| `POST /api/v1/cards` | 受保护 | 导入并入库 |
| `POST /api/v1/cards/import` | 受保护 | 同上（兼容别名） |
| `GET /api/v1/cards` | 受保护 | 卡库摘要列表 |
| `GET /api/v1/cards/{cardId}` | 受保护 | 单卡完整预览 |
| `DELETE /api/v1/cards/{cardId}` | 受保护 | 移除（不影响已有会话） |
| `GET /api/v1/cards/{cardId}/assets/{name}` | 公开 | 卡内资源字节 |

### 5.1 `POST /api/v1/cards` / `POST /api/v1/cards/import`

**接受的请求体形态**（按内容/头部自动识别）：

1. `multipart/form-data`：字段名先试 `file`，再试 `card`；表单内存阈值 8 MiB，超出落盘。
2. 原始字节体（非 multipart）：按魔数识别
   - `\x89PNG\r\n\x1a\n` → PNG（解析 `chara` / `ccv3` 元数据）；
   - `PK\x03\x04` → charx / ZIP 归档；
   - 其它先探测是否为"尾部带 ZIP 中央目录的 charx"，否则当 JSON 卡片解析。

**大小上限**：

| 项 | 值 |
| --- | --- |
| 上传整体硬上限 | 2 GiB（`maxCardUploadBytes`，超限 `413 REQUEST_TOO_LARGE`） |
| 内存内解析的 PNG/JSON | 64 MiB（超限返回解析错误，最终 `422 CARD_INVALID`） |
| 归一化后卡片 JSON | 8 MiB（超限 `422 CARD_TOO_LARGE`） |
| charx 单条目解压 | 64 MiB（跳过并追加 warning） |
| charx 解压总量 | 2 GiB（截断并追加 warning） |
| 卡库容量 | 256 张（超限 `409 CARD_LIBRARY_FULL`） |

**响应 200**（`CardPreview`，键名与 `web/src/app/types.ts` 对齐）：

```json
{
  "format": "chara_card_v2",
  "specVersion": "2.0",
  "source": "png",
  "name": "艾莲娜",
  "avatar": "data:image/jpeg;base64,…",
  "description": "…",
  "personality": "…",
  "scenario": "…",
  "firstMes": "…",
  "mesExample": "…",
  "systemPrompt": "…",
  "postHistoryInstructions": "…",
  "creatorNotes": "…",
  "tags": ["fantasy"],
  "creator": "…",
  "characterVersion": "1.0",
  "nickname": "…",
  "supported": ["name", "description"],
  "ignored": ["…说明文字…"],
  "warnings": [],
  "characterJson": "…归一化后的卡 JSON 字符串…",
  "card": { "schemaVersion": 1, "cardId": "…", "name": "…", "characters": [ … ], "…": "…" },
  "openings": [
    { "variantId": "ov_1", "title": "开场 · 门前", "text": "……" }
  ],
  "capabilities": { "source": "charx", "imgCmd": true, "markerFamilies": ["scene"], "…": "…" },
  "varDefaults": { "weather": "晴" },
  "themeTokens": { "accent": "#2563eb" },
  "sdData": ["masterpiece, best quality"],
  "cardId": "card_<hash>",
  "shortName": "艾莲娜",
  "role": "流浪剑客",
  "assets": [
    { "name": "立绘", "type": "emotion", "url": "/api/v1/cards/card_123/assets/asset_0.png" }
  ]
}
```

- `capabilities/varDefaults/themeTokens/sdData` 仅在卡片带 UI 档案（Risu 扩展）时出现。
- `assets[].url` 是**可直接加载**的完整相对路径（未落地资源时可能是卡内的 `uri` 原文）。
- `card` 是归一化后的 `CharacterCard`：`schemaVersion, cardId, name, description,
  avatar?, personality?, scenario?, first_mes?, mes_example?, system_prompt?,
  post_history_instructions?, creator_notes?, tags?, creator?, character_version?,
  nickname?, characters[{characterId,name,description,participant,aliases?,attributes?,
  avatar?,personality?,scenario?,mes_example?,system_prompt?,post_history_instructions?,
  creator_notes?,tags?,creator?,character_version?,nickname?}], items[], initialState?,
  secrets?, openingVariants?, lorebookRefs?, assets?, rules?, uiProfile?`（注意这里的
  V2/V3 字段保留**蛇形命名**，与卡生态一致）。
- 同名卡重复导入按 `cardId`（内容哈希派生）覆盖，不产生副本。

错误：`422 CARD_INVALID`、`422 CARD_TOO_LARGE`、`400 BAD_REQUEST`（空体/坏表单）、
`413 REQUEST_TOO_LARGE`、`409 CARD_LIBRARY_FULL`、`503 STORAGE_UNAVAILABLE`。

### 5.2 `GET /api/v1/cards`

200：

```json
{
  "cards": [
    {
      "cardId": "card_123",
      "name": "艾莲娜",
      "shortName": "艾莲娜",
      "avatar": "data:image/jpeg;base64,…",
      "format": "chara_card_v2",
      "role": "流浪剑客",
      "contentHash": "…",
      "createdAt": "2026-09-01T10:00:00Z",
      "updatedAt": "2026-09-01T10:00:00Z",
      "lastUsedAt": "2026-09-20T08:00:00Z"
    }
  ]
}
```

列表**不含** `characterJson`（避免兆级下发）。空库返回 `{"cards":[]}`。

### 5.3 `GET /api/v1/cards/{cardId}`

- 200：与 §5.1 相同形状的 `CardPreview`（服务端重新解析卡内容并回填 `format` 等）。
- 解析失败时降级为原始 `CharacterCardEntry`（含 `characterJson`）。
- 404 `CARD_NOT_FOUND`。

### 5.4 `DELETE /api/v1/cards/{cardId}`

- 200：`{ "ok": true }`。删除是幂等的（不存在也当成功）。
- 不影响任何已有会话；仅当没有任何会话引用该角色时才会回收其资源。

### 5.5 `GET /api/v1/cards/{cardId}/assets/{name}`

- 鉴权：公开。返回资源原始字节，`Content-Type` 由资源存储决定；
  `Cache-Control: public, max-age=31536000, immutable`。
- 不存在/存储未启用：`404`（纯文本）。`name` 取自 `assets[].url` 的最后一段。

---

## 6. 会话

路由（`internal/adapters/http/sessions.go` 与 `server.go`）。

| 方法+路径 | 鉴权 | 说明 |
| --- | --- | --- |
| `POST /api/v1/sessions` | 受保护 | 创建（开局） |
| `GET /api/v1/sessions` | 受保护 | 全部会话 |
| `GET /api/v1/sessions/{id}` | 受保护 | 会话视图（窗口化） |
| `DELETE /api/v1/sessions/{id}` | 受保护 | 永久删除 |
| `GET /api/v1/sessions/{id}/branches` | 受保护 | 分支列表 |
| `POST /api/v1/sessions/{id}/branches` | 受保护 | 从节点分叉（不生成） |
| `GET /api/v1/sessions/{id}/export` | 受保护 | 导出 .tavernpack |
| `POST /api/v1/sessions/import` | 受保护 | 导入 .tavernpack |

### 6.1 `POST /api/v1/sessions`

请求（上限 8 MiB，与卡片 JSON 同额度）：

```json
{
  "idempotencyKey": "uuid-…（≤128 字节，可选）",
  "title": "风暴前夕",
  "cardId": "card_123",
  "characterJson": "…（cardId 与 characterJson 至少一个，cardId 优先）",
  "playerName": "旅人",
  "playerRole": "流浪剑客",
  "playerBackpack": [
    { "instanceId": "it_1", "templateId": "tpl_sword", "name": "旧剑", "quantity": 1, "protection": 0, "keepsake": false, "unique": true }
  ],
  "openingVariantId": "ov_2",
  "openingText": "…（非空时优先于 openingVariantId）"
}
```

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `idempotencyKey` | string | 否 | ≤128 字节；确定性会话 ID，见下 |
| `title` | string | 否 | 空时取卡名 |
| `cardId` | string | 二选一 | 卡库中的卡（优先） |
| `characterJson` | string | 二选一 | 归一化卡片 JSON（内置预设/旧客户端） |
| `playerName` | string | 否 | 空时服务端用默认称呼 `"旅人"` |
| `playerRole` | string | 否 | 身份/倾向描述 |
| `playerBackpack` | `ItemInstance[]` | 否 | 起始背包 |
| `openingVariantId` | string | 否 | 缺省回退第一个变体 |
| `openingText` | string | 否 | 非空时优先于变体 |

- `openingVariantId` 不存在 → `422 CARD_VARIANT_NOT_FOUND`；卡没有任何开场 →
  `422 CARD_NO_OPENING`。
- 幂等：`idempotencyKey` 非空时，`sessionId = "session_" + sha256(key)`；同键同载荷重放
  返回同一会话（201）；同键不同载荷 `409 IDEMPOTENCY_CONFLICT`。

响应 201：

```json
{
  "sessionId": "session_…",
  "title": "风暴前夕",
  "rootNodeId": "root_session_…",
  "branchId": "branch_main_session_…",
  "branchName": "main",
  "branchVersion": 0,
  "openingText": "……",
  "state": {
    "relationships": {}, "items": {}, "promises": {}, "moods": {}, "goals": {},
    "milestones": {}, "characters": {}, "scene": { "sceneId": "scene_1", "title": "门前" }
  }
}
```

其它错误：`422 CARD_MISSING`（两个卡字段都空）、`422 CARD_INVALID`、`404 CARD_NOT_FOUND`、
`400 BAD_REQUEST`（幂等键 >128 字节）、`413 REQUEST_TOO_LARGE`、`503 STORAGE_UNAVAILABLE`。

### 6.2 `GET /api/v1/sessions`

200：`{"sessions":[ Session ]}`，空库为 `[]`。`Session`：

```json
{
  "sessionId": "session_…",
  "rootNodeId": "root_…",
  "title": "风暴前夕",
  "createdAt": "2026-09-01T10:00:00Z",
  "updatedAt": "2026-09-20T08:00:00Z",
  "rulesetVersion": "v1",
  "characterId": "card_123"
}
```

（`rulesetVersion`/`characterId` 为 `omitempty`，旧数据可能没有。）

### 6.3 `GET /api/v1/sessions/{id}` —— SessionView

查询参数：

| 参数 | 类型 | 默认 | 说明 |
| --- | --- | --- | --- |
| `branchId` | string | 第一条分支 | 写入位置 |
| `viewNodeId` | string | 分支头 | 查看位置（浏览历史不改分支） |
| `limit` | int ≥0 | 100 | 窗口内最多回合数；上限 500（超出按 500） |
| `before` | string | — | 向上翻页游标：只取该节点之前的回合 |

响应 200（`application.SessionView`）：

```json
{
  "director": { "title": "第一幕", "revisionId": "…", "status": "active",
                "currentBeatId": "beat_1", "currentBeatTitle": "初遇",
                "completed": 1, "total": 3, "warning": "" },
  "sessionId": "session_…",
  "characterId": "card_123",
  "title": "风暴前夕",
  "rootNodeId": "root_…",
  "branch": { "branchId": "branch_…", "name": "main", "headNodeId": "node_…",
              "version": 7, "activeTurnId": "turn_…" },
  "branches": [ { "branchId": "…", "sessionId": "…", "name": "main",
                  "headNodeId": "…", "version": 7 } ],
  "viewNodeId": "node_…",
  "viewNode": { "nodeId": "…", "sessionId": "…", "parentId": "…", "kind": "turn",
                "depth": 3, "turnNumber": 3, "schemaVersion": 1,
                "createdAt": "…", "contentJson": "…" },
  "headNode": { "……": "同上" },
  "nodes": [ { "……": "root 节点始终在最前，其后是窗口内回合，按时间升序" } ],
  "candidateGroups": { "<parentNodeId>": [ { "nodeId": "…", "kind": "turn", "…": "…" } ] },
  "state": { "…世界状态快照（见 §10）…" },
  "secrets": [ { "secretId": "sec_1", "title": "失落遗迹", "order": 1,
                 "content": "…（仅已揭示时非空）", "revealed": true } ],
  "outline": { "milestones": [ { "milestoneId": "m1", "description": "穿越黑森林" } ],
               "goals": [ { "goalId": "g1", "characterId": "npc_liel", "text": "找到石板" } ],
               "scene": "古堡大门" },
  "activeSummary": { "summaryId": "…", "fromNodeId": "…", "toNodeId": "…",
                     "sourceHash": "…", "visibilityScope": "…", "text": "…",
                     "modelConfigVersion": "…", "summaryVersion": 1 },
  "actions": [ { "actionId": "act_search", "label": "搜寻线索", "attribute": "perception",
                 "dc": 12, "itemIds": ["it_1"] } ],
  "hasMore": true,
  "oldestTurnId": "node_…",
  "capabilities": {
    "busy": false,
    "canSend": true,
    "canFork": true,
    "atTurnNode": true,
    "viewingHistory": true
  }
}
```

要点：
- `nodes[0]` 恒为 root；`hasMore`+`oldestTurnId` 驱动 `before` 向上翻页。
- `candidateGroups` 只包含**多于一个**回合子节点的父节点（同父兄弟=候选版本）。
- `capabilities`：`busy=true` 时额外有 `activeTurnId`；`canSend=false` 时额外带
  `block:{code,message}`（如 `TURN_IN_FLIGHT`）。服务端只回答服务端可知的部分，
  客户端与本地瞬时状态做合取（可用 = capabilities 允许 && 无本地阻塞）。
- `viewNode`/`headNode`/`nodes[]` 里的 `contentJson` 是**字符串**，需要二次 JSON 解析
  （root 节点内容含 `openingText`、`initialState`、`templates`、`cardUI` 等）。
- 每个节点正文里的图片标记会被服务端切段，见 `TextBlock.segments`（§7.4）。
- 错误：404 `NOT_FOUND`（会话）、404 `BRANCH_NOT_FOUND`、404 `NODE_NOT_FOUND`、
  400 `NODE_SESSION_MISMATCH`、400 `INVALID_CURSOR`、400 `BAD_REQUEST`（limit 非法）、
  500 `STATE_CORRUPT`。

### 6.4 `DELETE /api/v1/sessions/{id}`

- 200：`{"ok":true}`。永不可撤销；前端需二次确认。
- 有在途回合（`branch.activeTurnId != ""`）→ `409 SESSION_BUSY`。
- 404 `NOT_FOUND`。

### 6.5 分支

`GET /api/v1/sessions/{id}/branches` → 200：

```json
{ "branches": [ { "branchId": "branch_…", "sessionId": "session_…", "name": "main",
                  "headNodeId": "node_…", "version": 7 } ] }
```

（`domain.Branch.ActiveTurnID` 是 `json:"-"`，**不会**出现在分支列表里；在途信息看
SessionView 的 `branch.activeTurnId` 或 `capabilities`。）

`POST /api/v1/sessions/{id}/branches`（fork，只建分支不生成）：

```json
{ "fromNodeId": "node_…", "name": "平行分支_7F3A", "expectedCharacterId": "card_123" }
```

- 201：`{ "branch": { "branchId": "…", "sessionId": "…", "name": "…", "headNodeId": "…", "version": 0 } }`
- `name` 缺省为 `"分叉 · 第N轮"` / `"分叉 · 开局"`。
- 错误：`400 NODE_REQUIRED`、`404 NOT_FOUND`（节点）、`400 NODE_SESSION_MISMATCH`、
  `400 CHAR_SESSION_MISMATCH`、`503 STORAGE_UNAVAILABLE`。**非幂等**，重复调用建多个分支。

### 6.6 导出 `GET /api/v1/sessions/{id}/export?branchId=`

- 200：`Content-Type: application/zip`，`Content-Disposition: attachment`（同时给 ASCII
  回退名与 RFC 5987 UTF-8 名），文件名 `<清洗后标题>-YYYYMMDD-HHmmss.tavernpack`。
- `branchId` 为空导出整个会话，否则导出该分支及其必要祖先。
- 包格式：ZIP，含 `manifest.json`、`templates.json`、`nodes.jsonl`、`events.jsonl`、
  `branches.json`、`memories.jsonl`、`receipts.jsonl`、`bookmarks.json`、
  `snapshots.jsonl`、`assets/`。`manifest.formatVersion ∈ {1,2}`（2 = 含导演事件）。
- 错误：404 `NOT_FOUND`、503 `STORAGE_UNAVAILABLE`、500 `EXPORT_FAILED`。

### 6.7 导入 `POST /api/v1/sessions/import`

- 请求体：原始 ZIP 字节（`Content-Type` 不被校验；Web 用 `application/zip`）。
- 硬上限 256 MiB（`pack.MaxPackBytes`），超过 → `413 PACK_TOO_LARGE`。
- 响应 201：

```json
{
  "sessionId": "session_…",
  "title": "风暴前夕",
  "rootNodeId": "root_…",
  "scope": { "full": true, "branchId": "", "branchName": "" },
  "counts": { "templates": 3, "nodes": 120, "events": 240, "branches": 2,
              "memories": 40, "bookmarks": 0, "snapshots": 5, "receipts": 1 },
  "assets": 4
}
```

- 错误：`400 PACK_UNSAFE`（安全拒绝，message 含原因）、`400 INVALID_PACK`（结构/校验）、
  `409 UNSUPPORTED_RULESET`、`400 BAD_REQUEST`（读体失败）、`413 PACK_TOO_LARGE`、
  `503 STORAGE_UNAVAILABLE`。导入是单事务：失败不产生半截会话。**非幂等**。

---

## 7. 回合（生成 / 重生成 / 编辑 / 取消 / 续写）

路由（`internal/adapters/http/server.go` 与 `application/turn_*.go`、`branch.go`）。

| 方法+路径 | 鉴权 | 说明 |
| --- | --- | --- |
| `POST /api/v1/sessions/{id}/branches/{branchId}/turns` | 受保护 | 受理一轮玩家输入（异步生成） |
| `POST /api/v1/sessions/{id}/branches/{branchId}/regenerations` | 受保护 | 重新生成候选 |
| `POST /api/v1/sessions/{id}/branches/{branchId}/edits` | 受保护 | 编辑玩家输入 or 编辑助手正文 |
| `GET /api/v1/turns/{turnId}` | 受保护 | 回合状态/结果/在途草稿 |
| `POST /api/v1/turns/{turnId}/cancel` | 受保护 | 取消 |
| `POST /api/v1/turns/{turnId}/continue` | 受保护 | 续写截断草稿 |
| `GET /api/v1/turns/{turnId}/events` | 受保护 | 回合 SSE |
| `GET /api/v1/sessions/{id}/events` | 受保护 | 会话 SSE |

### 7.1 回合状态机

`status` 取值（`domain.TurnStatus`）：

```
accepted | queued | preparing | generating | validating
awaiting_approval | awaiting_continuation | committed | cancelled | failed | conflicted
```

终态 = `committed | cancelled | failed | conflicted`（终态不可再变；重试永远是**新请求**）。
`awaiting_continuation` 是唯一可"继续"的非终态；`awaiting_approval` 目前不会由服务端主动
进入（代码保留）。

### 7.2 `POST .../turns` 受理一轮输入

请求（上限 1 MiB）：

```json
{
  "idempotencyKey": "k_1727000000000_1",
  "expectedHeadId": "node_…",
  "expectedVersion": 7,
  "afterTurnId": "",
  "mode": "",
  "expectedCharacterId": "card_123",
  "input": {
    "kind": "text",
    "text": "我推开那扇门。",
    "note": "节奏放慢，让 NPC 先开口",
    "options": "auto",
    "optionRef": { "nodeId": "node_…", "optionId": "opt_1" },
    "actionRef": ""
  }
}
```

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `idempotencyKey` | string | **是** | 空 → `422 MISSING_IDEMPOTENCY_KEY` |
| `expectedHeadId` | string | 建议 | 客户端已知的分支头；见下面"头匹配"规则 |
| `expectedVersion` | int64 | 建议 | 与 `expectedHeadId` 配套 |
| `afterTurnId` | string | 否 | 透传存储（展示用；不参与校验） |
| `mode` | string | 否 | `""`/`structured`（默认）/`narrative`，受理时固定 |
| `expectedCharacterId` | string | 否 | 非空时与会话 `characterId` 比对，不符 → `400 CHAR_SESSION_MISMATCH` |
| `input.kind` | string | 否 | `text`（默认）\| `option` \| `action` |
| `input.text` | string | 是* | 玩家输入原文（`option` 时服务端以卡里选项原文为准） |
| `input.note` | string | 否 | 玩家注记，`trim`；>2000 字符 → `400 NOTE_TOO_LONG` |
| `input.options` | string | 否 | `auto`\|`always`\|`never`；空=auto；其它值 → `400 BAD_REQUEST` |
| `input.optionRef` | object | option 时必填 | `{nodeId, optionId}`；缺失 → handler 直接 `422 STALE_OPTION` |
| `input.actionRef` | string | action 时必填 | 必须命中当前规则包已登记动作，否则 `400 ACTION_REQUIRED` / 后续 `403 ACTION_UNAUTHORIZED` |

**头匹配（乐观并发）语义**：

- `expectedHeadId` 等于当前分支头且 `expectedVersion` 相等 → 通过。
- 否则沿父链回溯：客户端给的 `expectedHeadId` 必须是分支头的祖先，且回溯经过的节点
  **全部**是维护节点（`memory_change` / `director_event`），并且 `branch.version >=
  expectedVersion`。任何一个对话节点挡在中间即 `409 HEAD_CONFLICT`。
- `expectedHeadId` 为空串时退化为"仅版本相等"（不推荐；请总是发送视图里的
  `branch.headNodeId` 与 `branch.version`）。
- 受理时服务端以**真实分支头**为回合基准（不是客户端声明值），所以后台维护节点抢先落地
  不会让受理通过却在提交时撞车。

**选项输入的三条规则**：必须来自当前头（可隔着维护节点）、必须真实存在于该回合、
文本被改写时按自由输入处理（丢弃选项引用与动作授权）。

**幂等**：键相同且分支/基准/载荷一致 → 返回既有回合（202，同一 `turnId`），即使它已经
进入终态；键相同但内容不同 → `409 IDEMPOTENCY_CONFLICT`。载荷哈希覆盖
`input + mode + expectedHeadId + DeriveNodeId + ReuseRollId + Recheck`。

响应 202：

```json
{
  "turnId": "turn_…",
  "status": "queued",
  "statusUrl": "/api/v1/turns/turn_…",
  "eventsUrl": "/api/v1/turns/turn_…/events"
}
```

常见错误：`422 MISSING_IDEMPOTENCY_KEY`、`409 HEAD_CONFLICT`（`retryable=true`）、
`409 IDEMPOTENCY_CONFLICT`、`409 STALE_OPTION`、`429 QUEUE_FULL`（该分支已有在途回合，
该幂等键被绑定到一条 `failed/QUEUE_FULL` 的回合上；换新键重试）、
`400 BRANCH_SESSION_MISMATCH`、`404 NOT_FOUND`（分支）、`400 CHAR_SESSION_MISMATCH`、
`503 STORAGE_UNAVAILABLE` / `SERVER_SHUTTING_DOWN`。

### 7.3 `POST .../regenerations` 重生成

请求：

```json
{
  "nodeId": "node_…",
  "idempotencyKey": "rg_1727000000000_1",
  "name": "候选 · 第3轮",
  "recheck": false,
  "note": "让 NPC 先质疑我",
  "options": "never",
  "expectedCharacterId": "card_123"
}
```

- `nodeId` 目标必须是**回合节点**（`kind == "turn"`），否则 `400 NODE_NOT_TURN`。
- 输入默认复用原回合（`inputKind/inputText/inputNote/optionsMode/optionRef/actionRef`）；
  `note`（引导）与 `options` **非空时覆盖**原设置；`input` 字段不存在于本接口。
- `recheck=true` 重新掷骰：要求原回合的选项带 `actionRef`，否则 `400 CHECK_REQUIRED`；
  否则（带动作的选项）自动复用原 RollID（"重生成不重掷"）。
- 通过 `application.BranchService.DeriveTurn` 从 `parent(N)` 建**候选分支**
  （`head = parent(N)`，`version = 0`）并受理新回合；原节点 N 不改写。
- 幂等键缺省时由服务端生成（`derive_<id>`）→ 不会幂等；请客户端自带键。同键不同载荷
  → `409 IDEMPOTENCY_CONFLICT`。

响应 202：

```json
{
  "branch": { "branchId": "branch_…", "sessionId": "…", "name": "候选 · 第3轮",
              "headNodeId": "node_parent", "version": 0 },
  "branchId": "branch_…",
  "turnId": "turn_…",
  "status": "queued",
  "statusUrl": "/api/v1/turns/turn_…",
  "eventsUrl": "/api/v1/turns/turn_…/events",
  "headNodeId": "node_parent"
}
```

### 7.4 `POST .../edits` 编辑

请求二选一，不能同时给（同时 → `400 EDIT_AMBIGUOUS`，都没有 → `400 EDIT_EMPTY`）：

```json
{
  "nodeId": "node_…",
  "expectedCharacterId": "card_123",
  "idempotencyKey": "ed_1727000000000_1",
  "name": "候选 · 第3轮",
  "input": { "kind": "text", "text": "改写后的玩家输入", "note": "", "options": "auto" }
}
```

→ 202，与 §7.3 **相同形状**（但没有 `headNodeId`）：

```json
{ "branch": { "…": "…" }, "branchId": "branch_…", "turnId": "turn_…",
  "status": "queued", "statusUrl": "…", "eventsUrl": "…" }
```

或：

```json
{
  "nodeId": "node_…",
  "expectedCharacterId": "card_123",
  "name": "手改候选",
  "blocks": [
    { "kind": "narration", "text": "她把烟按灭在石栏上。" },
    { "kind": "dialogue", "speakerId": "npc_liel", "text": "走吧。" }
  ]
}
```

→ **201**（同步提交，零状态变化的纯叙事候选）：

```json
{
  "branch": { "…": "…" },
  "branchId": "branch_…",
  "nodeId": "node_new",
  "mode": "narrative"
}
```

- `blocks[].kind` 空 → `narration`；`text` 为空的块被跳过；全部为空 →
  `400 BLOCKS_REQUIRED`。改写正文会**丢弃原回合的状态变化提议**（设计如此，
  避免"叙述改了但背包少了"）。
- 编辑玩家输入走的是"从 parent(N) 重新生成"路径，旧后续不自动复用。

### 7.5 `GET /api/v1/turns/{turnId}`

200：

```json
{
  "turnId": "turn_…",
  "sessionId": "session_…",
  "branchId": "branch_…",
  "status": "generating",
  "mode": "structured",
  "input": { "kind": "text", "text": "我推开那扇门。", "note": "", "options": "auto" },
  "expectedHeadId": "node_…",
  "expectedVersion": 7,
  "failureCode": "",
  "failureMessage": "",
  "capabilities": {
    "canCancel": true,
    "canContinue": false,
    "canRetry": false,
    "terminal": false
  },
  "resultNodeId": "node_…",
  "resultNode": { "nodeId": "node_…", "kind": "turn", "contentJson": "…", "…": "…" },
  "draft": {
    "attemptId": "a_turn_…_1",
    "inFlight": { "seq": 2, "kind": "dialogue", "speakerId": "npc_liel", "text": "你来得太晚了……" }
  }
}
```

- `input` 是受理时持久化的输入对象（可能为 `null`）。
- `resultNodeId`/`resultNode` 仅在已提交时出现。
- `draft` 仅在服务端仍在生成该回合时出现；`inFlight` 只在当前块已有未收尾文本时出现。
  它是断线/刷新后补齐"正在写的这一段"的**权威来源**（增量事件不会重放）。
- `failureCode` 是终态失败码（如 `PROVIDER_UNAVAILABLE`、`PROTOCOL_INVALID`、
  `PROVIDER_EMPTY_OUTPUT`、`CONTEXT_OVER_BUDGET`、`STATE_MISSING`、`PROCESS_INTERRUPTED`…）。
- 404 `NOT_FOUND`。

### 7.6 `POST /api/v1/turns/{turnId}/cancel`

- 请求体可省略，或 `{ "expectedCharacterId": "card_123" }`（省略时不做归属校验）。
- 200：`{ "turnId": "turn_…", "status": "cancelled" }`。
- 可重复调用；已提交时返回 `{…,"status":"committed"}`。取消是"意图 + 打断流"：
  已生成的草稿作废、分支锁释放；在途 worker 会在下一个检查点停止。
- 错误：404 `NOT_FOUND`（回合）、400 `CHAR_SESSION_MISMATCH`、503。

### 7.7 `POST /api/v1/turns/{turnId}/continue`

- 请求体可省略，或 `{ "expectedCharacterId": "card_123" }`。
- 仅 `status == "awaiting_continuation"` 可调用；响应 202：

```json
{ "turnId": "turn_…", "status": "preparing", "eventsUrl": "/api/v1/turns/turn_…/events" }
```

- 语义：恢复已持久化草稿帧 → 再次调用模型补全 → 与普通回合相同的提交路径。
- 错误：`409 TURN_NOT_CONTINUABLE`（状态不对，或已被继续/取消/分支已变化）。
- "放弃截断草稿"就是调 cancel。

### 7.8 已提交回合的正文（`TurnContent`）

`node.contentJson`（JSON 字符串）解析后的字段：

```json
{
  "inputKind": "text",
  "inputText": "我推开那扇门。",
  "inputNote": "节奏放慢",
  "optionRef": { "nodeId": "…", "optionId": "…" },
  "actionRef": "",
  "blocks": [
    { "kind": "narration", "text": "门后是雨。",
      "segments": [
        { "type": "text", "text": "门后是雨。" },
        { "type": "image", "assetKey": "asset_3" }
      ] },
    { "kind": "dialogue", "speakerId": "npc_liel", "text": "……" },
    { "kind": "inner_monologue", "text": "（她在这里。）" }
  ],
  "options": [ { "optionId": "opt_1", "intent": "clever", "text": "…", "actionRef": "" } ],
  "mood": { "characterId": "npc_liel", "moodCode": "vigilant", "text": "警惕环视" },
  "checks": [
    { "rollId": "roll_…", "actionId": "act_search", "attribute": "perception",
      "attributeValue": 12, "attributeModifier": 2, "natural": 15, "total": 17,
      "dc": 12, "outcome": "success", "rulesetVersion": "v1",
      "permanentEffect": "感知敏锐", "effects": [] }
  ],
  "changes": [
    { "kind": "relationship", "label": "莉尔 · 好感", "text": "+2（25 → 27）", "delta": 2 }
  ],
  "injectedMemories": [ { "memoryId": "mem_…", "text": "……", "kind": "observed" } ],
  "optionsMode": "auto",
  "suppressedOptions": 0,
  "provenance": { "turnId": "turn_…", "attemptId": "a_turn_…_1",
                  "mode": "structured", "modelConfig": "" },
  "uiHints": [ { "kind": "hud", "values": { "hp": "90" }, "text": "" } ]
}
```

- `blocks[].segments` 只在正文含图片标记时出现；`type="image"` 的 `assetKey` 用
  `/api/v1/cards/{session.characterId}/assets/{assetKey}`（或卡预览 `assets[].url`）拼装。
- `uiHints[].kind ∈ {hud, scene, var}`；状态类标记由服务端从正文剥离并结构化下发，
  客户端**不要**自己解析标记语法（`GET /api/v1/catalog` 的 `cardMarkers` 描述语法）。
- `check.outcome ∈ {critical_success, success, failure, critical_failure}`。
- `changes[].kind ∈ {relationship, mood, goal, promise, item, scene, milestone, secret}`。

---

## 8. SSE 事件流

`internal/adapters/http/sse.go`。三个订阅入口共用同一循环：

| 订阅 | 端点 | 聚合 ID（= SSE `id:` 前缀） |
| --- | --- | --- |
| 回合 | `GET /api/v1/turns/{turnId}/events` | `turnId` |
| 会话 | `GET /api/v1/sessions/{id}/events` | `sessionId` |
| 导演 | `GET /api/v1/director-requests/{requestId}/events` | `requestId` |

协议行为：

- 响应头：`Content-Type: text/event-stream`、`Cache-Control: no-cache`、`Connection: keep-alive`，
  随后立即 flush（HTTP 200；不是 206/101）。
- 事件块格式：

```
event: turn.committed
id: turn_abc:12
data: {"nodeId":"node_x","branchId":"branch_main_…","branchVersion":8}

```

- 心跳：每 **15 秒** 一行注释 `: ping`（空行结束）。对账轮询每 **1 秒**；服务端单次写
  截止 30 秒（写入会刷新）。
- 断线恢复：请求头 `Last-Event-ID: <aggregateId>:<sequence>`。仅当前缀与聚合 ID 完全
  相等且序号 >0 时才用作游标，从该序号**之后**重放持久事件；否则从头。
  没有其它游标参数（`?cursor=` 不存在）。
- 服务端**不会**在终态自动关闭连接：收到 `turn.committed` 等终态事件后由客户端关闭
  （否则留在心跳上）。会话流/导演流同理。
- 客户端取消 = 断开连接（`r.Context().Done()`），服务端不产生业务副作用（回合取消要显式
  POST cancel）。
- 只有 `turn.thinking` 与 `block.delta` 是"临时事件"（无 `id:`）；其余事件都有持久序号，
  可重放、可去重。

### 8.1 回合事件（`aggregateId = turnId`）

持久事件：

| event | data | 说明 |
| --- | --- | --- |
| `turn.started` | `{"turnId":"…","branchId":"…"}` | 开始准备 |
| `block.appended` | `{"attemptId":"…","frameSeq":N,"frame":{"v":1,"seq":N,"type":"block","kind":"…","speakerId":null|"…","text":"…"}}` | 一个完整正文块已落盘 |
| `turn.cancelled` | `{"turnId":"…","attemptId":"…","code":"","message":"已取消","reason":"已取消","retryable":false}` | 终态 |
| `turn.failed` | `{"turnId":"…","attemptId":"…","code":"PROTOCOL_INVALID","message":"…","reason":"…","retryable":true|false}` | 终态 |
| `turn.conflicted` | `{"turnId":"…","attemptId":"…","code":"HEAD_CONFLICT","message":"…","reason":"…","retryable":false}` | 终态 |
| `turn.truncated` | 同 failed 形状（`code` 多为空，`message`/`reason` 为截断原因） | 进入 `awaiting_continuation` |
| `turn.committed` | `{"nodeId":"…","branchId":"…","branchVersion":8}` | 终态；随后 GET turn/会话视图取权威内容 |

临时事件（无 `id:`，不重放）：

| event | data |
| --- | --- |
| `turn.thinking` | `{"attemptId":"…","thinking":"增量思考文本"}` |
| `block.delta` | `{"attemptId":"…","seq":2,"kind":"dialogue","speakerId":"npc_liel"|null,"delta":"增量"}` |

注意：`turn.state.updated` 只用于内部唤醒订阅者（不落库、不发送给客户端），客户端**不会**
在流里看到它。

### 8.2 会话事件（`aggregateId = sessionId`）

| event | data |
| --- | --- |
| `session.updated` | `{"reason":"summary.created","summaryId":"…","sessionId":"…","branchId":"…","fromNodeId":"…","toNodeId":"…"}` |
| `session.updated` | `{"sessionId":"…","branchId":"…","nodeId":"…","reason":"director"}` |

用途：另一客户端提交了回合、导演命令落地、后台摘要生成。客户端通常只需据此重新拉取
`GET /api/v1/sessions/{id}`；`data.branchId` 与当前激活分支不一致时可忽略。

### 8.3 导演事件（`aggregateId = requestId`）

| event | data |
| --- | --- |
| `director.started` | `{"requestId":"…"}` |
| `director.completed` | `{"requestId":"…"}` |
| `director.failed` | `{"requestId":"…"}` |
| `director.cancelled` | `{"requestId":"…"}` |
| `director.interrupted` | `{"requestId":"…"}`（服务重启时恢复逻辑写入） |

终态后客户端 `GET /api/v1/director-requests/{requestId}` 取 `reply`/`candidate`。

---

## 9. 节点、图谱、世界书

### 9.1 `GET /api/v1/nodes/{nodeId}`

200（`application.NodeView`）：

```json
{
  "node": { "nodeId": "…", "sessionId": "…", "parentId": "…", "kind": "turn",
            "depth": 3, "turnNumber": 3, "schemaVersion": 1,
            "createdAt": "2026-09-01T10:00:00Z", "contentJson": "…" },
  "state": { "…该节点那一刻的世界状态（无快照时为 null）…" },
  "candidates": [ { "nodeId": "…", "kind": "turn", "…": "…" } ]
}
```

- `candidates` 是**同父兄弟回合**（同一次输入的其它候选版本），不含非回合节点。
- 只读，不推进任何分支。
- `kind ∈ {root, turn, configuration_change, memory_change, director_event}`。
- 404 `NOT_FOUND`。

### 9.2 `GET /api/v1/sessions/{id}/graph`

查询参数：

| 参数 | 默认 | 上限 | 说明 |
| --- | --- | --- | --- |
| `nodeId` | 当前查看节点 | — | 子图中心 |
| `up` | 6 | 50 | 向上（父链）层数 |
| `down` | 2 | 10 | 向下（子树）层数 |

200（`application.GraphView`）：

```json
{
  "sessionId": "session_…",
  "targetNodeId": "node_…",
  "nodes": [
    { "nodeId": "node_root", "kind": "root", "depth": 0, "childCount": 1 },
    { "nodeId": "node_2", "parentId": "node_root", "kind": "turn",
      "turnNumber": 2, "depth": 2, "childCount": 1, "isCandidate": true }
  ],
  "truncated": false
}
```

- **没有独立的 edges 数组**：边由每个节点的 `parentId` 隐含；`kind` 为 `root` 的节点没有
  `parentId`。
- 总节点数上限 300；任何原因截断时 `truncated=true`（含目标上方仍有父节点、
  子节点未全部展开等情况），客户端据此提供"加载更多"。
- 错误：404 `NOT_FOUND`、400 `NODE_SESSION_MISMATCH`、503。

### 9.3 `GET /api/v1/sessions/{id}/lorebook`

查询参数：`q`（≤512 字符）、`offset`（0..100000）、`limit`（默认 40，上限 100）。

200：

```json
{
  "entries": [
    {
      "bookIndex": 0,
      "entryIndex": 3,
      "bookName": "世界设定",
      "entry": {
        "entryId": "",
        "title": "黑森林",
        "keys": ["黑森林", "密林"],
        "secondaryKeys": ["夜晚"],
        "content": "……",
        "enabled": true
      }
    }
  ],
  "hasMore": false,
  "nextOffset": 4
}
```

- 只检索会话创建时冻结的世界书模板，未揭示的秘密**不在**本索引（秘密走 SessionView 的
  `secrets`）。
- 错误：400 `INVALID_QUERY`（q 过长/offset 非法）、404 `NOT_FOUND`、503。

---

## 10. 状态与面板数据

`SessionView.state` 是 `domain.WorldState` 的原始 JSON（与具体节点那一刻对应）。
它是所有结构化面板（关系/物品/承诺/心境/目标）的唯一权威来源：

```json
{
  "relationships": {
    "npc_liel": { "affection": 27, "trust": 40, "alertness": 5 }
  },
  "items": {
    "it_1": {
      "instanceId": "it_1", "templateId": "tpl_watch", "name": "旧怀表",
      "aliases": ["怀表"], "ownerId": "player", "locationId": "",
      "quantity": 1, "protection": 0, "keepsake": true, "unique": true
    }
  },
  "promises": {
    "prom_1": {
      "promiseId": "prom_1", "participantIds": ["player", "npc_liel"],
      "content": "……", "sourceNodeId": "node_…", "state": "active",
      "settledByReceiptId": ""
    }
  },
  "moods": { "npc_liel": { "moodCode": "calm", "text": "心如止水" } },
  "goals": { "goal_1": { "text": "找到石板", "characterId": "npc_liel" } },
  "scene": { "sceneId": "scene_1", "locationId": "loc_gate", "title": "古堡大门" },
  "milestones": { "milestone_1": "穿越黑森林" },
  "unlockedSecrets": ["sec_1"],
  "characters": {
    "player": { "characterId": "player", "name": "旅人", "description": "",
                "participant": true },
    "npc_liel": {
      "characterId": "npc_liel", "name": "莉尔", "aliases": ["小莉"],
      "description": "…", "participant": true,
      "attributes": { "perception": 12 },
      "avatar": "data:image/jpeg;base64,… 或 /api/v1/cards/…/assets/…",
      "personality": "…", "scenario": "…", "mes_example": "…",
      "system_prompt": "…", "post_history_instructions": "…",
      "creator_notes": "…", "tags": ["…"], "creator": "…",
      "character_version": "1.0", "nickname": "…"
    }
  },
  "variables": { "weather": "晴" }
}
```

面板映射（与 Web 参考消费一致）：

| 面板 | 字段路径 | 备注 |
| --- | --- | --- |
| 角色列表/档案 | `state.characters[charId]` | 排除 `characterId=="player"`；优先 `participant:true` |
| 关系量表 | `state.relationships[charId]` | `affection ∈ [-100,100]`；`trust/alertness ∈ [0,100]` |
| 心境 | `state.moods[charId]` | `{moodCode, text}` |
| 目标 | `state.goals[goalId]` 或 `view.outline.goals[]` | 后者带 `goalId` |
| 承诺 | `state.promises[promiseId]` | `state ∈ proposed/active/fulfilled/broken/cancelled` |
| 物品/背包 | `state.items[instanceId]` | 玩家背包 = `quantity>0 && ownerId=="player"` |
| 场景 | `state.scene`、`view.outline.scene`、回合 `uiHints(kind=scene).text` | 三处来源，优先级按 UI 设计 |
| 里程碑 | `view.outline.milestones[].description` | 服务端从 `state.milestones` 派生 |
| 秘密 | `view.secrets[]` | 未揭示只给 `title`，内容仅 `revealed` 时下发 |
| 前情摘要 | `view.activeSummary.text` | 当前查看路径上最新摘要 |
| 主角立绘/头像 | `state.characters[charId].avatar` | 可能是 data URL，或 `/api/v1/cards/.../assets/...` 相对路径 |
| 卡内图片资源 | 卡预览 `assets[].url`；`TextBlock.segments[].assetKey` | 资源路由公开 |
| 可执行规则动作 | `view.actions[]` | `{actionId,label,attribute,dc,itemIds?}` |

两个容易踩的点：

1. `attributes` 是**规则属性**（来自规则包），不是人设字段。
2. 状态是"查看位置那一刻"的快照：`viewNodeId` 指向历史节点时，`state` 也是历史值；
   提交写操作前应先回到分支头（`capabilities.canSend` / `viewingHistory`）。

---

## 11. 记忆

路由（`server.go` + `application/memory*.go`）。

| 方法+路径 | 鉴权 | 说明 |
| --- | --- | --- |
| `GET /api/v1/sessions/{id}/memories` | 受保护 | 分页列表（路径内有效性） |
| `PATCH /api/v1/sessions/{id}/branches/{branchId}/memories/{memoryId}` | 受保护 | 修订/置顶/隐藏（CoW） |
| `POST /api/v1/sessions/{id}/branches/{branchId}/memories/organize` | 受保护 | 主动整理 |

### 11.1 `GET /api/v1/sessions/{id}/memories`

查询参数：

| 参数 | 默认 | 约束 |
| --- | --- | --- |
| `branchId` | 第一条分支 | 决定"路径"与覆盖解析 |
| `nodeId` | 分支头 | 不可变检查位置（分页稳定性） |
| `search` | — | 小写子串匹配 `content + entityIds`；≤300 字符 |
| `kind` | `""`（全部生效） | `all` \| `hidden` \| `episodic` \| `inference` \| `observed` \| `reported` \| `inferred` \| `secret` |
| `limit` | 40 | 1..200 |
| `cursor` | — | 上一页的 `nextCursor` |

200（`application.MemoryPage`）：

```json
{
  "memories": [
    {
      "memoryId": "mem_…",
      "sourceNodeId": "node_…",
      "kind": "observed",
      "ownerIds": ["npc_liel"],
      "content": "艾莲娜佩戴着一枚带雄鹰印记的银徽章。",
      "entityIds": ["npc_liel"],
      "confidence": 0.95,
      "importance": 5,
      "pinned": false,
      "supersedes": "",
      "hidden": false,
      "evidence": {
        "sourceNodeId": "node_…",
        "sourceQuote": "……",
        "reasoning": "……",
        "confidence": "observed",
        "autoDowngraded": false
      },
      "mergedFrom": [],
      "secretId": "",
      "subjectKey": "npc_liel.badge",
      "createdTurn": 1,
      "validFromTurn": 1,
      "validUntilTurn": 0,
      "effective": true,
      "protected": false
    }
  ],
  "usage": { "limit": 80, "used": 12, "protected": 1, "headroom": 68,
             "tier": "Normal", "reclaimable": 3 },
  "nodeId": "node_…",
  "nextCursor": "eyJ2Ijox…",
  "total": 12,
  "counts": { "all": 11, "hidden": 1, "episodic": 9, "inference": 1, "secret": 1 }
}
```

- `effective=false` 表示该记录在当前路径上被 CoW 覆盖记录取代（不参与上下文）。
- `protected=true` 表示被系统保护（pinned/secret/与信物或承诺相关），整理时不会被合并。
- `usage.tier ∈ {Normal, Notice, Degraded, Critical}`（由 headroom 推导）。
- 游标语义：`nextCursor` 绑定 `session+branch+node+search+kind` 指纹；换筛选/换节点必须
  重读第一页。错误：`400 BAD_CURSOR`（损坏）、`400 CURSOR_MISMATCH`（不属于当前条件）、
  `409 CURSOR_EXPIRED`（快照已变，重读第一页）、`400 BAD_REQUEST`（limit/搜索超限）、
  404 `NOT_FOUND`。

### 11.2 `PATCH .../memories/{memoryId}`

请求（字段全可选，`null`/缺省 = 不修改）：

```json
{
  "content": "修正后的记忆文本（1..1200 字符）",
  "pinned": true,
  "hidden": false,
  "subjectKey": "npc_liel.status",
  "expectedCharacterId": "card_123",
  "expectedHeadId": "node_…",
  "expectedVersion": 7,
  "idempotencyKey": "mem_1727000000000_1"
}
```

- 这是 **copy-on-write**：服务端在当前分支新建一条覆盖记录（`supersedes` 指向旧版），
  原记录与其它分支不受影响；响应是新记录本体（`MemoryRecord`，无 `effective/protected`
  包装）：

```json
{
  "memoryId": "mem_new", "sourceNodeId": "mem_new", "kind": "observed",
  "content": "修正后的记忆文本", "pinned": true, "hidden": false,
  "supersedes": "mem_old", "subjectKey": "npc_liel.status",
  "createdTurn": 7, "validFromTurn": 7, "validUntilTurn": 0
}
```

- `expectedHeadId`/`expectedVersion` 若提供，则要求头匹配（允许中间夹维护节点）；不匹配
  → `409 HEAD_CONFLICT`。这是可选保护，客户端建议带上。
- `idempotencyKey` 非空时同键同载荷重放返回既有覆盖记录；同键不同载荷
  `409 IDEMPOTENCY_CONFLICT`。
- 错误：404 `MEMORY_NOT_FOUND`（不存在/不可见/不在路径上）、400 `BAD_REQUEST`
  （内容为空或 >1200 字）、500 `MEMORY_CORRUPT`（修订链成环）、503。

### 11.3 `POST .../memories/organize`

请求：`{ "expectedCharacterId": "card_123" }`（归属校验）。

- 语义：仅当记忆用量达到 `Degraded`/`Critical` 时才会规划合并（5 条同类小观察合并为
  1 条 + 5 条隐藏覆盖）；`Normal`/`Notice` 时返回空 merges 且**不写库**。合并不编造内容，
  只做"抽取式"拼接。

200：

```json
{
  "usage": { "limit": 80, "used": 52, "protected": 3, "headroom": 28,
             "tier": "Notice", "reclaimable": 20 },
  "merges": [
    { "sources": ["mem_1","mem_2","mem_3","mem_4","mem_5"],
      "content": "相关见闻整理：……；……" }
  ]
}
```

- 错误：404 `NOT_FOUND`、400 `CHAR_SESSION_MISMATCH`、503。

---

## 12. 导演

路由（`internal/adapters/http/director.go` + `internal/application/director.go`）。
所有导演路由先过 `security()`；若组合根未注入 DirectorService → `503 DIRECTOR_UNAVAILABLE`。
**所有带请求体的导演路由使用严格 JSON 解码**：字段名不认识的未知字段会让整个请求
`400 BAD_REQUEST`；空的请求体同样 `400 BAD_REQUEST`（cancel 也需要一个 JSON 对象，可传 `{}`）。

| 方法+路径 | 鉴权 | 说明 |
| --- | --- | --- |
| `GET /api/v1/sessions/{id}/branches/{branchId}/director` | 受保护+导演 | 工作区 |
| `PUT /api/v1/sessions/{id}/branches/{branchId}/director/draft` | 受保护+导演 | 保存草稿 |
| `POST /api/v1/sessions/{id}/branches/{branchId}/director/messages` | 受保护+导演 | 讨论（异步） |
| `POST /api/v1/sessions/{id}/branches/{branchId}/director/commands` | 受保护+导演 | 大纲命令 |
| `GET /api/v1/director-requests/{requestId}` | 受保护+导演 | 讨论请求状态 |
| `GET /api/v1/director-requests/{requestId}/events` | 受保护+导演 | 讨论 SSE |
| `POST /api/v1/director-requests/{requestId}/cancel` | 受保护+导演 | 停止讨论 |

### 12.1 `GET .../director?viewNodeId=`

- 200（`application.DirectorView`）：

```json
{
  "state": {
    "plan": {
      "planId": "node_…",
      "revisionId": "node_…",
      "title": "第一幕：启程",
      "guidance": "……",
      "beats": [
        { "beatId": "beat_1", "title": "初遇", "instruction": "……",
          "completionCriteria": "莉尔已与玩家交手" }
      ]
    },
    "status": "active",
    "currentBeatId": "beat_1",
    "progress": {
      "beat_0": { "status": "completed", "sourceNodeId": "node_…",
                  "evidence": [ { "blockSeq": 2, "quote": "……" } ] }
    },
    "warning": ""
  },
  "draft": {
    "sessionId": "session_…", "branchId": "branch_…", "version": 4,
    "baseRevisionId": "node_…",
    "plan": { "planId": "", "revisionId": "", "title": "…",
              "guidance": "…", "beats": [ "…" ] },
    "updatedAt": "2026-09-20T08:00:00Z"
  },
  "requests": [ { "…": "DirectorRequest，见 §12.4，最早在前" } ],
  "readOnly": false,
  "viewNodeId": "node_…"
}
```

- `state` 为 `null` 表示尚无活动大纲；`draft` 为 `null` 表示还没有草稿。
- `requests` 最多 50 条（升序），在 `readOnly=true` 时恒为 `[]`。
- `readOnly=true` 当 `viewNodeId != branch.headNodeId`（历史视图只读）。
- 错误：404 `NOT_FOUND`、400 `BRANCH_SESSION_MISMATCH`、503。

### 12.2 `PUT .../director/draft`

请求（严格）：

```json
{
  "expectedCharacterId": "card_123",
  "expectedDraftVersion": 4,
  "baseRevisionId": "node_…",
  "viewNodeId": "",
  "plan": {
    "planId": "",
    "revisionId": "",
    "title": "第一幕：启程",
    "guidance": "整体保持克制的叙事风格",
    "beats": [
      { "beatId": "beat_1", "title": "初遇", "instruction": "让莉尔先开口",
        "completionCriteria": "莉尔已与玩家交手" }
    ]
  }
}
```

- 草稿阶段允许标题/阶段为空（校验只查长度与 ID 唯一）。
- 校验上限：总 title ≤200 字、guidance ≤4000 字、beats ≤32 个；每 beat `beatId` 非空、
  ≤100 字符且唯一；`title` ≤200、`instruction` ≤4000、`completionCriteria` ≤2000。
  违反 → `422 DIRECTOR_INVALID`。
- `expectedDraftVersion` 与当前草稿版本不一致 → `409 DRAFT_CONFLICT`。
- `viewNodeId` 非空且不等于分支头 → `409 HEAD_CONFLICT`（历史视图只读）。
- 200：新建/更新后的 `DirectorDraft`（同 §12.1 的 `draft` 形状）。

### 12.3 `POST .../director/commands`

请求（严格）：`idempotencyKey` 必填（≤200 字符）：

```json
{
  "expectedCharacterId": "card_123",
  "expectedHeadId": "node_…",
  "expectedVersion": 7,
  "idempotencyKey": "cmd_1727000000000_1",
  "action": "complete",
  "beatId": "beat_1",
  "draftVersion": 4,
  "replace": false
}
```

| `action` | 必需字段 | 语义 |
| --- | --- | --- |
| `activate` | `draftVersion`（必须等于当前草稿版本）；`replace` 可选 | 用草稿启用大纲；从首个尚未完成的 beat 开始。已有活动大纲且 `replace=false`：保留已完成/跳过的阶段与当前阶段位置（一旦改动已完成阶段 → `422 DIRECTOR_INVALID`）；`replace=true` 整体替换并从首阶段重新开始 |
| `pause` | — | 仅 `active` 可暂停 |
| `resume` | — | 仅 `paused` 可恢复 |
| `complete` | `beatId`（必须等于当前阶段） | 标记当前阶段完成 |
| `skip` | `beatId`（必须等于当前阶段） | 跳过当前阶段 |
| `rewind` | `beatId`（必须是已完成/跳过的阶段） | 回退：删除该阶段及其之后的进度，把它设为当前阶段 |

- 头匹配：`expectedHeadId`/`expectedVersion` 与分支头严格相等（维护节点容忍同 §7.2），
  不等 → `409 HEAD_CONFLICT`；分支有在途回合 → `409 TURN_BUSY`。
- 幂等：同 `idempotencyKey` 同载荷重放返回同一结果；不同载荷 → `409 IDEMPOTENCY_CONFLICT`。
- 激活时若草稿版本对不上 → `409 DRAFT_CONFLICT`。
- 命令会作为 `director_event` 节点写进故事树（增加分支版本，不增加回合数），并向会话
  aggregate 广播 `session.updated`。
- 200：

```json
{
  "nodeId": "node_dir_…",
  "version": 8,
  "state": { "…DirectorState（同 §12.1）…" }
}
```

- 未知 `action` → `400 BAD_REQUEST`；`DIRECTOR_INVALID` 422。

### 12.4 `POST .../director/messages` 与讨论请求

请求（严格）：

```json
{
  "expectedCharacterId": "card_123",
  "expectedHeadId": "node_…",
  "expectedDraftVersion": 4,
  "idempotencyKey": "dir_1727000000000_1",
  "text": "让第二幕提前一点，先让莉尔暴露身份"
}
```

- `text` trim 后非空且 ≤32000 **字节**；`idempotencyKey` 非空 ≤200；否则 400。
- 服务端异步调用 `assist` 槽位模型（最长 3 分钟）；任何 `director.*` 事件后建议
  `GET /api/v1/director-requests/{requestId}` 对账。
- 讨论成功且模型给出完整 plan 时，**若该请求期间分支头与草稿版本都没变**，会自动写回
  草稿（`draftApplied=true`，草稿 version +1）；否则仅返回 candidate 由用户手动保存。
- 请求体（严格语义）与响应：

响应 **202**：

```json
{
  "request": {
    "requestId": "req_…",
    "sessionId": "session_…",
    "branchId": "branch_…",
    "baseNodeId": "node_…",
    "draftVersion": 4,
    "text": "让第二幕提前一点……",
    "status": "generating",
    "reply": "",
    "candidate": null,
    "draftApplied": false,
    "error": "",
    "createdAt": "2026-09-20T08:00:00Z"
  },
  "requestId": "req_…",
  "status": "generating",
  "statusUrl": "/api/v1/director-requests/req_…",
  "eventsUrl": "/api/v1/director-requests/req_…/events"
}
```

`DirectorRequest.status ∈ generating | completed |
failed | cancelled | interrupted`。错误：`409 HEAD_CONFLICT`（提交时分支头已变，或创建时
与当前 head 不符）、`409 DRAFT_CONFLICT`（草稿版本已变）、`409 DIRECTOR_BUSY`（同一分支
已有讨论在生成）、`409 IDEMPOTENCY_CONFLICT`、`400 BAD_REQUEST`（缺 key/文本超长）、
404（分支/session 不存在）、503 `DIRECTOR_UNAVAILABLE`/`SERVER_SHUTTING_DOWN`。
讨论失败原因写在 `request.error`，`status="failed"`；服务重启时未完成的请求被标为
`interrupted`。

### 12.5 `GET /api/v1/director-requests/{requestId}`

- 200：`DirectorRequest` 本体（同 §12.4 的 `request`）。
- 404 `NOT_FOUND`。

### 12.6 `POST /api/v1/director-requests/{requestId}/cancel`

- 严格请求体：`{ "expectedCharacterId": "card_123" }`。
- 200：`DirectorRequest`，`status="cancelled"`，`error="已停止讨论，草稿已保留"`。
- 幂等：已终态时原样返回。
- 404 `NOT_FOUND`；400 `CHAR_SESSION_MISMATCH`。

---

## 13. 模型配置

路由（`internal/adapters/http/models.go` + `application/providers.go` + `internal/ports/providerconfig.go`）。
服务未注入 ProviderManager 时全部 `501 NOT_IMPLEMENTED`。

| 方法+路径 | 鉴权 | 说明 |
| --- | --- | --- |
| `GET /api/v1/config/models` | 受保护 | 实例列表 + 槽位（密钥脱敏） |
| `POST /api/v1/config/models` | 受保护 | 新建实例 |
| `PUT /api/v1/config/models/{modelId}` | 受保护 | 更新实例（路径 ID 优先） |
| `DELETE /api/v1/config/models/{modelId}` | 受保护 | 删除（被引用则 409） |
| `POST /api/v1/config/models/{modelId}/probe` | 受保护 | 实例连通/格式探测 |
| `GET /api/v1/config/provider` | 受保护 | 三槽位视图 |
| `PUT /api/v1/config/provider` | 受保护 | 槽位指派（只接受引用） |
| `POST /api/v1/config/provider/probe` | 受保护 | 槽位探测 |

### 13.1 数据形状

`ModelInstance`（`ports.ModelInstance`）：

```json
{
  "id": "m_…",
  "name": "DeepSeek 官方",
  "kind": "openai-chat",
  "baseUrl": "https://api.deepseek.com",
  "model": "deepseek-chat",
  "apiKey": "",              // 只写；读取时为脱敏值
  "hasApiKey": true,
  "temperature": 0.8,
  "maxTokens": 4096,
  "contextWindow": 131072,
  "reasoningEffort": "",
  "inputBudget": 122368       // 计算字段（服务端算好），窗口未配置时省略
}
```

- `kind ∈ {openai-chat, openai-responses, anthropic-messages}`。
- `reasoningEffort ∈ {"", low, medium, high}`（空 = 不发送思考参数）。
- `inputBudget = max(0, contextWindow − maxTokens − max(512, contextWindow/20))`。

`SlotBinding`（槽位解析视图）：

```json
{
  "slot": "primary",
  "enabled": true,
  "modelId": "m_…",            // 保存的引用；空 = 跟随 primary
  "resolvedModelId": "m_…",    // 实际生效的实例（空引用时指 primary）
  "model": "deepseek-chat",
  "kind": "openai-chat",
  "baseUrl": "https://api.deepseek.com",
  "apiKey": "sk-1…abcd",       // 脱敏
  "hasApiKey": true,
  "temperature": 0.8,
  "maxTokens": 4096,
  "contextWindow": 131072,
  "reasoningEffort": ""
}
```

三个槽位固定为（顺序即界面顺序）：`primary`（对话生成）、`assist`（导演协商）、
`reflection`（后台记忆/摘要）。`GET /api/v1/config/models` 一次返回完整目录：

```json
{ "models": [ "ModelInstance…" ], "slots": [ "SlotBinding…" ] }
```

### 13.2 写入

`POST /api/v1/config/models` / `PUT /api/v1/config/models/{modelId}`：请求体是
`ModelInstance`（`id` 在 PUT 时以路径为准）。校验（`validateInstance`）：

- `name` 非空 → 否则 `422 PROVIDER_BAD_CONFIG`；
- `model` 非空 → 否则 `422 MISSING_MODEL`；
- `baseUrl` 若非空必须是 http(s)、有 host、无 userinfo → 否则 422；
- `contextWindow` ∈ [0, 2_000_000] 且 `contextWindow − maxTokens ≥ 2048`（窗口 >0 时）
  → 否则 422；
- `reasoningEffort` 归一化失败 → 422。

密钥处理：`apiKey` 为空串或恰好等于服务端之前返回的脱敏值（如 `sk-1…abcd`）时**保留原密钥**；
否则替换。响应 200：保存后的实例（密钥脱敏 + `inputBudget`）。

`PUT /api/v1/config/provider`：

```json
{ "slot": "assist", "enabled": true, "modelId": "m_…" }
```

- 只接受这三个字段；出现 `kind`/`baseUrl`/`model`/`apiKey` 任一 →
  `422 SLOT_TAKES_REFERENCE_ONLY`；slot 非法 → `422 BAD_SLOT`。
- 200：`{ "ok": true, "providers": [ SlotBinding… ] }`。`modelId` 传空串 = 该槽位跟随主线。

`DELETE /api/v1/config/models/{modelId}`：200 `{"ok":true}`；被槽位引用 →
`409 MODEL_IN_USE`（message 列出占用的槽位）；不存在 → `404 MODEL_NOT_FOUND`。

### 13.3 探测

`POST /api/v1/config/models/{modelId}/probe`：请求体**可省略**；
若 `Content-Length > 0` 则解析 `{ "format": false }`（`slot`/`modelId` 字段被接受但忽略）。

`POST /api/v1/config/provider/probe`：请求体必填，非严格解析：

```json
{ "slot": "primary", "format": true }
```

（`modelId` 字段存在但当前实现忽略；slot 非法 → `422 BAD_SLOT`。）

响应 200（`application.ProbeResult`）：

```json
{
  "slot": "probe",            // 实例探测固定 "probe"；槽位探测回显槽位名
  "ok": true,
  "kind": "openai-chat",
  "model": "deepseek-chat",
  "connectMsg": "",
  "formatTested": true,
  "formatOk": true,
  "formatMsg": "帧协议校验通过",
  "latencyMs": 812
}
```

- 槽位未启用/未配置：仍 200，`ok=false` 且 `connectMsg` 说明原因（不报错）。
- 实例不存在：404 `MODEL_NOT_FOUND`。
- 探测**不落库、不改配置**；30 秒超时。`format=false` 只做一次"回复 OK"的连通性检查；
  `format=true` 额外校验模型能否稳定输出帧协议。

---

## 14. TTS / 图像 / 向量

### 14.1 TTS

`GET /api/v1/tts/voices`（**公开**）→ 200：

```json
{
  "voices": [
    { "id": "zh-CN-XiaoxiaoNeural", "name": "晓晓 (温柔女声)", "gender": "female",
      "locale": "zh-CN", "description": "温暖亲切、抑扬顿挫，极适于对白与细腻情感演绎" },
    { "id": "zh-CN-YunxiNeural", "name": "云希 (少年/青年男声)", "gender": "male", "locale": "zh-CN", "description": "…" },
    { "id": "zh-CN-YunjianNeural", "name": "云健 (沉稳旁白男声)", "gender": "male", "locale": "zh-CN", "description": "…" },
    { "id": "zh-CN-XiaoyiNeural", "name": "晓伊 (抒情女声)", "gender": "female", "locale": "zh-CN", "description": "…" },
    { "id": "en-US-AvaMultilingualNeural", "name": "Ava (多语言英文女声)", "gender": "female", "locale": "en-US", "description": "…" },
    { "id": "en-US-AndrewMultilingualNeural", "name": "Andrew (多语言英文男声)", "gender": "male", "locale": "en-US", "description": "…" }
  ]
}
```

`POST /api/v1/tts/synthesize`（受保护）：

```json
{
  "voice": "zh-CN-XiaoxiaoNeural",
  "text": "要合成的文本（≤1000 字）",
  "instruction": "（可选）导演指示，仅部分引擎使用",
  "engine": "edge",
  "baseUrl": "http://127.0.0.1:9880/v1",
  "apiKey": "…",
  "model": "tts-1",
  "speed": 1.0
}
```

- 200：**原始音频字节**，`Content-Type: audio/mpeg`，
  `Cache-Control: public, max-age=86400`（不是 JSON）。
- `engine` 解析：空串或 `"web-speech"` → 有 `baseUrl` 时按 `openai`，否则 `edge`；
  显式 `mimo`（或 `model` 含 "mimo"、`baseUrl` 含 "xiaomimimo"）→ MiMo；
  其余按 `openai`/`edge`。`voice` 空时默认 `alloy`（openai）或 `zh-CN-XiaoxiaoNeural`
  （edge），且 edge 引擎下不含 `-` 的音色名会被回退为默认中文音色。
- `speed <= 0` 时按 1.0。进程内缓存键 = `engine:model:voice:%.2f:speed:baseUrl:instruction:text`
  的 md5；缓存条数 ≥500 或总量 >64 MiB 时整体清空。
- 错误：400 `BAD_REQUEST`（text 空）、400 `TEXT_TOO_LONG`（>1000 字）、
  503 `TTS_UNAVAILABLE`（合成失败，`retryable=true`）。
- 合成失败的原始错误文本会拼进 503 的 message。

`POST /api/v1/tts/plan`（受保护；`TTSPlans` 未注入 → 501）：

```json
{
  "sessionId": "session_…",
  "branchId": "branch_…",
  "nodeId": "node_…",
  "blockIndexes": [1, 2],
  "mode": "dialogue",
  "director": false,
  "engine": "edge",
  "voice": "zh-CN-XiaoxiaoNeural",
  "baseUrl": "",
  "speed": 1.0
}
```

200：

```json
{
  "segments": [
    {
      "segmentId": "seg_ab12cd34ef56",
      "speakerId": "npc_liel",
      "speakerName": "莉尔",
      "text": "（剥离标签后的）台词文本",
      "emotion": "警惕",
      "contextSummary": "场景：古堡大门 · 莉尔 · 心境：calm 心如止水",
      "cacheKey": "sha256:…"
    }
  ],
  "voiceHint": { "presetId": "zh-CN-XiaoxiaoNeural", "speed": 1.0 }
}
```

- `mode` 只认 `full`，其它值（含空）一律按 `dialogue`。
- `director=true` 时注入导演标签/指令；非导演模式剥离音频标签。
- 上下文缺失（无卡/无状态）降级为"中性演播"，不报错。
- 错误：400 `BAD_REQUEST`（缺 nodeId）、404 `NOT_FOUND`、400 `NODE_SESSION_MISMATCH`。

### 14.2 图像

`POST /api/v1/images/plan`（受保护；未注入 → 501）：

```json
{
  "sessionId": "session_…",
  "branchId": "branch_…",
  "nodeId": "node_…",
  "blockIndexes": [1, 2],
  "stylePreset": "anime",
  "characterName": ""
}
```

200：

```json
{
  "prompt": "masterpiece, 1girl, ……",
  "negativePrompt": "lowres, bad anatomy, ……",
  "sdDataBase": { "prompts": ["…卡片自带模板…"], "text": "…拼接后的基底…" },
  "cardId": "card_123",
  "refs": [ { "assetKey": "asset_3" } ]
}
```

- `stylePreset ∈ {anime（默认）, fantasy, cinematic, realistic}`。
- 错误：400 `BAD_REQUEST`（缺 nodeId）、404 `NOT_FOUND`、400 `NODE_SESSION_MISMATCH`。

`POST /api/v1/images/generate`（受保护）：

```json
{
  "engine": "comfyui",
  "prompt": "……",
  "negativePrompt": "……",
  "size": "1024x576",
  "baseUrl": "http://127.0.0.1:8188",
  "apiKey": "",
  "model": "image_qwen_image_2_1_t2i",
  "quality": "",
  "style": ""
}
```

- `engine ∈ {openai, sd-webui, comfyui}`；留空时按 baseUrl/model 关键字推断
  （8188/qwen→comfyui、7860/sdapi→sd-webui、openai/siliconflow→openai，默认 comfyui）。
- 200：

```json
{
  "id": "img_1727000000_ab12cd34ef56",
  "url": "/api/v1/images/img_1727000000_ab12cd34ef56.png",
  "prompt": "……",
  "mimeType": "image/png",
  "createdAt": "2026-09-20T08:00:00Z"
}
```

- 错误：400 `BAD_REQUEST`（prompt 空）、503 `IMAGE_GENERATION_FAILED`（含上游错误文本）、
  503 `IMAGE_SERVICE_UNAVAILABLE`（服务初始化失败）。
- `GET /api/v1/images/{filename}`（**公开**）：返回图片字节，`Cache-Control: public,
  max-age=31536000, immutable`；路径穿越/不存在 → 404。文件名即 URL 最后一段。

### 14.3 向量（embeddings，简要）

`POST /api/v1/embeddings/probe`（受保护）：

```json
{ "baseUrl": "http://127.0.0.1:11434/v1", "apiKey": "", "model": "bge-small-zh-v1.5",
  "sampleText": "" }
```

- 响应**恒为 200**（即便探测失败），`embedding.ProbeResult`：

```json
{ "ok": true, "model": "bge-small-zh-v1.5", "dimension": 512, "latencyMs": 33,
  "preview": [0.01, -0.02, 0.0, 0.03, 0.11], "error": "" }
```

- `model` 空时默认 `bge-small-zh-v1.5`；`preview` 最多 5 维。
- 请求体解析失败仍是 400 `BAD_REQUEST`。

`POST /api/v1/embeddings`（受保护）：`{texts:[…], baseUrl?, apiKey?, model?, dimension?}`。

- `texts` 1..128 段；200：`{ "model": "…", "dimension": 512, "embeddings": [[…], …] }`。
- 错误：400 `BAD_REQUEST`（空）、400 `TOO_MANY_TEXTS`（>128）、400 `PROVIDER_INIT_FAILED`、
  503 `EMBEDDING_FAILED`。

---

## 15. 展示目录

`GET /api/v1/catalog`（受保护）。进程内常量 + 内容哈希版本：

- 响应头 `ETag: "<version>"`、`Cache-Control: private, max-age=300`；
  请求头 `If-None-Match` 包含该 version 时返回 **304**（无 body）。
- 200 body（`application.Catalog`；除 `version` 为运行时的内容哈希外，其余取值即当前实现）：

```json
{
  "version": "6f3a1c9d2b4e8a70",
  "cardMarkers": {
    "grammar": 1,
    "stateKinds": ["hud", "scene", "var"],
    "imageForms": ["<img cmd=\"key\">", "[IMG:key]"],
    "escapeHint": "[[ 表示一个字面量 [",
    "stripNotice": "状态类标记（HUD/SCENE/VAR）由服务端剥离并结构化下发（uiHints），表现类标记（图片）由服务端切段下发（segments）：客户端不解析语法。"
  },
  "attributes": [
    { "key": "strength", "name": "力量", "abbr": "STR" },
    { "key": "str", "name": "力量", "abbr": "STR" },
    { "key": "dexterity", "name": "敏捷", "abbr": "DEX" },
    { "key": "dex", "name": "敏捷", "abbr": "DEX" },
    { "key": "constitution", "name": "体质", "abbr": "CON" },
    { "key": "con", "name": "体质", "abbr": "CON" },
    { "key": "intelligence", "name": "智力", "abbr": "INT" },
    { "key": "int", "name": "智力", "abbr": "INT" },
    { "key": "wisdom", "name": "察觉", "abbr": "WIS" },
    { "key": "wis", "name": "察觉", "abbr": "WIS" },
    { "key": "perception", "name": "感知", "abbr": "PER" },
    { "key": "per", "name": "感知", "abbr": "PER" },
    { "key": "charisma", "name": "交涉", "abbr": "CHA" },
    { "key": "cha", "name": "交涉", "abbr": "CHA" },
    { "key": "luck", "name": "运气", "abbr": "LCK" },
    { "key": "lck", "name": "运气", "abbr": "LCK" },
    { "key": "agility", "name": "身手", "abbr": "AGI" },
    { "key": "agi", "name": "身手", "abbr": "AGI" }
  ],
  "checkOutcomes": [
    { "key": "critical_success", "label": "大成功", "tone": "crit-up", "symbol": "✦", "isSuccess": true, "isCrit": true },
    { "key": "success", "label": "成功", "tone": "up", "symbol": "✓", "isSuccess": true },
    { "key": "critical_failure", "label": "大失败", "tone": "crit-down", "symbol": "✕", "isCrit": true },
    { "key": "failure", "label": "失败", "tone": "down", "symbol": "✕" }
  ],
  "protocols": [
    { "value": "openai-chat", "label": "OpenAI 兼容" },
    { "value": "openai-responses", "label": "OpenAI Responses" },
    { "value": "anthropic-messages", "label": "Anthropic" }
  ],
  "defaultProtocol": "openai-chat",
  "thinkingEfforts": [
    { "value": "", "label": "默认", "hint": "不发送思考参数，完全按供应商自己的默认行为" },
    { "value": "low", "label": "低", "hint": "思考最少，响应最快" },
    { "value": "medium", "label": "中", "hint": "速度与深度的折中" },
    { "value": "high", "label": "高", "hint": "思考最充分，响应最慢" }
  ],
  "slots": [
    { "key": "primary", "label": "对话生成", "hint": "推进剧情时使用" },
    { "key": "assist", "label": "导演协商", "hint": "导演模式下与模型协商大纲" },
    { "key": "reflection", "label": "后台记忆", "hint": "后台抽取记忆、维护摘要" }
  ],
  "providerPresets": [
    { "id": "deepseek", "label": "DeepSeek 官方", "kind": "openai-chat", "baseUrl": "https://api.deepseek.com", "models": ["deepseek-chat", "deepseek-reasoner"], "keyHint": "在 platform.deepseek.com 申请" },
    { "id": "openai", "label": "OpenAI 官方", "kind": "openai-responses", "baseUrl": "https://api.openai.com/v1", "models": ["gpt-4o-mini", "gpt-4o"], "keyHint": "在 platform.openai.com 申请" },
    { "id": "anthropic", "label": "Anthropic（Claude）", "kind": "anthropic-messages", "baseUrl": "https://api.anthropic.com", "models": ["claude-sonnet-4-5", "claude-haiku-4-5"], "keyHint": "在 console.anthropic.com 申请" },
    { "id": "ollama", "label": "本地 Ollama", "kind": "openai-chat", "baseUrl": "http://127.0.0.1:11434/v1", "models": ["qwen3:8b", "llama3.1:8b"], "keyHint": "本地服务，无需密钥" },
    { "id": "oneapi", "label": "本地 OneAPI / NewAPI", "kind": "openai-chat", "baseUrl": "http://127.0.0.1:3000/v1", "models": [], "keyHint": "填网关里生成的令牌" },
    { "id": "custom", "label": "自定义（手动填写）", "kind": "openai-chat", "baseUrl": "", "models": [], "isCustom": true }
  ],
  "contextPolicy": {
    "defaultWindow": 8192,
    "defaultReservedOutput": 2048,
    "minMargin": 512,
    "marginDivisor": 20,
    "reservedOutputHint": 2048
  }
}
```

目录只承载**数据**（枚举与标签），不承载行为；客户端不要另存一份会分叉的枚举。
错误：503 `CATALOG_UNAVAILABLE`（版本为空，理论不会发生）。

---

## 16. 附录

### 16.1 错误码总表

按功能分组（状态码为显式传入值，与 §3 默认表一致或覆盖之）：

**鉴权 / 通用**

| code | 状态 | 出现位置 |
| --- | --- | --- |
| `AUTH_REQUIRED` | 401 | 受保护路由，局域网未配对 |
| `FORBIDDEN` | 403 | Host 不在私网/回环、跨来源写、pair 来源不支持 |
| `INVALID_PIN` | 401 | pair |
| `TOO_MANY_ATTEMPTS` | 429 | pair（带 `Retry-After`） |
| `BAD_REQUEST` | 400 | 请求体非法/尾随内容/参数非法 |
| `REQUEST_TOO_LARGE` | 413 | JSON 超限、卡片上传超限 |
| `NOT_FOUND` | 404 | 会话/分支/节点/回合/记忆/请求不存在 |
| `NOT_IMPLEMENTED` | 501 | 可选服务未注入（记忆 501、模型配置 501、计划服务 501） |
| `INTERNAL` | 500 | 未知异常/panic 兜底 |
| `SHUTTING_DOWN` | 503 | 关停排水期 |
| `SSE_UNSUPPORTED` | 500 | 连接不支持流式 |
| `INVALID_QUERY` | 400 | 世界书分页参数非法 |
| `CATALOG_UNAVAILABLE` | 503 | 目录版本为空 |

**角色卡**

`CARD_INVALID`(422)、`CARD_TOO_LARGE`(422)、`CARD_LIBRARY_FULL`(409)、
`CARD_NOT_FOUND`(404)、`STORAGE_UNAVAILABLE`(503)。

**会话 / 剧情包 / 分支**

`CARD_MISSING`(422)、`CARD_NO_OPENING`(422)、`CARD_VARIANT_NOT_FOUND`(422)、
`IDEMPOTENCY_CONFLICT`(409)、`SESSION_BUSY`(409)、`CHAR_SESSION_MISMATCH`(400)、
`BRANCH_SESSION_MISMATCH`(400)、`BRANCH_NOT_FOUND`(404)、`NODE_NOT_FOUND`(404)、
`NODE_SESSION_MISMATCH`(400)、`INVALID_CURSOR`(400)、`STATE_CORRUPT`(500)、
`STATE_MISSING`(500/422)、`NODE_REQUIRED`(400)、`CANNOT_DERIVE_ROOT`(400)、
`NODE_NOT_TURN`(400)、`NODE_CONTENT_INVALID`(500)、`UNSUPPORTED_RULESET`(409)、
`PACK_TOO_LARGE`(413)、`PACK_UNSAFE`(400)、`INVALID_PACK`(400)、`EXPORT_FAILED`(500)。

**回合**

`MISSING_IDEMPOTENCY_KEY`(422)、`HEAD_CONFLICT`(409, retryable)、`STALE_OPTION`(409, retryable)、
`QUEUE_FULL`(429)、`NOTE_TOO_LONG`(400)、`ACTION_REQUIRED`(400)、`CHECK_REQUIRED`(400)、
`INPUT_REQUIRED`(400)、`BLOCKS_REQUIRED`(400)、`EDIT_AMBIGUOUS`(400)、`EDIT_EMPTY`(400)、
`TURN_NOT_CONTINUABLE`(409)、`TURN_CANCELLED`(409)、`TURN_NOT_ACTIVE`(409)、
`COMMIT_CONFLICT`(409)、`PROVIDER_NOT_CONFIGURED`(400)、`PROVIDER_UNAVAILABLE`(502/终态码)、
`PROTOCOL_INVALID`(422)、`OPERATION_REJECTED`(422)、`BUDGET_EXCEEDED`(507)、
`CONTEXT_OVER_BUDGET`(422)、`ACTION_UNAUTHORIZED`(403)、`ACTION_PRECONDITION`(409)、
`RULE_INVALID`(409/422/500)、`SERVER_SHUTTING_DOWN`(503)。

回合**终态失败码**（出现在 `GET /turns/{id}.failureCode` 与 `turn.failed` 事件里，
不一定作为 HTTP 状态返回）：`STATE_MISSING`、`STATE_CORRUPT`、`STORAGE_UNAVAILABLE`、
`PROVIDER_UNAVAILABLE`、`PROVIDER_EMPTY_OUTPUT`、`PROVIDER_TRUNCATED`、`PROTOCOL_INVALID`、
`CONTEXT_OVER_BUDGET`、`CONTEXT_FAILED`、`OPERATION_REJECTED`、`RULE_INVALID`、
`PROCESS_INTERRUPTED`、`SERVER_SHUTTING_DOWN`、`QUEUE_FULL`、`CHECK_FAILED`、`UNKNOWN`。

**记忆**

`BAD_CURSOR`(400)、`CURSOR_MISMATCH`(400)、`CURSOR_EXPIRED`(409)、`MEMORY_NOT_FOUND`(404)、
`MEMORY_CORRUPT`(500)。

**导演**

`DIRECTOR_UNAVAILABLE`(503)、`DIRECTOR_INVALID`(422)、`DIRECTOR_FORMAT`(422)、
`DIRECTOR_POLICY_VIOLATION`(422)、`DIRECTOR_OUTPUT_LIMIT`(422)、`DRAFT_CONFLICT`(409)、
`TURN_BUSY`(409)、`DIRECTOR_BUSY`(409)、`IDEMPOTENCY_CONFLICT`(409)、`HEAD_CONFLICT`(409)。

**模型配置 / TTS / 图像 / 向量**

`BAD_SLOT`(422)、`SLOT_TAKES_REFERENCE_ONLY`(422)、`MODEL_IN_USE`(409)、
`MODEL_NOT_FOUND`(404)、`PROVIDER_BAD_CONFIG`(422)、`MISSING_MODEL`(422)、
`TEXT_TOO_LONG`(400)、`TTS_UNAVAILABLE`(503)、`IMAGE_SERVICE_UNAVAILABLE`(503)、
`IMAGE_GENERATION_FAILED`(503)、`TOO_MANY_TEXTS`(400)、`PROVIDER_INIT_FAILED`(400)、
`EMBEDDING_FAILED`(503)。

### 16.2 并发、锁与幂等语义

1. **分支单飞**：每条分支同时最多一个未终态回合（`branches.active_turn_id`）。
   第二个受理请求 → `429 QUEUE_FULL`，同时服务端把该请求落成一条 `failed/QUEUE_FULL`
   的回合占住幂等键——客户端应换新键重试。
2. **乐观并发（回合）**：`expectedHeadId` + `expectedVersion`；允许中间夹
   `memory_change`/`director_event` 维护节点，遇到对话节点即 `409 HEAD_CONFLICT`
   （`retryable=true`）。受理后基准固定在服务端真实分支头上。
3. **乐观并发（记忆修订/导演命令）**：同款头匹配；导演草稿另用
   `expectedDraftVersion`（`409 DRAFT_CONFLICT`）。
4. **幂等键**：
   - 回合受理：**必填**；`input+mode+基准+派生参数` 的哈希绑定。
   - 重生成/编辑输入：可选（缺省服务端自生成，等于无幂等）。
   - 记忆修订：可选；键绑定在 `(session, branch, key) + payloadHash`。
   - 创建会话：可选；`session_<sha256(key)>` 确定性 ID。
   - 导演命令/讨论：必填；相对 `(session, key)` 唯一。
5. **权威终态**：HTTP 202 只表示"已受理"；提交结果以 `GET /turns/{id}` 或
   `turn.committed` 事件为准。SSE 的持久事件可回放去重（`id: aggregate:seq` 单调）。
6. **CoW**：记忆修订/整理不修改历史记录，只在当前路径新建覆盖记录；因此不同分支
   互不影响。
7. **非幂等操作**：分叉、导出、导入、卡片导入（按内容哈希 upsert，等效幂等但会刷新
   `updatedAt`）、图片/TTS 合成（各自有缓存但不保证同 id）。
8. **可重试性建议**：`retryable=true` 的错误（HEAD_CONFLICT、STALE_OPTION、
   PROVIDER_UNAVAILABLE、BUDGET_EXCEEDED、TTS/图像/向量失败、503）可原样重试；
   409 `IDEMPOTENCY_CONFLICT` 与 4xx 校验失败必须改载荷/换键。

### 16.3 端到端最短路径

```
GET  /healthz                                   # 探活（公开）
GET  /api/v1/auth/status                        # 判断是否需要配对
POST /api/v1/auth/pair {"pin"}                  # 拿 token（持久化）
GET  /api/v1/catalog                            # 枚举/标签（可选，带 ETag 缓存）
POST /api/v1/cards        (multipart/raw)       # 导入角色卡 → cardId
POST /api/v1/sessions     {cardId, playerName}  # → sessionId/branchId/rootNodeId/state
GET  /api/v1/sessions/{id}                      # SessionView（分支头/节点/状态/能力）
POST /api/v1/sessions/{id}/branches/{bid}/turns # → turnId（202）
GET  /api/v1/turns/{tid}/events                 # SSE：block.delta → … → turn.committed
GET  /api/v1/sessions/{id}                      # 用权威视图刷新故事/状态/选项
```

Android 特别注意：
- 连接本机/局域网都用 IP 直连（`http://192.168.x.x:8890`），不要用域名，否则受保护
  路由 403。
- 所有请求（含 SSE 与二进制上传/下载）都带 `Authorization: Bearer <token>`。
- 服务端重启后 token 失效，收到 401 `AUTH_REQUIRED` 就回到配对界面。


