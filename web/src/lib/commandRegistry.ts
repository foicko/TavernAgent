// 命令面板的**装配层**：把 store 里已有的动作映射成命令清单。
//
// 这里只做"翻译"，不新增业务逻辑：每条命令调用的都是界面按钮同一个 action，
// 因此命令面板不可能绕过只读判定、忙碌判定或确认流程。
// 契约与检索逻辑在 lib/commands.ts（那个文件不 import store，供 ui 叶子层复用）。
import { useStory, isTurnBusy } from "../stores/storyStore";
import { useSettings } from "../stores/settingsStore";
import { useUi, type LeftSubtab } from "../stores/uiStore";
import { listImportedCards } from "./characterCardStore";
import { openCard } from "./cardLaunch";
import type { Command } from "./commands";

/** 左栏子页签的展示名（顺序即左栏的实际顺序）。 */
const LEFT_TABS: { tab: LeftSubtab; label: string; keywords: string[] }[] = [
  { tab: "tree", label: "因果树", keywords: ["tree", "yinguoshu", "分支"] },
  { tab: "characters", label: "角色", keywords: ["characters", "juese", "卡库"] },
  { tab: "lore", label: "世界书", keywords: ["lore", "shijieshu", "设定"] },
  { tab: "sessions", label: "故事列表", keywords: ["sessions", "gushi", "存档"] },
];

export function buildCommands(): Command[] {
  const ui = useUi.getState();
  const story = useStory.getState();
  const view = story.view;
  const busy = isTurnBusy(story.phase);
  const commands: Command[] = [];

  // ---- 跳转：故事 ----
  for (const session of story.sessions) {
    const current = session.sessionId === view?.sessionId;
    commands.push({
      id: `session:${session.sessionId}`,
      title: `打开故事：${session.title || "未命名"}`,
      group: "navigation",
      keywords: ["story", "session", "gushi", "dakai", session.characterId],
      hint: current ? "当前" : undefined,
      enabled: !current,
      run: () => void useStory.getState().openSession(session.sessionId),
    });
  }

  // ---- 跳转：角色卡 ----
  for (const card of listImportedCards()) {
    commands.push({
      id: `card:${card.cardId}`,
      title: `切换到角色：${card.name}`,
      group: "navigation",
      keywords: ["card", "character", "juese", "qiehuan", card.shortName, card.role ?? ""],
      run: () => openCard(card),
    });
  }

  // ---- 跳转：分支与查看位置 ----
  const branches = view?.branches ?? [];
  if (branches.length > 1) {
    for (const branch of branches) {
      const current = branch.branchId === view?.branch.branchId;
      commands.push({
        id: `branch:${branch.branchId}`,
        title: `切换到分支：${branch.name}`,
        group: "navigation",
        keywords: ["branch", "fenzhi", branch.branchId],
        hint: current ? "当前分支" : undefined,
        enabled: !current,
        run: () => void useStory.getState().switchBranch(branch.branchId),
      });
    }
  }
  if (view) {
    commands.push({
      id: "story:back-to-head",
      title: "回到最新节点",
      group: "navigation",
      keywords: ["head", "latest", "zuixin", "huidao"],
      hint: story.viewNodeId ? "正在看历史" : undefined,
      enabled: story.viewNodeId !== null,
      run: () => void useStory.getState().backToHead(),
    });
  }

  // ---- 跳转：左栏子页签（顺带把左栏展开，一步到位） ----
  for (const { tab, label, keywords } of LEFT_TABS) {
    commands.push({
      id: `left-tab:${tab}`,
      title: `左栏：${label}`,
      group: "navigation",
      keywords: ["tab", "zuolan", ...keywords],
      enabled: ui.leftTab !== tab || ui.isLeftRailCollapsed,
      run: () => useUi.getState().openLeftTabAndExpand(tab),
    });
  }

  // ---- 剧情 ----
  commands.push(
    {
      id: "story:continue",
      title: "接着写（续写被截断的回合）",
      group: "story",
      keywords: ["continue", "jiezhe", "xuxie"],
      enabled: story.phase === "truncated",
      run: () => void useStory.getState().continueTurn(),
    },
    {
      id: "story:regenerate-head",
      title: "重新生成最后一段",
      group: "story",
      keywords: ["regenerate", "chongsheng", "重写"],
      enabled: !!view && !busy && view.headNode.nodeId !== view.rootNodeId,
      run: () => {
        const head = useStory.getState().view?.headNode.nodeId;
        if (head) void useStory.getState().regenerate(head);
      },
    },
    {
      id: "story:retry",
      title: "重试上一次提交",
      group: "story",
      keywords: ["retry", "chongshi", "失败"],
      enabled: !!view && !!story.lastInput && !story.viewNodeId && story.phase !== "generating" && story.phase !== "committed",
      run: () => void useStory.getState().retry(),
    },
    {
      id: "story:cancel",
      title: "停止当前生成",
      group: "story",
      keywords: ["stop", "cancel", "tingzhi", "zhongduan"],
      enabled: busy,
      run: () => void useStory.getState().cancel(),
    },
    {
      id: "story:new",
      title: "以当前角色开新故事",
      group: "story",
      keywords: ["new", "create", "xingushi", "kaitou"],
      run: () => void useStory.getState().startNewSessionForCurrentChar(),
    },
    {
      id: "story:organize-memories",
      title: "整理记忆（合并与归档）",
      group: "story",
      keywords: ["memory", "organize", "zhengli", "jiyi"],
      enabled: !!view && !busy,
      run: () => void useStory.getState().organizeMemories(),
    },
    {
      id: "story:export-pack",
      title: "导出剧情包",
      group: "story",
      keywords: ["export", "pack", "daochu", "beifen"],
      enabled: !!view,
      run: () => void useStory.getState().exportSession(),
    },
    {
      id: "story:reload",
      title: "重新加载当前故事",
      group: "story",
      keywords: ["reload", "refresh", "shuaxin", "chongxinjiazai"],
      enabled: !!view,
      run: () => void useStory.getState().refreshView(),
    },
  );

  // ---- 视图 ----
  commands.push(
    {
      id: "view:studio",
      title: "切换到工作台",
      group: "view",
      keywords: ["studio", "workbench", "gongzuotai"],
      hint: ui.viewMode === "studio" ? "当前" : undefined,
      enabled: ui.viewMode !== "studio",
      run: () => useUi.getState().switchViewMode("studio"),
    },
    {
      id: "view:reading",
      title: "切换到纯净编年（阅读模式）",
      group: "view",
      keywords: ["reading", "chunjing", "yuedu"],
      hint: ui.viewMode === "reading" ? "当前" : undefined,
      enabled: ui.viewMode !== "reading",
      run: () => useUi.getState().switchViewMode("reading"),
    },
    {
      id: "view:theme",
      title: ui.theme === "dark" ? "切换到乳白纸墨主题" : "切换到墨黑幽夜主题",
      group: "view",
      keywords: ["theme", "dark", "light", "zhuti", "shense", "qianse"],
      run: () => useUi.getState().toggleTheme(),
    },
    {
      id: "view:left-rail",
      title: ui.isLeftRailCollapsed ? "展开左栏" : "收起左栏",
      group: "view",
      keywords: ["rail", "sidebar", "zuolan"],
      hint: "Ctrl+[",
      enabled: ui.viewMode !== "reading",
      run: () => useUi.getState().toggleLeftRail(),
    },
    {
      id: "view:right-rail",
      title: ui.isRightRailCollapsed ? "展开右栏" : "收起右栏",
      group: "view",
      keywords: ["rail", "sidebar", "youlan", "litail"],
      hint: "Ctrl+]",
      enabled: ui.viewMode !== "reading",
      run: () => useUi.getState().toggleRightRail(),
    },
  );

  // ---- 面板 ----
  const panel = (id: string, title: string, keywords: string[], open: () => void, enabled = true): Command => ({
    id,
    title,
    group: "action",
    keywords,
    enabled,
    run: open,
  });

  commands.push(
    panel("panel:settings", "打开模型与偏好设置", ["settings", "shezhi", "peizhi", "moxing"], () => useSettings.getState().openSettings()),
    panel("panel:worldbook", "打开世界书", ["worldbook", "lore", "shijieshu", "shezhi"], () => useUi.getState().setWorldbookModalOpen(true)),
    panel("panel:memory", "打开记忆银行", ["memory", "jiyiyinhang", "jiyi"], () => useUi.getState().setMemoryModalOpen(true), !!view),
    panel("panel:mind-graph", "打开心像图谱", ["graph", "xinxiang", "tupu", "guanxi"], () => useUi.getState().setMindGraphOpen(true), !!view),
    panel("panel:dossier", "打开人物档案", ["dossier", "renwudangan", "renshe"], () => useUi.getState().setDossierModalOpen(true)),
    panel("panel:promises", "打开承诺清单", ["promise", "pledge", "chengnuo"], () => useUi.getState().setPromisesModalOpen(true)),
    panel("panel:tachie", "查看全身立绘", ["tachie", "litai", "quanshen"], () => useUi.getState().setFullTachieOpen(true)),
    panel("panel:import-card", "导入角色卡", ["import", "card", "daoru", "jiaoseka"], () => useUi.getState().setCharImportOpen(true)),
  );

  return commands;
}
