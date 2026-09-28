# 06 DTO 表示与字段命名

## 1. 原则

- **DTO 是跨端契约的一部分**：三端对同一资源必须使用相同的字段名、相同的可空性语义、相同的枚举取值。
- **落地方式**：**契约夹具 + 三端字段断言**（决策）。即：`testdata/contract/api.json` 声明每个 DTO 的必需字段名与枚举取值；Go 用反射、TS 用类型级断言、Kotlin 用 serializer 描述符各自断言，CI 必跑。
- **可空性**：Go 的 `omitempty` 字段在 TS 中应为可选（`?`），在 Kotlin 中应有默认值或可空类型。**「Go 必发」的字段在 TS 不得标可选**。

## 2. 当前 DTO 分布（事实）

| DTO | Go | TS | Kotlin |
|---|---|---|---|
| `SessionView` | `application/view.go` | `web/src/app/types.ts` `SessionView` | `dto/CardAndStoryDtos.kt` `SessionViewDto` |
| 会话摘要 | `domain.Session` | `SessionSummary` | `SessionDto` |
| 回合视图 | `adapters/http/turnview.go`（wire） | `TurnView` | `TurnViewDto` |
| 回合内容 | `domain.TurnContent` | `TurnContent` | `TurnContentDto` |
| `PlotNode` | `domain/types.go` | `types.ts` `PlotNode` | `PlotNodeDto` |
| `WorldState` | `domain/state.go` | `types.ts` `WorldState` | `WorldStateDto` |
| 记忆 | `application.MemoryView`（内嵌 `domain.MemoryRecord`） | `MemoryView` | `MemoryViewDto` |
| 角色卡 | `application.CharacterCard` / `domain.CharacterCardEntry` | `CardLibrarySummary` / `CardPreview` | `CharacterCardEntryDto` / `CardPreviewDto` |
| 分支 | `application.BranchView` | `BranchView` | `BranchDto` |
| 导演 | `domain/director.go` | `types.ts` `Director*` | `DirectorAndConfigDtos.kt` |

## 3. 已知跨端漂移（必须收敛）

以下差异是**缺陷**，不是自由：契约夹具建立后，三端断言会强制它们一致。

| 资源 | 差异 | 处理 |
|---|---|---|
| `SessionView` | TS **缺 `capabilities`** | TS 补 `SessionCapabilities` 类型 |
| `SessionView` | TS 把 `actions/branches/nodes/candidateGroups/...` 标为可选，Go 总是发出 | 按 Go 修正可空性 |
| 会话摘要 | TS **缺 `rulesetVersion`** | TS 补字段 |
| `TurnView` | TS **缺 `expectedHeadId`/`expectedVersion`/`capabilities`/`resultNodeId`** | TS 补齐 |
| `TurnContent` | TS 缺顶层 `optionRef`/`actionRef`；`checks` 类型不一致 | 以 Go 为准统一 |
| `PlotNode` | TS **缺 `schemaVersion`** | TS 补字段 |
| `WorldState` | TS **缺 `goals`/`milestones`/`unlockedSecrets`** | TS 补字段（或明确 Web 不消费并在契约中标注） |
| `MemoryView` | TS **缺 `secretId`** | TS 补字段 |
| `BranchDto`(Kotlin) | 多出 `sessionId`（Go/TS 的 `BranchView` 不含） | Kotlin 去掉或明确其为非契约字段 |
| `CharacterInfo` | Kotlin 是 Go/TS 的**缩减子集** | 允许（Android 子集），但在契约中标注「子集」 |
| `WorldState.characters` | 沿用 V2 `snake_case` 字段 | 见 [`07-conventions.md`](07-conventions.md) §3 |
| Kotlin 默认值 | `OptionDto.intent="unknown"`、`DirectorBeatProgressDto.status="pending"` **不是合法取值** | 改为合法枚举成员或可空 |
| 枚举类型 | TS/Kotlin 的 `status`/`mode`/`kind`/`inputKind` 是裸 `string` | 生成共享枚举联合/常量 |

> 说明：这些漂移**不会导致 Android 崩溃**（`ignoreUnknownKeys=true`、`explicitNulls=false`），但会导致**静默丢字段/默认值**，属于隐蔽缺陷。

## 4. 契约夹具格式（`testdata/contract/api.json`，待建）

建议结构（与 `markers.json` / `sse-events.json` 同风格）：

```jsonc
{
  "_comment": "API DTO 字段与枚举的唯一契约来源。Go 反射断言；TS/Kotlin 由脚本生成断言。",
  "dtos": {
    "SessionView": {
      "required": ["sessionId","characterId","title","rootNodeId","branch","headNode","state"],
      "optional": ["director","branches","viewNodeId","viewNode","nodes","candidateGroups","secrets","outline","activeSummary","actions","hasMore","oldestTurnId","capabilities"]
    },
    "TurnView": {
      "required": ["turnId","sessionId","branchId","status","mode","input","expectedHeadId","expectedVersion","capabilities"],
      "optional": ["failureCode","failureMessage","resultNodeId","resultNode","draft"]
    }
  },
  "enums": {
    "TurnStatus": ["accepted","queued","preparing","generating","validating","awaiting_approval","awaiting_continuation","committed","cancelled","failed","conflicted"],
    "MemoryKind": ["observed","reported","inferred","secret"],
    "ProviderKind": ["openai-chat","openai-responses","anthropic-messages"]
  }
}
```

## 5. 三端断言方法

| 端 | 方法 |
|---|---|
| Go | 用 `reflect` 读取目标结构体的 `json` tag，断言 `required` 全部存在、`optional` 存在且带 `omitempty`、枚举常量集合等于 `enums` |
| TS | 生成期望字段联合类型，用 `expectTypeOf<keyof T>().toEqualTypeOf<Generated>()` 做**类型级**断言；枚举用联合类型断言 |
| Kotlin | 对 `@Serializable` DTO 用 `serializer().descriptor.elementNames` 断言字段集合；或对样例实例 `Json.encodeToString` 后比对 key 集合 |

生成脚本沿用 `scripts/gen_*_contract.py` 模式，输出 TS/Kotlin 断言文件，`--check` 入 CI。

## 6. 演进规则

- **加字段**：先在契约的 `optional`/`required` 中声明 → 更新 Go → 生成 TS/Kotlin → 三端断言通过。自由演进允许直接加，但**必须同提交完成三端**。
- **改字段名/删字段**：视为破坏性变更，必须同提交改三端 + 契约源；`--check` 通过才可合。
- **枚举加值**：先在契约 `enums` 加值 → 三端同步（TS 联合、Kotlin 常量/`when`）。
- **禁止**：三端各自维护一份手写字段清单而不入契约（`web/src/app/__tests__/contract_fixtures.json` 目前就是手写的，应作为种子迁移进 `api.json`）。
