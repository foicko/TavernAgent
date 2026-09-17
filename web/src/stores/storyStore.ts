// storyStore：区分服务端快照 / 当前视图 / 未提交草稿（技术契约 §11.1）。
// 采用模块化 Zustand 切片架构 (SessionSlice / TurnSlice / BranchSlice / MemorySlice)
import { create } from "zustand";
import type { StoryState } from "./storyTypes";
import { bindStoryStore, createInitialStoryState } from "./storyHelpers";
import { createSessionSlice } from "./slices/sessionSlice";
import { createTurnSlice } from "./slices/turnSlice";
import { createBranchSlice } from "./slices/branchSlice";
import { createMemorySlice } from "./slices/memorySlice";
import { createDirectorSlice } from "./slices/directorSlice";

export const useStory = create<StoryState>((set, get) => {
  bindStoryStore(get, set);
  return {
    ...(createInitialStoryState() as StoryState),
    ...createSessionSlice(set, get),
    ...createTurnSlice(set, get),
    ...createBranchSlice(set, get),
    ...createMemorySlice(set, get),
    ...createDirectorSlice(set, get),
  };
});

// 完整重导出公共类型与辅助函数，保持 100% 向后兼容
export * from "./storyTypes";
export * from "./storyHelpers";
export { routeEvent, activeTurnSub, detachTurnStream } from "./slices/turnSlice";
