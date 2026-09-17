// memorySlice: 记忆读写、修订纠偏与整理
import { api } from "../../app/api";
import type { MemoryLoadOptions, StoreGet, StoreSet } from "../storyTypes";
import {
  getMemorySeq,
  emptyMemoryPaging,
  getPendingMemoryPatch,
  incrementMemorySeq,
  isTurnBusy,
  owns,
  scope,
  setPendingMemoryPatch,
  toErr,
} from "../storyHelpers";

export const createMemorySlice = (set: StoreSet, get: StoreGet) => ({
  loadMemories: async (options?: MemoryLoadOptions) => {
    const owner = scope();
    const ticket = incrementMemorySeq();
    const view = get().view;
    if (!view) { set({ memories: [], memoryUsage: null, memoryError: null, memoryLoading: false, memoryPaging: emptyMemoryPaging() }); return; }
    const previous = get().memoryPaging;
    const search = options?.search ?? previous.search;
    const kind = options?.kind ?? previous.kind;
    const sameQuery = search === previous.search && kind === previous.kind;
    const moving = sameQuery && (options?.page === "next" || options?.page === "previous");
    const anchored = sameQuery && options === undefined && previous.index > 0;
    if (moving && options?.page === "next" && !previous.nextCursor) return;
    const index = moving ? Math.max(0, previous.index + (options?.page === "next" ? 1 : -1)) : anchored ? previous.index : 0;
    const cursors = moving || anchored ? previous.cursors.slice(0, index + 1) : [""];
    if (moving && options?.page === "next") cursors[index] = previous.nextCursor!;
    const nodeId = moving || anchored ? previous.nodeId : (get().viewNodeId ?? undefined);
    set({ memoryLoading: true, memoryError: null });
    try {
      const { memories, usage, ...page } = await api.listMemories(view.sessionId, view.branch.branchId, nodeId, { search, kind, cursor: cursors[index], limit: 40 });
      if (owns(owner) && ticket === getMemorySeq()) set({ memories, memoryUsage: usage ?? null, memoryError: null,
        memoryPaging: { search, kind, index, cursors, nodeId: page.nodeId ?? nodeId, nextCursor: page.nextCursor, total: page.total ?? memories.length, counts: page.counts ?? {} } });
    } catch (error) {
      if (owns(owner) && ticket === getMemorySeq()) set({ memoryError: toErr(error) });
    } finally {
      if (owns(owner) && ticket === getMemorySeq()) set({ memoryLoading: false });
    }
  },

  reviseMemory: async (memoryId: string, patch: { content?: string; pinned?: boolean; hidden?: boolean }) => {
    const owner = scope();
    const view = get().view;
    if (!view || isTurnBusy(get().phase) || get().viewNodeId || get().memoryLoading || get().memoryError) return;
    const payload = JSON.stringify({ sessionId: view.sessionId, branchId: view.branch.branchId, head: view.branch.headNodeId, memoryId, patch });
    const patchState = getPendingMemoryPatch();
    if (!patchState || patchState.payload !== payload) setPendingMemoryPatch({ payload, key: crypto.randomUUID() });
    const requestKey = getPendingMemoryPatch()!.key;
    await api.patchMemory(view.sessionId, view.branch.branchId, memoryId, {
      ...patch, expectedCharacterId: view.characterId, expectedHeadId: view.branch.headNodeId, expectedVersion: view.branch.version,
      idempotencyKey: requestKey,
    });
    if (getPendingMemoryPatch()?.key === requestKey) setPendingMemoryPatch(null);
    if (owns(owner)) {
      set({ memoryPaging: { ...emptyMemoryPaging(), search: get().memoryPaging.search, kind: get().memoryPaging.kind } });
      await get().reloadView();
    }
  },

  organizeMemories: async () => {
    const owner = scope();
    const view = get().view;
    if (!view || isTurnBusy(get().phase) || get().viewNodeId || get().memoryLoading || get().memoryError) return;
    try {
      await api.organizeMemories(view.sessionId, view.branch.branchId, view.characterId);
      if (owns(owner)) await get().reloadView();
    } catch (e) { if (owns(owner)) set({ error: toErr(e) }); }
  },
});
