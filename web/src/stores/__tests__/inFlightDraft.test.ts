// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { recoveryView } from "./turnRecoveryFixtures";

const mocks = vi.hoisted(() => ({
  api: { getSession: vi.fn(), listMemories: vi.fn(), acceptTurn: vi.fn(), getTurn: vi.fn() },
  turn: vi.fn(), session: vi.fn(),
}));
vi.mock("../../app/api", () => ({ api: mocks.api, subscribeTurnEvents: mocks.turn, subscribeSessionEvents: mocks.session }));
import { clearSessionCache, useStory } from "../storyStore";
import { useUi } from "../uiStore";

beforeEach(async () => {
  await useStory.getState().startNewSessionForCurrentChar();
  vi.resetAllMocks();
  clearSessionCache();
  vi.useFakeTimers();
  mocks.turn.mockImplementation(() => vi.fn());
  mocks.session.mockImplementation(() => vi.fn());
  mocks.api.getSession.mockImplementation((id: string) => Promise.resolve(recoveryView(id)));
  mocks.api.listMemories.mockResolvedValue({ memories: [] });
  mocks.api.acceptTurn.mockResolvedValue({ turnId: "t1", status: "preparing" });
  mocks.api.getTurn.mockResolvedValue({ turnId: "t1", status: "generating" });
  useUi.setState({ notifyQuiet: vi.fn() });
});
afterEach(async () => {
  await useStory.getState().startNewSessionForCurrentChar();
  vi.useRealTimers();
});

function snapshot(seq: number, text: string, kind = "narration") {
  return { turnId: "t1", status: "generating", draft: { attemptId: "a1", inFlight: { seq, kind, text } } };
}

async function start() {
  await useStory.getState().openSession("A");
  await useStory.getState().send("沿着码头前行。");
  mocks.turn.mock.calls[0][1].onEvent({ id: "t1:1", event: "block.appended",
    data: { frameSeq: 1, frame: { kind: "narration", text: "向导收好了地图。" } } });
}

describe("in-flight block recovery", () => {
  it("fills the block being written after a reconnect gap", async () => {
    await start();
    // 断线期间模型写完了第 1 块并开始第 2 块：第 1 块由 durable 回放补齐，
    // 第 2 块只能由上途快照补齐。
    mocks.api.getTurn.mockResolvedValue({ ...snapshot(2, "「你听，海在说话", "dialogue"),
      resultNode: null });
    mocks.turn.mock.calls[0][1].onReconnect(1);
    await vi.advanceTimersByTimeAsync(0);
    const blocks = useStory.getState().draftBlocks;
    expect(blocks).toHaveLength(2);
    expect(blocks[1].text).toBe("「你听，海在说话");
    expect(blocks[1].kind).toBe("dialogue");
    // 快照之后的增量继续追加在同一块上，不重复。
    mocks.turn.mock.calls[0][1].onEvent({ event: "block.delta", data: { seq: 2, kind: "dialogue", delta: "。」" } });
    expect(useStory.getState().draftBlocks[1].text).toBe("「你听，海在说话。」");
  });

  it("never rewinds a block that already has more text", async () => {
    await start();
    mocks.turn.mock.calls[0][1].onEvent({ event: "block.delta", data: { seq: 2, kind: "narration", delta: "潮水漫过甲板，远处灯塔亮了。" } });
    mocks.api.getTurn.mockResolvedValue(snapshot(2, "潮水漫过甲板"));
    mocks.turn.mock.calls[0][1].onReconnect(1);
    await vi.advanceTimersByTimeAsync(0);
    expect(useStory.getState().draftBlocks[1].text).toBe("潮水漫过甲板，远处灯塔亮了。");
  });

  it("adopts the snapshot when it is ahead of the local draft", async () => {
    await start();
    mocks.turn.mock.calls[0][1].onEvent({ event: "block.delta", data: { seq: 2, kind: "narration", delta: "潮水" } });
    mocks.api.getTurn.mockResolvedValue(snapshot(2, "潮水漫过甲板，远处灯塔亮了。"));
    mocks.turn.mock.calls[0][1].onReconnect(1);
    await vi.advanceTimersByTimeAsync(0);
    expect(useStory.getState().draftBlocks[1].text).toBe("潮水漫过甲板，远处灯塔亮了。");
  });

  it("ignores a snapshot for a turn that is no longer on screen", async () => {
    await start();
    mocks.api.getTurn.mockResolvedValue(snapshot(2, "不应该出现"));
    await useStory.getState().startNewSessionForCurrentChar();
    mocks.turn.mock.calls[0][1].onReconnect(1);
    await vi.advanceTimersByTimeAsync(0);
    expect(useStory.getState().draftBlocks).toHaveLength(0);
  });
});
