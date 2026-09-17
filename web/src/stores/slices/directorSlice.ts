import { api, subscribeDirectorEvents } from "../../app/api";
import type { DirectorAction, DirectorDraft, DirectorPlan, DirectorView } from "../../app/types";
import type { StoreGet, StoreSet } from "../storyTypes";
import { isTurnBusy, owns, scope, toErr } from "../storyHelpers";

export const DIRECTOR_POLL_MS = 3000;

// 讨论在推进这件事属于故事，不属于打开它的面板：面板卸载、工作区关闭都不能让它
// 停在"执行中"。与回合恢复同一套路：durable SSE + 轮询兜底 + 归属守卫，终态后刷新。
let directorWatch: { requestId: string; stop: () => void; timer: ReturnType<typeof setInterval> } | null = null;

function detachDirectorWatch() {
  if (!directorWatch) return;
  directorWatch.stop();
  clearInterval(directorWatch.timer);
  directorWatch = null;
}

function attachDirectorRequest(get: StoreGet, set: StoreSet, requestId: string) {
  if (!requestId || directorWatch?.requestId === requestId) return;
  detachDirectorWatch();
  const owner = scope();
  let checking = false;
  let finished = false;
  const reconcile = async () => {
    if (finished || checking) return;
    if (!owns(owner)) { finished = true; detachDirectorWatch(); return; }
    checking = true;
    try {
      const latest = await api.getDirectorRequest(requestId);
      if (!owns(owner)) return;
      if (latest.status !== "generating") {
        finished = true;
        detachDirectorWatch();
        set((state) => ({ directorView: state.directorView ? { ...state.directorView,
          requests: state.directorView.requests.map(r => r.requestId === requestId ? { ...r, status: latest.status } : r),
        } : state.directorView }));
        try { await get().loadDirector(); } catch { /* 终态已写入，刷新失败只影响草稿展示 */ }
        return;
      }
    } catch {
      // SSE 与轮询各自独立恢复：单次读取失败不终止跟踪。
    } finally {
      checking = false;
    }
  };
  const stop = subscribeDirectorEvents(requestId, {
    onEvent: event => { if (event.event !== "director.started") void reconcile(); },
    onReconnect: () => { void reconcile(); },
    onFinalError: () => { void reconcile(); },
  });
  const timer = setInterval(() => { void reconcile(); }, DIRECTOR_POLL_MS);
  directorWatch = { requestId, stop, timer };
  void reconcile();
}

export interface DirectorSliceState {
  directorView: DirectorView | null;
  directorLoading: boolean;
  directorSaving: boolean;
  directorCommanding: boolean;
  directorError: string | null;
  directorScope: string;
  loadDirector: () => Promise<void>;
  saveDirectorDraft: (plan: DirectorPlan, version: number, baseRevisionId: string) => Promise<DirectorDraft>;
  commandDirector: (action: DirectorAction, beatId?: string, replace?: boolean) => Promise<void>;
  discussDirector: (text: string) => Promise<void>;
  cancelDirector: (requestId: string) => Promise<void>;
}

export const emptyDirectorState = {
  directorView: null, directorLoading: false, directorSaving: false, directorCommanding: false, directorError: null, directorScope: "",
};

export function directorScopeKey(sessionId: string, branchId: string, nodeId?: string | null): string {
  return `${sessionId}/${branchId}/${nodeId || "head"}`;
}

export function createDirectorSlice(set: StoreSet, get: StoreGet) {
  let readSequence = 0;
  let commandKey: { payload: string; key: string } | null = null;
  let messageKey: { payload: string; key: string } | null = null;
  return {
    ...emptyDirectorState,
    loadDirector: async () => {
      const view = get().view;
      if (!view) { set(emptyDirectorState); return; }
      const owner = scope(), ticket = ++readSequence;
      const key = directorScopeKey(view.sessionId, view.branch.branchId, get().viewNodeId);
      set({ directorLoading: true, directorError: null, ...(get().directorScope === key ? {} : { directorView: null, directorScope: key }) });
      try {
        const result = await api.getDirector(view.sessionId, view.branch.branchId, get().viewNodeId ?? undefined);
        if (owns(owner) && ticket === readSequence) {
          set(state => {
            const saved = state.directorScope === key ? state.directorView?.draft : null;
            return { directorView: !result.readOnly && saved && saved.version > (result.draft?.version ?? 0)
              ? { ...result, draft: saved } : result };
          });
          // 重新打开/刷新后接着跟踪在途讨论（例如另一标签页发起的）。
          const running = result.requests?.find(r => r.status === "generating");
          if (running) attachDirectorRequest(get, set, running.requestId);
        }
      } catch (error) {
        if (owns(owner) && ticket === readSequence) set({ directorError: toErr(error) });
      } finally {
        if (owns(owner) && ticket === readSequence) set({ directorLoading: false });
      }
    },
    saveDirectorDraft: async (plan: DirectorPlan, version: number, baseRevisionId: string) => {
      const view = get().view, owner = scope();
      if (!view || get().viewNodeId) throw new Error("历史视图只读，请回到当前节点或分叉");
      ++readSequence;
      set({ directorSaving: true, directorError: null });
      try {
        const draft = await api.saveDirectorDraft(view.sessionId, view.branch.branchId, {
          expectedCharacterId: view.characterId, expectedDraftVersion: version, baseRevisionId, plan,
        });
        if (owns(owner)) {
          set(state => ({ directorView: state.directorView ? { ...state.directorView,
            draft: (state.directorView.draft?.version ?? 0) > draft.version ? state.directorView.draft : draft,
          } : null }));
        }
        return draft;
      } catch (error) {
        if (owns(owner)) set({ directorError: toErr(error) });
        throw error;
      } finally {
        if (owns(owner)) set({ directorSaving: false, directorLoading: false });
      }
    },
    commandDirector: async (action: DirectorAction, beatId?: string, replace?: boolean) => {
      const view = get().view, owner = scope();
      if (!view || get().viewNodeId || isTurnBusy(get().phase) || get().directorCommanding) return;
      const body = { expectedCharacterId: view.characterId, expectedHeadId: view.branch.headNodeId, expectedVersion: view.branch.version,
        action, beatId, replace, draftVersion: get().directorView?.draft?.version };
      const payload = JSON.stringify({ sessionId: view.sessionId, branchId: view.branch.branchId, body });
      if (commandKey?.payload !== payload) commandKey = { payload, key: crypto.randomUUID() };
      const key = commandKey.key;
      set({ directorCommanding: true, directorError: null });
      try {
        await api.directorCommand(view.sessionId, view.branch.branchId, { ...body, idempotencyKey: key });
        if (commandKey?.key === key) commandKey = null;
        if (owns(owner)) {
          await get().reloadView();
          if (owns(owner)) await get().loadDirector();
        }
      } catch (error) {
        if (owns(owner)) set({ directorError: toErr(error) });
        throw error;
      } finally {
        if (owns(owner)) set({ directorCommanding: false });
      }
    },
    discussDirector: async (text: string) => {
      const view = get().view, owner = scope();
      if (!view || get().viewNodeId || !text.trim()) return;
      const body = { expectedCharacterId: view.characterId, expectedHeadId: view.branch.headNodeId,
        expectedDraftVersion: get().directorView?.draft?.version ?? 0, text: text.trim() };
      const payload = JSON.stringify({ sessionId: view.sessionId, branchId: view.branch.branchId, body });
      if (messageKey?.payload !== payload) messageKey = { payload, key: crypto.randomUUID() };
      const key = messageKey.key;
      set({ directorError: null });
      try {
        const result = await api.directorMessage(view.sessionId, view.branch.branchId, { ...body, idempotencyKey: key });
        if (messageKey?.key === key) messageKey = null;
        if (owns(owner)) {
          set(state => ({ directorView: state.directorView ? { ...state.directorView,
            requests: [...state.directorView.requests.filter(r => r.requestId !== result.requestId), result.request],
          } : null }));
          // 从发起的这一刻起由 store 负责推进：工作区关掉也照样对账到终态。
          if (result.request?.status === "generating") attachDirectorRequest(get, set, result.requestId);
        }
      } catch (error) {
        if (owns(owner)) set({ directorError: toErr(error) });
        throw error;
      }
    },
    cancelDirector: async (requestId: string) => {
      const view = get().view, owner = scope();
      if (!view || get().viewNodeId) return;
      try {
        await api.cancelDirectorRequest(requestId, view.characterId);
        if (owns(owner)) await get().loadDirector();
      } catch (error) {
        if (owns(owner)) set({ directorError: toErr(error) });
        throw error;
      }
    },
  };
}
