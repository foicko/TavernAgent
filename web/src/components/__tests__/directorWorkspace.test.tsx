// @vitest-environment jsdom
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { DirectorDraft, DirectorPlan, DirectorView, SessionView } from "../../app/types";

const mocks = vi.hoisted(() => ({ getDirector: vi.fn(), saveDirectorDraft: vi.fn(), directorCommand: vi.fn(), directorMessage: vi.fn(), cancelDirectorRequest: vi.fn(), getDirectorRequest: vi.fn() }));
vi.mock("../../app/api", () => ({ api: mocks, subscribeTurnEvents: vi.fn(), subscribeSessionEvents: vi.fn(), subscribeDirectorEvents: vi.fn(() => vi.fn()) }));
import { ComposerShelf } from "../ComposerShelf";
import { useStory } from "../../stores/storyStore";
import { useUi } from "../../stores/uiStore";
import { beginNavigation } from "../../stores/storyHelpers";
import { DEFAULT_PRESET_KEY } from "../../lib/characterPresets";

const initialPlan: DirectorPlan = { title: "雨夜重逢", guidance: "自然衔接，保留玩家选择", beats: [
  { beatId: "meeting", title: "交换近况", instruction: "让两人互相问候", completionCriteria: "双方说出自己的近况" },
  { beatId: "letter", title: "旧信", instruction: "展示一封旧信", completionCriteria: "旧信已经被读出" },
] };
const story: SessionView = { sessionId: "director-ui", characterId: "card_elena", title: "测试故事", rootNodeId: "root",
  branch: { branchId: "main", name: "main", headNodeId: "head", version: 1 }, state: null,
  headNode: { nodeId: "head", sessionId: "director-ui", parentId: "root", kind: "turn", depth: 1, turnNumber: 1, contentJson: "{}", createdAt: "" } };
let data: DirectorView;

beforeEach(() => {
  vi.clearAllMocks(); localStorage.clear();
  data = { state: null, draft: { sessionId: story.sessionId, branchId: "main", version: 1, baseRevisionId: "", plan: structuredClone(initialPlan), updatedAt: "" }, requests: [], readOnly: false, viewNodeId: "head" };
  mocks.getDirector.mockImplementation(async () => structuredClone(data));
  mocks.saveDirectorDraft.mockImplementation(async (_session, _branch, body) => {
    const draft: DirectorDraft = { sessionId: story.sessionId, branchId: "main", version: body.expectedDraftVersion + 1, baseRevisionId: body.baseRevisionId, plan: structuredClone(body.plan), updatedAt: "" };
    data.draft = draft; return structuredClone(draft);
  });
  useStory.setState({ ...useStory.getInitialState(), view: structuredClone(story), activeBranchId: "main", activeCharacterId: `card_${DEFAULT_PRESET_KEY}` }, true);
  useUi.setState({ ...useUi.getInitialState(), activeCharKey: DEFAULT_PRESET_KEY }, true);
});
afterEach(cleanup);

async function openWorkspace() {
  const user = userEvent.setup(); render(<ComposerShelf />);
  await user.click(screen.getByRole("button", { name: /导演模式/ }));
  await screen.findByRole("dialog", { name: "导演模式" });
  await waitFor(() => expect((screen.getByLabelText("大纲标题") as HTMLInputElement).value).toBe(data.draft?.plan.title || data.state?.plan.title));
  return user;
}

describe("director workspace", () => {
  it("opens before the first turn by creating only the opening record", async () => {
    useUi.setState({ activeCharKey: "test_card" });
    const create = vi.fn().mockImplementation(async () => {
      beginNavigation();
      useStory.setState({ view: structuredClone(story), activeBranchId: "main", activeCharacterId: "card_test_card" });
      return { sessionId: story.sessionId };
    });
    const send = vi.fn();
    useStory.setState({ view: null, activeBranchId: null, createSessionFromPreset: create, send });
    await openWorkspace();
    expect(create).toHaveBeenCalledWith("test_card");
    expect(send).not.toHaveBeenCalled();
    expect(mocks.directorMessage).not.toHaveBeenCalled();
    expect(mocks.directorCommand).not.toHaveBeenCalled();
  });

  it("opens without advancing the story and autosaves edits until explicit activation", async () => {
    const command = vi.fn().mockResolvedValue(undefined); useStory.setState({ commandDirector: command });
    const user = await openWorkspace();
    expect(command).not.toHaveBeenCalled(); expect(mocks.directorMessage).not.toHaveBeenCalled();
    await user.clear(screen.getByLabelText("大纲标题")); await user.type(screen.getByLabelText("大纲标题"), "新的相遇大纲");
    await waitFor(() => expect(mocks.saveDirectorDraft.mock.lastCall?.[2].plan.title).toBe("新的相遇大纲"));
    expect(command).not.toHaveBeenCalled(); expect(useStory.getState().view?.branch.headNodeId).toBe("head");
    await waitFor(() => expect((screen.getByRole("button", { name: "确认启用大纲" }) as HTMLButtonElement).disabled).toBe(false));
    await user.click(screen.getByRole("button", { name: "确认启用大纲" }));
    expect(command).toHaveBeenCalledWith("activate", undefined, false);
    expect(await screen.findByText("大纲已启用，从下一回合开始生效。")).toBeTruthy();
  });

  it("supports adding, reordering and removing stages", async () => {
    const user = await openWorkspace();
    await user.click(screen.getByRole("button", { name: "下移阶段 1" }));
    expect((screen.getByLabelText("阶段 1 名称") as HTMLInputElement).value).toBe("旧信");
    await user.click(screen.getByRole("button", { name: /添加阶段/ }));
    expect(screen.getByLabelText("阶段 3 名称")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "删除阶段 3" }));
    expect(screen.queryByLabelText("阶段 3 名称")).toBeNull();
    await user.click(screen.getByRole("button", { name: /返回剧情/ }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(data.draft?.plan.beats[0].beatId).toBe("letter");
  });

  it("keeps an unapplied AI suggestion separate until it is loaded", async () => {
    data.requests = [{ requestId: "reply", sessionId: story.sessionId, branchId: "main", baseNodeId: "head", draftVersion: 0, text: "增加转折", status: "completed", reply: "建议从分别开始。", candidate: { ...initialPlan, title: "AI 建议大纲" }, draftApplied: false, createdAt: "" }];
    const user = await openWorkspace();
    expect((screen.getByLabelText("大纲标题") as HTMLInputElement).value).toBe("雨夜重逢");
    await user.click(screen.getByRole("button", { name: "载入此建议" }));
    expect((screen.getByLabelText("大纲标题") as HTMLInputElement).value).toBe("AI 建议大纲");
    expect(mocks.directorCommand).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: /返回剧情/ }));
  });

  it("makes historical outlines read only", async () => {
    data.readOnly = true; data.draft = null;
    data.state = { plan: initialPlan, status: "active", currentBeatId: "meeting", progress: {} };
    useStory.setState({ viewNodeId: "history" });
    await openWorkspace();
    expect((screen.getByLabelText("大纲标题") as HTMLInputElement).disabled).toBe(true);
    expect(screen.queryByRole("button", { name: "确认启用大纲" })).toBeNull();
    expect(screen.queryByLabelText("与导演讨论")).toBeNull();
    expect(mocks.getDirector).toHaveBeenCalledWith(story.sessionId, "main", "history");
  });

  it("preserves local edits after failed autosave and reopening", async () => {
    mocks.saveDirectorDraft.mockRejectedValue(new Error("暂时无法保存"));
    const user = await openWorkspace();
    await user.clear(screen.getByLabelText("大纲标题")); await user.type(screen.getByLabelText("大纲标题"), "尚未同步的大纲");
    await screen.findByRole("alert");
    await user.click(screen.getByRole("button", { name: /返回剧情/ }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await user.click(screen.getByRole("button", { name: /导演模式/ }));
    await waitFor(() => expect((screen.getByLabelText("大纲标题") as HTMLInputElement).value).toBe("尚未同步的大纲"));
    expect(screen.getByText("已恢复尚未同步的本地编辑。")).toBeTruthy();
  });

  it.each(["activate", "discuss"])("does not %s in a different story after waiting for a save", async action => {
    const command = vi.fn().mockResolvedValue(undefined), discuss = vi.fn().mockResolvedValue(undefined);
    useStory.setState({ commandDirector: command, discussDirector: discuss });
    let resolve!: (draft: DirectorDraft) => void;
    mocks.saveDirectorDraft.mockReturnValueOnce(new Promise<DirectorDraft>(done => { resolve = done; }));
    const user = await openWorkspace();
    if (action === "discuss") await user.type(screen.getByLabelText("与导演讨论"), "安排重逢");
    await user.clear(screen.getByLabelText("大纲标题")); await user.type(screen.getByLabelText("大纲标题"), "新的大纲");
    await user.click(screen.getByRole("button", { name: action === "activate" ? "确认启用大纲" : "发送给导演" }));
    await waitFor(() => expect(mocks.saveDirectorDraft).toHaveBeenCalled());
    act(() => useStory.setState({ view: { ...story, sessionId: "another-story", branch: { ...story.branch, branchId: "another-branch" } }, activeBranchId: "another-branch" }));
    await act(async () => { resolve({ ...data.draft!, version: 2 }); });
    expect(command).not.toHaveBeenCalled(); expect(discuss).not.toHaveBeenCalled();
    expect(localStorage.getItem("tavernagent_director_draft_director-ui_main")).toContain("新的大纲");
  });
});
