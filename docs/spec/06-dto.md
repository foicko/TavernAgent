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

## 4. Go 代码生成（已落地）

**决策**：以 Go 类型为唯一源，生成 TS 与 Kotlin 的 DTO（`cmd/gencontract`，用 `reflect` 读取 `json` tag）。

- 生成物：`web/src/app/generated/dto.generated.ts`（`export interface`）与
  `android/.../data/remote/dto/GeneratedDtos.kt`（`@Serializable data class`，全字段带默认值）。
- 手写定义**不得与生成物重复**：TS 在 `types.ts` 里再导出（`export type { … } from "./generated/dto.generated"`）；
  Kotlin 生成物与手写 DTO 同包，直接删除被覆盖的手写 data class。
- CI：`go run ./cmd/gencontract --check`（生成物过期即失败）。

**当前覆盖**：`DirectorEvidence` / `DirectorBeat` / `DirectorPlan` / `DirectorBeatProgress` /
`DirectorState` / `DirectorSummary` / `ModelInstance` / `MemoryUsage` / `BranchView` /
`TurnCapabilities` / `AcceptResult` / `DeriveResult` / `AssistantEditResult`。清单在 `cmd/gencontract/main.go` 的 `manifest`。

**尚未覆盖**：`TurnView`/`TurnContent`、`SessionView`、`WorldState`、`MemoryView`、`CardPreview` 等。
`TurnView` 等曾是 `http` 层的 **ad-hoc `map[string]any` 响应**——已把回合受理/派生/编辑结果改成具名
struct 并纳入生成；其余同类 map 响应仍待重构。这是「覆盖全部」的前置。

## 5. 生成器的类型映射

| Go | TS | Kotlin |
|---|---|---|
| `string` | `string` | `String` |
| `bool` | `boolean` | `Boolean` |
| `int`/`int32` | `number` | `Int` |
| `int64`/`uint64` | `number` | `Long` |
| `float64` | `number` | `Double` |
| `[]T` | `T[]` | `List<T>` |
| `map[string]T` | `Record<string, T>` | `Map<String, T>` |
| `*T` | `T \| null` | `T?` |
| `time.Time` | `string` | `String` |
| `json.RawMessage` | `unknown` | `JsonElement`（**暂不支持**，需先在生成器补映射） |
| 匿名嵌入（无 tag） | 展平 | 展平 |

- `omitempty` → TS 可选（`?`）；Kotlin 一律给默认值（保证 `ignoreUnknownKeys`/缺字段时能解码）。
- 未登记进 manifest 的具名结构体 → 生成失败（强制显式登记，避免静默降级）。

## 6. 演进规则

- **加字段**：改 Go 结构体 → `go run ./cmd/gencontract` → 三端自动跟随；`--check` 必须通过。
- **改字段名/删字段**：改 Go → 重新生成 → 修因类型变化而编译失败的消费点；视为破坏性变更，同提交完成。
- **枚举**：生成器目前只输出裸 `string`（Go 的枚举是字符串常量，无类型信息）。如需联合类型/常量，另建枚举清单（待做）。
- **禁止**：手写定义与生成物重复；未登记类型混入生成物。

