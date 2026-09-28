# TavernAgent 三端开发规范（接口宪法）

> 本目录是 **Go 后端 / Web 前端 / Android 客户端** 三端共同遵守的强制规范。
> 与任何散文文档冲突时，**以本规范 + 契约源文件为准**；与源码实现冲突时，视为**源码需要修正**（见「差距」）。
> 规范由项目所有者拍板；任何契约变更必须随 PR 附全门禁证据（见 [`14-docs-sync.md`](14-docs-sync.md)）。

## 读者

三端的实现者、评审者与 CI。新增功能前先读 [`02-features.md`](02-features.md) 与对应章节；改接口前先读 [`14-docs-sync.md`](14-docs-sync.md)。

## 章节索引

| 章节 | 内容 |
|---|---|
| [`01-principles.md`](01-principles.md) | 三端同规范原则、权威来源（Go 为源 + 生成）、版本策略、功能对齐 |
| [`02-features.md`](02-features.md) | 主要功能与三端实现方法 |
| [`03-api.md`](03-api.md) | REST 契约：路由、请求/响应、状态码 |
| [`04-errors.md`](04-errors.md) | 统一错误信封与错误码枚举 |
| [`05-sse.md`](05-sse.md) | SSE 事件名 + 负载字段 + 顺序/终态/重连 |
| [`06-dto.md`](06-dto.md) | DTO 表示、字段命名、契约夹具与三端字段断言 |
| [`07-conventions.md`](07-conventions.md) | 命名/编码/时间/ID 约定 |
| [`08-auth-security.md`](08-auth-security.md) | 鉴权与安全基线 |
| [`09-sync-idempotency.md`](09-sync-idempotency.md) | 快照 + 重放 + 幂等键 |
| [`10-provider-config.md`](10-provider-config.md) | 模型供应商配置契约 |
| [`11-data-migrations.md`](11-data-migrations.md) | 数据目录与迁移契约 |
| [`12-layering.md`](12-layering.md) | 三端分层与目录（机器强制） |
| [`13-testing-gates.md`](13-testing-gates.md) | 测试与 CI 门禁 |
| [`14-docs-sync.md`](14-docs-sync.md) | 文档同步义务与变更流程 |

## 一句话基线

- **权威来源**：跨端契约以 **Go 类型/常量为源**，用脚本生成 TS/Kotlin 契约物，CI `--check` 防漂移（[`01-principles.md`](01-principles.md)）。
- **命名**：JSON 一律 `camelCase`；时间一律 RFC3339（UTC）；实体 ID 带类型前缀（[`07-conventions.md`](07-conventions.md)）。
- **错误**：统一 `{code,message,retryable,turnId,traceId?}` 信封 + 共享错误码枚举（[`04-errors.md`](04-errors.md)）。
- **事件流**：事件名 + 负载字段单源化，三端解析测试绑定（[`05-sse.md`](05-sse.md)）。
- **写操作**：必须携带幂等键；断线用快照 + 事件重放恢复（[`09-sync-idempotency.md`](09-sync-idempotency.md)）。
- **分层**：三端都定义依赖方向并机器强制（[`12-layering.md`](12-layering.md)）。
- **门禁**：契约测试 + 安全边界测试必须常绿（[`13-testing-gates.md`](13-testing-gates.md)）。

## 合规矩阵（现状 → 目标）

`✅ 已符合` / `⚠️ 部分符合（有差距）` / `❌ 未实现`

| 规范项 | 现状 | 差距 / 待补 | 门禁 |
|---|---|---|---|
| REST 路由与请求/响应 | ✅ 已实现 | 无机器校验，靠散文 + e2e | playwright e2e |
| 统一错误信封 | ✅ 扁平信封已实现 | 未知 `/api/*` 路径与 stdlib `404/405` 仍是纯文本 | Go 测试 |
| 错误码枚举（跨端共享） | ⚠️ Go 侧已有映射 | 无单一枚举源，TS/Kotlin 未绑定 | 待补 |
| SSE 事件名 | ✅ 单源化 | — | `gen_sse_contract.py --check` |
| SSE 负载字段 | ❌ 未机器化 | 需在契约中声明每事件必需字段并三端断言 | 待补 |
| DTO 字段与命名 | ⚠️ Go 生成（第一批） | 覆盖 Director*/ModelInstance/MemoryUsage；SessionView/Turn/WorldState/MemoryView 与 44 处 map 响应待覆盖 | `go run ./cmd/gencontract --check` |
| camelCase | ⚠️ 主体符合 | V2 卡片字段保留 `snake_case`（历史兼容，见 [`07`](07-conventions.md)） | `check.mjs` / 评审 |
| 时间 RFC3339 | ✅ 已实现 | — | Go 测试 |
| 实体 ID 前缀 | ⚠️ 部分 | 多数 ID 是裸 UUIDv7；文档示例中的 `card_` 与实际不符 | 待补 |
| 鉴权与安全基线 | ✅ 已实现 | 无 CSRF/重放保护（按「保持宽松」不强制） | 安全边界测试 |
| 快照 + 重放 + 幂等键 | ✅ 已实现 | 无 `snapshot` 事件名，快照走 REST（见 [`09`](09-sync-idempotency.md)） | Go 测试 |
| 模型供应商配置 | ✅ 已实现 | — | Go 测试 |
| 数据目录与迁移 | ✅ 已实现（16 迁移，事务 + 备份） | — | `migration_safety_test.go` |
| Go 分层 | ✅ 机器强制 | — | `layering_check.py` |
| Web 分层 | ✅ 机器强制 | — | `pnpm lint` |
| Android 分层 | ❌ 无检查 | 需加依赖方向检查 | 待补 |
| 契约测试门禁 | ⚠️ SSE/markers 已入 CI | DTO 契约未入 CI | `--check` |
| 安全边界测试门禁 | ✅ 已入 CI | — | `go test` / e2e |
| 文档同步义务 | ⚠️ 已声明 | `API_CONTRACT.md` 无机器校验 | 评审 |

> 「待补」项不是自由发挥，而是本规范已经确定的**必须实现项**；实现时按 [`14-docs-sync.md`](14-docs-sync.md) 的流程随 PR 补齐门禁。
