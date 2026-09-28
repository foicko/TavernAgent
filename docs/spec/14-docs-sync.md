# 14 文档同步义务与变更流程

> 决策：**强制同步契约 + 三端文档**；变更由项目所有者拍板，PR 必须附全门禁证据。

## 1. 变更流程

1. **判定变更类型**：
   - 契约变更（REST/DTO/SSE/错误码/枚举/ID/命名）
   - 实现变更（不改对外契约）
   - 内部重构（不改行为）
2. **契约变更**必须：
   - 先改**契约源**（`testdata/contract/*.json`）与 **Go 源**（类型/常量）；
   - 重新生成 TS/Kotlin 契约物；
   - 同步三端 DTO 与解析逻辑；
   - 同步文档（见 §2）；
   - 跑全门禁（见 [`13-testing-gates.md`](13-testing-gates.md)），PR 附结果。
3. **实现变更**：跑全门禁即可。
4. **内部重构**：跑全门禁 + 若触及分层，跑 `layering_check.py`。

**裁决**：所有契约变更由项目所有者确认。规范允许「自由演进」，但**不允许三端不同步**。

## 2. 文档同步矩阵（改什么 → 同步什么）

| 改动 | 必须同步 |
|---|---|
| REST 路由/请求/响应 | 本规范 [`03-api.md`](03-api.md) + `android/docs/API_CONTRACT.md` + `docs/PROJECT_STATUS.md` |
| DTO 字段/可空性/枚举 | 本规范 [`06-dto.md`](06-dto.md) + `testdata/contract/api.json` + 三端 DTO + 生成物 |
| SSE 事件名/负载 | `testdata/contract/sse-events.json` + 本规范 [`05-sse.md`](05-sse.md) + `android/docs/API_CONTRACT.md` §8 + 生成物 |
| 错误码 | 本规范 [`04-errors.md`](04-errors.md) + `android/docs/API_CONTRACT.md` 错误码附录 |
| 卡片标记语法 | `testdata/contract/markers.json` + `docs/CARD_UI_MARKERS.md` + 生成物 |
| 鉴权/安全 | 本规范 [`08-auth-security.md`](08-auth-security.md) + `android/docs/API_CONTRACT.md` §2 |
| 数据目录/迁移 | 本规范 [`11-data-migrations.md`](11-data-migrations.md) + `docs/OPERATIONS.md` |
| 模型配置 | 本规范 [`10-provider-config.md`](10-provider-config.md) + `android/docs/API_CONTRACT.md` §13 |
| 分层/门禁 | 本规范 [`12-layering.md`](12-layering.md)、[`13-testing-gates.md`](13-testing-gates.md) + `AGENTS.md` |
| 架构/已交付能力/技术债 | `docs/PROJECT_STATUS.md`（唯一权威文档） |

## 3. 契约源的机器校验

| 契约 | 生成脚本 | CI 校验 |
|---|---|---|
| 卡片标记 | `scripts/gen_marker_contract.py` | `--check`（已入 CI） |
| SSE 事件名 | `scripts/gen_sse_contract.py` | `--check`（已入 CI） |
| API DTO | `scripts/gen_api_contract.py`（**待建**） | `--check`（**待入 CI**） |
| 错误码 | 并入 API 契约或独立脚本（**待建**） | **待入 CI** |

**规则**：新增任何跨端契约，必须提供「JSON 源 + 生成脚本 + `--check` + 入 CI」四件套，否则不予合入。

## 4. 权威文档关系

- `docs/PROJECT_STATUS.md`：项目唯一权威文档（架构、已交付、门禁、技术债）。
- `docs/spec/`（本目录）：三端开发规范与基线。
- `android/docs/API_CONTRACT.md`：REST/SSE 细节散文（最全，需随契约同步）。
- `docs/CARD_UI_MARKERS.md`：卡片标记语法说明。
- `AGENTS.md`：仓库硬约束与构建顺序（工程操作层面）。
- `docs/plan/REFACTOR_PLAN.md`：重构批次计划（B1–B5）与未完成项。

冲突时：**本规范 + 契约源 > PROJECT_STATUS > API_CONTRACT > 源码**；源码与契约冲突 → 改源码。

## 5. 差距

- `android/docs/API_CONTRACT.md` **无机器校验**（纯散文，2000+ 行），存在漂移风险。目标：随 DTO 契约（[`06-dto.md`](06-dto.md)）落地，逐步把可由契约覆盖的部分改为生成或加引用校验。
- 错误码枚举尚无源文件（[`04-errors.md`](04-errors.md) §4）。
