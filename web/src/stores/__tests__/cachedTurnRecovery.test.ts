// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { recoveryView, deferred } from "./turnRecoveryFixtures";
import type { SessionView } from "../../app/types";

const mocks = vi.hoisted(() => ({
  api: { getSession: vi.fn(), listMemories: vi.fn(), getTurn: vi.fn() },
  turn: vi.fn(), session: vi.fn(),
}));
vi.mock("../../app/api", () => ({ api: mocks.api, subscribeTurnEvents: mocks.turn, subscribeSessionEvents: mocks.session }));
import { useStory } from "../storyStore";
import { useUi } from "../uiStore";

function runningView(id = "A", turnId = "t9"): SessionView {
  const view = recoveryView(id);
  view.branch = { ...view.branch, activeTurnId: turnId };
  return view;
}

beforeEach(async () => {
  await useStory.getState().startNewSessionForCurrentChar();
  vi.resetAllMocks();
  mocks.turn.mockImplementation(() => vi.fn());
  mocks.session.mockImplementation(() => vi.fn());
  mocks.api.getSession.mockImplementation(() => Promise.resolve(runningView()));
  mocks.api.listMemories.mockResolvedValue({ memories: [] });
  mocks.api.getTurn.mockResolvedValue({ turnId: "t9", status: "generating" });
  useUi.setState({ notifyQuiet: vi.fn() });
});
afterEach(async () => {
  await useStory.getState().startNewSessionForCurrentChar();
});

describe("cached story reopen", () => {
  it("attaches the running turn on the cached view before the validation fetch resolves", async () => {
    await useStory.getState().openSession("A");
    mocks.turn.mockClear();
    const pending = deferred<SessionView>();
    mocks.api.getSession.mockReturnValue(pending.promise);
    const opening = useStory.getState().openSession("A");
    await Promise.resolve();
    expect(mocks.turn).toHaveBeenCalledWith("t9", expect.anything());
    expect(useStory.getState().phase).toBe("generating");
    pending.resolve(runningView());
    await opening;
  });

  it("keeps the running turn alive when the validation fetch fails", async () => {
    await useStory.getState().openSession("A");
    mocks.turn.mockClear();
    mocks.api.getSession.mockRejectedValueOnce(new Error("offline"));
    await useStory.getState().openSession("A");
    const st = useStory.getState();
    expect(mocks.turn).toHaveBeenCalledWith("t9", expect.anything());
    expect(st.phase).toBe("generating");
    expect(st.lastTurnId).toBe("t9");
    expect(st.error).toContain("offline");
  });
});
