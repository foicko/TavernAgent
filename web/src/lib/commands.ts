// 命令面板的数据契约与检索逻辑——**本文件不依赖任何 store**。
//
// 为什么把"契约"和"装配"分开：`ui/` 是叶子层（只能依赖 react 与 lib）。模块只要被
// import 就会整体求值，因此如果这里写 `import { useUi } from "../stores/uiStore"`，
// ui/CommandPalette 引用本文件里的一个纯函数也会把整张 store 依赖图拖进叶子层。
// 具体命令的装配放在 lib/commandRegistry.ts，那里才 import store。

export type CommandGroup = "navigation" | "story" | "view" | "action";

/** 分组展示顺序：先跳转，再剧情，再视图，最后面板。 */
export const GROUP_ORDER: CommandGroup[] = ["navigation", "story", "view", "action"];

export const GROUP_LABELS: Record<CommandGroup, string> = {
  navigation: "跳转",
  story: "剧情",
  view: "视图",
  action: "面板",
};

export interface Command {
  /** 稳定标识：测试与快捷键绑定都用它，不要用标题（标题会随文案变）。 */
  id: string;
  title: string;
  group: CommandGroup;
  /** 别名：英文名、拼音首字母、角色短名等，参与匹配但不显示。 */
  keywords?: string[];
  /** 右侧提示：当前值或已有快捷键。 */
  hint?: string;
  /** 不可用判定；传函数则每次渲染时求值（例如"正在生成中"会变）。 */
  enabled?: boolean | (() => boolean);
  run: () => void | Promise<void>;
}

export function isCommandEnabled(command: Command): boolean {
  if (command.enabled === undefined) return true;
  return typeof command.enabled === "function" ? command.enabled() : command.enabled;
}

/** 顺序包含：查询串的字符按序出现在标题里即算命中（"sj" 命中"世界书"）。 */
function subsequenceScore(needle: string, haystack: string): number {
  let cursor = 0;
  for (const ch of haystack) {
    if (ch === needle[cursor]) cursor += 1;
    if (cursor === needle.length) return 60;
  }
  return 0;
}

/** 打分越高越靠前；0 表示不匹配。标题命中优于别名命中，短标题优于长标题。 */
export function scoreCommand(command: Command, query: string): number {
  const q = query.trim().toLowerCase();
  if (!q) return 1;
  const title = command.title.toLowerCase();
  if (title === q) return 1000;
  if (title.startsWith(q)) return 600 - Math.min(title.length, 60);
  if (title.includes(q)) return 400 - Math.min(title.length, 60);
  for (const keyword of command.keywords ?? []) {
    const k = keyword.toLowerCase();
    if (!k) continue;
    if (k === q) return 350;
    if (k.startsWith(q)) return 300;
    if (k.includes(q)) return 200;
  }
  return subsequenceScore(q, title);
}

/**
 * 过滤并排序。空查询按分组顺序返回全部命令（面板因此可以直接按顺序渲染分组标题）。
 * 同分时回落到分组顺序与定义顺序，保证结果稳定——否则每敲一个字符列表都会跳动。
 */
export function filterCommands(commands: Command[], query: string): Command[] {
  const q = query.trim();
  if (!q) {
    return GROUP_ORDER.flatMap((group) => commands.filter((command) => command.group === group));
  }
  return commands
    .map((command, index) => ({ command, index, score: scoreCommand(command, q) }))
    .filter((entry) => entry.score > 0)
    .sort(
      (a, b) =>
        b.score - a.score ||
        GROUP_ORDER.indexOf(a.command.group) - GROUP_ORDER.indexOf(b.command.group) ||
        a.index - b.index,
    )
    .map((entry) => entry.command);
}

export interface CommandSection {
  group: CommandGroup;
  label: string;
  /** 每项带上它在**扁平列表**里的下标：键盘高亮与 aria-activedescendant 都按下标走。 */
  items: { command: Command; index: number }[];
}

/** 按组切段，同时给出扁平下标（渲染分组标题时不能打乱下标）。 */
export function sectionize(commands: Command[]): CommandSection[] {
  const sections: CommandSection[] = [];
  commands.forEach((command, index) => {
    const last = sections[sections.length - 1];
    if (last && last.group === command.group) last.items.push({ command, index });
    else sections.push({ group: command.group, label: GROUP_LABELS[command.group], items: [{ command, index }] });
  });
  return sections;
}
