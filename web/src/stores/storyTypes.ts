// storyTypes: 统一状态与契约类型定义
import type {
  CheckResult,
  MemoryRef,
  MemoryView,
  MemoryUsage,
  SessionSummary,
  SessionView,
  StateChange,
  TextBlock,
  WorldState,
} from "../app/types";
import type { api } from "../app/api";
import type { DirectorSliceState } from "./slices/directorSlice";

export type TurnPhase =
  | "idle"
  | "generating"
  | "truncated"
  | "awaiting_confirm"
  | "failed"
  | "committed";

export interface StoryMessage {
  id: string;
  parentId?: string;
  role: "opening" | "user" | "assistant";
  blocks: TextBlock[];
  options: OptionView[];
  draft?: boolean;
  inputText?: string;
  /** 玩家注记（非叙事）：回看时要能分清“当时要求怎么演”。 */
  inputNote?: string;
  checks?: CheckResult[];
  /** 已生效的状态变化（服务端从领域事件推导）。 */
  changes?: StateChange[];
  /** 本轮实际注入上下文的记忆快照。 */
  injectedMemories?: MemoryRef[];
  /** 模型给出但按"只在关键节点"规则未呈现的选项条数。 */
  suppressedOptions?: number;
  turnNumber?: number;
  thinking?: string;
}

export interface OptionView {
  optionId: string;
  intent: "aggressive" | "clever" | "emotional" | "chaotic";
  text: string;
}

export interface CreateSessionOpts {
  idempotencyKey?: string;
  title?: string;
  // cardId 优先：服务端直接从卡库取卡内容，请求体不必携带兆级 characterJson。
  cardId?: string;
  characterJson?: string;
  playerName: string;
  playerRole?: string;
  openingVariantId?: string;
  openingText?: string;
}

export interface TurnInput {
  kind: string;
  text: string;
  /** 玩家注记（非叙事）：对本次演绎的要求，不会出现在正文里。 */
  note?: string;
  /** 本回合的选项呈现模式（auto/always/never）。 */
  options?: string;
  actionRef?: string;
  optionRef?: { nodeId: string; optionId: string };
}

export interface MemoryPaging {
  search: string;
  kind: string;
  index: number;
  cursors: string[];
  nextCursor?: string;
  nodeId?: string;
  total: number;
  counts: Record<string, number>;
}

export interface MemoryLoadOptions {
  search?: string;
  kind?: string;
  page?: "next" | "previous" | "first";
}

export interface StoryState extends DirectorSliceState {
  // 会话与元数据
  sessions: SessionSummary[];
  view: SessionView | null;
  messages: StoryMessage[];
  draftBlocks: TextBlock[];
  draftOptions: OptionView[];
  phase: TurnPhase;
  thinkingText: string;
  turnThinking: Record<string, string>;
  lastTurnId: string | null;
  error: string | null;
  hud: WorldState | null;
  memories: MemoryView[];
  memoryUsage: MemoryUsage | null;
  memoryError: string | null;
  memoryLoading: boolean;
  memoryPaging: MemoryPaging;
  activeCharacterId: string | null;
  pendingInput: string | null;
  pendingAction: string | null;
  lastInput: TurnInput | null;
  activeBranchId: string | null;
  viewNodeId: string | null;
  packError: string | null;
  hasMoreHistory: boolean;
  oldestTurnId: string | null;
  loadingOlder: boolean;
  sessionLoading: boolean;
  pendingSessionId: string | null;

  // Session Actions
  loadSessions: () => Promise<void>;
  openSession: (id: string) => Promise<void>;
  // 删除故事（不可撤销）。删的若是当前正在读的故事，会自动切到列表里的下一条，
  // 没有别的故事则回到空白起始态。
  deleteSession: (id: string) => Promise<void>;
  selectCharacter: (charKey: string) => Promise<void>;
  startNewSessionForCurrentChar: () => Promise<void>;
  createSession: (opts: CreateSessionOpts) => ReturnType<typeof api.createSession>;
  createSessionFromPreset: (charKey: string) => ReturnType<typeof api.createSession>;
  exportSession: () => Promise<void>;
  importSession: (file: File) => Promise<void>;
  clearPackError: () => void;

  // Turn Actions
  send: (text: string, actionRef?: string, note?: string) => Promise<void>;
  chooseOption: (o: OptionView) => Promise<void>;
  cancel: () => Promise<void>;
  continueTurn: () => Promise<void>;
  retry: () => Promise<void>;
  clearPending: () => void;
  prepareAction: (text: string, actionRef: string) => void;
  refreshView: () => Promise<void>;
  reloadView: () => Promise<void>;

  // Memory Actions
  loadMemories: (options?: MemoryLoadOptions) => Promise<void>;
  reviseMemory: (memoryId: string, patch: { content?: string; pinned?: boolean; hidden?: boolean }) => Promise<void>;
  organizeMemories: () => Promise<void>;

  // Branch Actions
  switchBranch: (branchId: string) => Promise<void>;
  viewAt: (nodeId: string) => Promise<void>;
  backToHead: () => Promise<void>;
  regenerate: (nodeId: string, recheck?: boolean, guidance?: string) => Promise<void>;
  editPlayerInput: (nodeId: string, text: string) => Promise<void>;
  editAssistantText: (nodeId: string, blocks: TextBlock[]) => Promise<void>;
  forkFrom: (nodeId: string, name?: string) => Promise<void>;
  loadOlder: () => Promise<void>;
}

import type { StoreApi } from "zustand";

export type StoreSet = StoreApi<StoryState>["setState"];

export type StoreGet = StoreApi<StoryState>["getState"];
