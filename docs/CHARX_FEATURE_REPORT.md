# charx / RisuAI 角色卡格式与特性调研报告

> 调研对象：`realm.risuai.net`（RisuRealm）公开角色卡 + 用户提供的示例卡
> 目的：评估 TavernAgent 在当前架构下能支持哪些 charx 特性
> 方法：调用 RisuRealm API 下载 `charx-v3`，解包 ZIP，解码 `module.risum`（RPack），逐卡统计

---

## 0. 结论摘要（TL;DR）

1. **charx 本质是一个"卡片即应用"的 ZIP 包**：`card.json`（`chara_card_v3` 数据）+ `module.risum`（可执行逻辑）+ `assets/**`（立绘/音频）+ `x_meta/**`（逐资源元数据）。
2. **`module.risum` 是 RPack 编码的二进制**，内含 **正则脚本、触发脚本（含 Lua）、世界书、模块资源、全局背景 HTML** 等。
3. 在我们抽样的 11 张卡（含用户那张）中：
   - **100%** 带 `module.risum`；
   - **100%** 用正则脚本（`editdisplay` 最普遍）；
   - **3/11 用 Lua 触发脚本**（真正的可编程逻辑）；
   - **2/11 用非 Lua 的 v2 触发脚本**（变量/条件/随机/改聊天）；
   - **8/11 注入全局 `backgroundHTML`**（自定义 UI 主题）；
   - **1/11 带 `lowLevelAccess=true`**（高风险能力开关）；
   - **1/11 含音频资源（mp3）**。
4. **Lua 的用途非常明确**：渲染自定义 UI（聊天气泡、状态 HUD）、响应按钮点击改聊天变量、解析模型输出、维护游戏状态。它不是"脚本玩具"，而是**在宿主里跑一个前端应用**。
5. 对 TavernAgent 的建议：**把 charx 当"表现层 + 上下文层的富集包"来支持，不把它当"插件运行时"**。低风险子集（世界书、`<img cmd>`、正则文本改写、深度提示、生图模板、情绪映射）可直接做；Lua/任意 HTML 属于代码执行层，需要独立沙箱与显式授权，建议单独立项。

---

## 1. 数据来源与获取方法

### 1.1 RisuRealm 下载 API

官方文档：`https://realm.risuai.net/help/api`

```
GET /api/v1/download/:format/:id
```

- `format`：`png-v2 | json-v2 | png-v3 | json-v3 | charx-v3`（角色）、`module-v1 | lorebook-v2 | lorebook-v3`（模块）、`preset-risu-v1 | preset-st-chat`（预设）。
- 查询参数：`cors`、`non_commercial`、`access_token`（默认 `guest`）。

**实测坑（重要）**：
- 带上 `non_commercial=true`（或 `cors=true`）反而返回 **403** `{"error":"Forbidden","message":"This card is not allowed to be downloaded in this format: "}`。
- **不带任何查询参数**时 `charx-v3` 正常返回 200。
- `json-v3`/`png-v3` 对这批卡普遍返回 **404 "Card not found"**——即这些卡**只以 charx 形式提供**，API 不做格式转换。
- 61 张首页卡中约 40 张可下（200），其余 403（作者禁用了 API 下载/授权限制）。
- 文档声称 `charx-v3` 返回 403 "not supported yet"，与实测不符（实测可下）；以实测为准。

### 1.2 module.risum（RPack）解码

`module.risum` 结构（来自 RisuAI `src/ts/process/modules.ts`）：

```
byte  0x6F           magic (111)
byte  0x00           version
uint32 LE            mainLen
mainData[mainLen]    RPack 编码的 JSON: { type:"risuModule", module:{...} }
repeat: byte mark(1=asset) + uint32 len + RPack(compressed asset) 直到 mark=0
```

RPack 是**单字节替换表**：`rpack_map.bin` 512 字节，前 256 为 encode 表、后 256 为 decode 表；`decoded[i] = decodeMap[raw[i]]`。拿到该 map 即可离线解出 `module.json`。

本次使用的分析工具（临时，已移除）：
- `cmd/charxreport`：批量统计 card.json + module 特性；
- `cmd/charxdump`：导出单张卡的完整 card.json / module.json。

---

## 2. charx 容器格式解剖

一个典型 charx（ZIP）的目录：

```
card.json                    # chara_card_v3 信封（数据层）
module.risum                 # RPack 二进制模块（逻辑层，可选）
assets/icon/image/*.png      # 图标/头像资源
assets/other/image/*.webp    # 立绘 / 场景 / 贴纸（也可含 png / mp3）
x_meta/*.json                # 每个资源的生成元数据（NovelAI prompt 等）
```

`card.json` 关键字段（与 TavernAgent 现有关注点相关）：

| 字段 | 说明 |
|---|---|
| `spec` / `spec_version` | 恒为 `chara_card_v3` / `3.0` |
| `data.name/description/personality/scenario` | 基础人设 |
| `data.first_mes` / `alternate_greetings` / `group_only_greetings` | 开场 |
| `data.mes_example` / `system_prompt` / `post_history_instructions` | 提示词（本批 `system_prompt` 全为空，`post_history` 大量使用） |
| `data.character_book.entries` | 卡内世界书 |
| `data.assets[]` | `{type, uri, name, ext}`；`type` 见下 |
| `data.extensions.risuai` | RisuAI 扩展（见下） |
| `data.extensions.depth_prompt` / `pngExif` | 深度提示 / PNG 元数据 |

`data.assets[].type` 取值：
- `icon`：头像/图标（`name="main"` 为主头像）。
- `x-risu-asset`：RisuAI 附加资源（立绘、贴纸、场景、音频），`name` 即正文指令键。
- `emotion`：情绪立绘（V2 体系常见）。

`data.extensions.risuai` 常见键（本批出现）：
`bias, viewScreen, utilityBot, sdData, backgroundHTML, additionalText, virtualscript, largePortrait, lorePlus, inlayViewScreen, newGenData, vits, lowLevelAccess, defaultVariables, prebuiltAssetCommand, prebuiltAssetExclude, prebuiltAssetStyle, toggles, hideChatIcon`。

---

## 3. 样本总览（11 张）

> `R`=正则数，`T`=触发数，`L`=模块世界书条目，`BG`=backgroundHTML 字符数，`DV`=defaultVariables 字符数，`LLA`=lowLevelAccess，`Lua`=是否有 triggerlua。

| 卡（截断 ID） | 大小 | 资源 | 资源类型 | 卡内书 | R | T | L | BG | DV | LLA | Lua |
|---|---|---|---|---|---|---|---|---|---|---|---|
| 出包王女 中文模擬器 `b7b51ed6` | 97.6 MB | 684 | icon1 + x683（png598 / webp3 / **mp3 83**） | 29 | 74 | 28 | 29 | 38434 | 3235 | false | 否 |
| `d173c566` | 92.2 MB | 752 | icon4 + x748（webp748 / png4） | 8 | 2 | 2 | 8 | 14391 | – | false | 否 |
| **用户示例**（魔法革命） | 84 MB | 719 | icon7 + x712（webp/png） | 1 | 4 | 2 | 13 | – | – | false | 否 |
| Ceres `80ede2e0` | 22.7 MB | 189 | icon5 + x184（webp184 / png5） | 17 | 4 | 1 | 17 | 15909 | 9 | false | **是** |
| LUMEN `9eda6a94` | 10 MB | 87 | icon2 + x85（webp85 / png2） | 30 | 9 | 1 | 30 | – | – | false | **是** |
| Z46 `0072b807` | 7.7 MB | 68 | icon1 + x67（png67） | 0 | 1 | 2 | 0 | 4443 | – | false | 否 |
| Go Eun-bi `fff0461b` | 7.6 MB | 130 | icon1 + x129（png130） | 27 | 8 | 2 | 27 | 943 | – | false | 否 |
| Political Hostage `2548f04f` | 5.7 MB | 72 | icon2 + x70（png72） | 36 | 23 | 9 | 36 | 1681 | 787 | false | 否 |
| Sandthrower `03e7f8a1` | 0.8 MB | 2 | icon2 | 29 | 16 | 2 | 29 | 29636 | – | **true** | 否 |
| `7090d0e0` | 0.8 MB | 7 | icon7 | 7 | 0 | 2 | 7 | – | – | false | 否 |
| Classroom of the Elite `04bfbe59` | 0.5 MB | 1 | icon1 | 43 | 18 | 1 | 43 | 30501 | 44 | false | **是** |

> 另有 16 张首页卡 API 返回 403（授权限制），未纳入统计。

---

## 4. 特性频率与分类

### 4.1 资源系统（assets）
- 全部为 `chara_card_v3.data.assets`，键名即正文指令；模型输出 `<img cmd="...">` / `<img="...">`（两种写法都存在）。
- 规模跨度极大：从 1 张图标到 **752 张立绘**。
- 类型：`icon`（头像）、`x-risu-asset`（立绘/场景/贴纸/音频）。
- 扩展名：`webp`（主流）、`png`、**`mp3`（音频，To Love-Ru 83 个）**。
- 变体命名：`{prefix}_{outfit}_{expr}{1,2,3}`，模型常只写不带数字的键（我们已做模糊匹配）。

### 4.2 正则脚本（`module.regex` / `extensions.risuai.customScripts`）
**出现率 100%**，类型分布：

| 类型 | 作用 | 本批次数 |
|---|---|---|
| `editdisplay` | 改写**显示**（注入 HTML/CSS，构建自定义 UI） | 最多（单卡最高 37 条） |
| `editoutput` | 改写**模型输出**（落档前，如追加 `{{roll::3}}`、剥离标记） | 次多 |
| `editprocess` | 改写**发送给模型的内容**（隐藏标记） | 少量 |
| `editinput` | 改写**用户输入** | 少量 |
| `edittrans` | 翻译相关 | 少量 |
| `disabled` | 停用占位 | 少量 |

其中带 HTML 输出的（`regexWithHTML`）在 8/11 张卡里出现——即**正则直接往消息里注入 HTML**。

### 4.3 触发脚本（`module.trigger`）
两种形态：

**(a) 非 Lua 的 v2 指令图**（RisuAI 内置触发效果，无代码执行）：
- 事件类型：`start`、`input`、`output`、`manual`、`display` 等。
- 效果类型（本批出现）：`v2Header`、`v2SetVar`、`v2If`/`v2Else`/`v2IfAdvanced`、`v2Random`、`v2EndIndent`、`v2LoopNTimes`、`v2BreakLoop`、`v2ConcatString`、`v2Wait`、`v2StopTrigger`、`v2ModifyChat`、`v2GetLastMessage`/`v2GetLastUserMessage`/`v2GetMessageAtIndex`/`v2GetMessageCount`、`v2Impersonate`、`v2GetAlertInput`、`v2SetLorebookActivation`/`v2GetLorebookIndexViaName`。
- To Love-Ru 单卡就有 **28 个触发**、`v2SetVar 230` 次、`v2If 81` 次、`v2Random 31` 次——**用可视化指令图实现了一套游戏状态机**。

**(b) Lua 触发脚本**（`effect.type = "triggerlua"`，`code` 字段是 Lua 5.4 源码）：
- 出现率 **3/11**。
- 详见第 5 节。

### 4.4 全局背景注入（`extensions.risuai.backgroundHTML`）
- 出现率 **8/11**，长度 943–38434 字符。
- 内容通常是 `<style>` 主题变量 + 背景纹理 + 全局组件样式（聊天卡片、状态条外观）。
- 与 `module.backgroundEmbedding` 同源（后者本批为空，RisuAI 会把 module 的合并进卡片）。

### 4.5 其他扩展
- `defaultVariables`：4/11 非空（44–3235 字符），预置聊天变量初值。
- `sdData`：Stable Diffusion 提示词模板（`solo, 1girl` + 外貌/姿势/情绪/地点占位）。
- `viewScreen`：`none` / `emotion` / `imggen`（情绪立绘 / 生图视图）。
- `lowLevelAccess`：**仅 1/11 为 true**（Sandthrower），其余 false。
- `toggles` / `customModuleToggle`：模块开关定义（本批 `customModuleToggle` 为空）。
- `vits`：存在键但全为空（无 VITS 语音模型）。
- `cjs` / `mcp` / `namespace`：**本批全部未使用**。

### 4.6 世界书
- 卡内 `character_book.entries`：0–43 条。
- 模块 `lorebook`：0–43 条，常与卡内重复（模块导出时复制了一份）。
- 条目普遍带 `<lore name="...">` 包裹、`@@depth`/`@@role` 等指令、folder 分组。

---

## 5. Lua 专题：它到底在干什么

RisuAI 的 Lua 是**沙箱内的 Lua 5.4**（内置 `json.lua`，禁文件/网络）。通过 `effect.type="triggerlua"` 挂到触发事件上，并可与 HTML 按钮的 `risu-trigger="函数名"` 联动。

### 5.1 三种真实用途（来自本批样本）

**用途 A — 自定义 UI 渲染（LUMEN `9eda6a94`）**
```lua
listenEdit("editDisplay", function(triggerId, data)
  local html = data:gsub('<talkroom([^>]*)>(.-)</talkroom>', tr_block)
  html = renderCallsTolerant(html)
  html = html:gsub('</?callend>', renderCallEnd)
  return html
end)
```
把模型输出的 `<talkroom>...</talkroom>` 渲染成**完整聊天软件界面**：头像、名字标签、气泡（区分 user/角色）、群聊/私聊判定、`<call>`/`<callend>` 语音/视频通话卡、`<pic:speaker:kind>` 图片气泡。颜色/皮肤全部写死在 Lua 里。

**用途 B — 交互控件 + 聊天变量（LUMEN / Ceres `80ede2e0` / Classroom `04bfbe59`）**
```lua
function lm_fm_go_1(id) setChatVar(id, 'lm_fm', '1'); reloadDisplay(id) end
-- 配合 editdisplay 生成的 <button risu-trigger="lm_fm_go_1"> 首消息选择器
```
```lua
-- Ceres：语言 + 开场场景选择
function LNG_SetLang_KO(trigger_id)
    setChatVar(trigger_id, "lang", "KO")
    setChatVar(trigger_id, "num", "")
    reloadDisplay(trigger_id)
end
```
HTML 里的按钮点击 → 调用 Lua 函数 → `setChatVar` 持久化 → `reloadDisplay` 重渲染。**这就是"轻 UI 的交互"落地方式**。

**用途 C — 解析模型输出、维护游戏状态（Classroom `04bfbe59`）**
```lua
function onOutput(triggerId)
    local message = getCharacterLastMessage(triggerId)
    parseAndSaveDate(triggerId, message)   -- 解析正文里的日期，setState/setChatVar
end
```
```lua
function ImgHoverOn(triggerId)  coteSettingSet(triggerId, "IMG_HOVER", "1")  end
function ImageLimitOn(triggerId) coteSettingSet(triggerId, "IMAGE_LIMIT", "1") end
```
读最后一条消息、解析日期、写状态；用按钮切换 UI 开关。该卡描述里的"**Lua 先掷骰，AI 不能看到结果再改**"即此类逻辑。

### 5.2 Lua 暴露的宿主 API（从样本归纳）
`getChatVar` / `setChatVar`、`setState`、`reloadDisplay`、`getCharacterLastMessage`、`listenEdit("editDisplay"/"editInput"/"editOutput"/...)`，以及 `onStart` / `onInput` / `onOutput` 回调。

### 5.3 与"非 Lua v2 触发"的关系
- v2 指令图：**无代码**，用内置效果组合，能改变量/条件/随机/改聊天，但表达力受限、体量大（To Love-Ru 用 28 触发堆出游戏）。
- Lua：**有代码**，表达力强、体积小，能直接生成 HTML，但需要解释器沙箱。

---

## 6. 典型玩法案例

### 案例 1：To Love-Ru 中文模擬器（`b7b51ed6`，97.6MB）
- 684 资源（598 立绘 + **83 音效**），74 正则，28 触发（16 output / 10 manual / 2 start）。
- 触发用 `v2SetVar`×230、`v2If`×81、`v2Random`×31 构建**好感系统 / 多角色同屏 / 动态背景与音乐**。
- `backgroundHTML` 38KB 定义全局 UI。
- 无 Lua——纯 v2 指令图也能做 galgame，但极其臃肿。

### 案例 2：LUMEN（`9eda6a94`，10MB）
- Lua 渲染**聊天软件界面**（群聊/私聊/通话）+ 首消息**多场景选择器**。
- `editdisplay` 生成选择器 HTML，Lua 处理点击与变量。

### 案例 3：Ceres（`80ede2e0`，22.7MB）
- `editdisplay` 用**内联 SVG 画"纸娃娃"状态 HUD**，模型输出 `[CeresStatus:date=…|marks=…]` 标签驱动身体部位标记高亮。
- Lua 处理语言/场景选择与 `setChatVar`。
- 是"模型输出结构化标签 → 前端渲染为游戏 HUD"的典范。

### 案例 4：Survive as a Political Hostage（`2548f04f`，5.7MB）
- 纯 v2 触发（9 个）：`v2SetVar`/`v2If`/`v2Impersonate`/`v2GetAlertInput`/`v2SetLorebookActivation`。
- 实现时间循环、关系、状态窗口；无 Lua。

### 案例 5：用户示例（魔法革命，84MB）
- 719 资源 + 4 正则（1 个 `editdisplay` 立绘相框 + 3 个 `editoutput` 变体随机）。
- 触发为**空壳**，无 Lua，无 backgroundHTML——属于"轻量"一端。

---

## 7. 与 TavernAgent 现状对照

| charx 特性 | TavernAgent 现状 | 可行性 | 风险 |
|---|---|---|---|
| `chara_card_v3` 基础字段 | ✅ 已解析 | — | 低 |
| `assets` + `<img cmd>` 渲染 | ✅ 已支持（含变体匹配） | — | 低 |
| 世界书关键词注入 | ✅ 已支持（预算/消融） | — | 低 |
| `post_history_instructions` | ✅ 已注入（尾部锚点） | — | 低 |
| `depth_prompt` | ❌ | ✅ 易（编译层） | 低 |
| `sdData` 生图模板 | ❌ | ✅ 易（复用 image service） | 低 |
| 情绪立绘 `viewScreen=emotion` | ❌ | ✅ 易（映射到现有 mood/关系） | 低 |
| CBS 安全子集（`{{roll}}`/变量/`{{#if}}`） | ❌（仅 `{{char}}/{{user}}`） | ✅ 中 | 低–中（随机需播种以保回放） |
| 正则 `editinput`/`editoutput`/`editprocess`（纯文本） | ❌ | ✅ 中 | 中（改写会污染存档/记忆引文，宜限"展示层"） |
| 正则 `editdisplay`（受控模板，禁任意 HTML） | ❌ | ⚠️ 需设计安全模板层 | 中–高（XSS） |
| `backgroundHTML`（仅 CSS/主题子集） | ❌ | ⚠️ 可做受限版 | 中–高 |
| `defaultVariables` / `toggles` | ❌ | ✅ 数据层 + 轻 UI | 低 |
| v2 触发指令图 | ❌ | ⚠️ 可做映射子集 | 中（状态机与因果树冲突） |
| **Lua 触发脚本** | ❌ | ⚠️ 需嵌入 Lua 5.4 沙箱（Go: `gopher-lua`） | **高**（代码执行/不确定性） |
| `lowLevelAccess` 能力（调 LLM/网络） | ❌ | ❌ 不建议 | 高 |
| CJS 代码 | ❌ | ❌ RisuAI 自己已移除 | 高 |
| VITS TTS / MCP | ❌ / 部分 | 大工程 / 已有 MCP 适配器 | 中 |

### 核心冲突（必须正视）
TavernAgent 的立身之本是**确定性因果记录**（CoW 因果树、`重生成不重掷`、记忆引文可溯源）。而 charx 的脚本会：
- **任意时机改消息/变量/世界书** → 破坏分支隔离与回放；
- **每次渲染重掷** → 同一节点回看不一致；
- **执行第三方代码/注入 HTML** → 攻击面。

因此正确的接入姿势是**分层**：
- **表现层**（立绘、主题、HUD、相框）：接受"意图"，用 TavernAgent 自己的组件实现，**不执行卡里的任意 HTML/CSS**。
- **上下文层**（世界书、depth_prompt、sdData、变量初值）：安全，直接映射。
- **逻辑层**（Lua/v2 触发改状态）：默认不做；若要，须独立沙箱 + 显式授权 + 结果不入因果存档。

---

## 8. 实现建议与优先级

> 实施状态以 [CARD_UI_MARKERS.md](CARD_UI_MARKERS.md) 为准（该文按「卡片 UI 映射 M2–M4」跟踪）。
> 截至目前：P0 的 `sdData` 生图基底、情绪立绘、CBS 只读子集 `{{getvar}}` 已实现；
> P1 的 `defaultVariables`/`toggles`（并入会话变量）已实现，`backgroundHTML` 主题 token 与
> 标记驱动的受控 UI 已实现；`depth_prompt`、CBS 写/随机子集、文本类正则与 Lua/v2 触发仍未做。

**P0（低风险，性价比高）**
1. `depth_prompt` 注入（编译层，一天级）。
2. `sdData` → 生图默认模板（复用 `internal/adapters/image`）。
3. 情绪立绘：`viewScreen=emotion` / `assets[type=emotion]` → 用现有心情/关系自动选图。
4. CBS 安全子集：`{{roll::N}}`（**按回合播种**以保回放）、`{{random}}`、`{{getvar/setvar}}` 会话变量、`{{#if}}`。

**P1（中风险，需设计）**
5. 文本类正则（`editinput`/`editoutput`/`editprocess`）：只在不落档的环节生效，带长度/次数上限。
6. `defaultVariables` / `toggles`：数据层 + 设置面板。
7. `editdisplay` **受控模板层**：识别卡作者的"意图"（相框/HUD/折叠面板），映射到我们的组件，而非执行任意 HTML。

**P2（高风险，单独立项）**
8. Lua 触发运行时：`gopher-lua` 沙箱（禁文件/网络、指令数/时限、白名单 API），默认关闭、显式授权、**不写因果存档**。
9. v2 触发指令图子集：优先做只读/展示类效果，缓做改状态类。

**明确不做**
- 任意 `editdisplay`/`backgroundHTML` 的 HTML/JS 注入；
- CJS；
- `lowLevelAccess` 允许的任意 LLM/网络调用。

---

## 9. 附录

### 9.1 charx 目录结构
```
card.json                    # chara_card_v3
module.risum                 # RPack: magic 0x6F, ver 0, uint32 len, decodeMap JSON + assets
assets/icon/image/*          # 图标
assets/other/image/*         # 立绘/场景/贴纸（webp/png/mp3）
x_meta/*.json                # 逐资源元数据（NovelAI prompt 等）
```

### 9.2 module.risum 关键字段（`module` 对象）
`name, description, id, trigger[], regex[], lorebook[], assets[], cjs, backgroundEmbedding, customModuleToggle, lowLevelAccess, namespace, mcp, hideIcon`

### 9.3 触发事件类型
`start, input, output, manual, display, request, ...`

### 9.4 triggerlua 宿主 API（样本归纳）
`getChatVar, setChatVar, setState, reloadDisplay, getCharacterLastMessage, listenEdit(editDisplay/editInput/editOutput/...), onStart, onInput, onOutput`

### 9.5 复现命令
```powershell
# 下载（注意：不要带 non_commercial / cors 参数）
curl.exe -sL -o card.charx "https://realm.risuai.net/api/v1/download/charx-v3/<id>"
# 解码 module.risum 需要 rpack_map.bin（RisuAI 仓库 src/ts/rpack/rpack_map.bin）
```

### 9.6 抽样清单
见第 3 节表格（10 张可下载卡 + 用户示例；另 16 张 403 未纳入）。
