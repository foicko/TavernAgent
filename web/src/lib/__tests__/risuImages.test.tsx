// 单测：RisuAI 图像指令 <img cmd> → 卡内资源图片的渲染。
import { describe, expect, it } from "vitest";
import { renderWithCardImages } from "../risuImages";

const assets = { smile: "/api/v1/cards/card_x/assets/smile.webp" };

function flatten(nodes: unknown): string {
  if (nodes == null) return "";
  if (typeof nodes === "string" || typeof nodes === "number") return String(nodes);
  if (Array.isArray(nodes)) return nodes.map(flatten).join("");
  const el = nodes as { type: unknown; props?: { children?: unknown } };
  if (el.type === "img") return "";
  return flatten(el.props?.children);
}

describe("renderWithCardImages", () => {
  it("无资源映射时保留字面文本", () => {
    const out = renderWithCardImages("你好。<img cmd=\"smile\">", undefined);
    expect(out.every((n) => typeof n === "string")).toBe(true);
    expect(flatten(out)).toContain("<img cmd=\"smile\">");
  });

  it("命中的 cmd 渲染为图片，且前后文本保留", () => {
    const out = renderWithCardImages("前<img cmd=\"smile\">后", assets);
    const img = out.find((n) => typeof n !== "string" && (n as { type: unknown }).type === "img") as {
      props: { src: string; alt: string };
    };
    expect(img).toBeDefined();
    expect(img.props.src).toBe(assets.smile);
    expect(img.props.alt).toBe("smile");
    expect(flatten(out)).toContain("前");
    expect(flatten(out)).toContain("后");
  });

  it("未命中的 cmd 原样保留，不产生图片", () => {
    const out = renderWithCardImages("<img cmd=\"unknown\">", assets);
    expect(out.some((n) => typeof n !== "string" && (n as { type: unknown }).type === "img")).toBe(false);
    expect(flatten(out)).toContain("<img cmd=\"unknown\">");
  });

  it("无后缀 cmd 回退到带数字后缀的变体（RisuAI 命名）", () => {
    const variants = {
      "anis_casual_excited1": "/a/1.webp",
      "anis_casual_excited2": "/a/2.webp",
      "anis_casual_excited3": "/a/3.webp",
    };
    const out = renderWithCardImages("<img cmd=\"anis_casual_excited\">", variants);
    const img = out.find((n) => typeof n !== "string" && (n as { type: unknown }).type === "img") as {
      props: { src: string };
    };
    expect(img).toBeDefined();
    expect(["/a/1.webp", "/a/2.webp", "/a/3.webp"]).toContain(img.props.src);
  });
});
