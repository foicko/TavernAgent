# 卡片界面标记（Card UI Markers）

> 目的：把角色卡（尤其是 charx / RisuAI 卡）自定义的状态/场景标记，归一到一组稳定的、
> 语言中立的标记上，由 TavernAgent 自己的原生组件渲染——**不执行卡内的 HTML/CSS/JS/Lua**。

## 1. 为什么需要它

charx 卡常用 `editdisplay` 正则或 Lua 把模型输出变成自定义 UI（状态 HUD、场景选择器、
聊天界面等）。直接执行卡内代码会破坏本应用的确定性因果记录，并带来 XSS / 代码执行面。
本方案只接受卡的**意图**，不接受卡的**代码**：把卡的标记语法翻译成一组标准标记，
各端用原生组件渲染。

原生 `.tacard` 卡（见 [TACARD_SPEC](TACARD_SPEC.md)）直接以这组标记为标准语法：
`[IMG:id]` 引用 `assets` 清单中的资源 id，`[HUD]/[SCENE]/[VAR]` 承载状态；导入时其
`ui_contract.default_variables` 作为会话变量初值、`theme_tokens` 作为卡片主题 token。

## 2. 语法（v1）

标记写在模型输出的 `block` 帧 `text` 里（不是 `final` 帧）。

| 标记 | 含义 | 类别 |
|---|---|---|
| `<img cmd="key">` 或 `[IMG:key]` | 卡内资源图片 | 表现 |
| `[HUD:名称=值\|名称=值]` | 状态面板 | 状态 |
| `[SCENE:地点或场景标题]` | 场景 | 状态 |
| `[VAR:键=值\|键=值]` | 会话变量（正文不展示） | 状态 |

- 键值用 `|` 分隔、`=` 连接；值内不要出现 `[` `]` `|` 或换行。
- `[HUD]` 的值写 `n/m` 或 `n%` 时渲染为进度条（如 `理智=82/100`、`体力=40%`）；其余为纯文本。
- `[SCENE]` 只做**展示**（常驻场景标签 + 该回合的场景标签），不改权威 `WorldState.Scene`；
  权威场景仍由导演/规则驱动。
- `[VAR:key=value]` 写入**会话变量**（`variable_set` 事件，落到 `WorldState.Variables`，
  空值表示删除）。事件按 key 排序，保证可重放、可哈希。卡片 `defaultVariables` 在建会话时
  作为初值预置（不覆盖已有值）。
- 转义：`[[` 表示一个字面量 `[`。
- 标记名大小写不敏感（`[hud:…]` 等价 `[HUD:…]`）。
- 无法解析的标记**原样保留**（宁可让玩家看到原文，也不静默吞掉内容）。

## 3. 处理流程

```
导入 ──▶ CardUIProfile（能力清单/变量初值）           [随卡/根节点冻结]
编译 ──▶ 静态前缀注入 CardUIContract（语法 + 别名提示） [KV Cache 稳定]
模型 ──▶ 在 block 文本里输出标准标记
提交 ──▶ cardmarkers.Extract：剥离状态类标记 → TurnContent.UIHints
             表现类标记（图片）保留在正文
渲染 ──▶ 各端从 UIHints（已提交）或正文（实时流）渲染原生组件
```

关键取舍：
- **状态类标记从正文剥离**（`HUD/SCENE/VAR`），落到结构化字段，避免污染记忆引文与证据；
- **表现类标记保留在正文**（图片），由渲染层就地展示，与既有 `<img cmd>` 行为一致。

## 4. 代码位置

| 层 | 位置 |
|---|---|
| 语法解析（Go） | `internal/cardmarkers` |
| 能力检测 | `internal/application/cardui.go` |
| 提交期归一化 | `internal/application/commitplan.go`（`preparePlanBlocksAndOptions`） |
| 标记契约 | `internal/context/cardui.go`（`CardUIContract`） |
| 语法解析（Web） | `web/src/lib/cardMarkers.tsx` + `web/src/components/CardHUD.tsx` |

两端解析器必须保持一致：同一组黄金用例分别在 `internal/cardmarkers`、`web/src/lib/__tests__/cardMarkers.test.tsx` 里断言。

## 5. 能力检测（导入兼容报告）

`CardCapabilities` 汇总卡声明的 RisuAI 特性：`backgroundHTML`、`defaultVariables`、`sdData`、
`viewScreen`、`depth_prompt`、`vits`、`lowLevelAccess`、资源计数、世界书条目数、图片指令、
以及卡内出现的标记族名（如 `CeresStatus`）。这些用于导入预览里的兼容报告。

> 注意：**不解析 `module.risum`**。它是 RisuAI 的 RPack 二进制，其解码表随 RisuAI 仓库以
> GPL-3.0 分发；本项目为 MIT，不内嵌该表。因此只登记 `module.risum` 是否存在，不读取其内容。
> 完整模块解析（regex/trigger/Lua 细节）留待干净室实现或显式授权后再做。

## 6. 主题 token（backgroundHTML）

导入时从 `backgroundHTML` **白名单抽取**配色/字体，归一化为 `accent / bg / ink / font`
（`CardUIProfile.ThemeTokens`）：

- 只接受 `#hex`、`rgb()/rgba()/hsl()/hsla()` 形式的颜色，以及纯字体名（字母数字与常见分隔符）；
  拒绝 `url()`、`var()`、`expression`、`javascript:`、`@`、`\` 等。
- 变量名按子串归类（`accent` / `font` / `ink|text|fg` / `bg|background|surface|paper|panel`），
  同名先到先得，保证抽取结果确定。
- Web 端把 token 写进根元素的 `--card-accent/--card-bg/--card-ink/--card-font`，仅 `CardHUD` 与
  `SceneBar` 消费这些变量，因此不影响整体界面。

**不执行卡内 CSS**：只读取字面量值。

## 7. 会话变量（M3）

会话变量是卡片的会话级键值状态，来源有三：`[VAR]` 标记、`defaultVariables`、`toggles`（开关初值）。

- **落库**：`[VAR]` → `variable_set` 事件 → `WorldState.Variables`（空值删除；事件按 key 排序）。
- **面板**：Web 右栏与桌面右栏都新增「会话变量」小节，读 `WorldState.Variables`，只读展示。
- **提示词注入**：编译层动态块新增【会话变量】小节（`internal/context/prompts.go:renderSessionVariables`），
  把当前取值告知模型；放动态块而非静态前缀，避免变量每变一次就击穿 KV Cache。
  提示词版本 v5 → v6（黄金指纹不变，属动态内容）。
- **CBS 只读子集**：正文/开场里的 `{{getvar::键}}` 在展示层解析为变量当前值（web `characterMacros.ts`、
  桌面 `NarrativeText.ReplaceMacros`）。`{{setvar}}` / `{{roll}}` 需按回合播种的确定性，不在本层实现。
- **toggles**：`extensions.risuai.toggles` 的标量值并入 `VarDefaults`（`defaultVariables` 覆盖同名），
  使卡片声明的开关出现在会话变量里。

## 8. 情绪立绘（M4）

`viewScreen=emotion` 的卡把立绘存在 `assets[].type="emotion"`。按资源名关键词归一到四个神态键
（`alert / calm / smile / whisper`），有匹配时优先用于右栏立绘，否则回退默认头像。

- 归一化逻辑：web `emotionMapFromAssets`（`cardAssetStore.ts`）、桌面 `EmotionAssets.Resolve`（Core）。

## 9. 生图模板（sdData，M4）

卡片的 `extensions.risuai.sdData` 是自带的正向提示词模板。`CardUIProfile.SdData` 抽取（兼容 `string[]`
与 `string[][]`，去空、限长、限条数）后经 HTTP 暴露，作为插画 prompt 的**基底前置**：
web `IllustrationModal` 与桌面 `ImagePromptExtractor.Extract` 都接受该基底；缺失时不影响现有行为。

## 10. 关于 editdisplay（M4.1）

`editdisplay` 正则脚本位于 `module.risum`，本项目**不解析该二进制**（见 §5 的 GPL 约束），因此无法读取
其内容。可达成的「受控 UI 模板层」只由两条已有通路实现，**不执行卡内任意 HTML**：

1. `backgroundHTML` → 主题 token（§6）；
2. 模型输出的标准标记 → 原生 HUD / 场景 / 图片（§2）。

卡作者自定义的 `editdisplay` 标记族会通过契约里的「别名提示」引导模型改用标准标记。

## 11. 明确不做

- 执行卡内任意 HTML / CSS / JS / Lua；
- 解析 `module.risum`（GPL 约束）；
- 注入卡内正则改写存档/记忆引文；
- `lowLevelAccess` 允许的任意 LLM/网络调用；
- 用户手动编辑会话变量（当前为只读面板）。
