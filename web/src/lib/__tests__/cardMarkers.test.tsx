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
