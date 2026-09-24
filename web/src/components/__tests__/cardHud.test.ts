import { describe, expect, it } from "vitest";
import { parseMeter } from "../CardHUD";
import { latestSceneTitle } from "../../lib/cardScene";
import type { UIHint } from "../../app/types";

describe("parseMeter", () => {
  it("n/m 解析为进度", () => {
    expect(parseMeter("82/100")).toEqual({ text: "82/100", fraction: 0.82 });
  });
  it("n% 解析为进度", () => {
    expect(parseMeter("40%")).toEqual({ text: "40%", fraction: 0.4 });
  });
  it("越界裁剪到 0–1", () => {
    expect(parseMeter("150/100").fraction).toBe(1);
    expect(parseMeter("-5/100").fraction).toBe(0);
  });
  it("非数值只返回文本", () => {
    expect(parseMeter("静谧")).toEqual({ text: "静谧" });
    expect(parseMeter("1/0").fraction).toBeUndefined();
  });
});

describe("latestSceneTitle", () => {
  it("取最近一条 scene", () => {
    const messages: { uiHints?: UIHint[] }[] = [
      { uiHints: [{ kind: "scene", text: "教堂" }] },
      { uiHints: [{ kind: "hud", values: { a: "1" } }] },
      { uiHints: [{ kind: "scene", text: "集市" }] },
    ];
    expect(latestSceneTitle(messages)).toBe("集市");
  });
  it("无 scene 返回空串", () => {
    expect(latestSceneTitle([{ uiHints: [{ kind: "hud", values: { a: "1" } }] }])).toBe("");
    expect(latestSceneTitle([])).toBe("");
  });
});
