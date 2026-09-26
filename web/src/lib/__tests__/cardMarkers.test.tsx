// 单测：卡片 UI 标记（[IMG]/[HUD]/[SCENE]/[VAR] 与 <img cmd>）的解析与渲染。
import { describe, expect, it } from "vitest";
import { parseCardMarkers, renderCardMarkers } from "../cardMarkers";

const assets = { smile: "/api/v1/cards/card_x/assets/smile.webp" };

function flatten(nodes: unknown): string {
  if (nodes == null) return "";
  if (typeof nodes === "string" || typeof nodes === "number") return String(nodes);
  if (Array.isArray(nodes)) return nodes.map(flatten).join("");
  const el = nodes as { type: unknown; props?: { children?: unknown } };
  if (el.type === "img") return "";
  return flatten(el.props?.children);
}

describe("parseCardMarkers", () => {
  it("剥离状态标记并保留文本", () => {
    const segs = parseCardMarkers("雪停了。[SCENE:教堂][HUD:理智=80|体力=55]");
    expect(segs.map((s) => s.kind)).toEqual(["text", "scene", "hud"]);
    expect(segs[0].text).toBe("雪停了。");
    expect(segs[1].hint).toEqual({ kind: "scene", text: "教堂" });
    expect(segs[2].hint).toEqual({ kind: "hud", values: { 理智: "80", 体力: "55" } });
  });

  it("[IMG:key] 与 <img cmd> 解析为图片", () => {
    const segs = parseCardMarkers(`<img cmd="smile"> [IMG:smile]`, assets);
    expect(segs.filter((s) => s.kind === "image")).toHaveLength(2);
  });

  it("兼容 <img=、<img_obj=、<imgBG= 与 <img src=> 多种写法", () => {
    const segs = parseCardMarkers(
      `<img="smile"> <img_obj="smile"> <imgBG="smile"> <img src="smile" relation="Sister" favorability="20">`,
      assets
    );
    expect(segs.filter((s) => s.kind === "image")).toHaveLength(4);
    // 带附加属性的 src 写法必须整段吃掉，不能把 relation=… 残留为文本。
    const leftovers = segs
      .filter((s) => s.kind === "text")
      .map((s) => s.text ?? "")
      .join("");
    expect(leftovers).not.toContain("relation");
    expect(leftovers).not.toContain("favorability");
  });

  it("忽略大小写命中资源（neutral vs Neutral）", () => {
    const mixed = { "has_Neutral": "/api/v1/cards/c/assets/has_Neutral.webp" };
    const segs = parseCardMarkers(`<img src="has_neutral" relation="x">`, mixed);
    expect(segs.filter((s) => s.kind === "image")).toHaveLength(1);
  });

  it("无扩展名的键回退到带扩展名的资源（robe.default → robe.default.avif）", () => {
    const withExt = { "robe.default.avif": "/api/v1/cards/c/assets/robe.default.avif" };
    const segs = parseCardMarkers(`<img_obj="Robe.default">`, withExt);
    expect(segs.filter((s) => s.kind === "image")).toHaveLength(1);
  });

  it("VAR 不在正文中展示", () => {
    const segs = parseCardMarkers("[VAR:lang=KO]正文");
    expect(segs.map((s) => s.kind)).toEqual(["text"]);
    expect(segs[0].text).toBe("正文");
  });

  it("转义 [[ 还原为字面量 [", () => {
    const segs = parseCardMarkers("写作 [[HUD:x]");
    expect(segs.map((s) => s.kind)).toEqual(["text"]);
    expect(segs[0].text).toBe("写作 [HUD:x]");
  });

  it("未命中的图片标记原样保留", () => {
    const segs = parseCardMarkers(`<img cmd="unknown">`, assets);
    expect(segs).toHaveLength(1);
    expect(segs[0].kind).toBe("text");
    expect(segs[0].text).toBe(`<img cmd="unknown">`);
  });
});

describe("renderCardMarkers", () => {
  it("命中图片渲染为 img，且前后文本保留", () => {
    const out = renderCardMarkers(`前<img cmd="smile">后`, assets);
    const img = out.find((n) => typeof n !== "string" && (n as { type: unknown }).type === "img") as {
      props: { src: string };
    };
    expect(img.props.src).toBe(assets.smile);
    expect(flatten(out)).toContain("前");
    expect(flatten(out)).toContain("后");
  });

  it("无资源映射时图片标记保留字面文本", () => {
    const out = renderCardMarkers(`你好。[IMG:smile]`, undefined);
    expect(flatten(out)).toContain("[IMG:smile]");
  });
});
