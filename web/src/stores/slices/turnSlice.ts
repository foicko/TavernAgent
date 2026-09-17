// turnSlice: 对话推进、流式演义、思考过程与状态对齐
import { api, subscribeTurnEvents } from "../../app/api";
import type { AcceptResult, SessionView, TextBlock, TurnDraftSnapshot } from "../../app/types";
import { useSettings } from "../settingsStore";
import { useUi } from "../uiStore";
import { emptyDirectorState } from "./directorSlice";
import type { OptionView, StoreGet, StoreSet, TurnInput } from "../storyTypes";
import {
  isTurnBusy,
  storyBusy,
  emptyMemoryPaging,
  messagesFromNodes,
  owns,
  scope,
  sessionViewCache,
  toErr,
  navigationEpoch,
  incrementSendSeq,
  getReloadSeq,
  incrementReloadSeq,
  resolveCharKeyFromSession,
  getStoryState,
  setStoryState,
} from "../storyHelpers";

export let turnCursor: { turnId: string; id: string } | null = null;
export let activeTurnSub: { turnId: string; sessionId: string; stop: () => void; synchronize: (committed?: boolean) => void } | null = null;
export const TURN_STATUS_INTERVAL_MS = 5000;

export function detachTurnStream(turnId?: string) {
  if (turnId && activeTurnSub?.turnId !== turnId) return;
  activeTurnSub?.stop();
  activeTurnSub = null;
}

export function recoverActiveTurn(view: SessionView) {
  const id = view.branch.activeTurnId;
  if (!id) return;
  turnCursor = null;
  setStoryState({ phase: "generating", lastTurnId: id, draftBlocks: [], draftOptions: [] });
  // 恢复场景立刻同步一次：把服务端的在途正文快照取回来，不必等第一次轮询。
  attachTurnStream(view.sessionId, id, false, true);
}

export function attachTurnStream(sessionId: string, turnId: string, alreadyCommitted = false, snapshotFirst = false) {
  detachTurnStream();
  const owner = scope();
  let timer: ReturnType<typeof setTimeout> | undefined;
  let stopStream = () => {};
  let stopped = false;
  let syncing = false;
  let requested = false;
  let committed = alreadyCommitted;
  let syncError: string | null = null;
  const current = () => !stopped && activeTurnSub === subscription && owns(owner) && getStoryState().lastTurnId === turnId;
  const stopTransport = () => { const stop = stopStream; stopStream = () => {}; stop(); };
  const schedule = (delay: number) => {
    clearTimeout(timer);
    if (current()) timer = setTimeout(() => { void synchronize(); }, delay);
  };
  const requestSync = (knownCommitted = false) => {
    if (!current()) return;
    committed ||= knownCommitted;
    if (committed) stopTransport();
    requested = true;
    if (!syncing) schedule(0);
  };
  const onFocus = () => requestSync();
  const onVisible = () => { if (!document.hidden) requestSync(); };

  async function synchronize() {
    if (!current() || syncing) return;
    clearTimeout(timer);
    syncing = true;
    requested = false;
    try {
      const turn = committed ? null : await api.getTurn(turnId);
      if (!current()) return;
      const status = committed ? "committed" : turn?.status;
      if (!status) throw new Error("服务未返回回合状态");
      if (syncError && getStoryState().error === syncError) setStoryState({ error: null });
      syncError = null;
      if (status === "committed") {
        committed = true;
        stopTransport();
        setStoryState({ phase: "committed" });
        await getStoryState().reloadView();
      } else if (status === "awaiting_continuation" || status === "awaiting_approval") {
        const phase = status === "awaiting_continuation" ? "truncated" : "awaiting_confirm";
        const changed = getStoryState().phase !== phase;
        setStoryState({ phase, ...(turn?.input ? { lastInput: turn.input } : {}) });
        if (changed && phase === "truncated" && useSettings.getState().autoContinue) void getStoryState().continueTurn();
      } else if (["failed", "cancelled", "conflicted"].includes(status)) {
        routeEvent({ event: `turn.${status}`, data: { message: turn?.failureMessage } }, sessionId, turnId, () => detachTurnStream(turnId));
      } else {
        mergeInFlightDraft(turn?.draft);
        setStoryState({ phase: "generating" });
      }
    } catch (error) {
      if (current()) {
        syncError = `回复进度暂时无法同步，正在重试：${toErr(error)}`;
        setStoryState({ error: syncError });
      }
    } finally {
      syncing = false;
      if (current() && (requested || syncError || ["generating", "committed"].includes(getStoryState().phase))) {
        schedule(requested ? 0 : TURN_STATUS_INTERVAL_MS);
      }
    }
  }

  const subscription = {
    sessionId, turnId, synchronize: requestSync,
    stop: () => {
      stopped = true;
      clearTimeout(timer);
      stopTransport();
      if (typeof window !== "undefined") window.removeEventListener("focus", onFocus);
      if (typeof document !== "undefined") document.removeEventListener("visibilitychange", onVisible);
    },
  };
  activeTurnSub = subscription;
  if (typeof window !== "undefined") window.addEventListener("focus", onFocus);
  if (typeof document !== "undefined") document.addEventListener("visibilitychange", onVisible);
  if (!committed) {
    stopStream = subscribeTurnEvents(turnId, {
      lastEventId: turnCursor?.turnId === turnId ? turnCursor.id : undefined,
      onEvent: ev => {
        if (!current() || committed) return;
        if (ev.id?.startsWith(turnId + ":")) turnCursor = { turnId, id: ev.id };
        routeEvent(ev, sessionId, turnId, () => detachTurnStream(turnId));
      },
      onReconnect: () => requestSync(),
      onFinalError: () => requestSync(),
    });
  }
  if (committed) requestSync(true);
  else schedule(snapshotFirst ? 0 : TURN_STATUS_INTERVAL_MS);
}

// mergeInFlightDraft 用服务端快照补齐"正在写的这一段"。
// 断线期间当前块的行内增量（Sequence 0）不会重放，本地文本可能缺字；
// 快照是服务端权威文本，但只允许把它变长——迟到的状态读取不许抹掉后续增量。
function mergeInFlightDraft(draft?: TurnDraftSnapshot | null) {
  const inFlight = draft?.inFlight;
  if (!inFlight?.text) return;
  const blocks = [...getStoryState().draftBlocks];
  const seq = inFlight.seq > 0 ? inFlight.seq : blocks.length + 1;
  const existing = seq <= blocks.length ? blocks[seq - 1] : undefined;
  if (existing && existing.text.length >= inFlight.text.length) return;
  while (blocks.length < seq - 1) blocks.push({ kind: "narration", text: "" });
  blocks[seq - 1] = {
    kind: (inFlight.kind as TextBlock["kind"]) || "narration",
    speakerId: inFlight.speakerId ?? undefined,
    text: inFlight.text,
  };
  setStoryState({ draftBlocks: blocks });
}

export function routeEvent(ev: { event: string; data: unknown }, sessionId: string, turnId: string, onEnd: () => void) {  if (getStoryState().view?.sessionId !== sessionId || getStoryState().lastTurnId !== turnId) {
    onEnd();
    return;
  }
  switch (ev.event) {
    case "turn.thinking": {
      const payload = ev.data as { thinking?: string };
      if (payload?.thinking) {
        setStoryState((prev) => ({
          thinkingText: prev.thinkingText + payload.thinking,
          turnThinking: {
            ...prev.turnThinking,
            [turnId]: (prev.turnThinking[turnId] || "") + payload.thinking,
          },
        }));
      }
      break;
    }
    case "block.delta": {
      const payload = ev.data as { seq?: number; kind?: string; speakerId?: string | null; delta?: string };
      if (payload?.delta) {
        const seq = payload.seq ?? 1;
        const kind = (payload.kind as TextBlock["kind"]) || "narration";
        const blocks = [...getStoryState().draftBlocks];
        if (seq <= blocks.length) {
          const target = blocks[seq - 1];
          blocks[seq - 1] = {
            ...target,
            text: target.text + payload.delta,
          };
        } else {
          while (blocks.length < seq - 1) {
            blocks.push({ kind: "narration", text: "" });
          }
          blocks.push({
            kind,
            speakerId: payload.speakerId ?? undefined,
            text: payload.delta,
          });
        }
        setStoryState({ draftBlocks: blocks });
      }
      break;
    }
    case "block.appended": {
      const payload = ev.data as { frameSeq?: number; frame?: TextBlock };
      if (payload?.frame) {
        const frameSeq = (payload.frame as unknown as { seq?: unknown }).seq;
        const seq = payload.frameSeq ?? (typeof frameSeq === "number" ? frameSeq : undefined);
        const blocks = [...getStoryState().draftBlocks];
        if (seq && seq <= blocks.length) {
          blocks[seq - 1] = payload.frame;
        } else {
          blocks.push(payload.frame);
        }
        setStoryState({ draftBlocks: blocks });
      }
      break;
    }
    case "turn.started":
    case "turn.truncated":
    case "turn.awaiting_continuation":
    case "turn.awaiting_approval":
      activeTurnSub?.synchronize();
      break;
    case "turn.committed":
      activeTurnSub?.synchronize(true);
      break;
    case "turn.cancelled":
      onEnd();
      setStoryState({ phase: "idle", draftBlocks: [], draftOptions: [] });
      break;
    case "turn.failed": {
      const d = ev.data as { message?: string };
      onEnd();
      setStoryState({ phase: "failed", error: d.message ?? "生成失败", draftBlocks: [], draftOptions: [] });
      break;
    }
    case "turn.conflicted":
      onEnd();
      setStoryState({ phase: "failed", error: "分支已变化，请刷新后再试", draftBlocks: [], draftOptions: [] });
      break;
  }
}

let pendingSubmission: { owner: ReturnType<typeof scope>; view: SessionView; key: string; input: TurnInput } | null = null;

async function startTurn(view: SessionView, key: string, input: TurnInput, owner = scope()) {
  if (!owns(owner) || owner.sessionId !== view.sessionId) return;
  pendingSubmission = { owner, view, key, input };
  setStoryState({ lastInput: input, lastTurnId: null });
  const result: AcceptResult = await api.acceptTurn(view.sessionId, view.branch.branchId, {
    idempotencyKey: key,
    expectedHeadId: view.branch.headNodeId,
    expectedVersion: view.branch.version,
    expectedCharacterId: view.characterId,
    input,
  });
  if (!owns(owner)) return;
  if (pendingSubmission?.key === key) pendingSubmission = null;
  setStoryState({ lastTurnId: result.turnId, lastInput: input, thinkingText: "", draftBlocks: [], draftOptions: [] });
  attachTurnStream(view.sessionId, result.turnId);
}

export async function openDerivedBranch(sessionId: string, branchId: string, turnId?: string) {
  setStoryState({ ...emptyDirectorState, activeBranchId: branchId, viewNodeId: null, lastTurnId: turnId ?? null,
    phase: turnId ? "generating" : "idle", pendingInput: null, pendingAction: null,
    memories: [], memoryUsage: null, memoryError: null, memoryLoading: false, memoryPaging: emptyMemoryPaging(), error: null });
  const owner = scope();
  try { await getStoryState().reloadView(); }
  catch (error) { if (owns(owner)) setStoryState({ error: toErr(error) }); }
  if (turnId && owns(owner) && getStoryState().lastTurnId === turnId) attachTurnStream(sessionId, turnId);
}

export const createTurnSlice = (set: StoreSet, get: StoreGet) => ({
  send: async (text: string, actionRef?: string) => {
    if (get().viewNodeId) { set({ error: "历史快照为只读，请先返回最新进度或从此处新建分支" }); return; }
    if (!text.trim() || isTurnBusy(get().phase) || get().directorCommanding) return;
    const key = useUi.getState().activeCharKey;
    const epoch = navigationEpoch;
    if (!get().view) {
      if (!key || key === "custom") { set({ error: "请先打开自定义角色会话或导入角色卡" }); return; }
      await get().createSessionFromPreset(key);
      if (navigationEpoch !== epoch + 1 || useUi.getState().activeCharKey !== key) return;
    }
    const view = get().view;
    if (!view || view.characterId !== get().activeCharacterId) {
      // 静默 return 会让输入台已经把文字清空、文字却既没上屏也没进回合。
      // 给出可见失败，让输入台的恢复逻辑把文字还回去。
      set({ error: "当前会话与角色不匹配，请重新打开会话后再发送", ...(storyBusy(get()) ? {} : { phase: "failed" as const }) });
      return;
    }
    if (resolveCharKeyFromSession(view) !== useUi.getState().activeCharKey) { set({ error: "角色归属已变化，请重新打开会话" }); return; }
    const owner = scope();
    const input: TurnInput = { kind: actionRef ? "action" : "text", text, ...(actionRef ? { actionRef } : {}) };
    set({ phase: "generating", error: null, pendingInput: null, lastInput: input });
    try {
      await startTurn(view, `k_${Date.now()}_${incrementSendSeq()}`, input, owner);
    } catch (e) {
      if (owns(owner)) set({ phase: "failed", error: toErr(e) });
    }
  },

  chooseOption: async (o: OptionView) => {
    if (get().viewNodeId) return;
    const owner = scope();
    const view = get().view;
    if (!view || isTurnBusy(get().phase) || get().directorCommanding) return;
    if (useSettings.getState().optionMode === "fill-edit") {
      set({ pendingInput: o.text, pendingAction: null });
      return;
    }
    set({ phase: "generating", error: null, lastInput: { kind: "option", text: o.text } });
    const key = `k_${Date.now()}_${incrementSendSeq()}`;
    try {
      await startTurn(view, key, { kind: "option", text: o.text, optionRef: { nodeId: [...(view.nodes ?? [])].reverse().find(n => n.kind === "turn")?.nodeId ?? view.branch.headNodeId, optionId: o.optionId } }, owner);
    } catch (e) {
      if (owns(owner)) set({ phase: "failed", error: toErr(e) });
    }
  },

  cancel: async () => {
    const owner = scope(), id = get().lastTurnId;
    if (!id) return;
    try {
      const result = await api.cancelTurn(id, get().view?.characterId);
      if (!owns(owner) || get().lastTurnId !== id) return;
      if (result.status === "committed") {
        attachTurnStream(get().view!.sessionId, id, true);
      } else if (result.status === "cancelled") {
        detachTurnStream(id);
        set({ phase: "idle", draftBlocks: [], draftOptions: [] });
        await get().refreshView();
      }
    } catch (e) {
      if (owns(owner) && get().lastTurnId === id) set({ error: toErr(e) });
    }
  },

  continueTurn: async () => {
    const owner = scope();
    const id = get().lastTurnId;
    const view = get().view;
    if (!id || !view || get().phase !== "truncated") return;
    detachTurnStream(id);
    set({ phase: "generating", error: null });
    try {
      await api.continueTurn(id, view.characterId);
      if (!owns(owner) || get().lastTurnId !== id) return;
    } catch (e) {
      if (owns(owner) && get().lastTurnId === id) {
        set({ phase: "truncated", error: toErr(e) });
        attachTurnStream(view.sessionId, id);
      }
      return;
    }
    attachTurnStream(view.sessionId, id);
  },

  retry: async () => {
    const owner = scope();
    const st = get();
    const last = st.lastInput;
    const view = st.view;
    if (!last || !view || st.viewNodeId || st.phase === "generating" || st.phase === "committed") return;
    // A lost acceptance response does not establish failure. Replay the exact
    // original request/key, including its immutable head and option reference.
    if (pendingSubmission && owns(pendingSubmission.owner)) {
      const pending = pendingSubmission;
      set({ phase: "generating", error: null });
      try { await startTurn(pending.view, pending.key, pending.input, owner); }
      catch (e) { if (owns(owner)) set({ phase: "failed", error: toErr(e) }); }
      return;
    }
    if (st.lastTurnId) {
      let status: string;
      try {
        status = (await api.cancelTurn(st.lastTurnId, view.characterId)).status;
      } catch (cancelError) {
        if (!owns(owner) || get().lastTurnId !== st.lastTurnId) return;
        try { status = (await api.getTurn(st.lastTurnId)).status; }
        catch { if (owns(owner)) set({ error: toErr(cancelError) }); return; }
      }
      if (!owns(owner) || get().lastTurnId !== st.lastTurnId) return;
      if (status === "committed") {
        attachTurnStream(view.sessionId, st.lastTurnId, true);
        return;
      }
      if (!["cancelled", "failed", "conflicted"].includes(status)) {
        set({ error: "上一回合尚未结束，请等待取消完成后重试" });
        attachTurnStream(view.sessionId, st.lastTurnId);
        return;
      }
      detachTurnStream(st.lastTurnId);
    }
    if (!owns(owner)) return;
    set({ phase: "generating", error: null, draftBlocks: [], draftOptions: [] });
    const key = `retry_${Date.now()}_${incrementSendSeq()}`;
    try {
      await startTurn(view, key, last, owner);
    } catch (e) {
      if (owns(owner)) set({ phase: "failed", error: toErr(e) });
    }
  },

  clearPending: () => set({ pendingInput: null, pendingAction: null }),

  prepareAction: (text: string, actionRef: string) => {
    const current = get();
    if (!current.view || isTurnBusy(current.phase) || current.viewNodeId) return;
    if (!current.view.actions?.some(action => action.actionId === actionRef)) return;
    set({ pendingInput: text, pendingAction: actionRef });
  },

  reloadView: async () => {
    const owner = scope();
    const ticket = incrementReloadSeq();
    const st = get();
    const view = st.view;
    if (!view) return;
    const fresh = await api.getSession(view.sessionId, {
      branchId: st.activeBranchId ?? view.branch.branchId,
      viewNodeId: st.viewNodeId ?? undefined,
    });
    if (!owns(owner) || ticket !== getReloadSeq()) return;
    const completedTurn = get().phase === "committed";
    const completedTurnId = get().lastTurnId;
    const completedThinking = get().thinkingText;
    const turnThinking = { ...get().turnThinking };
    if (completedTurn && completedTurnId && completedThinking) {
      turnThinking[completedTurnId] = completedThinking;
      if (fresh.headNode?.nodeId) {
        turnThinking[fresh.headNode.nodeId] = completedThinking;
      }
    }
    if (completedTurn) detachTurnStream(get().lastTurnId ?? undefined);
    if (!st.viewNodeId) {
      sessionViewCache.set(fresh.sessionId, fresh);
    }
    set({
      view: fresh,
      hud: fresh.state,
      turnThinking,
      messages: messagesFromNodes(fresh, turnThinking),
      ...(["generating", "truncated", "awaiting_confirm"].includes(get().phase) && get().lastTurnId ? {} : { draftBlocks: [], draftOptions: [] }),
      ...(completedTurn ? { phase: "idle" as const, thinkingText: "" } : {}),
      error: null,
      pendingInput: null,
      hasMoreHistory: !!fresh.hasMore,
      oldestTurnId: fresh.oldestTurnId ?? null,
      loadingOlder: false,
    });
    await get().loadMemories();
  },

  refreshView: async () => {
    const owner = scope();
    const turnId = get().lastTurnId;
    try {
      await get().reloadView();
      if (owns(owner) && get().lastTurnId === turnId && !["generating", "truncated", "awaiting_confirm"].includes(get().phase)) set({ phase: "idle" });
    } catch (e) { if (owns(owner)) set({ error: toErr(e) }); }
  },
});
