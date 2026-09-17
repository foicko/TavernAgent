import { beforeEach, describe, expect, it, vi } from "vitest";
import type { DirectorDraft, DirectorPlan, DirectorView, SessionView } from "../../app/types";

const mocks = vi.hoisted(() => ({ getDirector: vi.fn(), saveDirectorDraft: vi.fn(), directorCommand: vi.fn(), directorMessage: vi.fn(), cancelDirectorRequest: vi.fn() }));
vi.mock("../../app/api", () => ({ api: mocks, subscribeTurnEvents: vi.fn(), subscribeSessionEvents: vi.fn() }));
import { useStory } from "../storyStore";
import { emptyDirectorState } from "../slices/directorSlice";

const plan: DirectorPlan = { title: "相遇", guidance: "自然", beats: [{ beatId: "one", title: "相遇", instruction: "交换近况", completionCriteria: "双方明确说出近况" }] };
function view(id: string): SessionView {
  return { sessionId: id, characterId: `char_${id}`, title: id, rootNodeId: `${id}_root`,
    branch: { branchId: `${id}_branch`, name: "main", headNodeId: `${id}_head`, version: 1 }, state: null,
    headNode: { nodeId: `${id}_head`, sessionId: id, parentId: `${id}_root`, kind: "turn", depth: 1, turnNumber: 1, contentJson: "{}", createdAt: "" } };
}
function workspace(id: string): DirectorView {
  return { state: null, requests: [], readOnly: false, viewNodeId: `${id}_head`,
    draft: { sessionId: id, branchId: `${id}_branch`, version: 2, baseRevisionId: "", plan: { ...plan, title: id }, updatedAt: "" } };
}
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>(done => { resolve = done; }); return { promise, resolve }; }
function select(id: string) { useStory.setState({ ...emptyDirectorState, view: view(id), activeBranchId: `${id}_branch`, viewNodeId: null, phase: "idle" }); }

beforeEach(() => { vi.clearAllMocks(); useStory.setState(useStory.getInitialState(), true); select("A"); });

describe("director request ownership", () => {
  it("ignores a director load that arrives after switching stories", async () => {
    const late = deferred<DirectorView>(); mocks.getDirector.mockReturnValueOnce(late.promise).mockResolvedValueOnce(workspace("B"));
    const first = useStory.getState().loadDirector(); select("B"); await useStory.getState().loadDirector();
    late.resolve(workspace("A")); await first;
    expect(useStory.getState().directorView?.draft?.plan.title).toBe("B");
    expect(useStory.getState().directorLoading).toBe(false);
  });

  it("saves to the captured branch and never replaces another branch's draft", async () => {
    useStory.setState({ directorView: workspace("A") });
    const late = deferred<DirectorDraft>(); mocks.saveDirectorDraft.mockReturnValueOnce(late.promise);
    const first = useStory.getState().saveDirectorDraft(plan, 2, ""); select("B"); useStory.setState({ directorView: workspace("B") });
    late.resolve({ ...workspace("A").draft!, version: 3 }); await first;
    expect(mocks.saveDirectorDraft.mock.calls[0].slice(0, 2)).toEqual(["A", "A_branch"]);
    expect(useStory.getState().directorView?.draft?.plan.title).toBe("B");
    expect(useStory.getState().directorSaving).toBe(false);
  });

  it("does not let a slow read replace a newer saved draft", async () => {
    useStory.setState({ directorView: workspace("A"), directorScope: "A/A_branch/head" });
    const write = deferred<DirectorDraft>(), read = deferred<DirectorView>();
    mocks.saveDirectorDraft.mockReturnValueOnce(write.promise);
    mocks.getDirector.mockReturnValueOnce(read.promise);
    const saving = useStory.getState().saveDirectorDraft({ ...plan, title: "最新修改" }, 2, "");
    const loading = useStory.getState().loadDirector();
    write.resolve({ ...workspace("A").draft!, version: 3, plan: { ...plan, title: "最新修改" } }); await saving;
    read.resolve(workspace("A")); await loading;
    expect(useStory.getState().directorView?.draft?.version).toBe(3);
    expect(useStory.getState().directorView?.draft?.plan.title).toBe("最新修改");
  });

  it("reuses the command key after a lost response", async () => {
    useStory.setState({ directorView: workspace("A"), reloadView: vi.fn().mockResolvedValue(undefined) });
    mocks.directorCommand.mockRejectedValueOnce(new Error("lost response")).mockResolvedValueOnce({ nodeId: "new", version: 2 });
    mocks.getDirector.mockResolvedValue(workspace("A"));
    await expect(useStory.getState().commandDirector("activate")).rejects.toThrow("lost response");
    await useStory.getState().commandDirector("activate");
    expect(mocks.directorCommand.mock.calls[0]).toEqual(mocks.directorCommand.mock.calls[1]);
    expect(mocks.directorCommand.mock.calls[0][2]).toMatchObject({ expectedHeadId: "A_head", expectedVersion: 1, draftVersion: 2 });
  });

  it("does not attach a late discussion to the newly selected story", async () => {
    useStory.setState({ directorView: workspace("A") });
    const late = deferred<{ requestId: string; request: { requestId: string } }>(); mocks.directorMessage.mockReturnValueOnce(late.promise);
    const first = useStory.getState().discussDirector("让他们重逢"); select("B"); useStory.setState({ directorView: workspace("B") });
    late.resolve({ requestId: "A_request", request: { requestId: "A_request" } }); await first;
    expect(useStory.getState().directorView?.requests).toEqual([]);
  });

  it("keeps history read only and prevents commands during an active turn", async () => {
    useStory.setState({ viewNodeId: "A_old" });
    await expect(useStory.getState().saveDirectorDraft(plan, 2, "")).rejects.toThrow("历史视图只读");
    await useStory.getState().commandDirector("pause"); await useStory.getState().discussDirector("修改");
    useStory.setState({ viewNodeId: null, phase: "truncated" }); await useStory.getState().commandDirector("activate");
    expect(mocks.directorCommand).not.toHaveBeenCalled(); expect(mocks.directorMessage).not.toHaveBeenCalled();
  });
});
