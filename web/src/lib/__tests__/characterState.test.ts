import { describe, expect, it } from "vitest";
import { affectionPercent, primaryCharacterId } from "../characterState";
import type { WorldState } from "../../app/types";

describe("character HUD", () => {
  it.each([[-100, 0], [-50, 25], [0, 50], [50, 75], [100, 100], [150, 100]])("maps affection %d to %d percent", (value, percent) => {
    expect(affectionPercent(value)).toBe(percent);
  });
  it("selects one participant even when relationship and mood maps have different ordering", () => {
    const state: WorldState = { characters: { player: { characterId: "player", name: "林舟" }, a: { characterId: "a", name: "向导", participant: true }, b: { characterId: "b", name: "路人" } },
      relationships: { b: { affection: 100, trust: 90, alertness: 0 } }, moods: { a: { moodCode: "calm", text: "平静" } }, items: {}, promises: {} };
    expect(primaryCharacterId(state, null)).toBe("a");
  });
});
