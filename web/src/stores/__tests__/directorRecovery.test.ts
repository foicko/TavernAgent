// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { DirectorDraft, DirectorPlan, DirectorView, SessionView } from "../../app/types";

const mocks = vi.hoisted(() => ({
  getDirector: vi.fn(), saveDirectorDraft: vi.fn(), directorCommand: vi.fn(), directorMessage: vi.fn(),
  cancelDirectorRequest: vi.fn(), getDirectorRequest: vi.fn(), subscribe: vi.fn(),
}));
vi.mock("../../app/api", () => ({
  api: mocks, subscribeTurnEvents: vi.fn(), subscribeSessionEvents: vi.fn(), subscribeDirectorEvents: mocks.subscribe,
}));
import { useStory } from "../storyStore";
import { emptyDirectorState } from "../slices/directorSlice";

const plan: DirectorPlan = { title: "相遇", guidance: "自然", beats: [{ beatId: "one", title: "相遇", instruction: "交换近况", completionCriteria: "双方明确说出近况" }] };
function view(id: string): SessionView {
  return { sessionId: id, characterId: `char_${id}`, title: id, rootNodeId: `${id}_root`,
    branch: { branchId: `${id}_branch`, name: "main", headNodeId: `${id}_head`, version: 1 }, state: null,
    headNode: { nodeId: `${id}_head`, sessionId: id, parentId: `${id}_root`, kind: "turn", depth: 1, turnNumber: 1, contentJson: "{}", createdAt: "" } };
}
function workspace(id: string, requests: DirectorView["requests"] = []): DirectorView {
  return { state: null, requests, readOnly: false, viewNodeId: `${id}_head`,
    draft: { sessionId: id, branchId: `${id}_branch`, version: 2, baseRevisionId: "", plan: { ...plan, title: id }, updatedAt: "" } as DirectorDraft };
}
function select(id: string) {
  useStory.setState({ ...emptyDirectorState, view: view(id), activeBranchId: `${id}_branch`, viewNodeId: null, phase: "idle",
    directorScope: `${id}/${id}_branch/head` });
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.useFakeTimers();
  useStory.setState(useStory.getInitialState(), true);
  select("A");
  mocks.subscribe.mockReturnValue(vi.fn());
});
afterEach(() => { vi.useRealTimers(); });

async function discuss(requestId = "req_1"): Promise<void> {
  useStory.setState({ directorView: workspace("A") });
  mocks.directorMessage.mockResolvedValue({ requestId, request: { requestId, status: "generating" } });
  await useStory.getState().discussDirector("让他们重逢");
}

describe("director discussion recovery", () => {
  it("keeps reconciling a running discussion with nothing watching the workspace", async () => {
    mocks.getDirectorRequest.mockResolvedValue({ requestId: "req_1", status: "generating" });
    await discuss();
    // 面板没挂载也要订阅：讨论属于故事，不属于某个打开的窗口。
    expect(mocks.subscribe).toHaveBeenCalledWith("req_1", expect.anything());
    await vi.advanceTimersByTimeAsync(0);
    expect(mocks.getDirectorRequest).toHaveBeenCalledTimes(1);

    mocks.getDirector.mockResolvedValue(workspace("A", [{ requestId: "req_1", status: "completed" }] as never));
    mocks.getDirectorRequest.mockResolvedValue({ requestId: "req_1", status: "completed" });
    await vi.advanceTimersByTimeAsync(3000);
    expect(mocks.getDirector).toHaveBeenCalledTimes(1);
    expect(useStory.getState().directorView?.requests?.[0]).toMatchObject({ requestId: "req_1", status: "completed" });

    const reads = mocks.getDirectorRequest.mock.calls.length;
    await vi.advanceTimersByTimeAsync(30000);
    expect(mocks.getDirectorRequest).toHaveBeenCalledTimes(reads);
  });

  it("shows the terminal status even when the follow-up refresh fails", async () => {
    mocks.getDirectorRequest.mockResolvedValue({ requestId: "req_1", status: "failed" });
    mocks.getDirector.mockRejectedValue(new Error("offline"));
    await discuss();
    await vi.advanceTimersByTimeAsync(0);
    await vi.advanceTimersByTimeAsync(0);
    expect(useStory.getState().directorView?.requests?.[0]).toMatchObject({ requestId: "req_1", status: "failed" });
  });

  it("stops tracking once the reader switches to another story", async () => {
    mocks.getDirectorRequest.mockResolvedValue({ requestId: "req_1", status: "generating" });
    await discuss();
    await vi.advanceTimersByTimeAsync(0);
    const reads = mocks.getDirectorRequest.mock.calls.length;
    select("B");
    await vi.advanceTimersByTimeAsync(9000);
    expect(mocks.getDirectorRequest).toHaveBeenCalledTimes(reads);
    expect(useStory.getState().directorView).toBeNull();
  });
});
