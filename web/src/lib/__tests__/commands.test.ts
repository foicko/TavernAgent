// 命令匹配的契约测试：面板的"能不能搜到、搜出来排第几"全靠这几个纯函数。
import { describe, expect, it } from "vitest";
import { filterCommands, isCommandEnabled, scoreCommand, sectionize, type Command } from "../commands";

function makeCommand(id: string, title: string, group: Command["group"] = "action", extra: Partial<Command> = {}): Command {
  return { id, title, group, run: () => {}, ...extra };
}

describe("命令匹配与排序", () => {
  it("空查询保留分组顺序，不按定义顺序打乱", () => {
    const commands = [
      makeCommand("panel", "打开设置", "action"),
      makeCommand("tab", "左栏：世界书", "navigation"),
      makeCommand("tab2", "左栏：角色", "navigation"),
    ];
    expect(filterCommands(commands, "  ").map((c) => c.id)).toEqual(["tab", "tab2", "panel"]);
  });

  it("标题命中优于别名命中", () => {
    const commands = [
      makeCommand("by-keyword", "记忆银行", "action", { keywords: ["世界"] }),
      makeCommand("by-title", "世界书", "action", { keywords: ["shijieshu"] }),
    ];
    expect(filterCommands(commands, "世界").map((c) => c.id)).toEqual(["by-title", "by-keyword"]);
  });

  it("拼音首字母别名可命中", () => {
    const commands = [makeCommand("lore", "打开世界书", "action", { keywords: ["lore", "sjs"] })];
    expect(filterCommands(commands, "sjs").map((c) => c.id)).toEqual(["lore"]);
  });

  it("同分时保持定义顺序，避免敲字时列表跳动", () => {
    const commands = [makeCommand("a", "打开甲"), makeCommand("b", "打开乙")];
    expect(scoreCommand(commands[0], "打开")).toBe(scoreCommand(commands[1], "打开"));
    expect(filterCommands(commands, "打开").map((c) => c.id)).toEqual(["a", "b"]);
  });

  it("完全不匹配时返回空列表", () => {
    const commands = [makeCommand("a", "打开世界书"), makeCommand("b", "整理记忆")];
    expect(filterCommands(commands, "zzzz")).toEqual([]);
  });

  it("英文标题支持顺序包含", () => {
    expect(scoreCommand(makeCommand("t", "Toggle Theme"), "tt")).toBeGreaterThan(0);
  });

  it("分组切段时保留扁平下标（键盘高亮按下标走）", () => {
    const sections = sectionize([
      makeCommand("nav", "跳转", "navigation"),
      makeCommand("act1", "面板一", "action"),
      makeCommand("act2", "面板二", "action"),
    ]);
    expect(sections.map((s) => [s.group, s.items.map((i) => i.index)])).toEqual([
      ["navigation", [0]],
      ["action", [1, 2]],
    ]);
  });
});

describe("命令可用性", () => {
  it("未声明即可用", () => {
    expect(isCommandEnabled(makeCommand("a", "命令"))).toBe(true);
  });

  it("布尔与函数两种写法都认", () => {
    expect(isCommandEnabled(makeCommand("a", "命令", "action", { enabled: false }))).toBe(false);
    expect(isCommandEnabled(makeCommand("b", "命令", "action", { enabled: () => false }))).toBe(false);
    expect(isCommandEnabled(makeCommand("c", "命令", "action", { enabled: () => true }))).toBe(true);
  });
});
