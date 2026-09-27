// branchSlice: 分支切换、节点回溯、平行世界分叉与重生
import { api } from "../../app/api";
import type { AssistantEditResult, DeriveResult, TextBlock } from "../../app/types";
import type { StoreGet, StoreSet, StoryState } from "../storyTypes";
import {
  isTurnBusy,
  messagesFromNodes,
  owns,
  reloadSeq,
  scope,
  incrementSendSeq,
  toErr,
} from "../storyHelpers";
import { clearPendingSubmission, detachTurnStream, openDerivedBranch, recoverActiveTurn } from "./turnSlice";
import { useSettings } from "../settingsStore";
import { emptyDirectorState } from "./directorSlice";

// branchBusy 的单调序号：迟到的 finally 不得清除更新操作的忙碌锁。
let branchOpSeq = 0;

export const createBranchSlice = (set: StoreSet, get: StoreGet) => {
  // navigateView 是切换分支/回溯节点/回到现在的唯一入口：先记录落点，
  // 再套用新落点并重新拉取视图；拉取失败时回滚落点并留下可见错误。
  // 旧实现失败会留下"指向不在当前视图里的节点"的 viewNodeId，输入台因此
  // 被当作只读历史锁死，而调用方用 void 吞掉了 rejection。
  const navigateView = async (apply: () => Partial<StoryState>, label: string) => {
    const before = {
      activeBranchId: get().activeBranchId,
      viewNodeId: get().viewNodeId,
      phase: get().phase,
      error: get().error,
      memories: get().memories,
      memoryUsage: get().memoryUsage,
      memoryError: get().memoryError,
      // 回滚必须连同生成草稿与分页游标一起还原，否则失败后输入台空白、历史分页错位。
      draftBlocks: get().draftBlocks,
      draftOptions: get().draftOptions,
      lastTurnId: get().lastTurnId,
      hasMoreHistory: get().hasMoreHistory,
      oldestTurnId: get().oldestTurnId,
      olderMessages: get().olderMessages,
    };
    // 落点变化意味着历史分页作废：清空已加载的更早消息，避免跨分支串页。
    clearPendingSubmission();
    set({ ...apply(), olderMessages: [] } as never);
    try {
      await get().reloadView();
      return true;
    } catch (e) {
      set({ ...before, error: `${label}失败，已还原到原位置：${toErr(e)}` } as never);
      return false;
    }
  };

  return {
  switchBranch: async (branchId: string) => {
    if (isTurnBusy(get().phase)) return;
    detachTurnStream();
    const moved = await navigateView(() => ({
      ...emptyDirectorState, lastTurnId: null, phase: "idle", activeBranchId: branchId, viewNodeId: null,
      draftBlocks: [], draftOptions: [], error: null, memories: [], memoryUsage: null, memoryError: null,
    }), "切换分支");
    if (!moved) return;
    const fresh = get().view;
    if (fresh?.branch.branchId === branchId) recoverActiveTurn(fresh);
  },

  viewAt: async (nodeId: string) => {
    if (isTurnBusy(get().phase)) return;
    await navigateView(() => ({
      ...emptyDirectorState, viewNodeId: nodeId, error: null, memories: [], memoryUsage: null, memoryError: null,
    }), "查看历史节点");
  },

  backToHead: async () => {
    await navigateView(() => ({
      ...emptyDirectorState, viewNodeId: null, error: null, memories: [], memoryUsage: null, memoryError: null,
    }), "返回当前进度");
  },

  // regenerate 的 guidance 是"重生成引导"：原有的两条路只有盲重掷与手工改正文，
  // 而"演得不对但说不清怎么改"才是最常见的处境。引导随请求走，并留在节点内容里
  // （TurnContent.InputNote），回看时能分辨这是不是一次带要求的重演。
  regenerate: async (nodeId: string, recheck = false, guidance?: string) => {
    const owner = scope();
    const view = get().view;
    if (!view || isTurnBusy(get().phase)) return;
    set({ phase: "generating", error: null });
    try {
      const note = (guidance ?? "").trim();
      const res = await api.regenerateTurn(view.sessionId, view.branch.branchId, {
        nodeId,
        expectedCharacterId: view.characterId,
        idempotencyKey: `rg_${Date.now()}_${incrementSendSeq()}`,
        recheck,
        ...(note ? { note } : {}),
        ...(useSettings.getState().optionsFrequency !== "auto"
          ? { options: useSettings.getState().optionsFrequency }
          : {}),
      });
      if (!owns(owner)) return;
      await openDerivedBranch(view.sessionId, res.branchId, res.turnId);
    } catch (e) {
      if (owns(owner)) set({ phase: "failed", error: toErr(e) });
    }
  },

  editPlayerInput: async (nodeId: string, text: string) => {
    const owner = scope();
    const view = get().view;
    if (!view || isTurnBusy(get().phase) || !text.trim()) return;
    set({ phase: "generating", error: null });
    try {
      const res = (await api.editTurn(view.sessionId, view.branch.branchId, {
        nodeId,
        expectedCharacterId: view.characterId,
        input: { kind: "text", text: text.trim() },
        idempotencyKey: `ed_${Date.now()}_${incrementSendSeq()}`,
      })) as DeriveResult;
      if (!owns(owner)) return;
      await openDerivedBranch(view.sessionId, res.branchId, res.turnId);
    } catch (e) {
      if (owns(owner)) set({ phase: "failed", error: toErr(e) });
    }
  },

  editAssistantText: async (nodeId: string, blocks: TextBlock[]) => {
    const owner = scope();
    const view = get().view;
    if (!view || isTurnBusy(get().phase) || get().branchBusy) return;
    const op = ++branchOpSeq;
    set({ branchBusy: true, error: null });
    try {
      const res = (await api.editTurn(view.sessionId, view.branch.branchId, {
        nodeId,
        expectedCharacterId: view.characterId,
        blocks,
        idempotencyKey: `ea_${Date.now()}_${incrementSendSeq()}`,
      })) as AssistantEditResult;
      if (!owns(owner)) return;
      await openDerivedBranch(view.sessionId, res.branchId);
    } catch (e) {
      if (owns(owner)) set({ error: toErr(e) });
    } finally {
      // 仅当没有更新的分支操作接管时才清除，避免迟到 finally 提前释放新的忙碌锁。
      if (op === branchOpSeq) set({ branchBusy: false });
    }
  },

  forkFrom: async (nodeId: string, name?: string) => {
    const owner = scope();
    const view = get().view;
    if (!view || isTurnBusy(get().phase) || get().branchBusy) return;
    const op = ++branchOpSeq;
    set({ branchBusy: true, error: null });
    try {
      const res = await api.forkBranch(view.sessionId, { fromNodeId: nodeId, name, expectedCharacterId: view.characterId });
      if (!owns(owner)) return;
      await openDerivedBranch(view.sessionId, res.branch.branchId);
    } catch (e) {
      if (owns(owner)) set({ error: toErr(e) });
    } finally {
      if (op === branchOpSeq) set({ branchBusy: false });
    }
  },

  loadOlder: async () => {
    const owner = scope();
    const ticket = reloadSeq;
    const st = get();
    const view = st.view;
    if (!view || !st.hasMoreHistory || st.loadingOlder) return;
    set({ loadingOlder: true });
    try {
      const older = await api.getSession(view.sessionId, {
        branchId: st.activeBranchId ?? view.branch.branchId,
        before: st.oldestTurnId ?? undefined,
        viewNodeId: st.viewNodeId ?? undefined,
      });
      if (!owns(owner) || reloadSeq !== ticket) {
        // 视图已经换过：确保不把"加载中"留在这个视图上。
        set({ loadingOlder: false });
        return;
      }
      const newOlder = messagesFromNodes(older, get().turnThinking).filter((m) => m.role !== "opening");
      const snapshot = get();
      const seen = new Set([...snapshot.messages.map((m) => m.id), ...snapshot.olderMessages.map((m) => m.id)]);
      const add = newOlder.filter((m) => !seen.has(m.id));
      set({
        olderMessages: [...add, ...snapshot.olderMessages],
        messages: [...add, ...snapshot.messages],
        hasMoreHistory: !!older.hasMore,
        oldestTurnId: older.oldestTurnId ?? null,
        loadingOlder: false,
      });
    } catch (e) {
      // 调用方用 void 调用，不能靠 throw 让用户看见；写进 error 由叙事流呈现。
      if (owns(owner)) set({ loadingOlder: false, error: "加载更早历史失败：" + toErr(e) });
    }
  },
  };
};
