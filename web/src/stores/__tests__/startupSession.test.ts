// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { recoveryView } from "./turnRecoveryFixtures";

const mocks = vi.hoisted(() => ({
  api: { getSession: vi.fn(), listMemories: vi.fn() },
  turn: vi.fn(), session: vi.fn(),
}));
vi.mock("../../app/api", () => ({ api: mocks.api, subscribeTurnEvents: mocks.turn, subscribeSessionEvents: mocks.session }));
import { useStory } from "../storyStore";
import { useUi } from "../uiStore";
import { chooseStartupSession, lastSessionId, rememberSession } from "../sessionMemory";

beforeEach(async () => {
  await useStory.getState().startNewSessionForCurrentChar();
  vi.resetAllMocks();
  localStorage.clear();
  mocks.turn.mockImplementation(() => vi.fn());
  mocks.session.mockImplementation(() => vi.fn());
  mocks.api.getSession.mockImplementation((id: string) => Promise.resolve(recoveryView(id)));
  mocks.api.listMemories.mockResolvedValue({ memories: [] });
  useUi.setState({ notifyQuiet: vi.fn() });
});
afterEach(async () => {
  await useStory.getState().startNewSessionForCurrentChar();
  localStorage.clear();
});

describe("startup picker", () => {
  const sessions = [{ sessionId: "s1", characterId: "card_other" }, { sessionId: "s2", characterId: "card_elena" }];

  it("prefers the story the user had open over character match and list order", () => {
    const picked = chooseStartupSession(sessions, "s1", (s) => s.characterId === "card_elena");
    expect(picked?.sessionId).toBe("s1");
  });

  it("falls back to the character match, then to the first story", () => {
    expect(chooseStartupSession(sessions, "gone", (s) => s.characterId === "card_elena")?.sessionId).toBe("s2");
    expect(chooseStartupSession(sessions, null, () => false)?.sessionId).toBe("s1");
    expect(chooseStartupSession([], "s1", () => true)).toBeNull();
  });
});

describe("remembered story", () => {
  it("remembers the story the user actually opened", async () => {
    await useStory.getState().openSession("A");
    expect(lastSessionId()).toBe("A");
    expect(useStory.getState().view?.sessionId).toBe("A");
  });

  it("does not overwrite the memory when a fresh open fails", async () => {
    rememberSession("kept");
    mocks.api.getSession.mockRejectedValueOnce(new Error("offline"));
    await useStory.getState().openSession("C");
    expect(lastSessionId()).toBe("kept");
    expect(useStory.getState().error).toContain("offline");
  });

  it("survives an unavailable localStorage", () => {
    const original = Object.getOwnPropertyDescriptor(window, "localStorage");
    Object.defineProperty(window, "localStorage", { configurable: true, get() { throw new Error("blocked"); } });
    expect(() => rememberSession("x")).not.toThrow();
    expect(lastSessionId()).toBeNull();
    if (original) Object.defineProperty(window, "localStorage", original);
  });
});
