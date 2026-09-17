"""生成项目架构图 SVG（docs/artifacts/architecture/architecture.svg）。

图是**代码结构的事实快照**，不是设计愿景：分层、包名、服务名、表数量、端点数
都取自仓库当前状态。改动架构后重新运行本脚本即可：

    python scripts/architecture_diagram.py
    # 可选：渲染 PNG（需要 web/node_modules 里的 Playwright）
    cd web && npx playwright screenshot --viewport-size=1780,1960 --full-page \
        ../docs/artifacts/architecture/architecture.svg \
        ../docs/artifacts/architecture/architecture.png

坐标由布局引擎计算（先量后画），避免手工摆位重叠。
"""
from __future__ import annotations

import math
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "docs/artifacts/architecture/architecture.svg"

W = 1780
MARGIN = 36
GAP = 32
LEFT_W = 1150
RIGHT_X = MARGIN + LEFT_W + GAP
RIGHT_W = W - RIGHT_X - MARGIN

FONT = '"Microsoft YaHei UI","Microsoft YaHei","Segoe UI",system-ui,sans-serif'

INK = "#16202b"
MUTED = "#5b6b7c"
LINE = "#c8d3de"
BG = "#f7f9fc"


def text_w(text: str, size: float) -> float:
    """粗估文本宽度：CJK 按 1em，拉丁按 0.56em（足够做盒子排布）。"""
    units = 0.0
    for ch in text:
        units += 1.0 if ord(ch) > 0x2E80 else 0.56
    return units * size


class Canvas:
    def __init__(self) -> None:
        self.parts: list[str] = []

    def add(self, svg: str) -> None:
        self.parts.append(svg)

    def rect(self, x, y, w, h, fill, stroke="none", rx=8, sw=1, opacity=None) -> None:
        op = f' opacity="{opacity}"' if opacity is not None else ""
        self.add(f'<rect x="{x:.1f}" y="{y:.1f}" width="{w:.1f}" height="{h:.1f}" rx="{rx}" fill="{fill}" stroke="{stroke}" stroke-width="{sw}"{op}/>')

    def text(self, x, y, body, size=13, fill=INK, weight="normal", anchor="start") -> None:
        body = (
            body.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")
        )
        self.add(
            f'<text x="{x:.1f}" y="{y:.1f}" font-family=\'{FONT}\' font-size="{size}" fill="{fill}"'
            f' font-weight="{weight}" text-anchor="{anchor}">{body}</text>'
        )

    def arrow(self, x, y1, y2, label="", color="#7b8a99") -> None:
        self.add(f'<line x1="{x:.1f}" y1="{y1:.1f}" x2="{x:.1f}" y2="{y2:.1f}" stroke="{color}" stroke-width="1.6"/>')
        self.add(f'<path d="M {x-5:.1f} {y2-7:.1f} L {x:.1f} {y2:.1f} L {x+5:.1f} {y2-7:.1f}" fill="none" stroke="{color}" stroke-width="1.6"/>')
        if label:
            self.text(x + 12, (y1 + y2) / 2 + 5, label, size=12, fill=MUTED)

    def render(self, height: float, title: str, subtitle: str) -> str:
        return (
            f'<svg xmlns="http://www.w3.org/2000/svg" width="{W}" height="{height:.0f}" viewBox="0 0 {W} {height:.0f}">'
            f"<title>{title}</title><desc>{subtitle}</desc>"
            + "".join(self.parts)
            + "</svg>"
        )


# ---------------------------------------------------------------- 布局引擎

class Layer:
    """一个分层盒子：标题栏 + 若干行（行内是自动折行的标签块）。"""

    def __init__(self, key: str, title: str, note: str, color: str, rows: list[tuple[str, list[str]]]):
        self.key = key
        self.title = title
        self.note = note
        self.color = color
        self.rows = rows
        self.height = 0.0
        self.y = 0.0

    # 量：标题栏 34 + 顶 12 + 每行（标签 17 + 块高 26 + 行距 10）+ 底 12
    def measure(self, width: float) -> float:
        h = 34 + 12
        for _, chips in self.rows:
            rows = self._wrap(chips, width - 28)
            h += 17 + len(rows) * 26 + (len(rows) - 1) * 8 + 10
        self.height = h + 6
        return self.height

    @staticmethod
    def _wrap(chips: list[str], avail: float) -> list[list[str]]:
        rows: list[list[str]] = []
        current: list[str] = []
        used = 0.0
        for chip in chips:
            w = text_w(chip, 12.5) + 20
            if current and used + w + 8 > avail:
                rows.append(current)
                current, used = [], 0.0
            current.append(chip)
            used += w + 8
        if current:
            rows.append(current)
        return rows

    def draw(self, c: Canvas, x: float, width: float) -> None:
        y = self.y
        c.rect(x, y, width, self.height, "#ffffff", LINE, rx=10, sw=1.2)
        c.rect(x, y, width, 34, self.color, rx=10, opacity=0.14)
        c.rect(x, y + 22, width, 12, self.color, rx=0, opacity=0.14)
        c.text(x + 14, y + 23, f"{self.key} · {self.title}", size=14.5, weight="600", fill=INK)
        if self.note:
            c.text(x + width - 14, y + 23, self.note, size=12, fill=MUTED, anchor="end")

        cursor = y + 34 + 12
        for label, chips in self.rows:
            if label:
                c.text(x + 14, cursor + 13, label, size=12, weight="600", fill=MUTED)
            cursor += 17
            line_x = x + 14
            line_y = cursor
            max_x = x + width - 14
            for chip in chips:
                w = text_w(chip, 12.5) + 20
                if line_x != x + 14 and line_x + w > max_x:
                    line_x = x + 14
                    line_y += 26 + 8
                c.rect(line_x, line_y, w, 26, "#eef3f8", LINE, rx=6, sw=0.8)
                c.text(line_x + 10, line_y + 17.5, chip, size=12.5, fill=INK)
                line_x += w + 8
            cursor = line_y + 26 + 10
        self.y = y  # 保持


class Panel:
    """右栏的说明面板：标题 + 若干条目（条目可带次级说明）。"""

    def __init__(self, title: str, items: list[tuple[str, str]], color: str, numbered: bool = False):
        self.title = title
        self.items = items
        self.color = color
        self.numbered = numbered
        self.height = 0.0
        self.y = 0.0

    def measure(self, width: float) -> float:
        h = 34 + 12
        for head, detail in self.items:
            h += 20
            if detail:
                h += 17 * max(1, math.ceil(text_w(detail, 11.5) / (width - 46)))
            h += 9
        self.height = h + 4
        return self.height

    def draw(self, c: Canvas, x: float, width: float) -> None:
        y = self.y
        c.rect(x, y, width, self.height, "#ffffff", LINE, rx=10, sw=1.2)
        c.rect(x, y, width, 34, self.color, rx=10, opacity=0.14)
        c.rect(x, y + 22, width, 12, self.color, rx=0, opacity=0.14)
        c.text(x + 14, y + 23, self.title, size=14.5, weight="600", fill=INK)
        cursor = y + 34 + 12
        for index, (head, detail) in enumerate(self.items, start=1):
            marker = f"{index}." if self.numbered else "·"
            c.text(x + 14, cursor + 13, marker, size=12, weight="600", fill=self.color)
            c.text(x + 32, cursor + 13, head, size=12.5, weight="600", fill=INK)
            cursor += 20
            if detail:
                avail = width - 46
                line = ""
                for token in detail:
                    if text_w(line + token, 11.5) > avail:
                        c.text(x + 32, cursor + 11, line, size=11.5, fill=MUTED)
                        cursor += 17
                        line = token
                    else:
                        line += token
                if line:
                    c.text(x + 32, cursor + 11, line, size=11.5, fill=MUTED)
                    cursor += 17
            cursor += 9


def build_layers() -> list[Layer]:
    return [
        Layer("A", "浏览器前端 · React 19 + Vite（构建产物嵌入二进制）",
              "SPA · 单机/局域网", "#2f6fd0", [
            ("组件（渲染与交互）", [
                "NarrativeStream 正文流", "ComposerShelf 输入台", "LeftRail 会话栏",
                "RightRail 人设·背包·关系", "TurnActions 回合操作", "HistoricalStory 历史",
                "StoryMapModal 故事图", "MindGraphModal 记忆图", "DirectorWorkspace 导演台",
                "WorldbookModal / LorebookPanel 世界书", "SettingsModal 设置",
                "CharacterImportModal 导入", "AuthPairingModal 配对",
                "ContextMetricsPopover 上下文读数", "FullTachieModal 立绘",
                "StoryCheckpointSection 摘要",
            ]),
            ("状态（zustand）", [
                "storyStore（切片：session / turn / branch / memory / director）",
                "uiStore", "settingsStore",
            ]),
            ("传输与展示工具（web/src/app · lib）", [
                "api.ts（REST + SSE 订阅）", "characterPresets 内置角色卡",
                "characterPresentation 呈现映射", "characterCardStore 本地缓存与配额",
                "dossierAttrs / dossierMarkdown 人设渲染", "storyCheckpoint 摘要展示",
                "tokenEstimator", "packFile 剧情包",
            ]),
        ]),
        Layer("B", "HTTP 适配层 · internal/adapters/http", "40 个端点 · JSON + SSE", "#12897f", [
            ("路由分组", [
                "POST/GET /api/v1/sessions", "…/branches（列出·分叉）",
                "…/branches/{b}/turns（受理）", "…/regenerations · /edits",
                "/turns/{id}（读取·取消·续写）", "/turns/{id}/events · /sessions/{id}/events（SSE）",
                "/memories（分页·PATCH·organize）", "/nodes/{id} · /graph · /lorebook",
                "/director-requests（讨论·事件·取消）", "/config/provider · /config/profiles",
                "/config/provider/probe", "/auth/pair · /auth/status",
                "/status · /healthz", "/sessions/{id}/export · /sessions/import · /cards/import",
            ]),
            ("横切关注点", [
                "同源校验（写请求）", "配对鉴权：六位码 → 令牌", "请求体限额（413 / 422）",
                "优雅停机与后台任务收尾", "启动恢复入口", "SPA 静态托管（embed dist）",
            ]),
        ]),
        Layer("C", "应用层 · internal/application", "编排 · 事务边界 · 后台任务", "#4b56c9", [
            ("前台服务", [
                "SessionService 会话与开场", "TurnService 受理/生成/续写/取消/编辑/重生成",
                "CommitPlan 提交计划与冲突", "TurnLifecycle 回合状态机",
                "Idempotency 幂等键", "BranchService 分支",
                "MemoryService 修订/置顶/隐藏/整理/配额", "DirectorService 讨论/草稿/命令/进度",
                "ArchiveService 剧情包", "Card 卡解析·头像缩略·宏展开",
                "Check / RuleEffects 检定与规则效果",
            ]),
            ("后台任务（排队合并，同分支只保留最新一次）", [
                "CognitiveService 记忆抽取（reflection 槽）· 修订单竞",
                "CompactorService 摘要维护（并发 1 · 压力触发）",
            ]),
            ("横切", [
                "EventBus 内存订阅 + outbox 落库", "ProviderManager 三槽位解析/探测/用量包装",
                "RuntimeMetrics 运行读数", "Recovery 中断回合恢复",
            ]),
        ]),
        Layer("D", "上下文编译 · internal/context", "每次生成重建 · 预算内裁剪", "#b8860b", [
            ("Compile 管线（顺序即优先级）", [
                "人设与系统规则", "开场白", "历史回合（尾部窗口）", "世界书关键词命中",
                "记忆召回（FTS5 + 实体 + 时序）", "摘要折叠：区间 → 摘要文本",
                "大检定账本（不可被叙事反转的硬下界）", "状态投影（物品/关系/承诺/情绪）",
                "预算裁剪 fitBundleBudget",
            ]),
            ("预算与压缩策略", [
                "淘汰顺序：世界书 → 记忆 → 历史 → 摘要",
                "TailWindowTurns / MinUncompactedTurns", "ResolveTailPolicy 按可用预算收敛尾部",
                "DecideCompaction 选未覆盖区间", "摘要提示词 + validateSummaryXML（层级校验）",
                "记忆修订使区间摘要失效", "CONTEXT_OVER_BUDGET 显式报错",
            ]),
        ]),
        Layer("E", "领域 · 协议 · 检索 · 剧情包", "internal/domain · protocol · search · pack", "#8a5cd0", [
            ("domain 领域模型", [
                "WorldState 角色/关系/物品/承诺/秘密/情绪/场景",
                "PlotGraph 写时复制节点 · 分支 · 快照", "MemoryRecord 修订链 / 可见性 / 保护",
                "TurnRequest·Attempt 状态机", "DirectorPlan · Beat · Progress",
                "CheckResult · ActionReceipt", "SessionView 视图装配",
            ]),
            ("protocol 生成协议", [
                "v1 帧协议 {\"v\":1,\"seq\",\"type\":\"block|final\"}",
                "块类型：叙述 / 对白 / 心声", "严格解析与错误分类",
                "续写草稿 draft_frames",
            ]),
            ("search 检索", [
                "FTS5 查询构造（中文分词 + trigram）", "词法 / 实体 / 时序混合召回与排序",
            ]),
            ("pack 剧情包", [".tavernpack（zip）· 回执校验 · ID 重写 · 拒收不一致包"]),
        ]),
        Layer("F", "端口 · internal/ports", "依赖倒置分界：应用层只认接口", "#5b6b7c", [
            ("Store 组合接口（按用途分组）", [
                "SessionStore", "StoryStore", "TurnStore", "MemoryStore", "MemoryBatchStore",
                "SummaryStore", "ArchiveStore", "OutboxStore", "OpsStore", "UsageStore",
            ]),
            ("其余端口", [
                "ProviderConfigStore / ModelProfileStore", "DirectorStore",
                "ModelProvider（流式 + 用量回调）", "Clock · Random",
            ]),
        ]),
        Layer("G", "适配器 · internal/adapters", "端口实现", "#2f8f46", [
            ("sqlite（唯一持久化）", [
                "表 42 张（含 FTS5 影子表）：会话/分支/节点/快照/事件/回执/记忆/摘要/导演/模板/用量",
                "WAL + 只读连接池", "user_version 迁移 + 迁移前备份",
                "单进程文件锁", "导入导出与引用重写",
            ]),
            ("providers 模型供应商", [
                "openai-chat（/chat/completions 流式）", "openai-responses", "anthropic-messages",
                "mock（开发回退）", "三层超时：首字 / 空闲 / 总时限",
                "取消与超时区分", "用量采集（token 与延迟）",
            ]),
            ("config 配置", [
                "settings.json（槽位 primary/assist/reflection · 模型档案）",
                "secrets.json（密钥只落盘不回显）", "环境变量注入 TAVERNAGENT_KEY_*",
            ]),
        ]),
        Layer("H", "运行时与数据", "单二进制 · 一个数据目录", "#33414f", [
            ("cmd/tavernagent（SillyDog）", [
                "-addr 监听地址", "-data 数据目录", "-pin 局域网配对码",
                "-provider 开发兜底", "-allow-origin 开发代理来源", "-version 版本指纹",
            ]),
            ("数据目录", [
                "storage.db（+wal/shm）", "config/settings.json · config/secrets.json",
                "logs/", "backups/（迁移与手动备份）", "导出 .tavernpack",
            ]),
        ]),
    ]


def build_panels() -> list[Panel]:
    return [
        Panel("① 一次生成回合的数据流", [
            ("浏览器提交", "输入台 → POST /turns，带幂等键与期望的 head / 版本 / 角色 ID"),
            ("受理", "同源校验 → 配对鉴权 → 请求体限额 → 幂等/冲突/分支锁判定 → 202 返回 turnId"),
            ("编译提示词", "应用层调 context.Compile：召回、折叠、账本、预算；预算不足直接 422"),
            ("流式生成", "供应商按 v1 帧协议输出 → 解析 block/final；失败分类：协议非法 / 截断续写 / 供应商不可用"),
            ("提交", "状态推进 + 写时复制新节点 + 回执与领域事件，同一事务落库"),
            ("推送", "EventBus → SSE（turn.* / session.* / director.*）→ 浏览器增量渲染"),
            ("后台", "记忆抽取写认知节点；摘要维护折叠区间；导演阶段判定——都不阻塞当前回合"),
            ("分叉与回退", "任意节点开新分支；历史不可变，旧分支摘要按区间复用"),
        ], "#2f6fd0", numbered=True),
        Panel("② 存储模型（不可变 + 派生）", [
            ("剧情图", "plot_nodes（parent_id 链）← branches（head / version）← state_snapshots"),
            ("事件溯源", "domain_events + action_receipts：重放校验、幂等、不被静默重写"),
            ("记忆", "memory_records（supersedes 修订链 · hidden · 可见性 · 保护）+ memory_fts"),
            ("摘要", "summary_artifacts：from→to 区间 + 来源哈希；区间内记忆被修订后失效"),
            ("用量台账", "turn_usage：in_flight → completed / failed / cancelled，按槽位与任务记账"),
        ], "#12897f"),
        Panel("③ 不变量与边界", [
            ("单进程单目录", "同一数据目录只允许一个进程打开；启动即取文件锁，附带的 wal/shm 一起备份"),
            ("历史不可变", "已提交节点不改写：要改故事就分叉；回退只调整导演计划进度"),
            ("乐观并发", "写请求带幂等键与期望 head / 版本 / 角色；不符即冲突，不猜用户意图"),
            ("密钥只落盘", "secrets.json 不回显；读取接口只给存在标识与掩码；可用环境变量注入"),
            ("预算显式失败", "必要上下文放不进窗口时报 CONTEXT_OVER_BUDGET，而不是静默截断历史"),
            ("摘要只替代区间", "摘要覆盖的区间才折叠，最近对话保留原文；不跨分支读取"),
            ("外部模型不可信", "历史与记忆作为不可信数据注入；摘要必须通过 XML 层级校验才落库"),
        ], "#8a5cd0"),
        Panel("④ 质量与发布脚本", [
            ("构建", "scripts/build.ps1（前端测试/lint/构建 → Go 测试/vet → 嵌入 dist）· dev.ps1"),
            ("真实模型验收", "release_eval.py（计量网关）· release_features.py（功能用例）· functional_eval.py（七场景 + 对话日志）"),
            ("产物与审计", "release.py（多平台包 + SHA256）· release_audit.py · quality_check.py"),
            ("测试", "scripts/tests（unittest）· web/src/**/__tests__（vitest）· web/e2e（Playwright）"),
        ], "#b8860b"),
    ]


def main() -> None:
    c = Canvas()
    height_probe = 0.0
    layers = build_layers()
    panels = build_panels()

    # 量
    total_left = 0.0
    for layer in layers:
        total_left += layer.measure(LEFT_W)
    total_left += (len(layers) - 1) * 34  # 层间箭头
    total_right = sum(panel.measure(RIGHT_W) for panel in panels) + (len(panels) - 1) * 24

    header_h = 152
    body_h = max(total_left, total_right)
    height = header_h + body_h + 84

    c.rect(0, 0, W, height, BG, rx=0)
    c.text(MARGIN, 62, "TavernAgent（SillyDog）项目架构", size=30, weight="700", fill=INK)
    c.text(MARGIN, 90, "单二进制 Go 后端 + React 单页前端 + SQLite；分层与端口倒置，浏览器只经 HTTP/SSE 访问",
           size=14, fill=MUTED)
    legend = [
        ("A", "浏览器前端", "#2f6fd0"), ("B", "HTTP 适配", "#12897f"), ("C", "应用层", "#4b56c9"),
        ("D", "上下文编译", "#b8860b"), ("E", "领域/协议/检索/剧情包", "#8a5cd0"), ("F", "端口", "#5b6b7c"),
        ("G", "适配器", "#2f8f46"), ("H", "运行时与数据", "#33414f"),
    ]
    lx = MARGIN
    for key, name, color in legend:
        label = f"{key} {name}"
        w = text_w(label, 12.5) + 26
        c.rect(lx, 106, w, 24, "#ffffff", color, rx=12, sw=1.2)
        c.rect(lx + 9, 116, 4, 4, color, rx=2)
        c.text(lx + 17, 123, label, size=12.5, fill=INK)
        lx += w + 9

    # 画左栏
    y = header_h
    for index, layer in enumerate(layers):
        layer.y = y
        layer.draw(c, MARGIN, LEFT_W)
        y += layer.height
        if index < len(layers) - 1:
            notes = {
                0: "HTTP / JSON + SSE",
                1: "应用服务调用（端口接口）",
                2: "编译提示词 / 提交计划",
                3: "使用领域类型与协议",
                4: "只依赖接口，不依赖实现",
                5: "适配器实现端口",
                6: "同一进程、同一数据目录",
            }
            c.arrow(MARGIN + LEFT_W / 2, y + 4, y + 30, notes.get(index, "调用"))
            y += 34

    # 画右栏
    ry = header_h
    for panel in panels:
        panel.y = ry
        panel.draw(c, RIGHT_X, RIGHT_W)
        ry += panel.height + 24

    c.text(MARGIN, height - 30,
           "包名、服务名、表数量（42）与端点数（40）取自仓库当前代码；"
           "流程与数据模型标注的是实际调用顺序与约束，不是规划。",
           size=11.5, fill=MUTED)
    c.text(W - MARGIN, height - 30, "生成：python scripts/architecture_diagram.py", size=11.5, fill=MUTED, anchor="end")

    OUT.parent.mkdir(parents=True, exist_ok=True)
    OUT.write_text(c.render(height, "TavernAgent 项目架构", "单二进制 Go 后端 + React 前端 + SQLite 的分层架构"), encoding="utf-8")
    print(f"已生成 {OUT.relative_to(ROOT)}（{W}×{height:.0f}）")


if __name__ == "__main__":
    main()
