// 批次 1 回归：失败必须可见（UX-02/03/13）。
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { SessionView } from "../../app/types";

const mocks = vi.hoisted(() => ({
  api: {
    listSessions: vi.fn(), getSession: vi.fn(), listMemories: vi.fn(),
    createSession: vi.fn(), acceptTurn: vi.fn(), patchMemory: vi.fn(),
    getTurn: vi.fn(), continueTurn: vi.fn(), cancelTurn: vi.fn(),
    forkBranch: vi.fn(), regenerateTurn: vi.fn(), editTurn: vi.fn(),
    importSessionPack: vi.fn(), listModels: vi.fn(),
  },
  turn: vi.fn(), session: vi.fn(),
}));
vi.mock("../../app/api", () => ({
  api: mocks.api,
  subscribeTurnEvents: mocks.turn,
  subscribeSessionEvents: mocks.session,
}));

import { useStory } from "../storyStore";
import { useUi } from "../uiStore";
import { useSettings } from "../settingsStore";

function view(id: string): SessionView {
  return {
    sessionId: id, characterId: `custom_${id}`, title: "测试",
    rootNodeId: `root_${id}`,
    branch: { branchId: `branch_${id}`, name: "main", headNodeId: `head_${id}`, version: 1 },
    headNode: { nodeId: `head_${id}`, sessionId: id, parentId: `root_${id}`, kind: "turn", depth: 1, turnNumber: 1, contentJson: "{}", createdAt: "" },
    nodes: [],
    state: { characters: {}, items: {}, relationships: {}, promises: {}, moods: {} },
    hasMore: false,
  };
}

beforeEach(() => {
  vi.resetAllMocks();
  useStory.setState(useStory.getInitialState(), true);
  mocks.turn.mockReturnValue(vi.fn());
  mocks.session.mockReturnValue(vi.fn());
});

describe("failure visibility", () => {
  it("surfaces a model-config load failure and clears it on a successful retry", async () => {
    mocks.api.listModels.mockRejectedValueOnce(new Error("网关 502"));
    await useSettings.getState().load();
    expect(useSettings.getState().loaded).toBe(true);
    expect(useSettings.getState().error).toBe("网关 502");

    mocks.api.listModels.mockResolvedValueOnce({ models: [], slots: [] });
    await useSettings.getState().load();
    expect(useSettings.getState().error).toBeNull();
  });

  it("marks a send failure as failed so the composer can restore the typed text", async () => {
    useUi.getState().setActiveCharKey("custom");
    await useStory.getState().send("第一句话");
    expect(useStory.getState().phase).toBe("failed");
    expect(useStory.getState().error).toBeTruthy();
  });

  it("reviseMemory reports false (not success) when the guard blocks it", async () => {
    useStory.setState({ view: view("A"), phase: "generating" });
    const applied = await useStory.getState().reviseMemory("m1", { pinned: true });
    expect(applied).toBe(false);
    expect(mocks.api.patchMemory).not.toHaveBeenCalled();
  });
});
