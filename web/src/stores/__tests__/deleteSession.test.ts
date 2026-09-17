// 删除故事：不可撤销，重点覆盖"删掉的正是当前故事"这条路径。
// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { recoveryView } from "./turnRecoveryFixtures";

const mocks = vi.hoisted(() => ({
  api: { getSession: vi.fn(), listMemories: vi.fn(), listSessions: vi.fn(), deleteSession: vi.fn() },
  turn: vi.fn(), session: vi.fn(),
}));
vi.mock("../../app/api", () => ({ api: mocks.api, subscribeTurnEvents: mocks.turn, subscribeSessionEvents: mocks.session }));
import { useStory } from "../storyStore";
import { useUi } from "../uiStore";

const summary = (id: string, title: string) => ({
  sessionId: id, title, characterId: `card_${id}`, createdAt: "2026-01-01T00:00:00Z", rootNodeId: `root_${id}`,
});

beforeEach(async () => {
  await useStory.getState().startNewSessionForCurrentChar();
  vi.resetAllMocks();
  localStorage.clear();
  mocks.turn.mockImplementation(() => vi.fn());
  mocks.session.mockImplementation(() => vi.fn());
  mocks.api.getSession.mockImplementation((id: string) => Promise.resolve(recoveryView(id)));
  mocks.api.listMemories.mockResolvedValue({ memories: [] });
  mocks.api.listSessions.mockResolvedValue({ sessions: [summary("s1", "一号故事"), summary("s2", "二号故事")] });
  mocks.api.deleteSession.mockResolvedValue({ ok: true });
  useUi.setState({ notifyQuiet: vi.fn() });
});

afterEach(async () => {
  await useStory.getState().startNewSessionForCurrentChar();
  localStorage.clear();
});

describe("删除故事", () => {
  it("deletes through the API and refreshes the list", async () => {
    await useStory.getState().loadSessions();
    await useStory.getState().openSession("s1");
    const callsBefore = mocks.api.listSessions.mock.calls.length;

    await useStory.getState().deleteSession("s1");

    expect(mocks.api.deleteSession).toHaveBeenCalledWith("s1");
    // 列表必须以服务端为准重新拉取，不能只在前端本地过滤。
    expect(mocks.api.listSessions.mock.calls.length).toBeGreaterThan(callsBefore);
    expect(useUi.getState().notifyQuiet).toHaveBeenCalledWith(expect.stringContaining("已删除"));
  });

  it("switches to another story when the deleted one was open", async () => {
    mocks.api.listSessions
      .mockResolvedValueOnce({ sessions: [summary("s1", "一号故事"), summary("s2", "二号故事")] })
      .mockResolvedValue({ sessions: [summary("s2", "二号故事")] });
    await useStory.getState().loadSessions();
    await useStory.getState().openSession("s1");
    expect(useStory.getState().view?.sessionId).toBe("s1");

    await useStory.getState().deleteSession("s1");

    // 不能停在一个已不存在的会话上：必须切到剩下的那条。
    expect(useStory.getState().view?.sessionId).toBe("s2");
    expect(localStorage.getItem("tavernagent.lastSessionId")).toBe("s2");
    expect(useUi.getState().notifyQuiet).toHaveBeenCalledWith(expect.stringContaining("已切换到其它故事"));
  });

  it("returns to the empty start state when the last story is deleted", async () => {
    mocks.api.listSessions
      .mockResolvedValueOnce({ sessions: [summary("s1", "唯一的故事")] })
      .mockResolvedValue({ sessions: [] });
    await useStory.getState().loadSessions();
    await useStory.getState().openSession("s1");

    await useStory.getState().deleteSession("s1");

    expect(useStory.getState().view).toBeNull();
    expect(useStory.getState().messages).toEqual([]);
    expect(localStorage.getItem("tavernagent.lastSessionId")).toBeNull();
  });

  it("keeps the current story when a different one is deleted", async () => {
    await useStory.getState().loadSessions();
    await useStory.getState().openSession("s2");

    await useStory.getState().deleteSession("s1");

    expect(useStory.getState().view?.sessionId).toBe("s2");
    expect(useUi.getState().notifyQuiet).toHaveBeenCalledWith("故事已删除");
  });

  it("surfaces the server refusal instead of pretending success", async () => {
    mocks.api.deleteSession.mockRejectedValue(new Error("该故事还有正在进行的回合，请先取消或等它结束后再删除"));
    await useStory.getState().loadSessions();
    await useStory.getState().openSession("s1");

    await expect(useStory.getState().deleteSession("s1")).rejects.toThrow(/正在进行的回合/);
    expect(useStory.getState().view?.sessionId).toBe("s1");
  });
});
