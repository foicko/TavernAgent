# UI 参考组件库 (Reference UI Components)

本目录汇总了从 5 大流行现代设计与 AI 交互组件网站下载的完整源码与动效规范，专门供 **TavernAgent** 网页端（Web React）与桌面端（C# Avalonia）进行界面设计、组件重构与交互动效实现时参考借鉴。

---

## 目录索引与资源来源

```
reference-components/
├── beautifului/        # beautifului.dev (AI 原生交互组件原语)
│   ├── primitives/     # 22 个 AI 原生核心组件 (Thinking, Streaming, Approval, Chips...)
│   └── atoms/          # 基础原子控制件 (按钮、开关、图标)
├── beui/               # beui.dev (基于 Tailwind & Motion 的流畅交互组件)
│   ├── motion/         # 80+ 动态交互组件 (Dock, Drawer, MorphModal, Shimmer, TiltCard...)
│   └── agents/         # AI 代理扩展组件
├── rareui/             # rareui.com (高辨识度个性化创意组件)
│   └── ui/             # 21 个精品创意组件 (FluidOrb, GravityLetters, GooeyNav, MatrixOrb...)
├── transitions/        # transitions.dev (物理弹簧阻尼与微动效库)
│   ├── transitions/    # 40+ 纯 CSS/HTML/JS 微动效案例与曲线
│   └── skills/         # AI 辅助设计与动效规约
├── shadcn-ui/          # ui.shadcn.com (现代极简设计基准组件库 - v4 New York)
│   └── ui/             # 60+ 官方基础交互原语 (Button, Dialog, Card, Sheet, Tabs, Tooltip...)
└── README.md           # 本说明索引文件
```

---

## 1. beautifului.dev (AI 原生交互组件)
- **官网**: https://www.beautifului.dev
- **作者**: Shane Levine (Turbo Design)
- **核心定位**: 专为 **AI Native 界面**打造的精雕细琢组件，解决了传统 UI 库缺乏思考状态展示、流式文本输出、人机确认闭环等问题。
- **重点参考组件** ([`reference-components/beautifului/primitives/`](primitives/)):
  - `ThinkingState.tsx`: 树状思考状态展开器，展示步骤、推理逻辑与工具搜索链条。
  - `StreamingText.tsx`: 优雅打字机与流式文本输出，集成来源引用芯片与后续快捷提问。
  - `ApprovalCard.tsx`: 人在回路（Human-in-the-loop）交互决策卡片，提供多选/自定义输入与步骤跳转。
  - `ToolChips.tsx`: 折叠式工具调用状态胶囊与代码微改展示。
  - `TaskRows.tsx`: 代理子任务执行流（Running、Failed、Completed），带圆形微进度环。
  - `PromptBar.tsx` / `ChatComposer.tsx`: 现代悬浮流光输入控制台与快捷模型选择芯片。
  - `DiffTable.tsx` / `RecordsTable.tsx` / `FilterTable.tsx`: 差异对比与结构化数据呈现表格。

---

## 2. beui.dev (动效与现代交互组件)
- **官网**: https://beui.dev
- **作者**: Saurabh Chauhan (@saurra3h)
- **核心定位**: 深度集成 **Framer Motion** 与 **Tailwind CSS** 的高级交互组件库，完全兼容 shadcn 规范。
- **重点参考组件** ([`reference-components/beui/motion/`](motion/)):
  - `dock.tsx`: 类似 macOS / 悬浮灵动岛的边缘停靠交互栏，带平滑鱼眼缩放。
  - `dynamic-island.tsx`: 顶部状态动态收缩/展开灵动岛。
  - `morphing-modal.tsx` / `center-morph-modal.tsx`: 平滑元素形变展开的模态弹窗。
  - `card-folder.tsx` / `project-folder.tsx`: 文件夹卡片风琴展开效果（适用于世界书/记忆册）。
  - `command-palette.tsx`: 极速模糊搜索命令面板（对应 `Ctrl+K`）。
  - `tilt-card.tsx`: 3D 陀螺仪与鼠标跟随倾斜悬浮卡片。
  - `text-shimmer.tsx` / `chromatic-text-reveal.tsx`: 优雅文字流光渐变遮罩。

---

## 3. rareui.com (个性与创意组件)
- **官网**: https://rareui.com
- **作者**: Swami Malode
- **核心定位**: 具有强烈视觉个性与动感设计的 React 组件库，主打单文件即插即用与新颖交互。
- **重点参考组件** ([`reference-components/rareui/ui/`](ui/)):
  - `fluid-orb.tsx` / `matrix-orb.tsx`: 动态流体光球与粒子网格（适用于角色状态、好感度与神秘氛围）。
  - `gravity-letters.tsx`: 物理重力掉落字母交互特效。
  - `gooey-nav.tsx`: 粘性流体黏稠过渡效果的导航菜单。
  - `family-drawer.tsx`: 仿 iOS 弹性底栏抽屉，阻尼平滑拉拽。
  - `step-player.tsx`: 故事发展/剧本时间轴步进播放器。
  - `proximity-sidebar.tsx`: 鼠标感应距离自动展开/收起的侧边栏。

---

## 4. transitions.dev (物理曲线与微交互动效)
- **官网**: https://transitions.dev
- **作者**: Jakub Antalik
- **核心定位**: 收集 Web 应用程序最核心的交互过渡和微动画，聚焦于阻尼感（Spring Physics）与自然回弹。
- **核心阻尼曲线参考**:
  - `spring`: `cubic-bezier(0.16, 1, 0.3, 1)` (物理弹簧，用于窗口弹窗、浮岛呼出)
  - `smooth-out`: `cubic-bezier(0.22, 1, 0.36, 1)` (平滑淡出，用于列表出现、透明度过渡)
  - `bounce`: `cubic-bezier(0.34, 1.36, 0.64, 1)` (轻微回弹，用于按钮轻击、选中状态)
- **重点案例** ([`reference-components/transitions/transitions/`](transitions/)):
  - `modal-open-close`: 弹窗微缩放（`scale(0.96) -> scale(1)`）与微位移入场。
  - `card-resize`: 容器大小改变时的宽高平滑插值过渡（FLIP 技巧）。
  - `tabs-sliding`: 标签切换时背景滑块平滑平移滑动。
  - `error-state-shake`: 表单或鉴权失败时的物理微颤动画。
  - `streaming-text`: 打字机光标微闪烁与渐显字符。

---

## 5. ui.shadcn.com (现代极简设计系统基石)
- **官网**: https://ui.shadcn.com
- **作者**: shadcn (@shadcn)
- **核心定位**: 业界公认的无样式无障碍（Accessible）组件基准，采用纯净的几何线条与中性灰阶。
- **重点基础组件** ([`reference-components/shadcn-ui/ui/`](ui/)):
  - `button.tsx`: 经典 6 种变体（`default`, `destructive`, `outline`, `secondary`, `ghost`, `link`）设计基准。
  - `dialog.tsx` / `sheet.tsx`: 模态居中框与侧滑抽屉面板。
  - `dropdown-menu.tsx` / `popover.tsx`: 无边界遮罩悬浮菜单与轻气泡。
  - `card.tsx`: 标准卡片（Header, Title, Description, Content, Footer）布局结构。
  - `tabs.tsx`: 分段标签选择器。
  - `tooltip.tsx`: 微型轻量悬浮说明标签。

---

## 在 TavernAgent 中的落地指南

### A. 网页端 ([`web/src/`](../web/src/)) 引用与移植
1. **样式变量**: 直接对应 [`web/src/styles/tokens.css`](../web/src/styles/tokens.css) 中的 CSS 自定义属性（`--bg-base`, `--surface-panel`, `--ease-spring` 等）。
2. **组件抽取**:
   - 对话气泡与思考流：参考 `beautifului/primitives/ThinkingState.tsx` 与 `StreamingText.tsx` 重构 [`web/src/features/chat/`](../web/src/features/chat/)。
   - 浮岛输入控制台：参考 `beautifului/primitives/ChatComposer.tsx` 与 `PromptBar.tsx`。
   - 快捷动作按钮：参考 `shadcn-ui/ui/button.tsx` 的 variant 架构。

### B. 桌面端 Avalonia ([`desktop/src/TavernAgent.Desktop/`](../desktop/src/TavernAgent.Desktop/)) 引用与移植
1. **XAML 资源字典**: 在 [`desktop/src/TavernAgent.Desktop/Styles/DesignTokens.axaml`](../desktop/src/TavernAgent.Desktop/Styles/DesignTokens.axaml) 中已定义了对应色值与过渡时间。
2. **控件映射**:
   - `Pill` / `GlassCard` / `Modal`: 对应 `beautifului` 与 `shadcn-ui` 的 Card/Dialog/Badge 原语。
   - 侧边栏与 HUD 展开：参考 `transitions/transitions/panel-reveal`，在 Avalonia 中通过 `Transitions` 绑定 `Width` / `RenderTransform`。
   - 物理阻尼曲线：将 `transitions.dev` 的 `0.16, 1, 0.3, 1` 对应至 Avalonia 的 `KeyFrame Easing="BackEaseOut"` 或自定义 `SplineEasing`。
