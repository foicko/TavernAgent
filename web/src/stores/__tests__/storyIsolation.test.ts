import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { SessionView } from "../../app/types";

const mocks = vi.hoisted(() => ({
  api: { listSessions: vi.fn(), getSession: vi.fn(), listMemories: vi.fn(), createSession: vi.fn(), acceptTurn: vi.fn(), patchMemory: vi.fn(), getTurn: vi.fn(), continueTurn: vi.fn(), cancelTurn: vi.fn(), forkBranch: vi.fn(), regenerateTurn: vi.fn(), editTurn: vi.fn(), importSessionPack: vi.fn() },
  turn: vi.fn(), session: vi.fn(),
}));
vi.mock("../../app/api", () => ({ api: mocks.api, subscribeTurnEvents: mocks.turn, subscribeSessionEvents: mocks.session }));
import { clearSessionCache, routeEvent, useStory } from "../storyStore";
import { useUi } from "../uiStore";
import { useSettings } from "../settingsStore";
import { DEFAULT_PRESET_KEY } from "../../lib/characterPresets";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<T>((r, fail) => { resolve = r; reject = fail; });
  return { promise, resolve, reject };
}
function view(id: string, characterId = `custom_${id}`): SessionView {
  return { sessionId: id, characterId, title: "同名角色", rootNodeId: `root_${id}`,
    branch: { branchId: `branch_${id}`, name: "main", headNodeId: `head_${id}`, version: 2 },
    headNode: { nodeId: `head_${id}`, sessionId: id, parentId: `root_${id}`, kind: "memory_change", depth: 3, turnNumber: 1, contentJson: "{}", createdAt: "" },
    nodes: [{ nodeId: `turn_${id}`, sessionId: id, parentId: `root_${id}`, kind: "turn", depth: 1, turnNumber: 1, contentJson: JSON.stringify({ blocks: [{ kind: "narration", text: id }], options: [{ optionId: "o1", intent: "clever", text: "继续" }] }), createdAt: "" }],
    state: { characters: {}, items: {}, relationships: {}, promises: {}, moods: {} }, hasMore: true, oldestTurnId: `turn_${id}` };
}
beforeEach(async () => {
  await useStory.getState().startNewSessionForCurrentChar();
  clearSessionCache(); // 视图缓存在用例间共享，不清会互相污染
  vi.resetAllMocks();
  mocks.turn.mockReturnValue(vi.fn());
  mocks.session.mockReturnValue(vi.fn());
  mocks.api.listMemories.mockResolvedValue({ memories: [] });
  useStory.setState({ sessions: [] });
  useUi.getState().setActiveCharKey(null);
  useSettings.setState({ autoContinue: false, optionMode: "direct" });
});
afterEach(async () => {
  await useStory.getState().startNewSessionForCurrentChar();
  vi.useRealTimers();
});

describe("story request ownership", () => {
  it("prepares only registered inventory actions and preserves the action reference", async () => {
    const current = view("A"); current.actions = [{ actionId: "drink", label: "饮茶", attribute: "wisdom", dc: 12, itemIds: ["tea"] }];
    mocks.api.getSession.mockResolvedValue(current); await useStory.getState().openSession("A");
    useStory.getState().prepareAction("饮茶", "invented");
    expect(useStory.getState().pendingInput).toBeNull();
    useStory.getState().prepareAction("饮茶", "drink");
    expect(useStory.getState().pendingAction).toBe("drink");
    expect(useStory.getState().pendingInput).toBe("饮茶");
    useStory.getState().clearPending();
    useStory.setState({ viewNodeId: "history" });
    useStory.getState().prepareAction("饮茶", "drink");
    expect(useStory.getState().pendingAction).toBeNull();
  });

  it("a creation retry reuses its key and preserves the chosen identity and opening", async () => {
    const options = { characterJson: "{}", playerName: "林舟", playerRole: "游历者", openingVariantId: "harbor" };
    mocks.api.createSession.mockRejectedValueOnce(new Error("response lost")).mockResolvedValue({ sessionId: "new" });
    await expect(useStory.getState().createSession(options)).rejects.toThrow("response lost");
    mocks.api.getSession.mockResolvedValue(view("new"));
    mocks.api.listSessions.mockResolvedValue({ sessions: [] });
    await useStory.getState().createSession(options);
    const first = mocks.api.createSession.mock.calls[0][0];
    const second = mocks.api.createSession.mock.calls[1][0];
    expect(first.idempotencyKey).toBeTruthy();
    expect(second).toEqual(first);
    expect(second).toMatchObject(options);
  });

  it("a list refresh failure after creation still opens the durable story", async () => {
    mocks.api.createSession.mockResolvedValue({ sessionId: "new" });
    mocks.api.getSession.mockResolvedValue(view("new"));
    mocks.api.listSessions.mockRejectedValue(new Error("list failed"));
    await expect(useStory.getState().createSession({ characterJson: "{}", playerName: "林舟" })).resolves.toMatchObject({ sessionId: "new" });
    expect(useStory.getState().view?.sessionId).toBe("new");
    expect(useStory.getState().error).toBeNull();
    expect(useStory.getState().phase).toBe("idle");
  });

  it("a late creation response cannot switch away from a newly selected session", async () => {
    mocks.api.getSession.mockImplementation((id: string) => Promise.resolve(view(id)));
    const pending = deferred<{ sessionId: string }>();
    mocks.api.createSession.mockReturnValue(pending.promise);
    mocks.api.listSessions.mockResolvedValue({ sessions: [] });
    const creating = useStory.getState().createSession({ characterJson: "{}", playerName: "林舟" });
    await useStory.getState().openSession("B");
    pending.resolve({ sessionId: "A" }); await creating;
    expect(useStory.getState().view?.sessionId).toBe("B");
  });

  it("memory read failures preserve last known records and recover on retry", async () => {
    mocks.api.getSession.mockResolvedValue(view("A"));
    mocks.api.listMemories.mockResolvedValue({ memories: [{ memoryId: "kept", content: "原有记录" }] });
    await useStory.getState().openSession("A");
    mocks.api.listMemories.mockRejectedValueOnce(new Error("temporary read failure"));
    await useStory.getState().loadMemories();
    expect(useStory.getState().memories[0]?.memoryId).toBe("kept");
    expect(useStory.getState().memoryError).toBe("temporary read failure");
    expect(useStory.getState().memoryLoading).toBe(false);
    await useStory.getState().reviseMemory("kept", { hidden: true });
    expect(mocks.api.patchMemory).not.toHaveBeenCalled();
    await useStory.getState().loadMemories();
    expect(useStory.getState().memoryError).toBeNull();
    expect(useStory.getState().memories[0]?.memoryId).toBe("kept");
  });

  it("a late memory error cannot annotate a different session", async () => {
    mocks.api.getSession.mockImplementation((id: string) => Promise.resolve(view(id)));
    await useStory.getState().openSession("A");
    const pending = deferred<{ memories: unknown[] }>();
    mocks.api.listMemories.mockReturnValueOnce(pending.promise);
    const loading = useStory.getState().loadMemories();
    await useStory.getState().openSession("B");
    pending.reject(new Error("A storage failed"));
    await loading;
    expect(useStory.getState().memoryError).toBeNull();
    expect(useStory.getState().memoryLoading).toBe(false);
  });

  it.each(["success", "failure"])("an old memory search %s cannot overwrite a newer query", async outcome => {
    mocks.api.getSession.mockResolvedValue(view("A"));
    await useStory.getState().openSession("A");
    const old = deferred<{ memories: unknown[] }>();
    mocks.api.listMemories.mockReturnValueOnce(old.promise);
    const searching = useStory.getState().loadMemories({ search: "旧查询" });
    mocks.api.listMemories.mockResolvedValueOnce({ memories: [{ memoryId: "new" }], nodeId: "new-node", total: 1 });
    await useStory.getState().loadMemories({ search: "新查询", kind: "observed" });
    if (outcome === "success") old.resolve({ memories: [{ memoryId: "old" }] });
    else old.reject(new Error("obsolete query failed"));
    await searching;
    expect(useStory.getState().memories[0]?.memoryId).toBe("new");
    expect(useStory.getState().memoryPaging).toMatchObject({ search: "新查询", kind: "observed", total: 1 });
    expect(useStory.getState().memoryError).toBeNull();
    expect(useStory.getState().memoryLoading).toBe(false);
  });

  it("background head updates keep later memory pages bound to their original snapshot", async () => {
    mocks.api.getSession.mockResolvedValue(view("A"));
    mocks.api.listMemories.mockResolvedValueOnce({ memories: [{ memoryId: "first" }], nodeId: "snapshot", nextCursor: "second-page", total: 81 });
    await useStory.getState().openSession("A");
    mocks.api.listMemories.mockResolvedValue({ memories: [{ memoryId: "second" }], nodeId: "snapshot", nextCursor: "third-page", total: 81 });
    await useStory.getState().loadMemories({ page: "next" });
    expect(mocks.api.listMemories).toHaveBeenLastCalledWith("A", "branch_A", "snapshot", expect.objectContaining({ cursor: "second-page", limit: 40 }));
    const changed = view("A"); changed.branch.headNodeId = "background-head";
    mocks.api.getSession.mockResolvedValue(changed);
    await useStory.getState().reloadView();
    expect(mocks.api.listMemories).toHaveBeenLastCalledWith("A", "branch_A", "snapshot", expect.objectContaining({ cursor: "second-page" }));
    expect(useStory.getState().memoryPaging.index).toBe(1);
    await useStory.getState().loadMemories({ page: "previous" });
    expect(mocks.api.listMemories).toHaveBeenLastCalledWith("A", "branch_A", "snapshot", expect.objectContaining({ cursor: "" }));
    expect(useStory.getState().memoryPaging.index).toBe(0);
  });

  it("a successful revision refreshes the first page at the new head with the same filters", async () => {
    mocks.api.getSession.mockResolvedValue(view("A"));
    await useStory.getState().openSession("A");
    useStory.setState({ memoryPaging: { search: "钥匙", kind: "observed", index: 1, cursors: ["", "old-page"], nodeId: "old-node", total: 80, counts: {} } });
    mocks.api.patchMemory.mockResolvedValue({});
    mocks.api.listMemories.mockResolvedValue({ memories: [], nodeId: "new-node", total: 0 });
    await useStory.getState().reviseMemory("memory", { hidden: true });
    expect(mocks.api.listMemories).toHaveBeenLastCalledWith("A", "branch_A", undefined, { search: "钥匙", kind: "observed", cursor: "", limit: 40 });
    expect(useStory.getState().memoryPaging).toMatchObject({ index: 0, nodeId: "new-node", search: "钥匙", kind: "observed" });
  });

  it("a failed memory load after switching branches never shows the old branch records", async () => {
    mocks.api.getSession.mockResolvedValue(view("A"));
    mocks.api.listMemories.mockResolvedValue({ memories: [{ memoryId: "old" }] });
    await useStory.getState().openSession("A");
    const next = view("A"); next.branch.branchId = "fork";
    mocks.api.getSession.mockResolvedValue(next);
    mocks.api.listMemories.mockRejectedValue(new Error("fork unavailable"));
    await useStory.getState().switchBranch("fork");
    expect(useStory.getState().memories).toEqual([]);
    expect(useStory.getState().memoryError).toBe("fork unavailable");
  });

  it.each(["fork", "regenerate", "editPlayer", "editAssistant"] as const)("%s does not retain memories from its previous path if the new read fails", async operation => {
    mocks.api.getSession.mockResolvedValue(view("A"));
    mocks.api.listMemories.mockResolvedValue({ memories: [{ memoryId: "future-memory", content: "旧分支后续事件" }] });
    await useStory.getState().openSession("A");
    const next = view("A"); next.branch.branchId = "derived";
    const result = { branch: next.branch, branchId: "derived", turnId: "derived-turn" };
    mocks.api.forkBranch.mockResolvedValue(result);
    mocks.api.regenerateTurn.mockResolvedValue(result);
    mocks.api.editTurn.mockResolvedValue(result);
    mocks.api.getSession.mockResolvedValue(next);
    mocks.api.listMemories.mockRejectedValue(new Error("new path unavailable"));
    const actions = useStory.getState();
    if (operation === "fork") await actions.forkFrom("root_A");
    else if (operation === "regenerate") await actions.regenerate("turn_A");
    else if (operation === "editPlayer") await actions.editPlayerInput("turn_A", "改变选择");
    else await actions.editAssistantText("turn_A", [{ kind: "narration", text: "修改后的正文" }]);
    expect(useStory.getState().view?.branch.branchId).toBe("derived");
    expect(useStory.getState().memories).toEqual([]);
    expect(useStory.getState().memoryError).toBe("new path unavailable");
  });

  it.each(["regenerate", "editPlayer"] as const)("%s still subscribes to an accepted turn when the derived view initially fails to load", async operation => {
    mocks.api.getSession.mockResolvedValue(view("A"));
    await useStory.getState().openSession("A");
    const result = { branchId: "derived", turnId: "derived-turn" };
    mocks.api.regenerateTurn.mockResolvedValue(result);
    mocks.api.editTurn.mockResolvedValue(result);
    mocks.api.getSession.mockRejectedValue(new Error("view temporarily unavailable"));
    if (operation === "regenerate") await useStory.getState().regenerate("turn_A");
    else await useStory.getState().editPlayerInput("turn_A", "改变选择");
    expect(mocks.turn).toHaveBeenCalledOnce();
    expect(mocks.turn.mock.calls[0][0]).toBe("derived-turn");
    expect(useStory.getState().error).toBe("view temporarily unavailable");
    expect(useStory.getState().phase).toBe("generating");
  });

  it("a delayed archive failure cannot annotate another session", async () => {
    mocks.api.getSession.mockImplementation((id: string) => Promise.resolve(view(id)));
    await useStory.getState().openSession("A");
    const pending = deferred<{ sessionId: string }>();
    mocks.api.importSessionPack.mockReturnValue(pending.promise);
    const importing = useStory.getState().importSession(new File(["archive"], "test.tavernpack"));
    const rejection = expect(importing).rejects.toThrow("bad pack");
    await vi.waitFor(() => expect(mocks.api.importSessionPack).toHaveBeenCalledOnce());
    await useStory.getState().openSession("B");
    pending.reject(new Error("bad pack"));
    await rejection;
    expect(useStory.getState().view?.sessionId).toBe("B");
    expect(useStory.getState().packError).toBeNull();
  });

  it("an empty legacy session response cannot crash character navigation", async () => {
    mocks.api.listSessions.mockResolvedValue({ sessions: null });
    await useStory.getState().loadSessions();
    expect(useStory.getState().sessions).toEqual([]);
    await useStory.getState().selectCharacter(DEFAULT_PRESET_KEY);
    expect(useStory.getState().view).toBeNull();
  });
  it("reopens an active branch by replaying its durable draft", async () => {
    const active = view("A"); active.branch.activeTurnId = "active-A";
    mocks.api.getSession.mockResolvedValue(active);
    await useStory.getState().openSession("A");
    expect(useStory.getState().phase).toBe("generating");
    expect(mocks.turn.mock.calls[0][0]).toBe("active-A");
    expect(mocks.turn.mock.calls[0][1].lastEventId).toBeUndefined();
    mocks.turn.mock.calls[0][1].onEvent({ id: "active-A:3", event: "block.appended", data: { frameSeq: 1, frame: { kind: "narration", text: "未完成的雨夜" } } });
    await useStory.getState().refreshView();
    expect(useStory.getState().draftBlocks[0].text).toBe("未完成的雨夜");
    expect(useStory.getState().phase).toBe("generating");
  });

  it("continuation keeps the durable cursor and existing blocks", async () => {
    mocks.api.getSession.mockResolvedValue(view("A")); await useStory.getState().openSession("A");
    mocks.api.acceptTurn.mockResolvedValue({ turnId: "cursor-A" });
    await useStory.getState().send("继续");
    mocks.turn.mock.calls[0][1].onEvent({ id: "cursor-A:5", event: "block.appended", data: { frameSeq: 1, frame: { kind: "narration", text: "前半段" } } });
    useStory.setState({ phase: "truncated" });
    mocks.api.continueTurn.mockResolvedValue({ status: "preparing" });
    await useStory.getState().continueTurn();
    expect(mocks.api.continueTurn).toHaveBeenCalledWith("cursor-A", "custom_A");
    expect(mocks.turn.mock.calls[1][1].lastEventId).toBe("cursor-A:5");
    expect(useStory.getState().draftBlocks[0].text).toBe("前半段");
  });

  it("an earlier truncation replay cannot stop a newer attempt", async () => {
    vi.useFakeTimers();
    mocks.api.getSession.mockResolvedValue(view("A")); await useStory.getState().openSession("A");
    mocks.api.acceptTurn.mockResolvedValue({ turnId: "active" });
    await useStory.getState().send("继续");
    useSettings.setState({ autoContinue: true });
    mocks.api.getTurn.mockResolvedValue({ status: "preparing" });
    mocks.turn.mock.calls[0][1].onEvent({ id: "active:2", event: "turn.truncated", data: {} });
    await vi.advanceTimersByTimeAsync(0);
    expect(mocks.api.getTurn).toHaveBeenCalledExactlyOnceWith("active");
    expect(useStory.getState().phase).toBe("generating");
    expect(mocks.api.continueTurn).not.toHaveBeenCalled();
  });

  it("history is read-only for free text and choices", async () => {
    mocks.api.getSession.mockResolvedValue(view("A")); await useStory.getState().openSession("A");
    useStory.setState({ viewNodeId: "root_A" });
    await useStory.getState().send("改写历史");
    await useStory.getState().chooseOption({ optionId: "o1", intent: "clever", text: "继续" });
    expect(mocks.api.acceptTurn).not.toHaveBeenCalled();
  });

  it.each(["generating", "truncated", "awaiting_confirm", "committed"] as const)("blocks branch mutations while %s", async phase => {
    mocks.api.getSession.mockResolvedValue(view("A")); await useStory.getState().openSession("A");
    useStory.setState({ phase });
    await useStory.getState().forkFrom("root_A");
    await useStory.getState().editPlayerInput("turn_A", "更改输入");
    await useStory.getState().switchBranch("another");
    expect(mocks.api.forkBranch).not.toHaveBeenCalled();
    expect(mocks.api.editTurn).not.toHaveBeenCalled();
    expect(useStory.getState().activeBranchId).toBe("branch_A");
  });

  it("a late cancel cannot clear another session's active draft", async () => {
    mocks.api.getSession.mockImplementation((id: string) => Promise.resolve(view(id)));
    await useStory.getState().openSession("A"); useStory.setState({ lastTurnId: "old" });
    const pending = deferred<{ status: string }>(); mocks.api.cancelTurn.mockReturnValue(pending.promise);
    const cancelling = useStory.getState().cancel();
    await useStory.getState().openSession("B");
    useStory.setState({ lastTurnId: "new", phase: "generating", draftBlocks: [{ kind: "narration", text: "B 的剧情" }] });
    pending.resolve({ status: "cancelled" }); await cancelling;
    expect(useStory.getState().phase).toBe("generating");
    expect(useStory.getState().draftBlocks[0].text).toBe("B 的剧情");
  });

  it("explicit rule actions preserve their identity through retry", async () => {
    mocks.api.getSession.mockResolvedValue(view("A")); await useStory.getState().openSession("A");
    mocks.api.acceptTurn.mockResolvedValue({ turnId: "action-A" });
    await useStory.getState().send("尝试开门", "gate.open");
    expect(mocks.api.acceptTurn.mock.calls[0][2].input).toEqual({ kind: "action", text: "尝试开门", actionRef: "gate.open" });
    useStory.setState({ phase: "truncated" });
    mocks.api.cancelTurn.mockResolvedValue({ status: "cancelled" });
    await useStory.getState().retry();
    expect(mocks.api.acceptTurn.mock.calls[1][2].input).toEqual({ kind: "action", text: "尝试开门", actionRef: "gate.open" });
  });

  it("retry cancellation cannot submit or clear a different session", async () => {
    mocks.api.getSession.mockImplementation((id: string) => Promise.resolve(view(id)));
    await useStory.getState().openSession("A");
    useStory.setState({ phase: "failed", lastTurnId: "old-A", lastInput: { kind: "text", text: "A 的请求" } });
    const pending = deferred<{ status: string }>();
    mocks.api.cancelTurn.mockReturnValue(pending.promise);
    const retrying = useStory.getState().retry();
    await useStory.getState().openSession("B");
    useStory.setState({ phase: "generating", lastTurnId: "live-B", draftBlocks: [{ kind: "narration", text: "B 的草稿" }] });
    pending.resolve({ status: "cancelled" });
    await retrying;
    expect(mocks.api.acceptTurn).not.toHaveBeenCalled();
    expect(useStory.getState().lastTurnId).toBe("live-B");
    expect(useStory.getState().draftBlocks[0].text).toBe("B 的草稿");
  });

  it("an unknown acceptance outcome reuses its complete original request", async () => {
    mocks.api.getSession.mockResolvedValue(view("A"));
    await useStory.getState().openSession("A");
    mocks.api.acceptTurn.mockRejectedValueOnce(new Error("response lost"));
    await useStory.getState().send("开门", "gate.open");
    const original = mocks.api.acceptTurn.mock.calls[0];
    mocks.api.acceptTurn.mockResolvedValue({ turnId: "same-request" });
    await useStory.getState().retry();
    expect(mocks.api.acceptTurn.mock.calls[1]).toEqual(original);
    expect(mocks.api.cancelTurn).not.toHaveBeenCalled();
  });

  it("retry reconciles a committed result instead of generating a duplicate", async () => {
    mocks.api.getSession.mockResolvedValue(view("A"));
    await useStory.getState().openSession("A");
    useStory.setState({ phase: "failed", lastTurnId: "old-A", lastInput: { kind: "text", text: "继续" } });
    mocks.api.cancelTurn.mockRejectedValue(new Error("lost cancellation response"));
    mocks.api.getTurn.mockResolvedValue({ status: "committed", turnId: "old-A" });
    await useStory.getState().retry();
    expect(mocks.api.acceptTurn).not.toHaveBeenCalled();
  });

  it("overlapping opens keep the latest session and its pagination", async () => {
    const a = deferred<SessionView>();
    mocks.api.getSession.mockImplementation((id: string) => id === "A" ? a.promise : Promise.resolve(view("B")));
    const old = useStory.getState().openSession("A");
    await useStory.getState().openSession("B");
    a.resolve(view("A")); await old;
    expect(useStory.getState().view?.sessionId).toBe("B");
    expect(useStory.getState().activeCharacterId).toBe("custom_B");
    expect(useStory.getState().hasMoreHistory).toBe(true);
    expect(mocks.session).toHaveBeenCalledTimes(1);
  });

  it("switching to a character without sessions invalidates an unfinished open", async () => {
    const pending = deferred<SessionView>(); mocks.api.getSession.mockReturnValue(pending.promise);
    const opening = useStory.getState().openSession("A");
    await useStory.getState().selectCharacter(DEFAULT_PRESET_KEY);
    pending.resolve(view("A")); await opening;
    expect(useStory.getState().view).toBeNull();
    expect(useUi.getState().activeCharKey).toBe(DEFAULT_PRESET_KEY);
  });

  it("late memory results cannot replace another custom character's records", async () => {
    const pending = deferred<{ memories: unknown[] }>();
    mocks.api.getSession.mockImplementation((id: string) => Promise.resolve(view(id)));
    mocks.api.listMemories.mockImplementation((id: string) => id === "A" ? pending.promise : Promise.resolve({ memories: [{ memoryId: "B" }] }));
    const opening = useStory.getState().openSession("A");
    await vi.waitFor(() => expect(useStory.getState().view?.sessionId).toBe("A"));
    await useStory.getState().openSession("B");
    pending.resolve({ memories: [{ memoryId: "A" }] }); await opening;
    expect(useStory.getState().memories[0]?.memoryId).toBe("B");
  });

  it("a late accept response stays with its original session and sends identity", async () => {
    mocks.api.getSession.mockResolvedValue(view("A")); await useStory.getState().openSession("A");
    const pending = deferred<unknown>(); mocks.api.acceptTurn.mockReturnValue(pending.promise);
    const sending = useStory.getState().send("你好");
    await useStory.getState().selectCharacter(DEFAULT_PRESET_KEY);
    pending.resolve({ turnId: "old", status: "queued" }); await sending;
    expect(mocks.api.acceptTurn.mock.calls[0][2].expectedCharacterId).toBe("custom_A");
    expect(useStory.getState().lastTurnId).toBeNull();
    expect(mocks.turn).not.toHaveBeenCalled();
  });

  it("same-session events are also scoped to their turn", async () => {
    mocks.api.getSession.mockResolvedValue(view("A")); await useStory.getState().openSession("A");
    useStory.setState({ lastTurnId: "new", phase: "generating" });
    const stop = vi.fn();
    routeEvent({ event: "turn.failed", data: { message: "old" } }, "A", "old", stop);
    expect(stop).toHaveBeenCalledOnce();
    expect(useStory.getState().phase).toBe("generating");
    expect(useStory.getState().error).toBeNull();
  });

  it("maintenance heads retain the latest dialogue option reference", async () => {
    mocks.api.getSession.mockResolvedValue(view("A")); await useStory.getState().openSession("A");
    mocks.api.acceptTurn.mockResolvedValue({ turnId: "t1" });
    await useStory.getState().chooseOption({ optionId: "o1", intent: "clever", text: "继续" });
    const req = mocks.api.acceptTurn.mock.calls[0][2];
    expect(req.expectedHeadId).toBe("head_A");
    expect(req.input.optionRef.nodeId).toBe("turn_A");
    expect(req.expectedCharacterId).toBe("custom_A");
  });

  it("memory edits send a CAS version and reload the new head", async () => {
    mocks.api.getSession.mockResolvedValue(view("A")); await useStory.getState().openSession("A");
    mocks.api.patchMemory.mockResolvedValue({});
    const fresh = view("A"); fresh.branch = { ...fresh.branch, headNodeId: "newhead", version: 3 };
    mocks.api.getSession.mockResolvedValue(fresh);
    await useStory.getState().reviseMemory("m1", { pinned: true });
    expect(mocks.api.patchMemory.mock.calls[0][3]).toMatchObject({ expectedCharacterId: "custom_A", expectedHeadId: "head_A", expectedVersion: 2 });
    expect(useStory.getState().view?.branch.headNodeId).toBe("newhead");
  });

  it("terminal fallback restores continuation instead of leaving generation locked", async () => {
    mocks.api.getSession.mockResolvedValue(view("A")); await useStory.getState().openSession("A");
    mocks.api.acceptTurn.mockResolvedValue({ turnId: "t1" });
    mocks.api.getTurn.mockResolvedValue({ turnId: "t1", status: "awaiting_continuation" });
    await useStory.getState().send("你好");
    mocks.turn.mock.calls[0][1].onFinalError(new Error("offline"));
    await vi.waitFor(() => expect(useStory.getState().phase).toBe("truncated"));
  });

  it("cached sessions display history immediately before the network roundtrip completes (SWR)", async () => {
    mocks.api.getSession.mockImplementation((id: string) => Promise.resolve(view(id)));
    // First open populates the cache
    await useStory.getState().openSession("A");
    expect(useStory.getState().view?.sessionId).toBe("A");
    expect(useStory.getState().messages.length).toBeGreaterThan(0);

    await useStory.getState().openSession("B");
    expect(useStory.getState().view?.sessionId).toBe("B");

    // Switching back to A: defer network response
    const networkPending = deferred<SessionView>();
    mocks.api.getSession.mockReturnValueOnce(networkPending.promise);

    // Call openSession("A") synchronously without awaiting
    const switching = useStory.getState().openSession("A");

    // Because session "A" was cached, the view and messages are immediately present (0ms delay)
    expect(useStory.getState().view?.sessionId).toBe("A");
    expect(useStory.getState().messages.length).toBeGreaterThan(0);
    expect(useStory.getState().sessionLoading).toBe(false);

    // Resolve network validation
    const updatedView = view("A");
    updatedView.title = "已验证会话";
    networkPending.resolve(updatedView);
    await switching;

    expect(useStory.getState().view?.title).toBe("已验证会话");
  });
});
