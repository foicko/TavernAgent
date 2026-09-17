import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { deferred, recoveryView } from "./turnRecoveryFixtures";

const mocks = vi.hoisted(() => ({
  api: { getSession: vi.fn(), listMemories: vi.fn(), acceptTurn: vi.fn(), getTurn: vi.fn(), continueTurn: vi.fn() },
  turn: vi.fn(), session: vi.fn(),
}));
vi.mock("../../app/api", () => ({ api: mocks.api, subscribeTurnEvents: mocks.turn, subscribeSessionEvents: mocks.session }));
import { clearSessionCache, useStory } from "../storyStore";
import { useSettings } from "../settingsStore";
import { useUi } from "../uiStore";

beforeEach(async () => {
  await useStory.getState().startNewSessionForCurrentChar();
  clearSessionCache();
  vi.resetAllMocks();
  vi.useFakeTimers();
  mocks.turn.mockImplementation(() => vi.fn());
  mocks.session.mockImplementation(() => vi.fn());
  mocks.api.getSession.mockImplementation((id: string) => Promise.resolve(recoveryView(id)));
  mocks.api.listMemories.mockResolvedValue({ memories: [] });
  mocks.api.acceptTurn.mockResolvedValue({ turnId: "t1", status: "preparing" });
  mocks.api.getTurn.mockResolvedValue({ turnId: "t1", status: "generating" });
  mocks.api.continueTurn.mockResolvedValue({ status: "preparing" });
  useSettings.setState({ autoContinue: false });
  useUi.setState({ notifyQuiet: vi.fn() });
});
afterEach(async () => {
  await useStory.getState().startNewSessionForCurrentChar();
  vi.useRealTimers();
});

async function start() {
  await useStory.getState().openSession("A");
  await useStory.getState().send("沿着码头前行。");
  mocks.turn.mock.calls[0][1].onEvent({ id: "t1:1", event: "block.appended",
    data: { frameSeq: 1, frame: { kind: "narration", text: "向导收好了地图。" } } });
}
function complete() {
  mocks.api.getTurn.mockResolvedValue({ turnId: "t1", status: "committed" });
  mocks.api.getSession.mockResolvedValue(recoveryView("A", true));
}
function expectFinished() {
  expect(useStory.getState().phase).toBe("idle");
  expect(useStory.getState().messages.at(-1)?.options).toHaveLength(3);
  expect(useStory.getState().draftBlocks).toEqual([]);
  expect(useStory.getState().view?.branch.headNodeId).toBe("turn_A");
}

describe("turn completion reconciliation", () => {
  it("recovers the options when the terminal event is lost but the stream remains open", async () => {
    await start(); complete();
    await vi.advanceTimersByTimeAsync(5000);
    expectFinished();
    expect(mocks.api.acceptTurn).toHaveBeenCalledOnce();
    const reads = mocks.api.getTurn.mock.calls.length;
    await vi.advanceTimersByTimeAsync(60000);
    expect(mocks.api.getTurn).toHaveBeenCalledTimes(reads);
  });

  it("shows committed options without waiting for the auxiliary memory request", async () => {
    await start(); complete();
    const memories = deferred<{ memories: never[] }>();
    mocks.api.listMemories.mockReturnValue(memories.promise);
    mocks.turn.mock.calls[0][1].onEvent({ id: "t1:3", event: "turn.committed", data: {} });
    await vi.advanceTimersByTimeAsync(0);
    expectFinished();
    expect(useStory.getState().memoryLoading).toBe(true);
    memories.resolve({ memories: [] });
    await vi.advanceTimersByTimeAsync(0);
  });

  it("releases the completed turn before a slow memory read can interfere with the next send", async () => {
    await start(); complete();
    const memories = deferred<{ memories: never[] }>();
    mocks.api.listMemories.mockReturnValue(memories.promise);
    mocks.turn.mock.calls[0][1].onEvent({ id: "t1:3", event: "turn.committed", data: {} });
    await vi.advanceTimersByTimeAsync(0);
    expectFinished();
    const accepting = deferred<{ turnId: string; status: string }>();
    mocks.api.acceptTurn.mockReturnValueOnce(accepting.promise);
    const sending = useStory.getState().send("查看灯塔。");
    memories.resolve({ memories: [] });
    await vi.advanceTimersByTimeAsync(5000);
    expect(useStory.getState().phase).toBe("generating");
    expect(mocks.api.getSession).toHaveBeenCalledTimes(2);
    accepting.resolve({ turnId: "t2", status: "preparing" });
    await sending;
    expect(useStory.getState().lastTurnId).toBe("t2");
    expect(useStory.getState().phase).toBe("generating");
  });

  it("retries a failed final view refresh without allowing another turn against the old head", async () => {
    await start(); complete();
    mocks.api.getSession.mockRejectedValueOnce(new Error("temporary view failure"));
    mocks.turn.mock.calls[0][1].onEvent({ id: "t1:3", event: "turn.committed", data: {} });
    await vi.advanceTimersByTimeAsync(0);
    expect(useStory.getState().phase).toBe("committed");
    expect(useStory.getState().draftBlocks).toHaveLength(1);
    await useStory.getState().send("不要用旧进度重发。");
    expect(mocks.api.acceptTurn).toHaveBeenCalledOnce();
    await vi.advanceTimersByTimeAsync(5000);
    expectFinished();
    expect(useStory.getState().error).toBeNull();
  });

  it("keeps a transient status-read failure recoverable", async () => {
    await start(); complete();
    mocks.api.getTurn.mockRejectedValueOnce(new Error("temporary status failure"));
    await vi.advanceTimersByTimeAsync(5000);
    expect(useStory.getState().phase).toBe("generating");
    expect(useStory.getState().draftBlocks).toHaveLength(1);
    await vi.advanceTimersByTimeAsync(5000);
    expectFinished();
  });

  it("checks status immediately after reconnect", async () => {
    await start(); complete();
    mocks.turn.mock.calls[0][1].onReconnect?.(1);
    await vi.advanceTimersByTimeAsync(0);
    expectFinished();
  });

  it("does not regress a known commit when an older status read finishes", async () => {
    await start();
    const previous = deferred<{ status: string }>();
    mocks.api.getTurn.mockReturnValueOnce(previous.promise);
    await vi.advanceTimersByTimeAsync(5000);
    complete();
    mocks.turn.mock.calls[0][1].onEvent({ id: "t1:3", event: "turn.committed", data: {} });
    previous.resolve({ status: "generating" });
    await vi.advanceTimersByTimeAsync(0);
    expectFinished();
  });

  it("retries status failures after an ambiguous continuation response", async () => {
    await start();
    useStory.setState({ phase: "truncated" });
    mocks.api.continueTurn.mockRejectedValueOnce(new Error("continuation response lost"));
    await useStory.getState().continueTurn();
    complete();
    mocks.api.getTurn.mockRejectedValueOnce(new Error("temporary status failure"));
    await vi.advanceTimersByTimeAsync(5000);
    expect(useStory.getState().phase).toBe("truncated");
    await vi.advanceTimersByTimeAsync(5000);
    expectFinished();
    expect(mocks.api.continueTurn).toHaveBeenCalledOnce();
  });

  it("ignores a dead subscription after reopening the same session and same turn", async () => {
    await start();
    const old = mocks.turn.mock.calls[0][1];
    await useStory.getState().openSession("B");
    const active = recoveryView(); active.branch.activeTurnId = "t1";
    mocks.api.getSession.mockResolvedValue(active);
    await useStory.getState().openSession("A");
    const stopCurrent = mocks.turn.mock.results[1].value;
    old.onFinalError(new Error("late disconnect"));
    old.onEvent({ id: "t1:3", event: "turn.failed", data: { message: "late failure" } });
    await vi.advanceTimersByTimeAsync(0);
    expect(stopCurrent).not.toHaveBeenCalled();
    expect(useStory.getState().phase).toBe("generating");
    expect(useStory.getState().error).toBeNull();
  });

  it("does not let a late truncation status undo an accepted continuation", async () => {
    await start();
    const previous = deferred<{ status: string }>();
    mocks.api.getTurn.mockReturnValueOnce(previous.promise);
    mocks.turn.mock.calls[0][1].onEvent({ id: "t1:2", event: "turn.truncated", data: {} });
    await vi.advanceTimersByTimeAsync(0);
    useStory.setState({ phase: "truncated" });
    await useStory.getState().continueTurn();
    previous.resolve({ status: "awaiting_continuation" });
    await vi.advanceTimersByTimeAsync(0);
    expect(useStory.getState().phase).toBe("generating");
    expect(mocks.api.continueTurn).toHaveBeenCalledOnce();
  });

  it.each(["awaiting_continuation", "awaiting_approval"])("recovers %s when its notification is missing", async status => {
    await start();
    mocks.api.getTurn.mockResolvedValue({ turnId: "t1", status, input: { kind: "text", text: "原始输入" } });
    await vi.advanceTimersByTimeAsync(5000);
    expect(useStory.getState().phase).toBe(status === "awaiting_continuation" ? "truncated" : "awaiting_confirm");
    expect(useStory.getState().draftBlocks).toHaveLength(1);
    expect(mocks.api.continueTurn).not.toHaveBeenCalled();
  });

  it("does not time out a healthy long generation just because the event transport failed", async () => {
    await start();
    mocks.turn.mock.calls[0][1].onFinalError(new Error("stream unavailable"));
    await vi.advanceTimersByTimeAsync(65000);
    expect(useStory.getState().phase).toBe("generating");
    complete();
    await vi.advanceTimersByTimeAsync(5000);
    expectFinished();
  });
});
