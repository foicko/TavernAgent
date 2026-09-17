// 单测：会话→角色归属解析（自定义卡不再误判为内置角色的回归面）。
import { describe, expect, it } from "vitest";
import type { WorldState } from "../../app/types";
import { resolveCharKeyFromSession } from "../storyStore";

function ws(characters: WorldState["characters"]): WorldState {
  return { relationships: {}, items: {}, promises: {}, moods: {}, characters };
}

describe("resolveCharKeyFromSession", () => {
  it("无匹配预设的会话按 characters 字典返回 null（自定义角色）", () => {
    expect(
      resolveCharKeyFromSession({
        state: ws({ npc_other: { characterId: "npc_other", name: "旅人" } }),
      }),
    ).toBeNull();
  });

  it("自定义卡（随机 characterId）返回 null 而非误判内置角色（修复点）", () => {
    expect(
      resolveCharKeyFromSession({
        title: "我的私设角色",
        state: ws({ npc_a3f9c2_k: { characterId: "npc_a3f9c2_k", name: "阿雾" } }),
      }),
    ).toBeNull();
  });

  it("无状态无标题的会话返回 null", () => {
    expect(resolveCharKeyFromSession({})).toBeNull();
    expect(resolveCharKeyFromSession(null)).toBeNull();
  });

  it("无匹配预设的标题返回 null", () => {
    expect(resolveCharKeyFromSession({ title: "未知的故事" })).toBeNull();
  });

  it("custom 键不参与归属（它只是显示兜底）", () => {
    expect(
      resolveCharKeyFromSession({
        state: ws({ custom: { characterId: "custom", name: "自定义角色" } }),
      }),
    ).toBeNull();
  });
});
