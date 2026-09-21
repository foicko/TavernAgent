// @vitest-environment jsdom
// 命令面板原语的契约测试：键盘可达性与"Esc 只关自己"是它对外承诺的全部行为。
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CommandPalette } from "../CommandPalette";
import type { Command } from "../../lib/commands";

afterEach(cleanup);

function commands(overrides: Partial<Command>[] = []): Command[] {
  const base: Command[] = [
    { id: "a", title: "打开故事：深宫", group: "navigation", run: vi.fn() },
    { id: "b", title: "打开世界书", group: "action", run: vi.fn() },
    { id: "c", title: "停止当前生成", group: "story", enabled: false, run: vi.fn() },
  ];
  overrides.forEach((override, index) => Object.assign(base[index], override));
  return base;
}

/** 当前高亮项（面板用 aria-selected 表达虚拟焦点）。 */
function activeOption(): HTMLElement | undefined {
  return screen.getAllByRole("option").find((node) => node.getAttribute("aria-selected") === "true");
}

describe("CommandPalette", () => {
  it("挂载即聚焦搜索框，并把焦点语义接到活动项上", () => {
    render(<CommandPalette commands={commands()} onClose={() => {}} />);
    const input = screen.getByRole("combobox");
    expect(document.activeElement).toBe(input);
    expect(input.getAttribute("aria-activedescendant")).toBe(activeOption()?.id);
    expect(activeOption()?.textContent).toContain("打开故事：深宫");
  });

  it("输入即过滤，回车执行高亮项并关闭面板", () => {
    const onClose = vi.fn();
    const list = commands();
    render(<CommandPalette commands={list} onClose={onClose} />);

    fireEvent.change(screen.getByRole("combobox"), { target: { value: "世界书" } });
    expect(screen.queryByText("打开故事：深宫")).toBeNull();
    expect(screen.getByText("打开世界书")).toBeTruthy();

    fireEvent.keyDown(screen.getByRole("combobox"), { key: "Enter" });
    expect(list[1].run).toHaveBeenCalledTimes(1);
    expect(list[0].run).not.toHaveBeenCalled();
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("上下键跳过不可用命令，并在首尾循环", () => {
    const list: Command[] = [
      { id: "a", title: "命令甲", group: "action", run: vi.fn() },
      { id: "b", title: "命令乙（不可用）", group: "action", enabled: false, run: vi.fn() },
      { id: "c", title: "命令丙", group: "action", run: vi.fn() },
    ];
    render(<CommandPalette commands={list} onClose={() => {}} />);
    const input = screen.getByRole("combobox");

    // 首项 -> 跳过中间的禁用项 -> 末项
    fireEvent.keyDown(input, { key: "ArrowDown" });
    expect(activeOption()?.textContent).toContain("命令丙");
    // 末项 -> 回绕到首项
    fireEvent.keyDown(input, { key: "ArrowDown" });
    expect(activeOption()?.textContent).toContain("命令甲");
    // 反向同样跳过禁用项
    fireEvent.keyDown(input, { key: "ArrowUp" });
    expect(activeOption()?.textContent).toContain("命令丙");
  });

  it("不可用命令命中回车也不执行", () => {
    const list = commands();
    render(<CommandPalette commands={list} onClose={() => {}} />);
    const input = screen.getByRole("combobox");
    fireEvent.change(input, { target: { value: "停止" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(list[2].run).not.toHaveBeenCalled();
  });

  it("Esc 只关闭面板，并把事件拦在面板内（不触发下层关闭链）", () => {
    const onClose = vi.fn();
    const windowListener = vi.fn();
    window.addEventListener("keydown", windowListener);
    try {
      render(<CommandPalette commands={commands()} onClose={onClose} />);
      fireEvent.keyDown(screen.getByRole("combobox"), { key: "Escape" });
      expect(onClose).toHaveBeenCalledTimes(1);
      expect(windowListener).not.toHaveBeenCalled();
    } finally {
      window.removeEventListener("keydown", windowListener);
    }
  });

  it("Tab 不逃逸出面板", () => {
    render(<CommandPalette commands={commands()} onClose={() => {}} />);
    const input = screen.getByRole("combobox");
    fireEvent.keyDown(input, { key: "Tab" });
    expect(document.activeElement).toBe(input);
  });

  it("没有匹配时给出空状态，回车不执行任何命令", () => {
    const list = commands();
    render(<CommandPalette commands={list} onClose={() => {}} />);
    const input = screen.getByRole("combobox");
    fireEvent.change(input, { target: { value: "zzzz" } });
    expect(screen.queryAllByRole("option")).toHaveLength(0);
    expect(screen.getByText(/没有匹配的命令/)).toBeTruthy();
    fireEvent.keyDown(input, { key: "Enter" });
    for (const command of list) expect(command.run).not.toHaveBeenCalled();
  });

  it("点击命令行直接执行并关闭；点击遮罩只关闭", () => {
    const list = commands();
    const onCloseRow = vi.fn();
    const first = render(<CommandPalette commands={list} onClose={onCloseRow} />);
    fireEvent.mouseDown(screen.getByText("打开世界书"));
    expect(list[1].run).toHaveBeenCalledTimes(1);
    expect(onCloseRow).toHaveBeenCalledTimes(1);
    first.unmount();

    const onCloseBackdrop = vi.fn();
    render(<CommandPalette commands={commands()} onClose={onCloseBackdrop} />);
    fireEvent.mouseDown(screen.getByTestId("command-palette-backdrop"));
    expect(onCloseBackdrop).toHaveBeenCalledTimes(1);
  });
});
