import { describe, expect, it } from "vitest";
import { composerBusy, directorBusy, historyReadOnly, isTurnBusy, storyBusy } from "../storyHelpers";
import type { TurnPhase } from "../storyTypes";

const PHASES: TurnPhase[] = ["idle", "generating", "truncated", "awaiting_confirm", "committed", "failed"];

function state(phase: TurnPhase, extra: Partial<{ viewNodeId: string | null; directorCommanding: boolean }> = {}) {
  return { phase, viewNodeId: null, directorCommanding: false, ...extra };
}

// 这些判定是按钮可用性的唯一出处；关系一旦被改坏，就会出现
// "输入台能用但回合操作用不了"这类不一致。用例锁的是关系，不是具体实现。
describe("busy and read-only predicates", () => {
  it("keeps composer and director gates in step with the story gate", () => {
    for (const phase of PHASES) {
      for (const extra of [{}, { directorCommanding: true }, { viewNodeId: "n1" }]) {
        const s = state(phase, extra);
        if (composerBusy(s)) expect(storyBusy(s) || historyReadOnly(s)).toBe(true);
        if (directorBusy(s)) expect(storyBusy(s) || historyReadOnly(s)).toBe(true);
        if (historyReadOnly(s)) expect(composerBusy(s) && directorBusy(s)).toBe(true);
        if (isTurnBusy(phase)) expect(storyBusy(s) && composerBusy(s)).toBe(true);
      }
    }
  });

  it("treats every unfinished phase as busy and history as read-only", () => {
    expect(PHASES.filter(isTurnBusy)).toEqual(["generating", "truncated", "awaiting_confirm", "committed"]);
    expect(storyBusy(state("idle", { directorCommanding: true }))).toBe(true);
    expect(composerBusy(state("idle"))).toBe(false);
    expect(directorBusy(state("idle"))).toBe(false);
  });

  it("does not block normal composing while idle at the head", () => {
    expect(historyReadOnly(state("idle"))).toBe(false);
    expect(composerBusy(state("failed"))).toBe(false);
  });
});
