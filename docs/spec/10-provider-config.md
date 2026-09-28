# 10 模型供应商配置契约

## 1. 数据模型（v2）

核心原则：**连接信息挂在 model 实例上；slot 只引用 modelId**。

- **ModelInstance**：唯一携带连接信息的实体。
  - `modelId`、`kind`、`baseUrl`、`model`、`apiKey`（仅在写入时出现，读取时被掩码）、`hasApiKey`、`temperature`、`maxTokens`、`contextWindow`、`reasoningEffort`、`inputBudget`（计算值）。
- **SlotBinding**：`slot` + `enabled` + `modelId`（**只有引用，不允许内联连接信息**）。
- **Slots**：`primary` | `assist` | `reflection`。
- **ModelCatalog**：`{models, slots}`，`GET /api/v1/config/models` 返回。

> slot 内联连接信息 → `400 SLOT_TAKES_REFERENCE_ONLY`。

## 2. 支持的 kind

| kind | 适配器 | 别名（兼容） |
|---|---|---|
| `openai-chat` | `adapters/providers/openai` | `openai-compatible`、`openai` |
| `openai-responses` | `adapters/providers/responses` | `responses` |
| `anthropic-messages` | `adapters/providers/anthropic` | `anthropic` |
| `mock` | 开发回退（`mock.NewDemo()`） | **不落盘**，仅 `-provider mock` / 无配置时回退 |

未知 kind → 不支持错误。落盘时 `""`/`openai-compatible`/`openai-chat` 归一为 `openai-chat`。

## 3. 密钥存储

- 非密配置：`data/config/settings.json`。
- 密钥：`data/config/secrets.json`（`map[ref]key`，权限 `0o600`）。
- model 的 `apiKeyRef` 指向 secrets 中的 ref；新 ref 形如 `key_<uuid>`。
- 读取优先级：`secrets.json` → 环境变量 `TAVERNAGENT_KEY_MODEL_<ID>` → `TAVERNAGENT_KEY_<SLOT>`。
- 写入：**先写 secrets 再写 settings**，均原子写；读取响应中密钥一律掩码（`MaskAPIKey`，保留首4…末4 或 `****`）。
- 保存时若 key 为空或被掩码 → 保留旧 key；只提交掩码值会被拒绝。

## 4. 接口

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/config/models` | 返回整个 catalog（models + slots，密钥掩码） |
| POST | `/api/v1/config/models` | 新建 model |
| PUT | `/api/v1/config/models/{modelId}` | 更新 model |
| DELETE | `/api/v1/config/models/{modelId}` | 删除；被 slot 引用 → `409 MODEL_IN_USE` |
| POST | `/api/v1/config/models/{modelId}/probe` | 探测单 model |
| GET | `/api/v1/config/provider` | 读取 slots |
| PUT | `/api/v1/config/provider` | 保存 slot（只允许 `enabled`+`modelId`） |
| POST | `/api/v1/config/provider/probe` | 探测 provider |

错误码：`MODEL_NOT_FOUND`(404)、`MODEL_IN_USE`(409)、`SLOT_TAKES_REFERENCE_ONLY`(400)、`PROVIDER_UNAVAILABLE`(502)。

## 5. 三端实现要求

| 端 | 要求 |
|---|---|
| Go | 上述 v2 模型与优先级；密钥绝不回显 |
| Web | `settingsStore` 用单次 `GET /config/models` 填充 models+slots；`ConnectionForm` 默认 `kind:"openai-chat"`；协议下拉的 label 来自 `lib/modelLabels.ts` |
| Android | 至少支持查看与切换 slot/primary；不内联连接信息；未知字段容忍 |

## 6. 规则

- 新增 provider kind：先加适配器 → 加落盘归一化 → 更新 `ports/providerconfig.go` 的 kind 常量 → 更新三端枚举/下拉（契约见 [`06-dto.md`](06-dto.md)）。
- `temperature`/`maxTokens`/`reasoningEffort` 是**每 model** 属性，不在 slot 上。
- 不要在客户端缓存密钥；`hasApiKey` 只用于 UI 提示。
