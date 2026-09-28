// storyRuntime：Story 运行期的可变状态集中地。
//
// 此前这些状态以模块级 `let` 散落在 storyHelpers 与各 slice 里：既没有单一复位点
// （测试只能"间接重置"），也不便在 HMR/多实例下推理，还让 `export let` 的活绑定
// 被跨模块 import（稍有不慎就固化成快照值）。集中成一个对象后：
//   • 写入点显式（`storyRuntime.navigationEpoch`），一眼可见这不是 React state；
//   • `reset()` 是唯一的"切故事/清空"复位入口；
//   • 与 zustand store 同生命周期（模块加载即建）——store 本身也是单例。
import type { SessionView } from "../app/types";
import type { TurnInput } from "./storyTypes";

/** 归属快照：用于判断异步回调是否仍属于当前会话/分支/节点。 */
export interface StoryScope {
  epoch: number;
  sessionId?: string;
  branchId: string | null;
  nodeId: string | null;
}

export interface PendingSessionSetup {
  epoch: number;
  payload: string;
  key: string;
}

export interface PendingMemoryPatch {
  payload: string;
  key: string;
}

export interface PendingSubmission {
  owner: StoryScope;
  view: SessionView;
  key: string;
  input: TurnInput;
}

export interface DirectorWatch {
  requestId: string;
  stop: () => void;
  timer: ReturnType<typeof setInterval>;
}

export interface TurnSubscription {
  turnId: string;
  sessionId: string;
  stop: () => void;
  synchronize: (committed?: boolean) => void;
}

export interface StoryRuntime {
  sendSeq: number;
  navigationEpoch: number;
  reloadSeq: number;
  memorySeq: number;
  sessionListSeq: number;
  branchOpSeq: number;
  pendingSessionSetup: PendingSessionSetup | null;
  pendingMemoryPatch: PendingMemoryPatch | null;
  pendingSubmission: PendingSubmission | null;
  directorWatch: DirectorWatch | null;
  sessionStop: (() => void) | null;
  /** 新一轮流式回合的游标与订阅句柄。 */
  turnCursor: { turnId: string; id: string } | null;
  activeTurnSub: TurnSubscription | null;
  thinkingStartedAt: Map<string, number>;
  /** 复位"切故事/清空"应清掉的在途状态（不动各序号，调用方按需自增）。 */
  reset(): void;
}

export function createStoryRuntime(): StoryRuntime {
  const runtime: StoryRuntime = {
    sendSeq: 0,
    navigationEpoch: 0,
    reloadSeq: 0,
    memorySeq: 0,
    sessionListSeq: 0,
    branchOpSeq: 0,
    pendingSessionSetup: null,
    pendingMemoryPatch: null,
    pendingSubmission: null,
    directorWatch: null,
    sessionStop: null,
    turnCursor: null,
    activeTurnSub: null,
    thinkingStartedAt: new Map(),
    reset() {
      runtime.sessionStop?.();
      runtime.sessionStop = null;
      // 注意：**不清** pendingSessionSetup——幂等键必须跨导航存活，否则"响应丢失 →
      // 切走 → 回来重试"会用新键重复建故事（G38）。成功后再由 createSession 清掉。
      runtime.pendingMemoryPatch = null;
      runtime.pendingSubmission = null;
      runtime.thinkingStartedAt.clear();
    },
  };
  return runtime;
}

/** 单实例：与 store 同生命周期。 */
export const storyRuntime = createStoryRuntime();
