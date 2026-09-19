// storyHelpers: 状态计算、SWR缓存、节点解析与并发归属辅助函数
import type { PlotNode, SessionView, TurnContent, WorldState } from "../app/types";
import { CHARACTER_PRESETS, getPresetCardJson } from "../lib/characterPresets";
import { subscribeSessionEvents } from "../app/api";
import { useUi } from "./uiStore";
import { emptyDirectorState } from "./slices/directorSlice";
import type { OptionView, StoryMessage, StoryState, StoreGet, StoreSet, TurnPhase } from "./storyTypes";

// 运行期 Store 访问器绑定（避免循环引用）
let storeGetter: StoreGet = () => ({} as StoryState);
let storeSetter: StoreSet = () => {};

export function bindStoryStore(getter: StoreGet, setter: StoreSet) {
  storeGetter = getter;
  storeSetter = setter;
}

export function getStoryState(): StoryState {
  return storeGetter();
}

export function setStoryState(partial: Partial<StoryState> | ((state: StoryState) => Partial<StoryState>)) {
  storeSetter(partial);
}

// A committed turn remains read-only until its new head and options are loaded.
export function isTurnBusy(phase: TurnPhase): boolean {
  return phase === "generating" || phase === "truncated" || phase === "awaiting_confirm" || phase === "committed";
}

// ---- 只读与忙碌判定：全前端只此一处 ----
// 此前每个组件各写一遍（11 处），加一个新状态（例如导演命令中）就得逐处改，
// 漏一处就会出现"这个按钮能点、那个不能点"的不一致。组件请消费这些选择器。
type BusyState = Pick<StoryState, "phase" | "viewNodeId" | "directorCommanding">;

// 正在看历史节点：输入与大多数写操作都不可用（可从此处分叉）。
export function historyReadOnly(state: BusyState): boolean {
  return !!state.viewNodeId;
}

// 故事层面的"不能动"：回合在途（生成/待续写/待确认/已提交待刷新）或导演命令在途。
export function storyBusy(state: BusyState): boolean {
  return isTurnBusy(state.phase) || state.directorCommanding;
}

// 输入台：历史视图或故事忙碌时不可输入。
export function composerBusy(state: BusyState): boolean {
  return historyReadOnly(state) || storyBusy(state);
}

// 导演操作：与输入台同源（历史视图只读、回合在途、命令在途）。
export function directorBusy(state: BusyState): boolean {
  return historyReadOnly(state) || storyBusy(state);
}

export function toErr(e: unknown): string {
  if (e instanceof Error) return e.message;
  return String(e);
}

export function presetIdentity(key: string): string {
  return (JSON.parse(getPresetCardJson(key)) as { cardId: string }).cardId;
}

// 解析会话或视图归属的角色键 (精确匹配 characters 字典与标题)。
// 返回 null 表示自定义卡会话（无匹配预设）：显示层落 custom 通用皮肤，
// 会话路由按 null 分组——绝不误标成某个内置角色（如 elena）。
export function resolveCharKeyFromSession(
  sessionOrView: { characterId?: string; title?: string; state?: WorldState | null } | null
): string | null {
  if (!sessionOrView) return null;
  if (sessionOrView.characterId) {
    return Object.keys(CHARACTER_PRESETS).find(key => key !== "custom" && [presetIdentity(key), `card_${key}`].includes(sessionOrView.characterId!)) ?? null;
  }

  // 1. 优先从状态中的参与角色表解析 (例如 npc_elena, npc_saori)
  if (sessionOrView.state?.characters) {
    for (const [charId, info] of Object.entries(sessionOrView.state.characters)) {
      for (const key of Object.keys(CHARACTER_PRESETS)) {
        if (key === "custom") continue;
        if (charId === `npc_${key}` || charId === key) {
          return key;
        }
      }
      for (const [key, preset] of Object.entries(CHARACTER_PRESETS)) {
        if (key === "custom") continue;
        if (
          (info.name && (info.name === preset.name || info.name === preset.shortName)) ||
          (info.description && info.description.includes(preset.shortName))
        ) {
          return key;
        }
      }
    }
  }

  // 2. 从会话标题模糊匹配
  const title = (sessionOrView.title || "").toLowerCase();
  for (const [key, preset] of Object.entries(CHARACTER_PRESETS)) {
    if (key === "custom") continue;
    if (
      title.includes(key.toLowerCase()) ||
      title.includes(preset.shortName.toLowerCase()) ||
      title.includes(preset.name.toLowerCase())
    ) {
      return key;
    }
  }

  // 3. 自定义卡：明确返回 null，由消费端决定显示（custom 皮肤）或路由（按 null 分组）。
  return null;
}

// 前端会话内存缓存 (SWR 机制：秒开历史会话，静默异步校验)
class SessionViewCache extends Map<string, SessionView> {
  override set(key: string, value: SessionView): this {
    this.delete(key);
    super.set(key, value);
    while (this.size > 12) this.delete(this.keys().next().value!);
    return this;
  }
}
export const sessionViewCache = new SessionViewCache();

export function clearSessionCache() {
  sessionViewCache.clear();
}

// 并发序号与导航 Epoch
export let sendSeq = 0;
export function incrementSendSeq(): number { return ++sendSeq; }

export let navigationEpoch = 0;
export function getNavigationEpoch(): number { return navigationEpoch; }

export let reloadSeq = 0;
export function getReloadSeq(): number { return reloadSeq; }
export function incrementReloadSeq(): number { return ++reloadSeq; }

export let memorySeq = 0;
export function getMemorySeq(): number { return memorySeq; }
export function incrementMemorySeq(): number { return ++memorySeq; }

export let sessionListSeq = 0;
export function getSessionListSeq(): number { return sessionListSeq; }
export function incrementSessionListSeq(): number { return ++sessionListSeq; }

export let pendingSessionSetup: { epoch: number; payload: string; key: string } | null = null;
export function getPendingSessionSetup() { return pendingSessionSetup; }
export function setPendingSessionSetup(val: typeof pendingSessionSetup) { pendingSessionSetup = val; }

export let pendingMemoryPatch: { payload: string; key: string } | null = null;
export function getPendingMemoryPatch() { return pendingMemoryPatch; }
export function setPendingMemoryPatch(val: typeof pendingMemoryPatch) { pendingMemoryPatch = val; }

export let sessionStop: (() => void) | null = null;
export function setSessionStop(stop: (() => void) | null) { sessionStop = stop; }

export function scope() {
  const st = getStoryState();
  return { epoch: navigationEpoch, sessionId: st.view?.sessionId, branchId: st.activeBranchId, nodeId: st.viewNodeId };
}

export function owns(s: ReturnType<typeof scope>): boolean {
  const now = scope();
  return s.epoch === now.epoch && s.sessionId === now.sessionId && s.branchId === now.branchId && s.nodeId === now.nodeId;
}

export function beginNavigation(onDetachStream?: () => void): number {
  useUi.getState().hideInvBubble();
  onDetachStream?.();
  sessionStop?.();
  sessionStop = null;
  reloadSeq++;
  memorySeq++;
  pendingSessionSetup = null;
  pendingMemoryPatch = null;
  return ++navigationEpoch;
}

export function emptyStory(): Partial<StoryState> {
  return {
    ...emptyDirectorState,
    view: null, messages: [], draftBlocks: [], draftOptions: [], phase: "idle", thinkingText: "", turnThinking: {}, lastTurnId: null,
    error: null, hud: null, memories: [], memoryUsage: null, memoryError: null, memoryLoading: false, activeCharacterId: null, pendingInput: null, pendingAction: null, lastInput: null,
    memoryPaging: emptyMemoryPaging(), activeBranchId: null, viewNodeId: null, packError: null, hasMoreHistory: false, oldestTurnId: null, loadingOlder: false, sessionLoading: false, pendingSessionId: null,
  };
}

export const emptyMemoryPaging = () => ({ search: "", kind: "all", index: 0, cursors: [""], total: 0, counts: {} });

export function createInitialStoryState(): Partial<StoryState> {
  return {
    ...emptyDirectorState,
    sessions: [],
    view: null,
    messages: [],
    draftBlocks: [],
    draftOptions: [],
    phase: "idle",
    thinkingText: "",
    turnThinking: {},
    lastTurnId: null,
    error: null,
    hud: null,
    memories: [],
    memoryUsage: null,
    memoryError: null,
    memoryLoading: false,
    memoryPaging: emptyMemoryPaging(),
    activeCharacterId: null,
    pendingInput: null,
    pendingAction: null,
    lastInput: null,
    activeBranchId: null,
    viewNodeId: null,
    packError: null,
    hasMoreHistory: false,
    oldestTurnId: null,
    loadingOlder: false,
    sessionLoading: false,
    pendingSessionId: null,
  };
}

export function watchSession(sessionId: string, epoch: number, onSynchronize?: () => void) {
  sessionStop?.();
  let scheduled: ReturnType<typeof setTimeout> | undefined;
  let stop: (() => void) | null = null;
  // 退避用尽（onFinalError）意味着"这个订阅死了"，而不是"这个故事从此不变"。
  // 挂起重试标记，等读者回到窗口时重建订阅并补一次视图刷新——旧实现把
  // onFinalError 当无事发生，于是后台的 head 变化要等到用户手动操作才可见。
  let stalled = false;
  const sameStory = () => navigationEpoch === epoch && getStoryState().view?.sessionId === sessionId;
  const subscribe = () => {
    stop = subscribeSessionEvents(sessionId, {
      onEvent: ev => {
        const st = getStoryState();
        const data = ev.data as { branchId?: string };
        if (navigationEpoch !== epoch || st.view?.sessionId !== sessionId || data.branchId !== st.activeBranchId) return;
        if (ev.event !== "session.updated") return;
        if (isTurnBusy(st.phase)) {
          onSynchronize?.();
          return;
        }
        clearTimeout(scheduled);
        scheduled = setTimeout(() => {
          if (navigationEpoch === epoch && getStoryState().view?.sessionId === sessionId) {
            void getStoryState().reloadView().catch(() => undefined);
          }
        }, 100);
      },
      onFinalError: () => { stalled = true; stop = null; },
    });
  };
  subscribe();

  const revive = () => {
    if (!stalled || !sameStory()) return;
    stalled = false;
    subscribe();
    void getStoryState().reloadView().catch(() => undefined);
  };
  const onFocus = () => revive();
  const onVisible = () => { if (!document.hidden) revive(); };
  if (typeof window !== "undefined") window.addEventListener("focus", onFocus);
  if (typeof document !== "undefined") document.addEventListener("visibilitychange", onVisible);

  sessionStop = () => {
    clearTimeout(scheduled);
    if (typeof window !== "undefined") window.removeEventListener("focus", onFocus);
    if (typeof document !== "undefined") document.removeEventListener("visibilitychange", onVisible);
    stop?.();
  };
}

// ---- 节点解析器 ----
export function messagesFromNodes(view: SessionView, turnThinkingMap?: Record<string, string>): StoryMessage[] {
  const out: StoryMessage[] = [];
  const thinkingMap = turnThinkingMap || getStoryState().turnThinking || {};
  const nodes = [...(view.nodes ?? [])].sort((a, b) => (a.depth ?? 0) - (b.depth ?? 0) || (a.turnNumber ?? 0) - (b.turnNumber ?? 0));
  for (const n of nodes) {
    if (n.kind === "root") {
      const root = parseRoot(n);
      out.push({ id: n.nodeId + ":opening", role: "opening", blocks: [{ kind: "narration", text: root }], options: [] });
      continue;
    }
    if (n.kind !== "turn") continue;
    const tc = parseTurnContent(n);
    if (!tc) continue;
    const opts: OptionView[] = (tc.options ?? []).map((o) => ({
      optionId: o.optionId,
      intent: o.intent as OptionView["intent"],
      text: o.text,
    }));
    const turnId = tc.provenance?.turnId;
    const thinking = thinkingMap[n.nodeId] || (turnId ? thinkingMap[turnId] : undefined);
    out.push({
      id: n.nodeId,
      parentId: n.parentId || undefined,
      role: "assistant",
      blocks: tc.blocks ?? [],
      options: opts,
      inputText: tc.inputText,
      inputNote: tc.inputNote,
      checks: tc.checks,
      changes: tc.changes,
      injectedMemories: tc.injectedMemories,
      suppressedOptions: tc.suppressedOptions,
      turnNumber: n.turnNumber,
      thinking,
    });
  }
  if (out.length === 0) out.push(openingMessage(view));
  return out;
}

export function parseRoot(n: PlotNode): string {
  try {
    const j = JSON.parse(n.contentJson) as { openingText?: string };
    return j.openingText || "（故事开始）";
  } catch {
    return "（故事开始）";
  }
}

export function parseTurnContent(n: PlotNode): TurnContent | null {
  try {
    return JSON.parse(n.contentJson) as TurnContent;
  } catch {
    return null;
  }
}

export function openingMessage(view: SessionView): StoryMessage {
  return {
    id: view.sessionId + ":opening",
    role: "opening",
    blocks: [{ kind: "narration", text: parseRoot(view.headNode) }],
    options: [],
  };
}
