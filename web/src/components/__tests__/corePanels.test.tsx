// @vitest-environment jsdom
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { MemoryView, SessionView, WorldState } from "../../app/types";

const mocks = vi.hoisted(() => ({ listMemories: vi.fn() }));
vi.mock("../../app/api", () => ({ api: mocks, subscribeTurnEvents: vi.fn(), subscribeSessionEvents: vi.fn() }));
import { RightRail } from "../RightRail";
import { InventoryActionBubble } from "../InventoryActionBubble";
import { ComposerShelf } from "../ComposerShelf";
import { StoryCheckpointSection } from "../StoryCheckpointSection";
import { useStory } from "../../stores/storyStore";
import { useUi } from "../../stores/uiStore";
import { DEFAULT_PRESET_KEY } from "../../lib/characterPresets";

function makeView(): SessionView {
  const state: WorldState = {
    characters: { npc_elena: { characterId: "npc_elena", name: "灯塔向导", participant: true } },
    items: {
      tea: { instanceId: "tea", name: "旅行茶", ownerId: "player", quantity: 5 },
      watch: { instanceId: "watch", name: "航海怀表", ownerId: "player", quantity: 1, keepsake: true },
      empty: { instanceId: "empty", name: "已喝完的茶", ownerId: "consumed", quantity: 0 },
      npc: { instanceId: "npc", name: "向导的地图", ownerId: "npc_elena", quantity: 1 },
    },
    relationships: { other: { affection: -80, trust: 2, alertness: 90 }, npc_elena: { affection: 50, trust: 40, alertness: 10 } },
    moods: { other: { moodCode: "angry", text: "旁人的情绪" }, npc_elena: { moodCode: "calm", text: "等候旅人出发" } },
    promises: {},
  };
  return { sessionId: "quality", characterId: "quality_story_20260912", title: "港口", rootNodeId: "root",
    branch: { branchId: "main", name: "main", headNodeId: "head", version: 1 },
    headNode: { nodeId: "head", sessionId: "quality", parentId: "root", kind: "turn", depth: 1, turnNumber: 1, contentJson: "{}", createdAt: "" },
    state, actions: [{ actionId: "drink.tea", label: "饮用两份旅行茶", attribute: "wisdom", dc: 12, itemIds: ["tea"] }],
  };
}
const memory: MemoryView = { memoryId: "memory", sourceNodeId: "head", kind: "observed", content: "向导来自北方。", pinned: false, hidden: false, effective: true,
  evidence: { confidence: "high", sourceQuote: "她来自北方" } };

beforeEach(() => {
  vi.resetAllMocks();
  const view = makeView();
  useStory.setState({ ...useStory.getInitialState(), view, hud: view.state, activeBranchId: "main", memories: [{ ...memory }] }, true);
  useUi.setState({ ...useUi.getInitialState(), activeCharKey: DEFAULT_PRESET_KEY, isRightRailCollapsed: false, memoryModalOpen: true, notifyQuiet: vi.fn() }, true);
});
afterEach(cleanup);

describe("inventory, character state and memory panels", () => {
  it("shows only carried items and sends the registered inventory action through the composer", async () => {
    const send = vi.fn().mockResolvedValue(undefined);
    useStory.setState({ send });
    const user = userEvent.setup();
    render(<><RightRail /><InventoryActionBubble /><ComposerShelf /></>);
    expect(screen.getByText("2 件")).toBeTruthy();
    expect(screen.queryByText("已喝完的茶")).toBeNull();
    expect(screen.queryByText("向导的地图")).toBeNull();
    await user.click(screen.getByText("旅行茶", { selector: ".slot-name-label" }));
    await user.click(screen.getByRole("button", { name: "饮用两份旅行茶" }));
    expect((document.getElementById("story-input") as HTMLTextAreaElement).value).toBe("饮用两份旅行茶");
    await user.click(screen.getByRole("button", { name: "推进剧情" }));
    // 第三个参数是本轮注记（非叙事要求）：本次没有写，所以是空串。
    expect(send).toHaveBeenCalledExactlyOnceWith("饮用两份旅行茶", "drink.tea", "");
  });

  it("本轮注记与正文分开送，且发完即清空", async () => {
    const send = vi.fn().mockResolvedValue(undefined);
    useStory.setState({ send });
    const user = userEvent.setup();
    render(<ComposerShelf />);
    await user.type(document.getElementById("story-input") as HTMLTextAreaElement, "我推门进去。");
    await user.click(screen.getByRole("button", { name: /注记/ }));
    const note = screen.getByPlaceholderText(/本轮怎么演/) as HTMLInputElement;
    await user.type(note, "放慢节奏，多写环境。");
    await user.click(screen.getByRole("button", { name: "推进剧情" }));
    // 注记走独立参数：混进正文会被当成角色说过的话。
    expect(send).toHaveBeenCalledExactlyOnceWith("我推门进去。", undefined, "放慢节奏，多写环境。");
    // 注记只作用于这一轮，发完就清（留着它会让人以为还在生效）。
    expect(note.value).toBe("");
  });

  it("keeps the relationship bars and mood on the same character", () => {
    render(<RightRail />);
    expect(document.getElementById("meter-num-aff")?.textContent).toBe("+50");
    expect(document.getElementById("meter-fill-aff")?.style.width).toBe("75%");
    expect(document.getElementById("meter-num-trust")?.textContent).toBe("40%");
    expect(screen.getByText("🎭 神态：冷静自持")).toBeTruthy();
    expect(useUi.getState().tachieExpression).toBe("calm");
  });

  it("preserves last-read memories on failure, disables edits, and recovers through the retry button", async () => {
    mocks.listMemories.mockRejectedValueOnce(new Error("temporary storage failure")).mockResolvedValue({ memories: [{ ...memory, content: "恢复后的记录。" }] });
    const reviseMemory = vi.fn();
    useStory.setState({ reviseMemory });
    const user = userEvent.setup();
    render(<RightRail />);
    await act(async () => { await useStory.getState().loadMemories(); });
    expect(screen.getByRole("alert").textContent).toContain("仍显示上次读取的记录");
    expect(screen.getByText(memory.content)).toBeTruthy();
    const edit = screen.getByRole("button", { name: "纠正" }) as HTMLButtonElement;
    expect(edit.disabled).toBe(true);
    await user.click(edit);
    expect(reviseMemory).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "重试读取记忆" }));
    await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());
    expect(screen.getByText("恢复后的记录。")).toBeTruthy();
    expect((screen.getByRole("button", { name: "纠正" }) as HTMLButtonElement).disabled).toBe(false);
  });

  it("wires correction, pinning, hiding and restoration to the selected memory", async () => {
    const reviseMemory = vi.fn(async (id: string, patch: Partial<MemoryView>) => {
      useStory.setState(state => ({ memories: state.memories.map(m => m.memoryId === id ? { ...m, ...patch } : m) }));
    });
    useStory.setState({ reviseMemory });
    const user = userEvent.setup();
    render(<RightRail />);
    await user.click(screen.getByRole("button", { name: "纠正" }));
    const editor = screen.getByPlaceholderText("输入修订后的记忆事实与潜台词...");
    await user.clear(editor);
    await user.type(editor, "向导来自南方的港口。");
    await user.click(screen.getByRole("button", { name: "保存修订" }));
    expect(reviseMemory).toHaveBeenLastCalledWith("memory", { content: "向导来自南方的港口。" });
    expect(screen.getByText("向导来自南方的港口。")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "📌 置顶" }));
    expect(reviseMemory).toHaveBeenLastCalledWith("memory", { pinned: true });
    await user.click(screen.getByRole("button", { name: "隐藏" }));
    expect(screen.queryByText("向导来自南方的港口。")).toBeNull();
    await user.click(screen.getByRole("button", { name: "隐藏 (1)" }));
    await user.click(screen.getByRole("button", { name: "恢复" }));
    expect(reviseMemory).toHaveBeenLastCalledWith("memory", { hidden: false });
    await user.click(screen.getByRole("button", { name: "生效 (1)" }));
    expect(screen.getByText("向导来自南方的港口。")).toBeTruthy();
  });

  it("closes a staged item menu and disables memory edits when entering history", async () => {
    const user = userEvent.setup();
    render(<><RightRail /><InventoryActionBubble /><ComposerShelf /></>);
    await user.click(screen.getByText("旅行茶", { selector: ".slot-name-label" }));
    expect(screen.getByRole("button", { name: "饮用两份旅行茶" })).toBeTruthy();
    await act(async () => { useStory.setState({ viewNodeId: "root" }); });
    expect(screen.queryByRole("button", { name: "饮用两份旅行茶" })).toBeNull();
    expect((screen.getByRole("button", { name: "纠正" }) as HTMLButtonElement).disabled).toBe(true);
    expect((document.getElementById("story-input") as HTMLTextAreaElement).disabled).toBe(true);
  });

  it("does not carry an unsaved memory correction into another branch", async () => {
    const user = userEvent.setup();
    render(<RightRail />);
    await user.click(screen.getByRole("button", { name: "纠正" }));
    await user.type(screen.getByPlaceholderText("输入修订后的记忆事实与潜台词..."), "这只是旧分支的草稿。");
    const next = makeView(); next.branch.branchId = "sibling";
    await act(async () => { useStory.setState({ view: next, hud: next.state, activeBranchId: "sibling" }); });
    expect(screen.queryByPlaceholderText("输入修订后的记忆事实与潜台词...")).toBeNull();
    expect(screen.getByText(memory.content)).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "纠正" }));
    expect((screen.getByPlaceholderText("输入修订后的记忆事实与潜台词...") as HTMLTextAreaElement).value).toBe(memory.content);
  });

  it("opens secondary modals when clicking on the liquid glass navigation tiles", async () => {
    useUi.setState({ memoryModalOpen: false, dossierModalOpen: false, promisesModalOpen: false });
    const user = userEvent.setup();
    render(<RightRail />);
    expect(screen.queryByRole("dialog")).toBeNull();

    // Test clicking 角色设定与世界观
    await user.click(screen.getByRole("button", { name: /角色设定与世界观/ }));
    expect(useUi.getState().dossierModalOpen).toBe(true);

    // Test clicking 主动契约与承诺
    await user.click(screen.getByRole("button", { name: /主动契约与承诺/ }));
    expect(useUi.getState().promisesModalOpen).toBe(true);

    // Test clicking 认知记忆库
    await user.click(screen.getByRole("button", { name: /认知记忆库/ }));
    expect(useUi.getState().memoryModalOpen).toBe(true);
  });
});

describe("compressed story presentation", () => {
  it("renders valid XML entities, CDATA, comments and character names as readable text", () => {
    const current = makeView();
    current.activeSummary = { summaryId: "summary", fromNodeId: "root", toNodeId: "head", sourceHash: "source",
      text: `<!-- verified summary -->\n<story_checkpoint><narrative_arc><![CDATA[旅人 <林舟> 抵达港口。]]></narrative_arc><character_dynamics><mindset character='向导 &amp; 船长'>期待 &amp; 谨慎</mindset><hidden_tension>风暴将至。</hidden_tension></character_dynamics><open_loops>寻找 &lt;铜钥匙&gt;。</open_loops></story_checkpoint>` };
    useStory.setState({ view: current });
    render(<StoryCheckpointSection />);
    expect(screen.getByText("旅人 <林舟> 抵达港口。")).toBeTruthy();
    expect(screen.getByText("向导 & 船长：")).toBeTruthy();
    expect(screen.getByText("期待 & 谨慎")).toBeTruthy();
    expect(screen.getByText("寻找 <铜钥匙>。")).toBeTruthy();
    expect(document.querySelector("script")).toBeNull();
  });

  it("keeps legacy plain summaries readable without interpreting markup", () => {
    const current = makeView();
    current.activeSummary = { summaryId: "old", fromNodeId: "root", toNodeId: "head", sourceHash: "source", text: "旅人已到港口，等待天明。" };
    useStory.setState({ view: current });
    render(<StoryCheckpointSection />);
    expect(screen.getByText("旅人已到港口，等待天明。")).toBeTruthy();
  });
});
