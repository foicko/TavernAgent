// 单测：RisuAI 图像指令 <img cmd> → 卡内资源图片的渲染。
import { describe, expect, it } from "vitest";
import { renderWithCardImages, resolveCardAsset } from "../risuImages";

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

  it("兼容 <img=、<img_obj=、<imgBG= 旧式与变体写法", () => {
    for (const tag of [`<img="smile">`, `<img_obj="smile">`, `<imgBG="smile">`]) {
      const out = renderWithCardImages(`前${tag}后`, assets);
      const img = out.find((n) => typeof n !== "string" && (n as { type: unknown }).type === "img") as {
        props: { src: string };
      };
      expect(img?.props.src).toBe(assets.smile);
    }
  });

  it("<img src> 带附加属性时整段替换为图片，不残留属性文本", () => {
    const out = renderWithCardImages(
      `看<img src="smile" relation="Sister" favorability="20">了`,
      assets
    );
    const img = out.find((n) => typeof n !== "string" && (n as { type: unknown }).type === "img") as {
      props: { src: string };
    };
    expect(img?.props.src).toBe(assets.smile);
    expect(flatten(out)).not.toContain("relation");
  });

  it("resolveCardAsset 忽略大小写与文件扩展名", () => {
    expect(resolveCardAsset({ "has_Neutral": "/n.webp" }, "has_neutral")).toBe("/n.webp");
    expect(resolveCardAsset({ "robe.default.avif": "/r.avif" }, "Robe.default")).toBe("/r.avif");
    expect(resolveCardAsset({ "a1": "/1", "a2": "/2" }, "a")).toMatch(/^\/[12]$/);
    expect(resolveCardAsset({ "x": "/x" }, "missing")).toBeUndefined();
  });

  it("resolveCardAsset 覆盖「数字+扩展名」命名", () => {
    // sayaka 型：标记 dressed_happy，资源 dressed_happy1.png
    expect(resolveCardAsset({ "dressed_happy1.png": "/h1" }, "dressed_happy")).toBe("/h1");
    expect(resolveCardAsset({ "dressed_neutral2.webp": "/n2" }, "dressed_neutral")).toBe("/n2");
    // 标记带扩展名、资源不带
    expect(resolveCardAsset({ "main": "/m" }, "main.png")).toBe("/m");
    // 资源带扩展名、标记不带
    expect(resolveCardAsset({ "main.png": "/mp" }, "main")).toBe("/mp");
  });

  it("resolveCardAsset 不把非图片后缀当作扩展名剥离", () => {
    // nami.flustered 是完整资源名，标记 nami 不应命中它
    expect(resolveCardAsset({ "nami.flustered": "/n" }, "nami")).toBeUndefined();
  });
});
