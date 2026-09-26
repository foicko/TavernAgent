// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { deferred, recoveryView } from "./turnRecoveryFixtures";
import type { SessionView } from "../../app/types";

const mocks = vi.hoisted(() => ({
  api: { getSession: vi.fn(), listMemories: vi.fn() },
  turn: vi.fn(), session: vi.fn(),
}));
vi.mock("../../app/api", () => ({ api: mocks.api, subscribeTurnEvents: mocks.turn, subscribeSessionEvents: mocks.session }));
import { useStory } from "../storyStore";
import { useUi } from "../uiStore";

beforeEach(async () => {
  await useStory.getState().startNewSessionForCurrentChar();
  vi.resetAllMocks();
  mocks.turn.mockImplementation(() => vi.fn());
  mocks.session.mockImplementation(() => vi.fn());
  mocks.api.getSession.mockImplementation((id: string) => Promise.resolve(recoveryView(id)));
  mocks.api.listMemories.mockResolvedValue({ memories: [] });
  useUi.setState({ notifyQuiet: vi.fn() });
  await useStory.getState().openSession("A");
});
afterEach(async () => {
  await useStory.getState().startNewSessionForCurrentChar();
});

describe("view navigation failures", () => {
  it("rolls back to head when opening a historical node fails", async () => {
    mocks.api.getSession.mockRejectedValueOnce(new Error("network down"));
    await useStory.getState().viewAt("turn_A");
    const st = useStory.getState();
    expect(st.viewNodeId).toBeNull();
    expect(st.error).toContain("已还原");
    expect(st.error).toContain("network down");
    expect(st.phase).toBe("idle");
  });

  it("restores the node the reader was on when leaving history fails", async () => {
    await useStory.getState().viewAt("turn_A");
    expect(useStory.getState().viewNodeId).toBe("turn_A");
    mocks.api.getSession.mockRejectedValueOnce(new Error("boom"));
    await useStory.getState().backToHead();
    expect(useStory.getState().viewNodeId).toBe("turn_A");
    expect(useStory.getState().error).toContain("已还原");
  });

  it("keeps the previous branch when switching branch fails", async () => {
    mocks.api.getSession.mockRejectedValueOnce(new Error("boom"));
    await useStory.getState().switchBranch("branch_other");
    expect(useStory.getState().activeBranchId).toBe("branch_A");
    expect(useStory.getState().error).toContain("已还原");
    expect(useStory.getState().phase).toBe("idle");
  });

  it("clears the previous error once navigation succeeds", async () => {
    await useStory.getState().viewAt("turn_A");
    expect(useStory.getState().error).toBeNull();
  });

  // 缓存命中后的后台校验很慢时，用户已经切到别的分支：迟到的默认分支视图不能把它覆盖回来
  it("a slow cache-hit refresh does not undo a branch switch", async () => {
    const refresh = deferred<SessionView>();
    const other = recoveryView("A");
    other.branch = { ...other.branch, branchId: "branch_other" };
    mocks.api.getSession
      .mockImplementationOnce(() => refresh.promise)
      .mockImplementationOnce(() => Promise.resolve(other));

    const opening = useStory.getState().openSession("A");
    expect(useStory.getState().view?.sessionId).toBe("A"); // 缓存命中立即上屏
    await useStory.getState().switchBranch("branch_other");
    expect(useStory.getState().activeBranchId).toBe("branch_other");

    refresh.resolve(recoveryView("A"));
    await opening;
    expect(useStory.getState().activeBranchId).toBe("branch_other");
    expect(useStory.getState().view?.branch.branchId).toBe("branch_other");
  });
});
