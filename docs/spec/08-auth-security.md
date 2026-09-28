# 08 鉴权与安全基线

> 决策：**保持宽松** —— 不新增 CSRF/重放保护等加固，但**现有防线不得削弱**，且安全边界测试必须常绿（[`13-testing-gates.md`](13-testing-gates.md)）。

## 1. 防线总览

```
requestDrain → withLogging → apiHostGuard(/api/ Host 校验) → per-route:
  security   = Host + (非本机且启用 PIN → token) + (写请求带 Origin → 同源)
  hostOnly   = 仅 Host（用于浏览器无法带 token 的资源读取）
```

### Host 防线（`apiHostGuard`）

- 作用于**所有 `/api/` 路径**，与 token 无关；Host 必须是 loopback 或私有网段 IP/主机名，否则 `403 FORBIDDEN`。
- `/healthz` 与静态资源豁免。
- 目的：DNS rebinding 防护。

### token 鉴权（`security`）

- **来源**：`-pin` 提供配对码；若设置了 PIN 且未显式提供 token，服务端自动生成 24 字节 hex token（`generateRandomToken`）。
- **豁免**：`RemoteAddr` 为 **loopback 的对端免 token**（身份来自 TCP 对端，**绝不信任 `X-Forwarded-For`**）。
- **非 loopback + 已启用 PIN**：必须携带合法 token，否则 `401 AUTH_REQUIRED`。
- **携带方式（仅请求头）**：
  - `Authorization: Bearer <token>`
  - `X-Auth-Token: <token>`
  - **不接受** query 参数、cookie（有回归测试）。
- 比较用常量时间 `secureEqual`。

### 同源校验（仅写请求）

- 写方法（POST/PUT/PATCH/DELETE）若带 `Origin`，必须与 `r.Host` 同源（scheme 由 `r.TLS` 推导），或在 `-allow-origin` 列表 / 服务自身 origin 中，否则 `403 FORBIDDEN`。
- 无 `Origin` 头的写请求（如非浏览器客户端）不校验。
- **没有 CORS 中间件**：不写任何 `Access-Control-*` 响应头。浏览器必须同源访问，或经 Vite 代理（开发）。

### hostOnly（无 token 的资源读取）

用于浏览器无法附加 token 的路由：

- `GET /api/v1/cards/{cardId}/assets/{name}`
- `GET /api/v1/images/{filename}`
- `GET /api/v1/tts/voices`

这是 DNS rebinding 防护，**不是授权边界**：任何能通过 Host 校验的调用者都可读这些资源。

### 配对与限流

- `POST /api/v1/auth/pair`：校验 Host + 写 Origin；无 PIN 时返回 `{ok:true,token:""}`。
- 失败限流：每客户端 IP 5 分钟窗口最多 10 次失败，超限 `429 TOO_MANY_ATTEMPTS` + `Retry-After`；表上限 2048 项。
- 成功返回 `{ok:true,token:<authToken>}`。

## 2. 三端客户端要求

| 要求 | Web | Android |
|---|---|---|
| 保存 token | 本地存储 | 本地存储（DataStore/SharedPreferences） |
| 带 token | `Authorization: Bearer` 或 `X-Auth-Token` | 同 |
| 401 处理 | 清除 token，跳配对 | 同 |
| 写请求 Origin | 同源或走代理（不要伪造 Origin） | 无 Origin（原生客户端） |
| SSE | **不能**用原生 `EventSource`（无法设头）；用 `fetch` + ReadableStream | OkHttp 可设头 |
| 资源 URL | `<img>/<audio>` 直连 hostOnly 路由（无 token） | 同 |

## 3. 开发/运行提示

- 默认监听 `127.0.0.1:8890`；非 loopback 且未显式指定 `-tls` 时自动升级为自签 HTTPS。
- 局域网访问必须使用**私有网段 Host**（`192.168.x.x` / `10.x.x.x` / `172.16-31.x.x` / loopback），否则 `403`。
- **不要**把密钥写进仓库：`data/config/secrets.json` 在 `data/` 下（gitignore）；`release_audit.py --strict` 会拦截凭据。

## 4. 明确不做（按「保持宽松」）

- 不要求 CSRF token（写请求靠 Origin 校验）。
- 不要求一次性 nonce / 重放窗口（幂等键负责写去重，见 [`09-sync-idempotency.md`](09-sync-idempotency.md)）。
- 不要求 token 轮换/过期。

## 5. 差距

- **无 CORS**：跨源浏览器部署（Web 与后端不同源且不经代理）会失败。当前定位是本地优先 + 代理，保持现状；若要支持跨源，需显式设计（不在本规范范围）。
- **token 仅请求头** 与浏览器原生 `EventSource` 不兼容：三端都必须用可设头的流式客户端（已在 §2 强制）。
