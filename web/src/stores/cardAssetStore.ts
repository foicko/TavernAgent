// cardAssetStore：按卡 ID 缓存卡内嵌资源（charx 立绘/图标）的 name→URL 映射。
//
// 资源字节留在服务端，这里只保存轻量映射，且**只存内存**：
// 进 localStorage 会撞上本地卡缓存的 64KB 压缩预算。内置预设没有资源，
// 拉取失败时缓存空表并停止重试。
import { useEffect } from "react";
import { create } from "zustand";
import { api } from "../app/api";

interface CardAssetState {
  byCard: Record<string, Record<string, string>>;
  attempted: Record<string, boolean>;
  ensure: (cardId?: string | null) => void;
}

export const useCardAssets = create<CardAssetState>((set, get) => ({
  byCard: {},
  attempted: {},
  ensure: (cardId) => {
    if (!cardId) return;
    const state = get();
    if (state.attempted[cardId]) return;
    set({ attempted: { ...state.attempted, [cardId]: true } });
    // 测试或精简客户端可能未提供 getCard：直接视为无资源。
    if (typeof api.getCard !== "function") {
      set((s) => ({ byCard: { ...s.byCard, [cardId]: {} } }));
      return;
    }
    api
      .getCard(cardId)
      .then((preview) => {
        const map: Record<string, string> = {};
        for (const a of preview.assets ?? []) {
          if (a.name && a.url) map[a.name] = a.url;
        }
        set((s) => ({ byCard: { ...s.byCard, [cardId]: map } }));
      })
      .catch(() => {
        set((s) => ({ byCard: { ...s.byCard, [cardId]: {} } }));
      });
  },
}));

/** useCardAssetMap：订阅某张卡的资源映射，并在挂载/切换卡时按需拉取。 */
export function useCardAssetMap(cardId?: string | null): Record<string, string> | undefined {
  const ensure = useCardAssets((s) => s.ensure);
  const map = useCardAssets((s) => (cardId ? s.byCard[cardId] : undefined));
  useEffect(() => {
    ensure(cardId);
  }, [cardId, ensure]);
  return map;
}
