# 卡片界面标记（Card UI Markers）

> 目的：把角色卡（尤其是 charx / RisuAI 卡）自定义的状态/场景标记，归一到一组稳定的、
> 语言中立的标记上，由 TavernAgent 自己的原生组件渲染——**不执行卡内的 HTML/CSS/JS/Lua**。

## 1. 为什么需要它

charx 卡常用 `editdisplay` 正则或 Lua 把模型输出变成自定义 UI（状态 HUD、场景选择器、
聊天界面等）。直接执行卡内代码会破坏本应用的确定性因果记录，并带来 XSS / 代码执行面。
本方案只接受卡的**意图**，不接受卡的**代码**：把卡的标记语法翻译成一组标准标记，
各端用原生组件渲染。

## 2. 语法（v1）

标记写在模型输出的 `block` 帧 `text` 里（不是 `final` 帧）。

| 标记 | 含义 | 类别 |
|---|---|---|
| `<img cmd="key">` 或 `[IMG:key]` | 卡内资源图片 | 表现 |
| `[HUD:名称=值\|名称=值]` | 状态面板 | 状态 |
| `[SCENE:地点或场景标题]` | 场景 | 状态 |
| `[VAR:键=值\|键=值]` | 会话变量（正文不展示） | 状态 |

- 键值用 `|` 分隔、`=` 连接；值内不要出现 `[` `]` `|` 或换行。
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
| 语法解析（桌面） | `desktop/src/TavernAgent.Core/CardMarkers/CardMarkerParser.cs` |

三端解析器必须保持一致：同一组黄金用例分别在 `internal/cardmarkers`、`web/src/lib/__tests__/cardMarkers.test.tsx`、`desktop/tests/TavernAgent.Core.Tests/CardMarkerParserTests.cs` 里断言。

## 5. 能力检测（导入兼容报告）

`CardCapabilities` 汇总卡声明的 RisuAI 特性：`backgroundHTML`、`defaultVariables`、`sdData`、
`viewScreen`、`depth_prompt`、`vits`、`lowLevelAccess`、资源计数、世界书条目数、图片指令、
以及卡内出现的标记族名（如 `CeresStatus`）。这些用于导入预览里的兼容报告。

> 注意：**不解析 `module.risum`**。它是 RisuAI 的 RPack 二进制，其解码表随 RisuAI 仓库以
> GPL-3.0 分发；本项目为 MIT，不内嵌该表。因此只登记 `module.risum` 是否存在，不读取其内容。
> 完整模块解析（regex/trigger/Lua 细节）留待干净室实现或显式授权后再做。

## 6. 明确不做

- 执行卡内任意 HTML / CSS / JS / Lua；
- 注入卡内正则改写存档/记忆引文；
- `lowLevelAccess` 允许的任意 LLM/网络调用。
