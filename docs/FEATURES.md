# TavernAgent 功能说明（实现手册）

> 本文件说明**项目有哪些功能、每个功能是怎么实现的**：数据结构、算法、提示词、格式与契约、以及关键代码位置。
> 与 `docs/spec/`（对外接口宪法）互补：`docs/spec/` 管「三端必须一致的契约」，本文件管「功能内部如何运转」。
> 深入专题另见：`docs/TACARD_SPEC.md`（tacard 格式规范）、`docs/PROMPTS.md`（提示词全文，生成物）、`docs/CARD_UI_MARKERS.md`（卡片标记）、`android/docs/API_CONTRACT.md`（REST/SSE 细节）。

## 目录

1. [总览](#1-总览)
2. [角色卡系统](#2-角色卡系统)
3. [会话、分支与剧情图](#3-会话分支与剧情图)
4. [回合生成管线](#4-回合生成管线)
5. [提示词系统](#5-提示词系统)
6. [世界书（Lorebook）](#6-世界书lorebook)
7. [世界状态与规则判定](#7-世界状态与规则判定)
8. [记忆系统](#8-记忆系统)
9. [生成选项（Options）](#9-生成选项options)
10. [对话中的图片](#10-对话中的图片)
11. [导演系统](#11-导演系统)
12. [语音 TTS](#12-语音-tts)
13. [嵌入 Embeddings](#13-嵌入-embeddings)
14. [模型供应商与配置](#14-模型供应商与配置)
15. [上下文预算与压缩](#15-上下文预算与压缩)
16. [契约与格式总表](#16-契约与格式总表)
17. [已知差距与注意事项](#17-已知差距与注意事项)

---

## 1. 总览

TavernAgent（SillyDog）是本地优先的 AI 角色扮演 / 交互叙事引擎。

- **Go 后端**（`cmd/tavernagent`，单二进制 + SQLite）：唯一的行为实现方，REST + SSE。
- **Web 前端**（React 19 SPA，编译期嵌入）：全功能。
- **Android 客户端**（Kotlin/Compose）：核心功能子集。

数据流一句话：**导入角色卡 → 创建会话（冻结卡片/规则/世界书到根节点）→ 每回合组装上下文（提示词）→ 调模型流式输出 → 解析成帧 → 原子提交为节点+状态事件+记忆+outbox → SSE 推给客户端**。

核心不可变原则：
- 剧情节点**只增不改**；分支靠 fork，修正靠派生。
- 世界状态由**领域事件**驱动，用单一 reducer（`domain.ApplyEvents`）应用，可重放。
- 卡片/规则/世界书在会话创建时**冻结**为模板版本，之后读的是快照而非卡片文件（保证历史可复现）。

---

## 2. 角色卡系统

### 2.1 支持的格式与识别

HTTP 入口：`POST /api/v1/cards`（别名 `/api/v1/cards/import`）。**不看扩展名**，按内容识别（`internal/adapters/http/cards.go` 的 `parseCardFile`）：

| 判定 | 处理 |
|---|---|
| 头 8 字节是 PNG 签名 | `ImportCardPNG`（读 tEXt/zTXt/iTXt 元数据） |
| 头 4 字节是 `PK\x03\x04` | 先看是否含 `tacard.json`（`IsTacardArchive`）→ tacard；否则 charx |
| 其他（可能是 JPEG/ZIP 多形态的 `.charx`） | `IsTacardArchive` → `IsCharxArchive`（读 ZIP 中央目录）→ 否则按 JSON |
| 兜底 | `ImportCardJSON` |

上传限制：原始 1 GiB、内存阈值 8 MiB、PNG/JSON 64 MiB、归一化 JSON 8 MiB（`internal/adapters/http/request_body.go`）。

产出格式 `ImportReport.Format`：`native` / `tavern_v1` / `chara_card_v2` / `chara_card_v3` / `charx` / `tacard`；来源 `Source`：`png` / `json` / `charx` / `tacard`。

### 2.2 自创的 tacard 格式

规范全文见 `docs/TACARD_SPEC.md`。实现见 `internal/application/tacard.go`。

- **容器**：标准 ZIP，MIME `application/x-tavernagent-card+zip`，规范版本 `tacard/v1`。
- **权威文件**：根目录 `tacard.json` 是唯一权威；即使同时带兼容用的 `card.json` 也被忽略。
- **零脚本策略**：含任何可执行/脚本扩展名（`exe/dll/so/sh/bat/ps1/js/lua/py/wasm/risum…`，`tacardForbiddenExt`）→ **整卡拒绝**，不是警告。
- 版本处理：空或 `tacard/v1` 正常；其他 `tacard/*` 前缀接受但告警「未知字段已忽略」；非 `tacard` 前缀拒绝。

`tacard.json` 结构（`tacardDoc`）：

```jsonc
{
  "spec": "tacard/v1",
  "metadata": { "id", "name", "version", "author", "description", "tags": [] },
  "character": {
    "name", "aliases": [], "avatar_asset",
    "description", "personality", "scenario",
    "system_prompt", "post_history_instructions"
  },
  "openings": [ { "id", "title", "text" } ],
  "lorebook": {
    "name",
    "entries": [ { "id", "keys": [], "secondary_keys": [], "content",
                   "constant": false, "order": 0, "position": "before_char|after_char" } ]
  },
  "assets": [ { "id", "file", "type" } ],
  "ui_contract": { "theme_tokens": {}, "default_variables": {} }
}
```

**字段加载映射**（`mapTacardDoc` / `mapTacardLorebook` / `attachTacardUIProfile`）：

| tacard.json | → `application.CharacterCard` / `domain` |
|---|---|
| `metadata.id` | `CardID`（`tacardCardID` 清洗非法字符，>128 字节则内容派生） |
| `metadata.name` | `character.name` 为空时的兜底 `Name` |
| `metadata.version` / `author` / `description` / `tags` | `Version` / `Creator` / `CreatorNotes` / `Tags` |
| `character.name` | `Name`、`Characters[0].Name` |
| `character.aliases` | `Nickname`（逗号连接）+ `Characters[0].Aliases` |
| `character.avatar_asset` | 头像候选 id → 落地为 `Avatar` data URL |
| `character.description/personality/scenario` | 同名字段 |
| `character.system_prompt` | `SystemPrompt` |
| `character.post_history_instructions` | `PostHistory` |
| `openings[]` | `OpeningVariants[]`；第一条非空文本成为 `FirstMes` |
| `lorebook.entries[]` | `LorebookRefs[0].Entries`，`Schema:"tacard"`，`Enabled:true`；缺 `id` 时派生 `lbe_<hash>` |
| `assets[]` | `Assets[]`（`Name=id`，`URI`=归一化相对路径，`Ext`） |
| `ui_contract.theme_tokens` | `UIProfile.ThemeTokens` |
| `ui_contract.default_variables` | `UIProfile.VarDefaults` |

还会合成唯一主角 `Characters[0]`（`CharacterID = "npc_"+hash(name)[:8]`，`Participant:true`）。

**与 V2/V3 的差异**：ZIP 容器 + `tacard.json` 权威；零脚本强制；声明式 UI 契约（`theme_tokens`/`default_variables` 直接用，不执行卡片 CSS）；规范标记 `[IMG:key]`/`[HUD]`/`[SCENE]`/`[VAR]`；结构化 `openings[]`；显式世界书结构（`constant`/`order`/`position`）；自声明 `id` 会被清洗。

> 后端**没有 tacard 导出器**（仅导入）；生成/校验脚本在 `scripts/*tacard*.py`。

### 2.3 V1 / V2 / V3 字段映射

- PNG：优先读 `ccv3` 元数据（V3 全量），回退 `chara`（V2）；`Creator`/`Notes`/`Comment` 回填。
- `fillCardFromSpec`：`spec == "" | "native"` → 原生卡（`ParseCharacterCard`）或 V1（`looksLikeV1`）；`chara_card_v2/v3` → `mapSpecData`。
- V1：平铺字段（`first_mes`/`mes_example`/`personality`/`scenario`）归一为 V2 后复用同一映射。

主要映射（`mapSpecData`）：

| V2/V3 | → |
|---|---|
| `name` | `Name` |
| `description` + `personality` + `scenario` | 以 `\n\n` 合并进 `Description` |
| `first_mes` | `FirstMes` |
| `mes_example` | `MesExample` |
| `system_prompt` | `SystemPrompt` |
| `post_history_instructions` | `PostHistory` |
| `creator_notes` / `tags` / `creator` / `character_version` / `nickname` | 同名 |
| `avatar` | `Avatar`（缺省时取首个 `icon` 资源） |
| `assets[]` | `Assets[]` |
| `character_book.entries` | `LorebookRefs[0]`（`Schema:"v2"` 或 `"v3"`） |
| `first_mes` + `alternate_greetings[]` | `OpeningVariants[]`（标题「默认开场」「开场b」…） |
| `group_only_greetings[]` | 忽略（单人叙事） |

世界书条目映射：`enabled` 缺省为 true；`secondary_keys` 仅在 `selective` 时保留；`EntryID = "lbe_"+hash(keys|content)`；`Title` 取 `name` 或 `comment`。

**历史兼容**：`CharacterCard` 与 `domain.CharacterInfo` 保留 V2 `snake_case` 字段（`first_mes`、`mes_example`、`system_prompt`、`post_history_instructions`、`creator_notes`、`character_version`）——这是唯一允许的 `snake_case` 对外字段。

宏 `{{char}}`/`{{user}}`：会话创建时一次性展开（`application/macros.go`）；因持久化保留原始字节以做卡身份哈希，秘密内容在编译期再展开（`internal/context/compile.go`）。

### 2.4 内嵌资源：落地与提供

三种来源：

- **charx**：仅 `embeded://` URI 落地；`data:`/`http` 原样保留。`uniqueAssetFile` 清洗文件名并去重（同名加 `_1`）。
- **tacard**：`assets[].file` 相对路径（缺 `assets/` 前缀自动补；拒绝 `..`）。
- **RisuAI PNG**：`chara-ext-asset_:N` 块，`assets[].uri = "__asset:N"`，base64 解码 + 魔数判扩展名。

落地管线：`CardService.SaveReport` → `persistCharxAssets`/`persistEmbeddedAssets` → `data/cards/<cardID>/<ref>`（原子写 + 内容嗅探 MIME）。归档限制：单文件 64 MiB、总量 2 GiB、条目 8192。

URL 规则（`cardAssetsBody`）：内部资源 → `/api/v1/cards/{cardId}/assets/{ref}`；外部 `http(s)`/`data:` 原样；不可解析则 URL 为空。该路由是 **hostOnly（无 token）**，因为 `<img>` 无法带 Bearer。

资源的三元组：`name`（卡内逻辑键，标记引用它）、`ref`（落地文件名）、`uri`（原始引用）。Web 侧 `resolveCardAsset` 负责 name→URL 的模糊匹配。

删卡：仅当**没有会话引用该角色**时才清理资源（「删卡不删故事」）。

### 2.5 卡片 UI 标记

唯一语法源 `testdata/contract/markers.json`；Go `internal/cardmarkers`，Web `web/src/lib/cardMarkers.tsx`，Kotlin 生成契约测试。

两类：

- **状态类（服务端归一化并从正文剥离）**：`[HUD:label=value|…]`、`[SCENE:title]`、`[VAR:key=value|…]`。`Extract` 解析为 `UIHint` 存入 `TurnContent.UIHints`；`[VAR]` 另生成 `variable_set` 事件写入 `WorldState.Variables`。`[SCENE]` 仅展示，不改世界状态。
- **呈现类（保留在正文，原位渲染）**：`<img cmd="key">`、`<img="key">`、`<img_obj="key">`、`<imgBG="key">`、`<img src="key" …>`、`[IMG:key]`。`SplitImages` 切成 `Segment{type:text|image, text, assetKey}`；无法解析的形式原样保留（绝不静默吞内容）。

`TextBlock.Segments` 在**读取/投影时**由 `attachInlineSegments` 填充（不改写历史节点）。

### 2.6 卡片能力、UIProfile 与规则

- `CardCapabilities`（`application/cardui.go`）：来源、是否有 module/背景 HTML/默认变量/SD 数据/depth prompt/vits、资源计数、世界书条数、`imgCmd`、标记族。`module.risum` 从不解析（GPL 顾虑）。
- `CardUIProfile`：`Capabilities` + `VarDefaults` + `ThemeTokens`（从背景 HTML 提取，键归一为 `accent/bg/ink/font`，**卡片 CSS 从不执行**）+ `SdData`（图像生成的 base prompt）。
- `rules`（`domain.Ruleset`）：动作规则集，会话创建时校验并冻结为模板；见 §7.3。

### 2.7 卡片库与导入预览

`domain.CharacterCardEntry`（表 `character_cards`）：`cardId/name/shortName/avatar/format/role/characterJson/contentHash/createdAt/updatedAt/lastUsedAt`。

- 硬上限 **256 张**（`maxCards`）；同 `card_id` 覆盖（保留 `created_at`）。
- `shortName` = 名称首个连接词前段；`role` = 描述首个可读非标记行（≤60 runes）。
- 时间戳 RFC3339Nano；`lastUsedAt` 在会话创建时 best-effort 更新。

导入预览 `cardPreviewBody` 返回：格式/来源/名称/头像/描述/人格/场景/`firstMes`/`mesExample`/`systemPrompt`/`postHistoryInstructions`/`creatorNotes`/tags/creator/版本/nickname、`supported`/`ignored`/`warnings`、`characterJson`、`card`、`openings`、`assets`，以及（有 UIProfile 时）`capabilities`/`varDefaults`/`themeTokens`/`sdData`。

> 注意：同一概念在**预览**用 camelCase（`firstMes`），在**世界状态 `characters`** 用 V2 snake_case（`first_mes`）——见 `docs/spec/07-conventions.md`。

---

## 3. 会话、分支与剧情图

- **会话 Session**：`sessionId`、`rootNodeId`、`title`、`createdAt/updatedAt`、`rulesetVersion`、`characterId`。
- 创建时：生成 `root_<sessionId>` 根节点、`branch_main_<sessionId>` 主分支，并把**卡片、规则、世界书**冻结为模板版本写入根节点快照（`buildTemplateSnapshot`）。
- 带幂等键创建：`sessionId = "session_"+sha256(key)`，重复创建返回同一会话。
- **分支 Branch**：可从任意节点 fork，`BranchView` 暴露 `branchId/name/headNodeId/version/activeTurnId`（不含 `sessionId`）。
- **节点 PlotNode**：不可变，`kind ∈ {root,turn,configuration_change,memory_change,director_event}`，`parentId` 构成 DAG；`contentJson` 存 `domain.TurnContent`。
- 读取：`GET /api/v1/sessions/{id}` 返回 `SessionView`，可带 `viewNodeId` 查看历史时点（历史视图只读）。
- 状态权威：`state_snapshots`（每 20 层） + `domain_events`（重放）。`StateAt(N)` = 最近兼容检查点 + 有序重放，并校验指纹。

---

## 4. 回合生成管线

### 4.1 受理（Accept）

`TurnService.Accept`（`internal/application/turn_accept.go`）：

1. 全局 `acceptMu` 串行化受理。
2. 必须带 `idempotencyKey`，否则 `400 BAD_REQUEST`。
3. 校验分支属于会话、`expectedCharacterId` 绑定。
4. 查 `FindTurnByIdempotency`；命中则校验重放（分支/基线/载荷哈希）→ 返回既有回合，或 `409 IDEMPOTENCY_CONFLICT`。
5. 校验输入（见下）；`headMatchesClient` 容忍中间的 maintenance 节点（`memory_change`/`director_event`），否则 `409 HEAD_CONFLICT`。
6. 真实基线取**分支真实头**（后台维护节点可能已推进）。
7. `CreateTurnRequest`（状态 `queued`）+ `ClaimActiveTurn`；分支忙 → 该回合标记 `QUEUE_FULL`（429）并绑定幂等键。
8. 若选项带 `actionRef` → 受理时 `PrepareCheck` 先掷骰（失败则释放锁并以 `CHECK_FAILED` 结束）。
9. 投递到按分支的信号量（容量 1）异步生成。

载荷哈希 `acceptPayloadHash` 覆盖 `{Input, Mode, ExpectedHeadID, DeriveNodeID, ReuseRollID, Recheck}`，确保换骰/换源节点/换模式不会误命中。

### 4.2 状态机

`accepted → queued → preparing → generating → validating → committed`，异常分支 `awaiting_approval`（**当前无产出路径**）/ `awaiting_continuation` / `cancelled` / `failed` / `conflicted`。

- 终态：`committed|cancelled|failed|conflicted`。
- 允许迁移表见 `internal/domain/turn_state.go`（`CanTransitionTurn`）。
- 所有迁移经 `Store.TransitionTurn`：校验迁移合法 + attempt 围栏，终态时清空分支锁、废弃未结算 receipt、写终态事件（`turn.cancelled/failed/conflicted/truncated`）。

### 4.3 上下文组装

`ctxpkg.Compiler.Compile`（`internal/context/compile.go`）：

1. `loadSessionContext`：从根节点快照读开场白、玩家名、规则、秘密、世界书、卡片 UI。
2. `RecentTurnNodes`（默认最多 50 轮）。
3. `buildHaystack`（开场 + 本轮输入 + 近期回合输入/正文）→ `matchLorebook`。
4. 记忆召回（FTS5 + 路径过滤）。
5. 路径上的摘要、`compileLedger`（权威状态账本）。
6. 尾策略与摘要折叠。
7. `fitBundleBudget`（预算收敛，见 §15）。
8. `buildMessages`。

### 4.4 流式生成与解析

`runGenerate`（`turn_run.go`）：

- 注册取消、解析器 `protocol.NewStreamParser`、注册 live 草稿。
- thinking 回调 → `turn.thinking`（瞬时，Sequence 0）。
- `prov.Stream` 逐块喂解析器；`OnFrame` → 持久化 draft 帧并 `block.appended`（持久）。
- `afterStream` 判定：取消 → `turn.cancelled`；协议错误 → `PROTOCOL_INVALID`；供应商空输出 → `PROVIDER_EMPTY_OUTPUT`（可重试）；结构化缺 `final` 但有块 → 进入续写；成功 → 提交。

输出协议与解析细节见 §5.4。

### 4.5 提交计划与原子提交

`buildCommitPlan`（`turn_commit.go`）→ `buildPlan`（`commitplan.go`）：

1. 提取标记、剥离 HUD/SCENE/VAR、选项节流（never 清空 / auto 抑制连续两轮）。
2. `[VAR]` → `variable_set` 事件。
3. 规则 receipt 效果 → 事件（先应用到 `probe` 校验）。
4. `processProposalStream`：关系/心情/记忆/誓言/目标/物品。
5. 聚合关系增量（每角色每维度 ±10 封顶）。
6. **单一 reducer**：`domain.ApplyEvents(NewState, Events)`。
7. `evaluateSecretUnlocks`（在本轮效果**之后**评估）。
8. `assembleTurnContentJSON` 生成 `TurnContent`（含 `Changes: SummarizeChanges(...)`）。

`settleCommit` → `Store.CommitTurn`（**单个 SQLite 事务**）：

- 预检：回合状态、attempt 围栏、取消意图、分支锁、分支头/版本 CAS、`BaseStateHash`。
- 写入：节点、`domain_events`、导演投影缓存、记忆、投影快照（每 64 层）、`memory_mentions`、状态快照（每 20 层）、分支头 CAS。
- 收尾：会话 `updated_at`、回合 `committed` + `result_node_id`、receipt 结算、写**持久 `turn.committed` outbox 行**。

### 4.6 失败 / 续写 / 再生 / 编辑 / 恢复

- **续写**：`enterContinuation`（状态 `awaiting_continuation`，事件 `turn.truncated`）；`Continue` 从持久 draft 帧重建解析器，带 `ReserveInput` 与「(续写)」消息继续。
- **再生**：`BranchService.DeriveTurn` 以目标节点的父节点为新基线派生；纯再生会复用原 roll receipt（`ReuseRollID`）。
- **编辑**：玩家输入编辑 = 派生新回合；助手文本编辑 = 降级为纯叙述（丢弃 proposals，零状态变化），经 `SubmitBlocks` 提交。
- **恢复**：`TurnService.Recover`（启动时，锁之后）：清理终态回合的分支锁 → 对非终态回合重建 draft（逐帧校验哈希）→ 完整则重新提交（幂等），可解析但无 final 则转 `awaiting_continuation`，否则 `turn.failed PROCESS_INTERRUPTED`。恢复期间不触发压缩/反思。

---

## 5. 提示词系统

### 5.1 提示词都定义在哪

全部是 Go 字符串常量/渲染函数（**没有** `.md`/`text/template` 提示词文件）。核心在 `internal/context/`：

| 部分 | 符号 | 位置 |
|---|---|---|
| 扮演准则 | `NarratorRoleInstruction` | `context/director.go` |
| 输出协议 | `FrameProtocolInstruction` | `context/prompts.go` |
| 边界说明 | `ExternalBoundaryInstruction` | `context/external_content.go` |
| 卡片 UI 契约 | `CardUIContract` | `context/cardui.go` |
| 状态账本 | `RenderDomainLedger` | `context/ledger.go` |
| 导演指令 | `DirectorInstructionHead/Mid/Tail` | `context/director.go` |
| 压缩摘要 | `CompactionSystemPrompt` | `context/summarizer.go` |
| 导演编排 | `directorPlanningInstruction` | `application/director.go` |
| 后台心智提取 | `cognitiveInstruction` | `application/cognitive.go` |
| 探测 | `FrameProbeInstruction` / `probe()` | `context/prompts.go` / `application/providers.go` |

生成物 `docs/PROMPTS.md` 由 `scripts/prompt_docs.py` 生成，`scripts/tests/test_prompt_docs.py` 防漂移（**注意**：该脚本的模板表是硬编码的，且缺失的 `prompt_fingerprint_test.go` 导致指纹无回归守卫，见 §17）。

### 5.2 组装顺序

`buildMessages`（`prompts.go`），两种布局由 `SplitDynamicContext`（默认 true）控制：

- **拆分模式**（利于 KV cache）：`system`（静态前缀）→ 可选开场白 `assistant` → 逐轮 `user`/`assistant` → 本轮 `user`（输入 + 指令块）→ 末尾 `user` `<system-reminder>` 状态块。
- **合并模式**：单个 `system`（含账本/世界书/记忆/状态/变量）。

静态前缀 `staticSystemPrompt` 顺序：扮演准则+协议 → 动作引用 → 边界说明 → 卡片 UI 契约 → 角色档案（不含 post_history）→ 玩家身份。
尾部 `tailStatusBlock` 顺序：历史缺口提示 → 动态上下文 → 角色锚点（post_history）→ 导演指令（最后，最靠近生成）。

动态上下文 `dynamicContextPrompt` 顺序：账本 → 世界书 → 记忆 → 检定 → 秘密 → 摘要 → 角色状态 → 会话变量。

### 5.3 角色档案与边界

- 角色档案（`renderCharacterProfiles`）：`description`、`personality`（性格与说话方式）、`scenario`（所处情境）、`mes_example`（权威语气样本）、`system_prompt`（作者补充设定）、`post_history_instructions`（拆分模式移到尾部锚点）。每段用 `<external_content source="character_card" trust="untrusted">` 包裹。
- 边界标记：`<external_content source=… trust=untrusted|derived>`、`<system-reminder>`、`<player_note>`。正文里的边界标签会被 `neutralizeBoundaryTags` 改写为 `[tag]` 防注入。

### 5.4 输出协议与解析

**协议**（`FrameProtocolInstruction`）：逐行 JSON，每行一个对象；正文块帧 `{"v":1,"seq":N,"type":"block","kind":"narration|dialogue|inner_monologue","speakerId":null|"角色ID","text":"…"}`；最后一帧 `{"v":1,"seq":M,"type":"final","proposals":[],"options":[]}`。

- 限制：`v=1`；`seq` 从 1 连续递增；块 ≤2048 runes；选项 ≤4，`intent ∈ aggressive|clever|emotional|chaotic`；单帧 ≤64 KiB；整回合 ≤2 MiB；JSON 深度 ≤16。
- `proposals` 类型：`relationship_delta`、`mood_set`、`memory_add`、`promise_propose`、`goal_set`（字段名严格，不接受别名）。
- 选项是**展示层**：`normalizeOptionFrames` 会截断/去空/纠正 intent/补 id，不因选项问题否决 final；而 proposals 严格（重复 `proposalId` 直接拒绝）。

**解析器**（`internal/protocol/parser.go`）：

- 模式 `structured` / `narrative`。
- 容忍 `\r`、空行、markdown 围栏、一行多 JSON。
- `seq` 必须连续，否则 `non-contiguous block seq`；`final` 之后再有帧 → `ErrAfterFinal`。
- 结构化模式缺 `final` → `ErrMissingFinal`。
- **叙述回退**：首个非空行不是 `{` 且尚未发布帧 → 整段降级为叙述（原始文本成为 narration 块，空 proposals/options）。以 `{` 开头但解析失败是硬协议错误，不回退。
- `extractInFlightBlock` 提供进行中块的原子快照，供重连/刷新。

### 5.5 提示词版本

`internal/context/prompt_version.go`：`PromptVersion = 7`；`PromptFingerprint()` = 对所有注册模板按 `name\0text\0` 求 SHA-256 取前 12 位；`PromptManifest() = "v7/<fp>"`（当前 `v7/db1270b985da`）。

- 注册模板：`narrator_role`、`frame_protocol`、`frame_probe`、`external_boundary`、`director_head/mid/tail`、`summary_prompt`、`system_reminder`、`external_content`、`player_note`、`options_always/never`、`character_anchor`、`card_ui_contract`。
- 任何**模型可见内容**的改动都要升版本；注释/改名不必。
- 记录点：启动日志、CLI 版本横幅、`GET /api/status`。**每回合用量表记录的是供应商配置指纹，不是提示词版本**。

### 5.6 选项生成的提示词

- `auto`（默认）：协议里内建「只在关键节点给选项」规则。
- `always` / `never`：`turnDirectiveBlock` 在本轮输入后追加指令（`OptionsAlwaysDirective`/`OptionsNeverDirective`），不写入历史以保持前缀稳定。
- 服务端硬保证：`never` 强制清空；`auto` 在上一轮已有选项时抑制（防止「选项变固定配菜」）；`always` 保留。

---

## 6. 世界书（Lorebook）

### 6.1 数据模型

`domain.LorebookEntry`：`entryId`、`title`、`keys[]`、`secondaryKeys[]`、`content`、`enabled`、`constant`、`order`、`position`。

**注意与 SillyTavern 的差距**：没有 `recursive`、`probability`、`priority`、`sticky/cooldown`。激活只有：常驻优先 + 关键词子串；`order` 是唯一排序；`position` 仅保真记录（所有条目注入同一个 `【世界设定】` 块）。大小写不敏感（两侧转小写）。

### 6.2 载入与冻结

**没有独立世界书库**。世界书只随卡片存在，并在会话创建时冻结：

1. `buildTemplateSnapshot` 把 `card.LorebookRefs` 序列化为 `TemplateVersion{Kind:TemplateLorebook}`，并在根节点快照写 `lorebook.templateVersionId`。
2. 编译期从**根节点快照**读回（永不读卡片文件）。
3. 另建 FTS5 索引（`lorebook_sources`/`lorebook_entries`/`lorebook_fts`），按 `template_version_id` 作用域，`bm25(title,keys,content)` 排序。**秘密从不进索引**。

`GET /api/v1/sessions/{id}/lorebook`：`q`（≤512 runes）、`offset`、`limit`（默认 40，上限 100）→ `LorebookPage{entries:[{bookIndex,entryIndex,bookName,entry}], hasMore, nextOffset}`；无世界书返回空页。

### 6.3 激活规则（`context/lorebook.go`）

- **两轮**：先注入所有 `constant` 条目（不依赖关键词），再关键词命中；共享预算与去重（同一 `entryId` 只注入一次）。
- 排序：世界书声明顺序 → 条目 `order` 升序（同 order 保持声明顺序）。
- 命中：任一 `key` 作为子串出现在 haystack；若配置 `secondaryKeys`，需主词命中且至少一个次词命中。
- 短 key 忽略：`< 2` runes 的 key 不参与（单字 key 噪声太大）；次词全被忽略时视为未配置。
- 预算：`MaxLorebookEntries=6`、`MaxLorebookRunes=1800`（按 runes）；超预算的条目跳过，但后续更短条目仍有机会命中。
- **无递归**：单趟，注入内容不再被扫描。
- 渲染：`【世界设定】` + 每条 `<external_content source="lorebook" trust="untrusted">`。

---

## 7. 世界状态与规则判定

### 7.1 WorldState

`domain.WorldState`：`relationships`（key=角色ID）、`items`、`promises`、`moods`、`goals`、`scene`、`milestones`、`unlockedSecrets`、`characters`、`variables`。

- 关系值 `{affection(-100..100), trust(0..100), alertness(0..100)}`；每回合每角色聚合增量 ±10 封顶。
- 物品 `{instanceId,templateId,name,aliases,ownerId,locationId,quantity,protection,keepsake,unique}`；信物槽每位最多 3。
- 誓言状态 `proposed|active|fulfilled|broken|cancelled`。
- 状态哈希 `HashID = "v2:"+sha256(json)`（`unlockedSecrets` 排序压缩），`String()` 已废弃（会漏 Characters/Variables，不可用于比较）。

### 7.2 状态变更：事件驱动

模型只发 `proposals`，由 `buildPlan` 校验为 `domain.DomainEvent`，再用**单一 reducer** `domain.ApplyEvents` 应用（与重放同一套逻辑）。

事件类型：`relationship_delta`、`mood_set`、`goal_set`、`memory_add/revised/pinned/hidden`、`promise_propose/settle`、`item_transfer/consume/grant`、`scene_propose`、`milestone_propose`、`secret_unlock`、`configuration_change`、`variable_set`。

- 记忆事件对世界状态是 no-op（认知与状态分离）；`director_change` 对世界哈希也是 no-op。
- 玩家可读摘要 `StateChange{kind,label,text,delta}` 仅由**已应用事件**派生（`SummarizeChanges`），记忆/导演变化不计入。

### 7.3 规则与检定

- `domain.Ruleset{Version, Actions}`；`ActionRule{ActionID,Label,Attribute,DC,Requires,Consequences,PermanentEffects}`；未登记即未授权。`DefaultRuleset()` 提供 `check.strength/dexterity/wisdom/charisma/luck`。
- 受限声明式 AST（无 eval）：`and/or/not/cmp/hasItem/hasMilestone/hasSecret`；字段白名单；深度/节点上限。
- 检定 `CheckResult`：`d20 + floor((属性-10)/2)`；判定顺序：natural 20 → crit；natural 1 → crit-fail；`total ≥ DC+10` → crit；`total ≥ DC` → success；`total ≤ DC-10` → crit-fail；否则 failure。
- `RollID = hash(baseHeadID, actionID, rulesetVersion)` **确定性**——文本再生不重掷；`Recheck` 混入 turnID。
- 时序：选项带 `actionRef` 时，**受理时**掷骰并存 `prepared` receipt；提交时效果转为权威事件；模型自发的物品提案无匹配 receipt 会被 `OPERATION_REJECTED`。
- 结果进入 `TurnContent.Checks` 与上下文 `【检定结果】`，并进入历史账本（永久效果/暴击/高 DC 失败，最多 10 条）。

### 7.4 秘密

- 定义来自卡片（冻结）：`SecretItem{SecretID,Title,Content,RevealWhen,Order}`；`RevealWhen` 是声明式规则（nil = 仅显式解锁）。
- 解锁状态是分支局部的 `WorldState.UnlockedSecrets`。
- 提交时**在效果之后**评估 `RevealWhen` → 追加 `secret_unlock` 事件；模型无法自行解锁（其 `secret_propose` 被丢弃）。
- **时序**：第 N 轮解锁，内容最早第 N+1 轮进入上下文（编译用的是解锁判断前的基线状态）；编译层结构上看不到未揭示内容。
- 玩家侧 `SecretView`：未揭示只给标题，正文为空。

---

## 8. 记忆系统

### 8.1 数据模型

`domain.MemoryRecord`：`memoryId`、`sourceNodeId`、`kind`（`observed|reported|inferred|secret`）、`ownerIds`、`content`（1..1200 runes）、`entityIds`（≤16）、`confidence`、`importance`（1..10）、`pinned`、`supersedes`、`hidden`、`evidence`、`mergedFrom`、`secretId`、`subjectKey`、`createdTurn`、`validFromTurn`、`validUntilTurn`。

- **copy-on-write**：修订/置顶/隐藏从不改原记录，而是新建覆盖记录并 `supersedes` 指回。
- `subjectKey`：归一化主题键（点分小写），同一路径上最新值胜出。
- `MemoryView` = 记录 + `effective`（是否当前生效版本）+ `protected`。
- `MemoryEvidence{sourceNodeId, sourceQuote, reasoning, confidence, autoDowngraded}`。

> `kind=secret` / `secretId` 的管线存在但**当前无代码路径产出**（为导入包与未来卡片秘密提取预留）。

### 8.2 形成（两条路径）

**A. 同步提案**：模型在 final 帧发 `memory_add`（`processMemoryProposal`）——解析 kind/实体别名消歧/证据片段，`ValidateCognitiveMemory` 后生成 `mem_<nodeID>_<proposalID>`；`subjectKey` 命中旧事实时给旧记录设 `validUntilTurn`。

**B. 后台反思**（`application/cognitive.go`）：回合提交后异步触发，按 `(branchID,turnID)` 幂等；取最近 4 轮原文 + 活跃实体 + 状态增量 + 最多 12 条可编辑记忆，流式严格解码 `CognitivePlan`（`writeObserved`/`inferBelief`/`supersedeMemory`/`adjustRelationship`，每类 ≤8）；无反思 provider 时回退到确定性 organizer。

**证据核验**（`ValidateMemoryEvidence`，反幻觉核心）：`sourceQuote` 必须逐字出现在证据片段中，五层递进匹配（原样 → 去边界 → 去说话人前缀 → 标点归一 → 净字符 ≥8）。否则拒绝。置信度：high=0.95（引文 <6 字降为 medium 并标 `autoDowngraded`）、medium=0.75（无引文降为 low）、low=0.4。

**去重/合并/取代**：后台按 `kind:content` 去重；显式 `supersedeMemory`；`subjectKey` 冲突（`ApplyMemoryOverlays`，按 `validFromTurn→createdTurn→memoryId` 最新胜出）；organizer 合并生成聚合记录 + 隐藏覆盖（**从不物理删除**）。

### 8.3 投影

记忆挂在 `sourceNodeId`；路径可见性 = 源节点是查询节点的祖先。`ProjectedMemories` 用 CTE 从节点向上走到最近检查点（`memory_projection_snapshots`），并反连接掉被取代的记录，再 `ApplyMemoryOverlays`。

- 稀疏物化：`depth % 64 == 0`，且**在提交事务内**写（读路径零写入，有回归测试）。
- 读取路径 `TestProjectedMemoriesIsReadOnly` 守卫。

### 8.4 整理与配额

- 配额 `MemoryQuota = 80`；`MemoryUsage{limit,used,protected,headroom,tier,reclaimable}`。
- 档位：`headroom ≤5` Critical、`≤15` Degraded、`≤30` Notice、否则 Normal。
- `protected`：置顶 / secret / 引用有数量或保护的信物 / 引用活跃誓言。
- `PlanMemoryOrganization`：仅 Notice/Degraded/Critical 时整理；候选为非隐藏、非保护、importance ≤5、≤220 runes；贪心合并（同 kind、同 owner、相似度 ≥0.2、实体重叠）最多 5 条；合并内容为**抽取式**（只取原文），每合并一条释放 4 预算。
- `GateMemoryOrganization` 独立复核每次合并；`BuildOrganizedMemories` 生成 `mem_<node>_merge_<i>` 聚合 + 每条源一个隐藏覆盖。
- 回合提交时的同步配额（`applyMemoryQuota`）：used ≥80 或（≥75 且 observed 且 importance ≤5）时丢弃新记忆（保护记忆永远保留）。

`POST .../memories/organize` 返回 `{usage, merges:[{sources, content}]}`。

### 8.5 注入与排序

- 预算：`MaxMemories=5`、`MaxMemoryRunes=1200`、`MinMemoryBigramOverlap=2`。
- 候选召回（`RecallMemories`）是**多通道并集**：lexical（FTS5）、trigrams（CJK 回退）、short_hits、pinned、entity_hits、fallback（importance DESC）；每通道 LIMIT 200；路径可见性先过滤。
- 打分：`0.45*文本相关 + 0.25*实体重叠 + 0.20*重要性 + 0.10*新近度`；时间类输入（「最近/刚才/上次…」）改为 `0.30/0.20/0.20/0.30`。
- 新近度半衰期：importance ≤3 → 10 轮；≤7 → 30；否则 90。
- 未排序回退门：仅允许 `importance ≥7` 且最多 2 条。
- 注入记录：`TurnContent.InjectedMemories`（文本快照，≤240 runes）；只有**提交文本真正引用**的记忆才写 `memory_mentions`（注入本身不更新新近度）。

### 8.6 人工编辑

`PATCH .../memories/{memoryId}`，`MemoryPatch`（指针语义，nil=不改）：`content`/`pinned`/`hidden`/`subjectKey` + `expectedCharacterId`/`expectedHeadId`/`expectedVersion`/`idempotencyKey`。

- 幂等键 `"manual_"+hash(session,branch,key)`；重放同哈希返回既有批次，异哈希 `409`。
- 并发守卫：`HEAD_CONFLICT`。
- **copy-on-write**：沿取代链找到当前生效版本，新建记录（新 UUID，`supersedes` 指旧）；改 `content` 会清空 `evidence`（用户更正不需要模型引文支撑）。
- 隐藏记录仍可编辑（可恢复）。
- 事件类型按状态映射：`memory_revised`/`memory_hidden`/`memory_pinned`/`memory_add`。

---

## 9. 生成选项（Options）

- 模型 `Option{optionId,intent,text,actionRef?}`；`intent ∈ aggressive|clever|emotional|chaotic`；最多 4 条。
- 提示词：协议内建「只在关键节点给」，`always`/`never` 用指令块覆盖（见 §5.6）。
- `optionsMode` 校验：未知值**拒绝**（400），不是静默归一；`auto` 归一。
- 提交时硬保证：`never` 清空；`auto` 且上一轮有选项 → 抑制并记 `suppressedOptions`；`always` 保留。
- 玩家选择：客户端发 `input.kind="option"` + `optionRef{nodeId,optionId}`。
- 服务端校验（`validateTurnInput`）：选项必须来自当前分支头（允许中间隔 maintenance 节点，walk 上限 4096）；必须真实存在；**客户端改写文本会被降级为自由文本并丢弃 actionRef**；服务端始终用存储的选项文本。
- 过期 → `STALE_OPTION`（409，可重试：刷新后基于当前回合重选）。缺 `optionRef` 与「已过期」由应用层统一分类，语义一致。

---

## 10. 对话中的图片

### 10.1 标记与合法键

模型被告知只能用卡内已有的图片键，写 `<img cmd="资源键">` 或 `[IMG:资源键]`。编译期 `attachCardImageKeys` 附上具体键列表（`CollectImageKeys`，优先卡文已引用的键，上限 `ImageKeyLimit=80`）。

### 10.2 计划与生成

- **计划** `POST /api/v1/images/plan`（需 token）：输入 `{sessionId,branchId,nodeId,blockIndexes?,stylePreset?,characterName?}`；输出 `{prompt,negativePrompt,sdDataBase,cardId,refs:[{assetKey}]}`。流程：拼接待选块文本 → `imageExcerpt`（去括号/markdown、≤180 runes）→ 卡上下文（角色名 + `UIProfile.SdData`）→ prompt 顺序 `[Character: name, ] [sdData base, ] excerpt, style positive`。风格预设 `anime|fantasy|cinematic|realistic`（未知→anime）。
- **生成** `POST /api/v1/images/generate`（需 token）：`GenerateOptions{engine,prompt,negativePrompt,size,baseUrl,apiKey,model,quality,style}`；引擎 `openai`（默认 dall-e-3）/`sd-webui`（`/sdapi/v1/txt2img`）/`comfyui`（Qwen-Image workflow，轮询 history）；空 engine 按 baseUrl 推断。经 `endpointguard.Validate`（SSRF 守卫）。
- **存储**：`img_<unix>_<hex>`，扩展名按 MIME，原子写 `data/images/`；URL `/api/v1/images/{filename}`。
- **提供** `GET /api/v1/images/{filename}`：**hostOnly（无 token）**，`filepath.Base` + 拒绝 `..`（路径穿越守卫），`Cache-Control: private, max-age=31536000, immutable`。

> Web 前端**不调用** `/images/plan`，它有自己的 TS 提取器，只调 `/images/generate`；plan 端点是给其他客户端/Android 的统一契约。

### 10.3 正文中的图片引用

`TextBlock.Segments`（`InlineSegment{type:"text"|"image", text?, assetKey?}`）在读取时由 `attachInlineSegments` 填充；渲染端按 `/api/v1/cards/{cardId}/assets/{key}` 拼 URL。

### 10.4 三层图片检查（评测）

`scripts/functional_eval.py` 的 longplay 场景：① 资源清单（导入时 `assets[]` 非空、名称唯一、URL 指向 assets 路由、不泄漏 `embeded://`）；② 资源端点（真实 HTTP 取两次 200、非空、魔数有效、`Content-Type: image/*`、SHA-256 稳定；缺失 ref 必须 404）；③ 正文标记 vs 清单（自造键硬失败；无资源卡不得出现图片标记）。

---

## 11. 导演系统

导演是可选的叙事编排子系统：维护 beat 计划、草稿、请求。

- **模型**（`domain/director.go`）：`DirectorBeat`、`DirectorPlan`、`DirectorReport`（status `continue|completed|blocked`）、`DirectorState`（status `active|paused|completed`，`progress`）、`DirectorDraft`、`DirectorRequest`（status `generating|completed|failed|cancelled|interrupted`）。
- **领域逻辑**：`ValidateDirectorPlan`（32 beats，标题 200、指导 4000、指令 4000、标准 2000；已结算/当前 beat 不可改）；`ValidateDirectorEvidence`（≥6 字逐字引文，取自叙述/对白块）；单一 reducer `ApplyDirectorEvent`（`activate|pause|resume|rewind|complete|skip|report`）。
- **流程**：草稿 `PUT .../director/draft`（非头节点 `HEAD_CONFLICT`，版本 CAS）；讨论 `POST .../director/messages`（assist provider，3 分钟超时，`runDiscussion`）；命令 `POST .../director/commands`（`activate` 需草稿版本匹配，`RevisionID=PlanID=node.NodeID`）。
- **命令提交**：单事务写 `NodeKindDirectorEvent` 节点（不推进 turnNumber）+ `director_change` 事件 + 导演投影缓存 + 分支头 CAS + `session.updated` outbox。
- **请求事件流**：`GET /api/v1/director-requests/{requestId}/events`，事件 `director.started/completed/failed/cancelled/interrupted`；每分支同时只允许一个 `generating`（否则 `DIRECTOR_BUSY`）。
- **叙述期注入**：正常回合尾部注入 `directorPrompt`（只给当前 beat，不给未来 beat；引用用短 `rev_`/`beat_` 哈希；final 帧需带 `director` 字段报告进度）。
- **恢复**：启动时 `RecoverDirectorRequests` 把 `generating` → `interrupted` 并推 `director.interrupted`。

---

## 12. 语音 TTS

应用级朗读辅助，**不是卡片功能**。

- `GET /api/v1/tts/voices`（hostOnly）：6 个预设（Xiaoxiao/Yunxi/Yunjian/Xiaoyi/Ava/Andrew）。
- `POST /api/v1/tts/plan`（需 token）：计算**读什么/怎么读**（分段、情绪、缓存键），无音频 IO；`{sessionId,branchId,nodeId,blockIndexes?,mode(dialogue|full),director?,engine,voice,baseUrl,speed}` → `{segments:[{segmentId,speakerId,speakerName,text,emotion,contextSummary,cacheKey}], voiceHint}`。
- `POST /api/v1/tts/synthesize`（需 token）：真正合成；文本非空、≤1000 runes；失败 503 `TTS_UNAVAILABLE`。**返回原始音频字节**（`audio/mpeg`，非 JSON）。
- 缓存键：plan 侧 `sha256(engine:voice:speed:baseUrl:instruction:text)`；synth 侧内存 md5（额外含 model + 密钥哈希，≥500 条或 >64 MiB 整体清空）。
- 引擎：`openai`（`/audio/speech`）、`MiMo`（chat/completions 返回 base64 音频）、`edge`（WebSocket）。
- 播放：Web 取 Blob → object URL LRU（最多 20）→ `Audio`；失败回退 Web Speech API。Android 主要用系统 TTS。

---

## 13. 嵌入 Embeddings

- `POST /api/v1/embeddings/probe`：`{baseUrl,apiKey,model,sampleText}` → `{ok,model,dimension,latencyMs,preview,error}`（始终 200）。
- `POST /api/v1/embeddings`：`{texts[],baseUrl,apiKey,model,dimension}`；空 → 400，>128 条 → 400 `TOO_MANY_TEXTS`；→ `{model,dimension,embeddings:[[…]]}`；失败 503 `EMBEDDING_FAILED`。
- 端口：`ports.Vector`（`[]float32`）+ `EmbeddingProvider{EmbedText,EmbedBatch,Dimension}`；适配器 `adapters/embedding`（OpenAI 兼容，默认 Ollama `http://127.0.0.1:11434/v1`，model `bge-small-zh-v1.5`，dim 512）。
- **现状**：嵌入**不驱动任何运行时功能**（记忆检索用的是 FTS5 词法/三元组，不是向量）。原 `internal/search/hybrid.go`（RRF + 内存向量索引）从未接线，已删除；只保留端口、适配器与设置页「记忆向量」的探测/计算。

---

## 14. 模型供应商与配置

- **v2 原则**：连接信息挂在 **model 实例**上；slot 只引用 `modelId`。
- `ModelInstance`：`modelId/kind/baseUrl/model/apiKey(写入)/hasApiKey/temperature/maxTokens/contextWindow/reasoningEffort/inputBudget`。
- `SlotBinding`：`slot ∈ primary|assist|reflection` + `enabled` + `modelId`（内联连接信息 → `SLOT_TAKES_REFERENCE_ONLY`）。
- 支持的 kind：`openai-chat`、`openai-responses`、`anthropic-messages`；`mock` 是开发回退（不落盘）。
- 密钥：`data/config/secrets.json`（`map[ref]key`，`0o600`），读取优先级 secrets → `TAVERNAGENT_KEY_MODEL_<ID>` → `TAVERNAGENT_KEY_<SLOT>`；响应中一律掩码；先写 secrets 再写 settings。
- 接口：`GET/POST /config/models`、`PUT/DELETE /config/models/{id}`、`POST …/probe`、`GET/PUT /config/provider`、`POST /config/provider/probe`；错误 `MODEL_NOT_FOUND`(404)/`MODEL_IN_USE`(409)/`SLOT_TAKES_REFERENCE_ONLY`(400)。
- Web：`settingsStore` 用单次 `GET /config/models` 填充 models+slots；协议下拉 label 来自 `lib/modelLabels.ts`。

---

## 15. 上下文预算与压缩

- 预算：`inputBudget = contextWindow - reservedOutput - safetyMargin`；`safetyMargin = max(512, window/20)`；未知窗口 → 8192/2048。
- Token 估算：CJK 1 token/char，拉丁/数字/标点 3 chars/token，空白免费；带真实用量校准系数。
- 淘汰顺序（压力低→高）：世界书 → 记忆（低分先，从尾部丢）→ 非承重摘要 → 历史回合（最旧先，保护下限 `TailWindowTurns` 默认 20）→ **承重摘要不可丢**，超预算整回合失败 `422 CONTEXT_OVER_BUDGET`。
- 每阶段用二分找最小丢弃数；强制材料（规则/人设/状态/输入/账本）超预算立即失败。
- 丢弃历史时输出 `【历史缺口】` 标记，避免模型把「缺失」当「未发生」。
- 主动压缩阈值：窗口 80% 或输入预算 75%；压缩提示词产出 `<story_checkpoint>`（`narrative_arc`/`character_dynamics`/`open_loops`/`milestones`），严格校验（白名单层级、单段、非空），并做机械修复 `NormalizeSummaryXML`。

---

## 16. 契约与格式总表

| 契约 | 唯一源 | Go | Web | Kotlin | 校验 |
|---|---|---|---|---|---|
| 卡片标记语法 | `testdata/contract/markers.json` | `internal/cardmarkers` | `web/src/lib/cardMarkers.tsx` | 生成契约测试 | `gen_marker_contract.py --check` |
| SSE 事件名 | `testdata/contract/sse-events.json` | `internal/protocol/events.go` | `sseEvents.generated.ts` | `SseEventNames.kt` | `gen_sse_contract.py --check` |
| API DTO 字段 | `testdata/contract/api.json`（**待建**） | 反射断言（待建） | `keyof` 断言（待建） | serializer 断言（待建） | 待入 CI |
| 错误码 | 待建（现 Go `application/errors.go`） | — | — | — | 待入 CI |
| tacard 格式 | `docs/TACARD_SPEC.md` | `application/tacard.go` | — | — | `scripts/verify_all_tacards.py` |
| 提示词全文 | 源码常量 | `internal/context/*` | — | — | `test_prompt_docs.py` |

关键格式速查：

- **回合输出**：逐行 JSON 帧（block / final），`v=1`，`seq` 连续。
- **SSE 帧**：`event:` + `id:<aggregateId>:<sequence>`（仅持久事件）+ `data:`；心跳 `: ping`。
- **状态持久**：`state_snapshots`（每 20 层）+ `domain_events`（重放）+ `memory_projection_snapshots`（每 64 层）。
- **时间**：RFC3339(Nano)；**命名**：对外 camelCase（V2 卡片字段例外）；**ID**：见 `docs/spec/07-conventions.md`。

---

## 17. 已知差距与注意事项

1. **续写信号统一为 `turn.truncated`**：状态 `awaiting_continuation` 仍存在，但 `turn.awaiting_continuation` / `turn.awaiting_approval` 事件名已从契约与三端移除（后端从不产出）。
2. **SSE 负载字段未机器化**：只锁了事件名；负载靠散文/各端代码。`turn.committed` 的 live 唤醒负载是 `{}`，客户端必须读 `seq>0` 的持久事件。
3. **DTO 跨端漂移**：已落地 Go 代码生成（`cmd/gencontract`）第一批（Director*/ModelInstance/MemoryUsage）。TS 曾缺 `SessionView.capabilities`、`PlotNode.schemaVersion`、`WorldState.goals/milestones/unlockedSecrets`、`MemoryView.secretId` 等仍在，待覆盖 SessionView/Turn/WorldState/MemoryView（前置：把 44 处 `map[string]any` 响应重构成具名 struct）。
4. **错误码无共享枚举**；`/api/` 未知路径与 stdlib `404/405` 仍是纯文本，不符统一信封。
5. ~~`STALE_OPTION` 不一致~~（已收敛：缺 `optionRef` 与「已过期」统一为 409/可重试；`QUEUE_FULL`/`STORAGE_UNAVAILABLE`/`BUDGET_EXCEEDED` 的 retryable 已按 HTTP 语义修正）。
6. **提示词指纹无回归测试**：`internal/context/prompt_fingerprint_test.go` 被多处引用但**不存在**；`prompt_docs.py` 的模板表是硬编码，可能与源码漂移而不被测试发现。
7. **`MemoryQuota` 与 Web 夹具不一致**：实现是 80，`contract_fixtures.json` 写 200。
8. **嵌入未接线**：向量检索已删除，embeddings 仅配置/诊断。
9. **Web 不调用 plan 端点**：`/tts/plan`、`/images/plan` 只有 Android 契约消费者，Web 保留并行 TS 实现。
10. **Android 无分层检查**；`android/docs/API_CONTRACT.md` 无机器校验（纯散文）。
11. **缓存命中率**在本地网关不可测（无 `cache_read_input_tokens`），应用按设计记 `Cached=0`。

> 修复优先级与流程见 `docs/spec/README.md` 合规矩阵与 `docs/spec/14-docs-sync.md`。
