# 01 原则与权威来源

## 1. 三端同规范

TavernAgent 有 **三个客户端**共享同一套后端能力：

- **Go 后端**：单二进制服务 + SQLite，`cmd/tavernagent`。REST + SSE 的唯一实现方。
- **Web 前端**：React 19 SPA，编译期嵌入二进制（`web/embed.go` 的 `//go:embed all:dist`）。
- **Android 客户端**：Kotlin/Compose 原生客户端，独立构建，走同一 REST + SSE。

三端必须**遵循同一份对外契约**。契约的差异不是「各端自由」，而是**缺陷**：任何一端与契约不一致，都要么改代码，要么走变更流程改契约。

### 权威层级（自高到低）

1. **本规范 `docs/spec/`** + `testdata/contract/` 下的契约源文件。
2. `docs/PROJECT_STATUS.md`（项目唯一权威文档：架构、已交付、门禁、技术债）。
3. `android/docs/API_CONTRACT.md`（REST/SSE 散文契约，细节最全）。
4. 源码（`internal/adapters/http/*.go` 等）。

冲突处理：散文文档与源码行为冲突时，**以源码为准并修正文档**；源码与 `testdata/contract/` 契约冲突时，**以契约为准并修正源码**（这正是契约测试的作用）。

## 2. 跨端契约的唯一权威来源：Go 为源 + 生成 TS/Kotlin

**决策**：跨端契约以 **Go 侧的类型/常量为唯一源**，由脚本生成 TS/Kotlin 契约物，CI 用 `--check` 防止生成物过期。

理由：Go 后端是行为的实现方；以它为源可以保证「契约 = 实际行为」，避免中立 schema 与实现两处维护。

落地方式：

| 契约类型 | 源文件 | Go 消费 | TS 消费 | Kotlin 消费 |
|---|---|---|---|---|
| 卡片标记语法 | `testdata/contract/markers.json` | 直接读 | 生成用例 | 生成用例 |
| SSE 事件名 | `testdata/contract/sse-events.json` | 直接读 + 源码扫描 | 生成 `sseEvents.generated.ts` | 生成 `SseEventNames.kt` |
| API DTO 字段 | `testdata/contract/api.json`（**待建**） | 反射断言 | 类型级断言 | serializer 断言 |

生成脚本：`scripts/gen_marker_contract.py`、`scripts/gen_sse_contract.py`（均支持 `--check`）。新增契约时沿用同一模式：**一份 JSON 源 + 三端断言 + `--check` 入 CI**。

> 注意：对于「Go 是行为实现方」的契约，JSON 源只是**导出物/快照**，不是第二真相；真正的裁决仍在 Go 代码。JSON 源的价值是让 TS/Kotlin 能被机器校验。

## 3. 版本策略：自由演进

**决策**：三端同仓同步发版，**不做正式版本协商**。

- 路径保留 `/api/v1` 作为**命名空间**，不是兼容性承诺：Go 侧没有任何版本路由或内容协商（`/api/v1` 只是字面前缀）。
- 允许破坏性变更（重命名、删除字段、改语义），前提是**三端在同一提交内一起改**，且契约源与生成物同步更新、`--check` 通过。
- 非 `/api/v1` 的路由：`GET /healthz`（存活探针）、`GET /api/status`（可观测性）、`GET /` 及其余非 `/api/` 的 GET/HEAD（SPA 静态资源）。
- 禁止：为兼容旧客户端而保留**无人使用**的旧字段/旧路由；确认无引用后应删除（避免维护面膨胀）。

> 「自由演进」不等于「随意改」：变更仍需走 [`14-docs-sync.md`](14-docs-sync.md) 的同步义务与门禁。

## 4. 功能对齐：Android 是核心功能子集

**决策**：Android 覆盖**核心消费功能**，高级编辑功能仅 Web 实现。规范只强制三端共享的接口契约，不强制功能集合完全相同。

- **Android 必须实现**：配对/鉴权、会话列表与阅读、回合生成与 SSE 流、分支切换/查看、导演只读查看、记忆查看、卡片浏览、模型配置的查看/切换。
- **仅 Web 需要**：卡片导入与编辑、导演命令/草稿编辑、记忆整理（organize/patch）、复杂的图谱/状态面板、设置页的高级项。
- **共享契约对三端同等强制**：凡是 Android 调用的接口，其字段与语义必须与 Go 一致；Android 不调用的接口可以不在 Android DTO 中建模（Kotlin 用 `ignoreUnknownKeys` 容忍未知字段，见 [`06-dto.md`](06-dto.md)）。

## 5. 实现方法的一致性

「同一规范」不仅指接口字段，也指**实现方法**：

- 三端都按分层组织（[`12-layering.md`](12-layering.md)），都通过统一的 API/SSE 客户端层访问后端，不绕过契约直接拼 JSON。
- 三端都用同一套时间/ID/命名约定（[`07-conventions.md`](07-conventions.md)）。
- 三端都遵守同一错误模型与幂等语义（[`04-errors.md`](04-errors.md)、[`09-sync-idempotency.md`](09-sync-idempotency.md)）。
- 三端都遵守同一事件流契约与重连语义（[`05-sse.md`](05-sse.md)）。
