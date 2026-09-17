"""生成项目功能架构图 SVG（docs/artifacts/architecture/functions.svg）。

与技术架构图（architecture_diagram.py，按分层/包名写）不同，这张按**功能域**写：
每个功能域一行卡片，给出「用户入口 → 后端服务 → 数据落点 → 行为与约束」，
并在卡片角标上标出 functional_eval 的验收场景；右栏是功能之间的串接关系
（回合主链路、数据依赖、失败语义、后台任务、运行读数）。

事实取自仓库当前代码（组件名、服务名、表名、错误码、触发阈值）；
改动功能后重新运行本脚本即可：

    python scripts/function_architecture_diagram.py          # 只出 SVG
    python scripts/function_architecture_diagram.py --png    # 顺带渲染 PNG（需要 web/node_modules 的 Playwright）
"""
from __future__ import annotations

import subprocess
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))

from architecture_diagram import (  # noqa: E402  （复用画布与右栏面板，保持两张图同一套视觉语言）
    BG,
    FONT,
    INK,
    LINE,
    MUTED,
    Canvas,
    Layer,
    Panel,
    text_w,
)

ROOT = HERE.parent
OUT = ROOT / "docs/artifacts/architecture/functions.svg"
PNG = OUT.with_suffix(".png")

W = 1780
MARGIN = 36
GAP = 32
LEFT_W = 1150
RIGHT_X = MARGIN + LEFT_W + GAP
RIGHT_W = W - RIGHT_X - MARGIN

CARD_PAD = 14


class FeatureCard:
    """一个功能域：标题（含验收角标）、若干「标签 + 芯片」行、末尾一段行为约束。"""

    def __init__(self, key: str, title: str, note: str, color: str,
                 rows: list[tuple[str, list[str]]], behavior: str):
        self.key = key
        self.title = title
        self.note = note
        self.color = color
        self.rows = rows
        self.behavior = behavior
        self.lines: list[str] = []
        self.height = 0.0
        self.y = 0.0

    # 量：标题栏 34 + 顶 12 + 每行（标签 17 + 折行块 + 行距 10）+ 行为段（标签 17 + 全文 + 底 12）
    def measure(self, width: float) -> float:
        h = 34 + 12
        for _, chips in self.rows:
            rows = Layer._wrap(chips, width - CARD_PAD * 2)
            h += 17 + len(rows) * 26 + (len(rows) - 1) * 8 + 10
        self.lines = self._wrap_text(self.behavior, width - CARD_PAD * 2 - 12, 12)
        h += 17 + len(self.lines) * 17 + 12 + 6
        self.height = h + 6
        return self.height

    @staticmethod
    def _wrap_text(body: str, avail: float, size: float) -> list[str]:
        lines: list[str] = []
        line = ""
        for ch in body:
            if text_w(line + ch, size) > avail:
                lines.append(line)
                line = ch
            else:
                line += ch
        if line:
            lines.append(line)
        return lines

    def draw(self, c: Canvas, x: float, width: float) -> None:
        y = self.y
        c.rect(x, y, width, self.height, "#ffffff", LINE, rx=10, sw=1.2)
        c.rect(x, y, width, 34, self.color, rx=10, opacity=0.15)
        c.rect(x, y + 22, width, 12, self.color, rx=0, opacity=0.15)
        c.rect(x + CARD_PAD, y + 8, 34, 19, self.color, rx=5)
        c.text(x + CARD_PAD + 17, y + 22, self.key, size=12, weight="700", fill="#ffffff", anchor="middle")
        c.text(x + CARD_PAD + 44, y + 23, self.title, size=14.5, weight="600", fill=INK)
        if self.note:
            c.text(x + width - CARD_PAD, y + 23, self.note, size=11.5, fill=self.color, anchor="end")

        cursor = y + 34 + 12
        max_x = x + width - CARD_PAD
        for label, chips in self.rows:
            c.text(x + CARD_PAD, cursor + 13, label, size=12, weight="600", fill=MUTED)
            cursor += 17
            line_x = x + CARD_PAD
            line_y = cursor
            for chip in chips:
                w = text_w(chip, 12.5) + 20
                if line_x != x + CARD_PAD and line_x + w > max_x:
                    line_x = x + CARD_PAD
                    line_y += 26 + 8
                c.rect(line_x, line_y, w, 26, "#eef3f8", LINE, rx=6, sw=0.8)
                c.text(line_x + 10, line_y + 17.5, chip, size=12.5, fill=INK)
                line_x += w + 8
            cursor = line_y + 26 + 10

        box_top = cursor + 4
        box_h = 17 + len(self.lines) * 17 + 8
        c.rect(x + CARD_PAD, box_top, width - CARD_PAD * 2, box_h, "#f4f7fb", "#e2e9f1", rx=6, sw=0.8)
        c.rect(x + CARD_PAD, box_top, 3, box_h, self.color, rx=1.5)
        c.text(x + CARD_PAD + 12, box_top + 14, "行为与约束", size=11.5, weight="600", fill=self.color)
        ty = box_top + 31
        for line in self.lines:
            c.text(x + CARD_PAD + 12, ty, line, size=12, fill=INK)
            ty += 17


def build_features() -> list[FeatureCard]:
    return [
        FeatureCard("F1", "剧情对话与回合操作", "验收 · functional_eval basic", "#2f6fd0", [
            ("用户入口", [
                "ComposerShelf 输入台", "TurnActions 回合操作", "NarrativeStream 正文流",
                "HistoricalStory 历史回看", "POST …/turns 受理",
                "重生成 / 编辑 / 续写 / 取消",
            ]),
            ("后端服务", [
                "TurnService 受理·生成·续写·取消", "TurnLifecycle 回合状态机",
                "CommitPlan 提交与冲突", "Idempotency 幂等键", "EventBus → SSE 推送",
            ]),
            ("数据落点", [
                "plot_nodes 写时复制节点", "action_receipts 回执 + domain_events 事件",
                "turn_requests / turn_attempts", "draft_frames 续写草稿", "turn_usage 用量台账",
            ]),
        ], "提交携带幂等键与期望 head / 版本 / 角色，不符即 409，不猜用户意图；节点、回执与领域事件同一事务落库；"
           "流中断只保留完整块作为可续写草稿；已提交正文不可改写。"),

        FeatureCard("F2", "记忆与认知", "验收 · functional_eval memory", "#12897f", [
            ("用户入口", [
                "MindGraphModal 心智星图", "记忆修订 / 置顶 / 隐藏",
                "POST …/memories/organize 整理", "RightRail 记忆痕迹", "GET …/memories 分页",
            ]),
            ("后端服务", [
                "MemoryService 修订链·可见性·配额", "CognitiveService 后台抽取（reflection 槽）",
                "修订单竞：同一记忆只留一条新修订", "检索：FTS5 + 实体 + 时序",
            ]),
            ("数据落点", [
                "memory_records（supersedes 链）", "memory_fts 全文索引", "memory_mentions 实体关联",
                "memory_batches 抽取批次", "memory_projection_snapshots",
            ]),
        ], "记忆只追加修订、不原地改写；被隐藏或修订掉的记忆不进入提示词但保留回看；"
           "覆盖区间内出现新修订会让该区间摘要失效并在下次维护重做。"),

        FeatureCard("F3", "角色卡 · 人设 · 世界书", "验收 · functional_eval lorebook", "#b8860b", [
            ("用户入口", [
                "CharacterImportModal 导入卡片", "WorldbookModal / LorebookPanel 世界书",
                "LorePopover 命中提示", "RightRail 人设与立绘", "POST /cards/import",
            ]),
            ("后端服务", [
                "Card 解析（PNG 卡 / JSON）· 头像缩略", "宏展开 {{user}} / {{char}}",
                "世界书关键词命中与来源权重", "内置原创角色卡（池夏 · 潮汐港）",
            ]),
            ("数据落点", [
                "lorebook_entries 条目", "lorebook_sources 来源", "会话内角色 / 场景设定",
                "呈现映射（立绘 · 配色，仅前台）",
            ]),
        ], "世界书按关键词命中注入，不是全文常驻；导入的卡先转成本地角色再开新会话；"
           "立绘与配色只影响界面呈现，不进入模型输入。"),

        FeatureCard("F4", "上下文编译与压缩", "验收 · functional_eval compaction", "#8a5cd0", [
            ("用户入口", [
                "ContextMetricsPopover 上下文读数", "StoryCheckpointSection 摘要",
                "GET /api/status 预算计数",
            ]),
            ("后端服务", [
                "context.Compiler 召回·折叠·账本·预算", "CompactorService 摘要维护（压力触发）",
                "ResolveTailPolicy 尾部窗口自适应", "token 校准（reported / estimated）",
            ]),
            ("数据落点", [
                "summary_artifacts（区间 + 来源哈希）", "BudgetReport 与运行计数", "turn_usage 校准样本",
            ]),
        ], "淘汰顺序固定：世界书 → 记忆 → 历史 → 摘要；承重摘要装不下就显式 422，不静默丢历史；"
           "摘要必须严格小于被替代区间才落库；格式漂移先机械归一、再一次修复重试、仍失败进冷却。"),

        FeatureCard("F5", "导演模式", "验收 · functional_eval director", "#c2543a", [
            ("用户入口", [
                "DirectorWorkspace 导演台（左讨论 · 右大纲）", "PUT …/director/draft 草稿",
                "POST …/director/messages 讨论", "POST …/director/commands 命令",
                "/director-requests/{id}/events SSE",
            ]),
            ("后端服务", [
                "DirectorService 讨论·草稿版本·命令·进度", "assist 槽位（未配置回退 primary）",
                "进度判定：每轮最多自动完成一个阶段",
            ]),
            ("数据落点", [
                "director_drafts 工作草稿", "director_requests 异步讨论", "director_commands 幂等收据",
                "director_projections 可重建缓存", "director_event 节点（depth，不增 turnNumber）",
            ]),
        ], "模型只收到全局要求与当前阶段，未来阶段不进提示词；有效回复只成为草稿，不能自行启用；"
           "回退不清除已有正文；预算不足时明确报错并提示缩短大纲或调整窗口。"),

        FeatureCard("F6", "故事图 · 分支 · 历史", "验收 · functional_eval branch", "#2f8f46", [
            ("用户入口", [
                "StoryMapModal 故事图", "LeftRail 会话与分支栏", "HistoricalStory 按节点回看",
                "POST /branches 分叉", "GET /graph 图数据",
            ]),
            ("后端服务", [
                "BranchService 分支", "SessionView 按节点装配视图",
                "WorldState reducer 重放", "分叉按祖先链重建状态",
            ]),
            ("数据落点", [
                "branches（head / version）", "plot_nodes.parent_id 链", "state_snapshots 世界状态快照",
            ]),
        ], "历史不可变：要改变过去就分叉，旧分支原样保留；摘要按 from→to 区间键控，在分支间复用；"
           "回退只调整导演进度，不改写已有正文。"),

        FeatureCard("F7", "剧情包导入导出", "验收 · functional_eval pack", "#5b6b7c", [
            ("用户入口", [
                "导出剧情包（.tavernpack）", "POST /sessions/import 导入", "GET /sessions/{id}/export",
            ]),
            ("后端服务", [
                "ArchiveService 导出·导入", "回执校验与 ID 重写", "拒收引用不一致的包",
            ]),
            ("数据落点", [
                ".tavernpack（zip：剧情图 / 记忆 / 摘要 / 世界书 / 书签）", "导入后 ID 重写、引用重建",
            ]),
        ], "导出的是自洽快照，导入不覆盖已有故事；包内回执用于校验完整性；换机器只需携带这一个文件。"),

        FeatureCard("F8", "模型接入 · 鉴权 · 运行读数", "验收 · basic · provider probe", "#33414f", [
            ("用户入口", [
                "SettingsModal 设置（供应商 / 档案 / 槽位）", "AuthPairingModal 局域网配对",
                "GET /api/status 运行读数", "POST /config/provider/probe 探测",
            ]),
            ("后端服务", [
                "ProviderManager 三槽位 primary / assist / reflection", "探测与用量包装",
                "同源校验 · 六位码配对 · 请求体限额",
            ]),
            ("数据落点", [
                "config/settings.json（槽位与档案）", "config/secrets.json（只落盘不回显）",
                "TAVERNAGENT_KEY_* 环境变量注入", "RuntimeMetrics 运行计数",
            ]),
        ], "默认不调用任何未配置的外部模型；密钥只落盘不回显，读取接口只给存在标识与掩码；"
           "三个槽位可分别指定模型与协议。"),
    ]


def build_panels() -> list[Panel]:
    return [
        Panel("① 一次回合的功能串接", [
            ("输入（F1）", "ComposerShelf → POST /turns：幂等键 + 期望 head / 版本 / 角色"),
            ("受理（F8）", "同源校验 → 配对鉴权 → 请求体限额 → 幂等与冲突判定 → 202 返回 turnId"),
            ("编译（F3 + F2 + F4 + F5）", "角色卡与人设、世界书命中、记忆召回、摘要折叠、检定账本、当前导演阶段"),
            ("预算裁剪（F4）", "固定淘汰顺序；装不下即 422，并给出需要量与三档建议"),
            ("流式生成（F8）", "供应商按 v1 帧协议输出叙述 / 对白 / 心声；失败分类决定是否重试与计费"),
            ("提交（F1 + F6）", "写时复制节点 + 回执 + 领域事件同事务；分支 head 与版本推进"),
            ("推送（F1）", "EventBus → SSE（turn.* / session.* / director.*），浏览器增量渲染"),
            ("后台（F2 + F4 + F5）", "记忆抽取、摘要维护、导演进度判定——都不阻塞当前回合"),
        ], "#2f6fd0", numbered=True),

        Panel("② 功能之间的数据依赖", [
            ("角色卡 · 世界书 → 上下文", "命中条目按关键词进入当轮提示词，未命中不占预算"),
            ("记忆 → 上下文", "FTS5 + 实体 + 时序三路召回，按预算裁剪后注入"),
            ("摘要 ← 记忆修订", "覆盖区间内出现新修订即失效，下次维护重做"),
            ("检定 → 账本", "已发生的检定是不可被叙事反转的硬下界"),
            ("导演 → 上下文", "只注入全局要求与当前阶段，未来阶段不进入"),
            ("提交 → 后台任务", "回合落库后触发记忆抽取与摘要维护（同分支排队合并）"),
            ("分支 ↔ 摘要", "摘要按区间键控，同区间跨分支复用，不重复付费"),
        ], "#12897f"),

        Panel("③ 失败语义：显式而非静默", [
            ("409 冲突", "幂等键或期望 head / 版本 / 角色不符；不猜用户意图"),
            ("422 CONTEXT_OVER_BUDGET", "承重摘要装不下：报出预算、需要量与三档建议"),
            ("422 PROTOCOL_INVALID", "帧不合法（如一行两个 JSON）：不重试、不继续计费"),
            ("503 PROVIDER_UNAVAILABLE", "HTTP 2xx 但零 SSE 载荷、上游报错：可重试"),
            ("截断 → 续写草稿", "正文截断只保留完整块，由玩家决定是否续写"),
            ("摘要冷却", "同类失败连续 3 次 → 冷却 5 分钟或 head 前进，期间不再每回合重付"),
            ("摘要归一 + 一次修复", "格式漂移先机械归一；归一过的摘要计数并记日志"),
            ("摘要必须变小", "不严格小于被替代区间就不落库，避免越压越大"),
        ], "#c2543a"),

        Panel("④ 后台任务与触发条件", [
            ("记忆抽取（CognitiveService）", "回合提交后异步；同分支排队合并，最新一次胜出"),
            ("摘要维护（CompactorService）",
             "压力触发：预估 ≥ 80% 模型窗口，或 ≥ 75% 输入预算；默认保护最近 20 轮、至少 8 轮未压缩"),
            ("导演进度判定（DirectorService）", "正式回合提交时随正文同事务判定；每轮最多推进一个阶段"),
            ("启动恢复（Recovery）", "中断回合清理；未完成讨论标记「已中断」，保留用户消息、不自动重调模型"),
            ("队列语义", "记忆抽取最新胜出；摘要不取消在途任务——产物按区间键控，取消纯浪费"),
        ], "#8a5cd0"),

        Panel("⑤ 运行读数（GET /api/status）", [
            ("回合", "turnsAccepted / turnsCommitted / turnsFailed / turnsCancelled / continuations / "
                     "budgetExceeded / memoriesInjected"),
            ("上下文预算", "budgetDroppedHistory · budgetDroppedSummaries · budgetUnfit"),
            ("摘要维护", "summaryBlocked · summaryNormalized"),
            ("供应商用量", "turn_usage：in_flight → completed / failed / cancelled，含 token 与延迟"),
            ("用途", "functional_eval 与 release_eval 把计数写进 report.json 作为验收证据"),
        ], "#33414f"),
    ]


def render_png(svg_path: Path) -> None:
    """用 web/node_modules 里的 Playwright 把 SVG 渲染成 PNG（保持与 SVG 同宽，2 倍像素密度）。"""
    script = f"""
const fs = require("fs");
// 仓库用 pnpm：顶层只链接 @playwright/test，chromium 从它导出。
const {{ chromium }} = require("@playwright/test");
(async () => {{
  const svg = fs.readFileSync({str(svg_path)!r}, "utf8");
  const browser = await chromium.launch();
  const page = await browser.newPage({{ viewport: {{ width: 1780, height: 1200 }}, deviceScaleFactor: 2 }});
  await page.setContent(`<body style="margin:0">${{svg}}</body>`, {{ waitUntil: "load" }});
  await page.evaluate(() => document.fonts.ready);
  await page.screenshot({{ path: {str(PNG)!r}, fullPage: true }});
  await browser.close();
}})();
"""
    result = subprocess.run(
        ["node", "-e", script],
        cwd=str(ROOT / "web"), capture_output=True, text=True, timeout=180,
    )
    if result.returncode != 0:
        raise SystemExit(f"PNG 渲染失败（SVG 已生成）：{result.stderr.strip()[:500]}")


def main() -> None:
    c = Canvas()
    features = build_features()
    panels = build_panels()

    total_left = sum(f.measure(LEFT_W) for f in features)
    total_right = sum(p.measure(RIGHT_W) for p in panels)

    header_h = 152
    gap = 26
    left_h = total_left + (len(features) - 1) * gap
    right_extra = 0.0
    if total_right < left_h and len(panels) > 1:
        right_extra = min((left_h - total_right) / (len(panels) - 1), 90.0)
    right_h = total_right + right_extra * (len(panels) - 1)
    body_h = max(left_h, right_h)
    height = header_h + body_h + 84

    c.rect(0, 0, W, height, BG, rx=0)
    c.text(MARGIN, 62, "TavernAgent（SillyDog）功能架构", size=30, weight="700", fill=INK)
    c.text(MARGIN, 92,
           "按功能域组织：用户入口 → 后端服务 → 数据落点 → 行为与约束；角标为 functional_eval 的验收场景。"
           "右栏是功能之间在运行时的串接方式。",
           size=14, fill=MUTED)

    legend = [(f.key, f.title, f.color) for f in features]
    lx = MARGIN
    for key, name, color in legend:
        label = f"{key} {name}"
        w = text_w(label, 12.5) + 26
        c.rect(lx, 108, w, 24, "#ffffff", color, rx=12, sw=1.2)
        c.rect(lx + 9, 118, 4, 4, color, rx=2)
        c.text(lx + 17, 125, label, size=12.5, fill=INK)
        lx += w + 8

    y = header_h
    for index, feature in enumerate(features):
        feature.y = y
        feature.draw(c, MARGIN, LEFT_W)
        y += feature.height
        if index < len(features) - 1:
            y += gap

    ry = header_h
    for index, panel in enumerate(panels):
        panel.y = ry
        panel.draw(c, RIGHT_X, RIGHT_W)
        ry += panel.height + 24 + right_extra

    c.text(MARGIN, height - 30,
           "功能域、组件名、服务名、表名与错误码取自仓库当前代码；验收场景取自 scripts/functional_eval.py 的 SCENARIOS。",
           size=11.5, fill=MUTED)
    c.text(W - MARGIN, height - 30,
           "生成：python scripts/function_architecture_diagram.py", size=11.5, fill=MUTED, anchor="end")

    OUT.parent.mkdir(parents=True, exist_ok=True)
    OUT.write_text(
        c.render(height, "TavernAgent 功能架构", "按功能域组织的入口、服务、数据落点与运行时串接"),
        encoding="utf-8",
    )
    print(f"已生成 {OUT.relative_to(ROOT)}（{W}×{height:.0f}）")
    if "--png" in sys.argv:
        render_png(OUT)
        print(f"已生成 {PNG.relative_to(ROOT)}")


if __name__ == "__main__":
    main()
