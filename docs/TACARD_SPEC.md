# TavernAgent 角色卡规范（.tacard 规范 v1.0）

> **格式名称**：TavernAgent Card  
> **标准后缀名**：`.tacard`  
> **MIME 类型**：`application/x-tavernagent-card+zip`  
> **规范版本**：`tacard/v1`  
> **适用环境**：TavernAgent 全平台客户端（服务端、Web 端、原生 Android、桌面端）

---

## 1. 规范背景与设计哲学

在 AI 角色扮演（RP）与交互叙事生态中，常见的 RisuAI / SillyTavern `charx` 格式长期存在以下技术痛点：
1. **黑盒脚本与安全隐患**：包含 `module.risum`（RPack 二进制脚本/Lua/动态正则注入），带来客户端任意代码执行风险、XSS 注入面及不可预测的状态篡改；
2. **标签形态严重碎片化**：历史角色卡中充斥着各类私有与变体标记（`<img cmd="...">`、`<img="...">`、`<img_obj="...">`、`<imgBG="...">`、`<img src="...">` 等），解析极其脆弱；
3. **冗余数据膨胀**：常混入孤儿资源文件、`x_meta/` 冗余配置文件与未清理的开发残留物。

**`.tacard`（TavernAgent Card）** 是专为安全、高性能、确定性交互叙事设计的现代化原生角色卡格式。其核心哲学是：
- **只接受意图，绝不执行代码（Strict Zero-Scripting）**：彻底剔除黑盒脚本，所有状态与视觉均由声明式契约与宿主原生组件渲染；
- **全要素完整保留（Lossless Lore & Openings）**：完整结构化保留角色人设、世界书（知识库/设定集）、多剧本开场白变体与分支剧情；
- **多模态资产清晰规范（Clean Asset Manifest）**：规范化图片资产清单与元数据，支持 WebP/PNG 高效无损加载；
- **统一轻量标记（Canonical Markers）**：全面推行以 `[IMG:asset_id]` 为核心的原生中立标记语法。

---

## 2. 容器与物理归档布局

`.tacard` 采用标准开放的 ZIP 归档格式存储，不设密码，采用 Deflate 压缩。文件目录布局如下：

```text
<character_slug>.tacard (标准 ZIP 归档)
├── tacard.json              # 核心卡片描述清单（唯一权威文本，UTF-8，无 BOM）
└── assets/                  # 媒体资源总目录（扁平或按类型分目录）
    ├── avatar.webp          # 角色主头像（标准首选入口）
    ├── Soraka_smile.webp    # 情绪表情/神态立绘
    ├── Soraka_worried.webp
    ├── scene_targon.webp    # 场景插图/背景
    └── item_starlight.webp  # 专属信物/道具图
```

- **容器要求**：
  - 根目录下**必须**存在且仅存在一个 `tacard.json` 文件；
  - 媒体资源**必须**存放于 `assets/` 路径下，推荐优先使用现代高压缩比的 `.webp` 或 `.png`；
  - 严禁包含 `.exe`, `.dll`, `.so`, `.sh`, `.bat`, `.risum`, `.lua`, `.js` 等可执行脚本与二进制。

---

## 3. tacard.json 核心数据结构

`tacard.json` 是角色卡的唯一权威元数据，包含规范版本、基础元信息、角色实体、开场白列表、世界书与资产清单：

```json
{
  "spec": "tacard/v1",
  "metadata": {
    "id": "char_soraka_starlight",
    "name": "索拉卡 (Soraka)",
    "version": "1.0.0",
    "author": "TavernAgent Community",
    "description": "来自天界众星的星之子，漫步于巨神峰脚下的温柔治愈者。",
    "tags": ["英雄联盟", "巨神峰", "治愈", "星之子", "奇幻RP"],
    "language": "zh-CN",
    "source": {
      "format": "charx",
      "converted_at": "2026-09-26T20:50:00Z"
    }
  },
  "character": {
    "name": "索拉卡",
    "aliases": ["Soraka", "星之子", "众星之子"],
    "avatar_asset": "avatar",
    "description": "【外貌与形态】拥有如银河般流淌的浅紫色长发与额间温润的玉质独角...",
    "personality": "温和慈悲、敏锐机智、纯粹包容，对凡人世界的生活琐事充满孩童般的好奇与同理心。",
    "scenario": "巨神峰险峻的寒风与流云之下，朝圣者与凡人旅者汇聚的温暖避风所。",
    "system_prompt": "你将扮演《英雄联盟》宇宙中的众星之子·索拉卡。请遵循温柔细腻的笔触进行交互叙事...",
    "post_history_instructions": "保持第一人称与第三人称客观叙述交融；在角色产生神态变化时，使用 [IMG:立绘名] 展现对应表情。"
  },
  "openings": [
    {
      "id": "open_morning_rest",
      "title": "破晓的小憩",
      "text": "巨神峰晨曦微露的冰凉岩壁旁，山风卷动着她暗红色的披风。\n\n[IMG:Soraka_smile]\n「你好呀，远道而来的旅人。先喝口热水吧。」"
    },
    {
      "id": "open_village_market",
      "title": "山脚集市的偶遇",
      "text": "山脚下的集市洋溢着烤面包与肉桂的暖香。索拉卡正驻足在一处烘焙摊位前..."
    }
  ],
  "lorebook": {
    "name": "巨神峰与星灵世界观设定",
    "entries": [
      {
        "id": "lore_mount_targon",
        "keys": ["巨神峰", "塔贡", "Targon", "巨神山"],
        "secondary_keys": ["星灵", "朝圣"],
        "content": "### 巨神峰 (Mount Targon)\n符文之地最高的山脉，连接天界星灵与凡间维度的枢纽...",
        "constant": false,
        "order": 100,
        "position": "before_char"
      },
      {
        "id": "lore_celestial_nature",
        "keys": ["星灵", "天界", "神圣体质"],
        "secondary_keys": [],
        "content": "### 索拉卡的神性特质\n放弃不朽形态降临凡间，她的每一次治愈都需要消耗自身凡人躯体的生命精血...",
        "constant": true,
        "order": 10,
        "position": "after_char"
      }
    ]
  },
  "assets": [
    {
      "id": "avatar",
      "file": "assets/avatar.webp",
      "type": "avatar",
      "label": "主头像"
    },
    {
      "id": "Soraka_smile",
      "file": "assets/Soraka_smile.webp",
      "type": "emotion",
      "category": "happy",
      "label": "温柔微笑"
    },
    {
      "id": "Soraka_worried",
      "file": "assets/Soraka_worried.webp",
      "type": "emotion",
      "category": "sad",
      "label": "担忧关切"
    },
    {
      "id": "scene_targon_dawn",
      "file": "assets/scene_targon_dawn.webp",
      "type": "scene",
      "label": "巨神峰破晓"
    }
  ],
  "ui_contract": {
    "theme_tokens": {
      "accent": "#7C3AED",
      "bg": "#0C0E14",
      "ink": "#E2E8F0",
      "font": "system-ui"
    },
    "default_variables": {
      "trust_level": "0",
      "encountered_location": "mount_targon_base"
    }
  }
}
```

---

## 4. 统一图片与状态标记体系（Canonical Markers）

`.tacard` 彻底废除 HTML 标签形式的图像指令（如 `<img cmd="...">`），统一归一化为语言中立、直观轻量的方括号标记。

| 标记语法 | 示例 | 类别 | 客户端处理行为 |
|---|---|---|---|
| **`[IMG:asset_id]`** | `[IMG:Soraka_smile]` | 表现层（多模态图片） | 检索 `assets` 表中匹配的立绘或插图，在对话流中原地渲染为轻量图片卡片，支持点击全屏缩放查看 |
| **`[SCENE:场景名称]`** | `[SCENE:巨神峰避风窟]` | 状态层（场景定位） | 原文剥离，更新当前对话场景浮动标签（SceneBar） |
| **`[HUD:键=值\|...]`** | `[HUD:好感度=10/100\|信任=稳固]` | 状态层（属性监控） | 原文剥离，渲染为卡片顶部的迷你状态浮动条或进度量表 |
| **`[VAR:键=值\|...]`** | `[VAR:已接受草药=true]` | 状态层（会话变量） | 原文静默剔除，作为因果事件同步沉淀到世界状态变量中 |

- **转义规则**：使用 `[[` 表示字面量 `[`（例如 `[[HUD]` 将被渲染为字面字符 `[HUD]`，不被当成指令）。
- **大小写兼容**：标记名与资源键保持大小写不敏感（`[img:Soraka_smile]` 等同于 `[IMG:Soraka_smile]`）。

---

## 5. 安全模型与黑名单（Zero-Scripting Policy）

为了确保跨设备运行（尤其是 Android 与 iOS 移动端）的绝对安全性与确定性，`.tacard` 实行强约束沙箱策略：

### 🚫 严禁项（禁止收录）
1. **可执行二进制与字节码**：禁止任何 `.risum`, `.wasm`, `.so`, `.dll` 模块；
2. **嵌入式脚本引擎**：禁止任何 Lua、JavaScript、Python 解释执行代码；
3. **动态正则改写器（Regex Overrides）**：禁止卡片携带动态篡改模型输出或记忆检索的客户端正则；
4. **底层系统/网络访问（Low Level Access）**：禁止发起任意 HTTP 请求或操作系统调用。

###  允许项（声明式意图）
1. **静态主题契约（Theme Tokens）**：仅允许声明静态的十六进制颜色与标准字体名；
2. **状态初始值（Default Variables）**：允许预置纯文本或数值类型的状态键值对；
3. **多模态静态媒体（Static Assets）**：仅允许标准图片（WebP / PNG / JPG / GIF）及可选音频。

---

## 6. 从 charx 迁移到 .tacard 的清洗转换矩阵

| charx (旧 RisuAI 格式) | .tacard (新标准格式) | 迁移处理规则 |
|---|---|---|
| `module.risum` | **直接丢弃** | 消除 GPL 许可冲突与二进制执行安全面 |
| `x_meta/` | **直接丢弃** | 剔除冗余碎片元数据 |
| `card.json` -> `data.character_book` | **`tacard.json` -> `lorebook`** | 保留关键词扫描、层级顺序与常驻策略 |
| `data.alternate_greetings` | **`tacard.json` -> `openings`** | 结构化转为开场白列表，支持自定义标题 |
| `<img cmd="X">` / `<img="X">` | **`[IMG:X]`** | 正则全文扫描，批量替换为无歧义标准标记 |
| `extensions.risuai.backgroundHTML` | **`tacard.json` -> `ui_contract.theme_tokens`** | 白名单解析提取配色与字体，剥离 HTML/DOM |
| `assets/` 媒体文件 | **`assets/` 媒体文件** | 完整保留，清洗扩展名，建立规范清单 |

---

## 7. 最佳实践建议

1. **统一资产格式为 WebP**：建议将角色立绘与背景图片压缩为 WebP 格式（质量 80~85%），不仅体积比 PNG 减小 60% 以上，且支持透明通道，极大加速移动端首屏加载；
2. **图片命名语义化**：资源 ID 推荐采用 `<角色名>_<情绪/动作>` 规则（如 `Soraka_smile`、`Soraka_surprised`），便于在提示词中引导模型调用；
3. **多开场白场景化**：为不同的开场白变体起明确的标题（如“初次相遇”、“久别重逢”、“风雪救援”），方便玩家在创建故事时自由挑选。
