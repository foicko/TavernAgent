// 单测：正文内联 Markdown 渲染器（设计 D2 契约 + 上一轮修复的回归面）。
import { describe, expect, it } from "vitest";
import { renderInlineMarkdown } from "../inlineMarkdown";

function flat(nodes: ReturnType<typeof renderInlineMarkdown>): string {
  return nodes
    .map((n) => (typeof n === "string" ? n : String((n as { props: { children: unknown } }).props.children)))
    .join("");
}

describe("renderInlineMarkdown", () => {
  it("无标记文本原样返回", () => {
    const out = renderInlineMarkdown("暴雨夜，你推开酒馆的门。");
    expect(flat(out)).toBe("暴雨夜，你推开酒馆的门。");
    expect(out.some((n) => typeof n !== "string")).toBe(false);
  });

  it("*动作* 渲染为 em（叙事动作描写）", () => {
    const out = renderInlineMarkdown("她*微微一笑*，推过酒杯。");
    const em = out.find((n) => typeof n !== "string");
    expect(em).toBeDefined();
    expect((em as unknown as { type: string }).type).toBe("em");
    expect(flat(out)).toBe("她微微一笑，推过酒杯。");
  });

  it("**强调** 渲染为 strong", () => {
    const out = renderInlineMarkdown("这是**关键信物**。");
    const strong = out.find((n) => typeof n !== "string");
    expect((strong as unknown as { type: string }).type).toBe("strong");
    expect(flat(out)).toBe("这是关键信物。");
  });

  it("粗体优先于斜体（** 不被误拆为两个斜体）", () => {
    const out = renderInlineMarkdown("**银鸢尾徽章**");
    expect(out.find((n) => typeof n !== "string")).toMatchObject({ type: "strong" });
  });

  it("空字符串与孤立星号不产生节点", () => {
    expect(renderInlineMarkdown("")).toEqual([]);
    const out = renderInlineMarkdown("3 * 4 的乘积");
    expect(out.every((n) => typeof n === "string")).toBe(true);
    expect(flat(out)).toBe("3 * 4 的乘积");
  });
});
