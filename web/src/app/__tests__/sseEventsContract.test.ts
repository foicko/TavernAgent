import { describe, expect, expectTypeOf, it } from "vitest";
import type { TurnEventType } from "../stream";
import {
  DIRECTOR_EVENT_TYPES,
  TERMINAL_TURN_EVENT_TYPES,
  TURN_EVENT_TYPES,
  type GeneratedTurnEventType,
} from "../sseEvents.generated";

// 契约测试：唯一来源是 testdata/contract/sse-events.json（Go 读同一份，Kotlin 由脚本生成）。
describe("SSE event contract", () => {
  it("TurnEventType 与契约完全一致", () => {
    expectTypeOf<TurnEventType>().toEqualTypeOf<GeneratedTurnEventType>();
  });

  it("事件清单非空、无重复，终态是子集", () => {
    expect(TURN_EVENT_TYPES.length).toBeGreaterThan(0);
    expect(new Set(TURN_EVENT_TYPES).size).toBe(TURN_EVENT_TYPES.length);
    for (const name of TERMINAL_TURN_EVENT_TYPES) {
      expect(TURN_EVENT_TYPES as readonly string[]).toContain(name);
    }
    expect(DIRECTOR_EVENT_TYPES.length).toBeGreaterThan(0);
  });
});
