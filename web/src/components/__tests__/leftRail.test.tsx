// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { SessionView, WorldState } from "../../app/types";

const mocks = vi.hoisted(() => ({
  listCards: vi.fn().mockResolvedValue({ cards: [] }),
  listSessions: vi.fn().mockResolvedValue({ sessions: [] }),
  searchLorebook: vi.fn().mockResolvedValue({ entries: [], hasMore: false, nextOffset: 0 }),
}));
vi.mock("../../app/api", () => ({
  api: mocks,
  subscribeTurnEvents: vi.fn(),
  subscribeSessionEvents: vi.fn(),
}));

import { LeftRail } from "../LeftRail";
import { useStory } from "../../stores/storyStore";
import { useUi } from "../../stores/uiStore";
import { DEFAULT_PRESET_KEY } from "../../lib/characterPresets";

function makeView(): SessionView {
  const state: WorldState = {
    characters: {
      npc_yamei: { characterId: "npc_yamei", name: "亚美", participant: true },
    },
    items: {},
    relationships: { npc_yamei: { affection: 10, trust: 20, alertness: 5 } },
    moods: {},
    promises: {},
  };
  return {
    sessionId: "test-session",
    characterId: "npc_yamei",
    title: "亚美的冒险",
    rootNodeId: "root",
    branch: { branchId: "main", name: "main", headNodeId: "head", version: 1 },
    headNode: {
      nodeId: "head",
      sessionId: "test-session",
      parentId: "root",
      kind: "turn",
      depth: 1,
      turnNumber: 1,
      contentJson: JSON.stringify({ inputText: "出发去森林" }),
      createdAt: "",
    },
    nodes: [
      {
        nodeId: "root",
        sessionId: "test-session",
        parentId: "",
        kind: "root",
        depth: 0,
        turnNumber: 0,
        contentJson: JSON.stringify({ openingText: "命运起点 · 酒馆初见" }),
        createdAt: "",
      },
      {
        nodeId: "head",
        sessionId: "test-session",
        parentId: "root",
        kind: "turn",
        depth: 1,
        turnNumber: 1,
        contentJson: JSON.stringify({ inputText: "出发去森林" }),
        createdAt: "",
      },
    ],
    state,
    actions: [],
  };
}

beforeEach(() => {
  vi.resetAllMocks();
  mocks.listCards.mockResolvedValue({ cards: [] });
  mocks.listSessions.mockResolvedValue({ sessions: [] });
  mocks.searchLorebook.mockResolvedValue({ entries: [], hasMore: false, nextOffset: 0 });
  const view = makeView();
  useStory.setState(
    {
      ...useStory.getInitialState(),
      view,
      hud: view.state,
      activeBranchId: "main",
      sessions: [
        {
          sessionId: "test-session",
          characterId: "npc_yamei",
          title: "亚美的冒险",
          createdAt: new Date().toISOString(),
          updatedAt: new Date().toISOString(),
        },
      ],
    },
    true,
  );
  useUi.setState(
    {
      ...useUi.getInitialState(),
      activeCharKey: DEFAULT_PRESET_KEY,
      isLeftRailCollapsed: false,
      leftTab: "tree",
      notifyQuiet: vi.fn(),
    },
    true,
  );
});
afterEach(cleanup);

describe("LeftRail left information rail", () => {
  it("renders tree panel when leftTab is 'tree'", () => {
    const { container } = render(<LeftRail />);
    const treePanel = container.querySelector("#view-tree") as HTMLElement;
    const charPanel = container.querySelector("#view-characters") as HTMLElement;
    const lorePanel = container.querySelector("#view-lore") as HTMLElement;
    const sessPanel = container.querySelector("#view-sessions") as HTMLElement;

    expect(treePanel).toBeTruthy();
    expect(treePanel.style.display).toBe("flex");
    expect(charPanel.style.display).toBe("none");
    expect(lorePanel.style.display).toBe("none");
    expect(sessPanel.style.display).toBe("none");

    expect(screen.getByText("分支拓扑 (Branch Graph)")).toBeTruthy();
  });

  it("switches to characters panel when clicking characters tab and displays content", async () => {
    const user = userEvent.setup();
    const { container } = render(<LeftRail />);

    const charTabBtn = container.querySelector("#subtab-btn-characters") as HTMLElement;
    expect(charTabBtn).toBeTruthy();
    await user.click(charTabBtn);

    const charPanel = container.querySelector("#view-characters") as HTMLElement;
    const treePanel = container.querySelector("#view-tree") as HTMLElement;

    expect(charPanel.style.display).toBe("flex");
    expect(treePanel.style.display).toBe("none");
    expect(screen.getByText("+ 导入 / 创建角色卡")).toBeTruthy();
    expect(screen.getByText(/已收录角色卡/)).toBeTruthy();
  });

  it("switches to lore panel and sessions panel correctly", async () => {
    const user = userEvent.setup();
    const { container } = render(<LeftRail />);

    const loreTabBtn = container.querySelector("#subtab-btn-lore") as HTMLElement;
    await user.click(loreTabBtn);
    const lorePanel = container.querySelector("#view-lore") as HTMLElement;
    expect(lorePanel.style.display).toBe("flex");

    const sessTabBtn = container.querySelector("#subtab-btn-sessions") as HTMLElement;
    await user.click(sessTabBtn);
    const sessPanel = container.querySelector("#view-sessions") as HTMLElement;
    expect(sessPanel.style.display).toBe("flex");
    expect(screen.getByText("历史故事会话")).toBeTruthy();
  });

  it("短输入的节点摘要不追加省略号", () => {
    render(<LeftRail />);
    expect(screen.getByText("玩家: 「出发去森林」")).toBeTruthy();
  });

  it("行内删除按钮上的 Enter 交给按钮本身，不触发打开故事", async () => {
    const openSession = vi.fn().mockResolvedValue(undefined);
    useStory.setState({ openSession });
    useUi.setState({ leftTab: "sessions" });
    const user = userEvent.setup();
    render(<LeftRail />);

    const trash = screen.getByRole("button", { name: "删除故事 亚美的冒险" });
    trash.focus();
    await user.keyboard("{Enter}");

    expect(openSession).not.toHaveBeenCalled();
    expect(screen.getByText("确认删除")).toBeTruthy();

    // 行本身获得焦点时，Enter 仍然打开故事
    screen.getByRole("button", { name: "打开故事 亚美的冒险" }).focus();
    await user.keyboard("{Enter}");
    expect(openSession).toHaveBeenCalledWith("test-session");
  });
});
