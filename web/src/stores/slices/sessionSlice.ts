// sessionSlice: 会话生命周期、角色切换、创建开局与剧情包导入导出
import { api } from "../../app/api";
import { CHARACTER_PRESETS, getPresetCardJson } from "../../lib/characterPresets";
import { downloadBlob, packFileName } from "../../lib/packFile";
import { useUi } from "../uiStore";
import type { CreateSessionOpts, StoreGet, StoreSet } from "../storyTypes";
import {
  beginNavigation,
  emptyStory,
  getPendingSessionSetup,
  getSessionListSeq,
  incrementSessionListSeq,
  messagesFromNodes,
  navigationEpoch,
  owns,
  resolveCharKeyFromSession,
  scope,
  clearSessionCache,
  sessionViewCache,
  setPendingSessionSetup,
  toErr,
  watchSession,
} from "../storyHelpers";
import { activeTurnSub, detachTurnStream, recoverActiveTurn } from "./turnSlice";
import { rememberSession } from "../sessionMemory";
import { DEFAULT_PLAYER_NAME } from "../../lib/characterMacros";

export const createSessionSlice = (set: StoreSet, get: StoreGet) => ({
  loadSessions: async () => {
    const ticket = incrementSessionListSeq();
    const { sessions } = await api.listSessions();
    if (ticket === getSessionListSeq()) set({ sessions: Array.isArray(sessions) ? sessions : [] });
  },

  openSession: async (id: string) => {
    const epoch = beginNavigation(() => detachTurnStream());
    const cached = sessionViewCache.get(id);
    if (cached) {
      rememberSession(id);
      useUi.getState().setActiveCharKey(resolveCharKeyFromSession(cached));
      const turnThinking = get().turnThinking || {};
      set({
        ...emptyStory(),
        view: cached,
        messages: messagesFromNodes(cached, turnThinking),
        hud: cached.state,
        activeCharacterId: cached.characterId,
        activeBranchId: cached.branch.branchId,
        hasMoreHistory: !!cached.hasMore,
        oldestTurnId: cached.oldestTurnId ?? null,
        sessionLoading: false,
        pendingSessionId: null,
      });
      // 缓存命中也立刻恢复在途回合：校验请求可能很慢或失败，而用户已经在看这个故事。
      watchSession(id, epoch, () => activeTurnSub?.synchronize());
      recoverActiveTurn(cached);
    } else {
      const targetSession = get().sessions.find((s) => s.sessionId === id);
      if (targetSession) {
        useUi.getState().setActiveCharKey(resolveCharKeyFromSession(targetSession));
      }
      set({
        ...emptyStory(),
        sessionLoading: true,
        pendingSessionId: id,
      });
    }
    try {
      const view = await api.getSession(id);
      if (epoch !== navigationEpoch) return;
      sessionViewCache.set(id, view);
      rememberSession(id);
      useUi.getState().setActiveCharKey(resolveCharKeyFromSession(view));
      const turnThinking = get().turnThinking || {};
      set({
        view,
        messages: messagesFromNodes(view, turnThinking),
        hud: view.state,
        activeCharacterId: view.characterId,
        activeBranchId: view.branch.branchId,
        hasMoreHistory: !!view.hasMore,
        oldestTurnId: view.oldestTurnId ?? null,
        sessionLoading: false,
        pendingSessionId: null,
      });
      watchSession(id, epoch, () => activeTurnSub?.synchronize());
      recoverActiveTurn(view);
      await get().loadMemories();
    } catch (e) {
      // 已恢复的在途回合不因一次校验失败被打回 failed：流仍在推，等它自己收敛。
      const running = ["generating", "truncated", "awaiting_confirm"].includes(get().phase) && !!get().lastTurnId;
      if (epoch === navigationEpoch) {
        set({ error: toErr(e), ...(running ? {} : { phase: "failed" as const }), sessionLoading: false, pendingSessionId: null });
      }
    }
  },

  // 删除故事：不可撤销，界面必须先做二次确认。
  //
  // 删掉当前正在读的故事时要特别小心：先脱离在途回合流、清掉视图缓存并回到空白态，
  // 再刷新列表；若还有别的故事就自动切过去，否则停在空白起始态——绝不留下一个
  // "指向已删除会话"的视图。删除他人故事只需让出一条缓存条目。
  deleteSession: async (id: string) => {
    try {
      await api.deleteSession(id);
    } catch (error) {
      // 删除失败也要让列表与服务端对齐：404 说明它本来就不存在了（例如在别处删过），
      // 这时把幽灵行留着比删掉更糟。刷新失败不掩盖原始错误。
      try {
        await get().loadSessions();
      } catch {
        // 列表刷新失败无关紧要，原始错误才是要报给用户的
      }
      throw error;
    }
    const activeId = get().view?.sessionId ?? get().pendingSessionId ?? null;
    const deletingActive = activeId === id;
    if (deletingActive) {
      beginNavigation(() => detachTurnStream());
      set(emptyStory());
      rememberSession(null);
    }
    clearSessionCache(); // 被删会话的缓存视图不能再用；整体清空成本极低
    try {
      await get().loadSessions();
    } catch {
      useUi.getState().notifyQuiet("故事已删除，会话列表暂未刷新，请稍后重试。");
      return;
    }
    if (!deletingActive) {
      useUi.getState().notifyQuiet("故事已删除");
      return;
    }
    const next = get().sessions[0];
    if (next) {
      await get().openSession(next.sessionId);
      useUi.getState().notifyQuiet("故事已删除，已切换到其它故事");
    } else {
      useUi.getState().notifyQuiet("故事已删除");
    }
  },

  selectCharacter: async (charKey: string) => {
    const targetKey = CHARACTER_PRESETS[charKey] ? charKey : "custom";
    useUi.getState().switchActiveCharacter(targetKey);
    const matched = targetKey === "custom" ? undefined : get().sessions.find(s => resolveCharKeyFromSession(s) === targetKey);
    if (matched) {
      await get().openSession(matched.sessionId);
    } else {
      beginNavigation(() => detachTurnStream());
      set(emptyStory());
    }
  },

  startNewSessionForCurrentChar: async () => {
    beginNavigation(() => detachTurnStream());
    set(emptyStory());
    useUi.getState().notifyQuiet("已就绪，可以开启新的故事");
  },

  createSession: async (opts: CreateSessionOpts) => {
    const owner = scope();
    const payload = JSON.stringify(opts);
    const pendingSetup = getPendingSessionSetup();
    if (!pendingSetup || pendingSetup.epoch !== owner.epoch || pendingSetup.payload !== payload) {
      setPendingSessionSetup({ epoch: owner.epoch, payload, key: opts.idempotencyKey ?? crypto.randomUUID() });
    }
    const requestKey = getPendingSessionSetup()!.key;
    set({ error: null, phase: "generating" });
    try {
      const created = await api.createSession({ ...opts, idempotencyKey: requestKey });
      if (getPendingSessionSetup()?.key === requestKey) setPendingSessionSetup(null);
      if (owns(owner)) await get().openSession(created.sessionId);
      try { await get().loadSessions(); }
      catch { useUi.getState().notifyQuiet("故事已创建，会话列表暂未刷新，请稍后重试刷新。"); }
      return created;
    } catch (e) {
      if (owns(owner)) set({ phase: "idle", error: toErr(e) });
      throw e;
    }
  },

  createSessionFromPreset: async (charKey: string) => {
    const preset = CHARACTER_PRESETS[charKey] || CHARACTER_PRESETS.custom;
    const cardJson = getPresetCardJson(charKey);
    return get().createSession({
      title: preset.name,
      characterJson: cardJson,
      playerName: DEFAULT_PLAYER_NAME,
      openingText: preset.prologue,
    });
  },

  exportSession: async () => {
    const view = get().view;
    if (!view) return;
    const owner = scope();
    set({ packError: null });
    try {
      const blob = await api.exportSessionPack(view.sessionId);
      downloadBlob(blob, packFileName(view.title));
    } catch (e) {
      if (owns(owner)) set({ packError: "导出失败：" + toErr(e) });
    }
  },

  importSession: async (file: File) => {
    const owner = scope();
    set({ packError: null });
    try {
      const buf = await file.arrayBuffer();
      const res = await api.importSessionPack(buf);
      clearSessionCache(); // 导入会重写 ID 与引用，缓存里的旧视图一律作废
      await get().loadSessions();
      if (owns(owner)) await get().openSession(res.sessionId);
    } catch (e) {
      if (owns(owner)) set({ packError: "导入失败：" + toErr(e) });
      throw e;
    }
  },

  clearPackError: () => set({ packError: null }),
});
