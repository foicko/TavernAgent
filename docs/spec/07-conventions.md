# 07 命名 / 编码 / 时间 / ID 约定

## 1. JSON 字段命名：`camelCase`

对外 JSON 字段一律 `camelCase`（`internal/domain/types.go` 明确声明该规则；`android/docs/API_CONTRACT.md` 亦声明「外部一律 camelCase」）。

- Go：`json:"camelCase"`。
- TS：直接使用同名属性。
- Kotlin：属性名即序列化名（**当前零 `@SerialName`**，见 §4）。

**禁止**：新增 `snake_case` / `PascalCase` 的对外字段。

## 2. 历史例外：V2 卡片字段保留 `snake_case`

SillyTavern V2/V3 卡片格式的字段是历史兼容字段，**保持原样**（不改名，避免破坏导入/导出）：

- `domain.CharacterInfo`：`mes_example`、`system_prompt`、`post_history_instructions`、`creator_notes`、`character_version`
- `application.CharacterCard`：`first_mes`、`mes_example`、`system_prompt`、`post_history_instructions`、`creator_notes`、`character_version`

**规则**：这些是**唯一允许的 `snake_case` 对外字段**，且必须与 V2 规范一致。除此之外不得新增。

> 已知不一致（待收敛）：同一概念在**卡片导入预览**路径用 camelCase（`firstMes`/`mesExample`/...，`adapters/http/cards.go`），在**世界状态 `characters`** 路径用 snake_case。规范要求：**对外 wire 以 V2 原始字段名为准（snake_case）**；导入预览若要 camelCase，必须在契约中显式声明为「预览视图专用」，不得与世界状态混用同一概念的两个名字。

## 3. 内部字段

- 供应商适配器内的字段（OpenAI `max_tokens`、Anthropic `cache_control` 等）与 PNG/V2/V3 解析字段（`spec_version` 等）是**外部协议字段**，不属于本项目对外契约，保持外部协议原样。
- Go 内部结构体若字段不对外，用 `json:"-"` 隐藏（如 `domain.Branch.ActiveTurnID`），对外视图另建 `BranchView`。

## 4. Kotlin 序列化配置

`android/.../data/remote/ApiJson.kt`：

```kotlin
ignoreUnknownKeys = true
explicitNulls = false
encodeDefaults = true
```

含义：未知字段被静默丢弃；缺失字段取默认值。**这既是兼容性缓冲，也是隐蔽缺陷来源**——重命名字段不会报错，只会静默变默认值。因此 DTO 契约断言（[`06-dto.md`](06-dto.md)）对 Android 尤其重要。

## 5. 时间：RFC3339（UTC）

所有对外时间字段是 **RFC3339 / RFC3339Nano 字符串**，不是 Unix 秒/毫秒。

- Go：`time.Time` 经 `encoding/json` 输出 RFC3339Nano；SQLite 也以 RFC3339Nano 字符串存取（`adapters/sqlite/store.go` 的 `now()`/`parseTime`）。
- TS：一律 `string`，**不做本地时区解析后再序列化**；需要格式化时按 UTC 展示。
- Kotlin：一律 `String`。
- 命名：`createdAt`、`updatedAt`、`lastUsedAt`、`validFromTurn`（回合号是整数，不是时间）。

**规则**：新增时间字段必须是 RFC3339 字符串；服务端输出 UTC；客户端不得把本地时区时间写回。

## 6. 实体 ID：类型前缀

ID 生成器：`internal/util/id`（UUIDv7，回退 UUIDv4）。

**规范**：对外实体 ID 应带**类型前缀**，便于日志与调试。现状多数是裸 UUIDv7，属于差距（§7）。

| 前缀 | 实体 | 现状 |
|---|---|---|
| `session_` | 会话（带幂等键创建时 = `session_`+sha256(key)） | ✅ |
| `root_` | 根节点 | ✅ |
| `branch_main_` | 主分支 | ✅ |
| `a_` | 回合尝试（SSE `attemptId`） | ✅ |
| `mem_` | 记忆（legacy commit-plan 路径） | ⚠️ 常规修订是裸 UUID |
| `goal_` / `secret_` / `npc_` / `open_` | 目标/秘密/角色/开场变体（卡片派生） | ✅ |
| `roll_` | 判定 roll ID | ✅ |
| `m_` / `key_` | 模型实例 / 密钥引用 | ✅ |
| `call_` / `img_` / `rev_` / `beat_` | 用量调用 / 图像 / 导演报告与 beat | ✅ |
| `direct_` / `derive_` | 内部幂等键 | ✅ |
| `card_` | 角色卡 | ❌ 生产是**无前缀 hash**；`card_` 只出现在测试/文档示例 |
| `turn_` / `node_` / `branch_` / `req_` | 回合/节点/分支/导演请求 | ❌ 生产是裸 UUIDv7 |

**规则**：
- 新实体 ID **必须**带类型前缀。
- 前缀一旦确定不得更改（会破坏日志/调试/外部引用）。
- 文档与测试示例中的 ID 必须与生产生成规则一致（不要用不存在的 `card_` 之类误导）。

## 7. 差距（必须收敛）

- 多数 ID 是裸 UUIDv7，与「类型前缀」规范不符；且 `card_` 只存在于测试/文档，生产是 hash。目标：要么给生产加前缀，要么把规范改为「ID 是不透明字符串，前缀仅用于派生 ID」并在契约中写明。**在收敛前，客户端必须把 ID 当不透明字符串，不得解析前缀。**
- 卡片字段 camelCase / snake_case 双名问题（§2）。
