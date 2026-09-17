// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { recoveryView } from "./turnRecoveryFixtures";

const mocks = vi.hoisted(() => ({
  api: { getSession: vi.fn(), listMemories: vi.fn() },
  turn: vi.fn(), session: vi.fn(),
}));
vi.mock("../../app/api", () => ({ api: mocks.api, subscribeTurnEvents: mocks.turn, subscribeSessionEvents: mocks.session }));
import { clearSessionCache, useStory } from "../storyStore";
import { useUi } from "../uiStore";

beforeEach(async () => {
  await useStory.getState().startNewSessionForCurrentChar();
  vi.resetAllMocks();
  clearSessionCache();
  mocks.turn.mockImplementation(() => vi.fn());
  mocks.session.mockImplementation(() => vi.fn());
  mocks.api.getSession.mockImplementation((id: string) => Promise.resolve(recoveryView(id)));
  mocks.api.listMemories.mockResolvedValue({ memories: [] });
  useUi.setState({ notifyQuiet: vi.fn() });
});
afterEach(async () => {
  await useStory.getState().startNewSessionForCurrentChar();
});

describe("session stream revival", () => {
  it("rebuilds the subscription when the reader returns after retries were exhausted", async () => {
    await useStory.getState().openSession("A");
    const before = mocks.session.mock.calls.length;
    const reads = mocks.api.getSession.mock.calls.length;
    mocks.session.mock.calls.at(-1)![1].onFinalError(new Error("SSE reconnect exhausted"));
    window.dispatchEvent(new Event("focus"));
    await vi.waitFor(() => expect(mocks.session.mock.calls.length).toBe(before + 1));
    await vi.waitFor(() => expect(mocks.api.getSession.mock.calls.length).toBeGreaterThan(reads));
  });

  it("stays quiet while the reader has not come back", async () => {
    await useStory.getState().openSession("A");
    const before = mocks.session.mock.calls.length;
    mocks.session.mock.calls.at(-1)![1].onFinalError(new Error("SSE reconnect exhausted"));
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(mocks.session.mock.calls.length).toBe(before);
  });

  it("does not resurrect a subscription for a story the reader has left", async () => {
    await useStory.getState().openSession("A");
    const failed = mocks.session.mock.calls.at(-1)![1];
    await useStory.getState().openSession("B");
    const before = mocks.session.mock.calls.length;
    failed.onFinalError(new Error("exhausted"));
    window.dispatchEvent(new Event("focus"));
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(mocks.session.mock.calls.length).toBe(before);
  });
});
