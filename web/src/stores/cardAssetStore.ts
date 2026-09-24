// cardAssetStore：按卡 ID 缓存卡内嵌资源（charx 立绘/图标）的 name→URL 映射。
//
// 资源字节留在服务端，这里只保存轻量映射，且**只存内存**：
// 进 localStorage 会撞上本地卡缓存的 64KB 压缩预算。内置预设没有资源，
// 拉取失败时缓存空表并停止重试。
import { useEffect, useMemo } from "react";
import { create } from "zustand";
import { api } from "../app/api";
import type { CardAssetView } from "../app/types";

interface CardAssetState {
  byCard: Record<string, Record<string, string>>;
  typedByCard: Record<string, CardAssetView[]>;
  themeByCard: Record<string, Record<string, string>>;
  sdDataByCard: Record<string, string[]>;
  attempted: Record<string, boolean>;
  ensure: (cardId?: string | null) => void;
}

const EMPTY = {};

export const useCardAssets = create<CardAssetState>((set, get) => ({
  byCard: {},
  typedByCard: {},
  themeByCard: {},
  sdDataByCard: {},
  attempted: {},
  ensure: (cardId) => {
    if (!cardId) return;
    const state = get();
    if (state.attempted[cardId]) return;
    set({ attempted: { ...state.attempted, [cardId]: true } });
    // 测试或精简客户端可能未提供 getCard：直接视为无资源。
    if (typeof api.getCard !== "function") {
      set((s) => ({
        byCard: { ...s.byCard, [cardId]: EMPTY },
        typedByCard: { ...s.typedByCard, [cardId]: [] },
        themeByCard: { ...s.themeByCard, [cardId]: EMPTY },
        sdDataByCard: { ...s.sdDataByCard, [cardId]: [] },
      }));
      return;
    }
    api
      .getCard(cardId)
      .then((preview) => {
        const map: Record<string, string> = {};
        for (const a of preview.assets ?? []) {
          if (a.name && a.url) map[a.name] = a.url;
        }
        set((s) => ({
          byCard: { ...s.byCard, [cardId]: map },
          typedByCard: { ...s.typedByCard, [cardId]: preview.assets ?? [] },
          themeByCard: { ...s.themeByCard, [cardId]: preview.themeTokens ?? {} },
          sdDataByCard: { ...s.sdDataByCard, [cardId]: preview.sdData ?? [] },
        }));
      })
      .catch(() => {
        set((s) => ({
          byCard: { ...s.byCard, [cardId]: EMPTY },
          typedByCard: { ...s.typedByCard, [cardId]: [] },
          themeByCard: { ...s.themeByCard, [cardId]: EMPTY },
          sdDataByCard: { ...s.sdDataByCard, [cardId]: [] },
        }));
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

/** 归一化主题键 → CSS 变量名。 */
const CARD_THEME_VARS: Record<string, string> = {
  accent: "--card-accent",
  bg: "--card-bg",
  ink: "--card-ink",
  font: "--card-font",
};

/** useCardTheme：订阅某张卡的主题 token（accent/bg/ink/font）。 */
export function useCardTheme(cardId?: string | null): Record<string, string> | undefined {
  const ensure = useCardAssets((s) => s.ensure);
  const theme = useCardAssets((s) => (cardId ? s.themeByCard[cardId] : undefined));
  useEffect(() => {
    ensure(cardId);
  }, [cardId, ensure]);
  return theme;
}

/**
 * useAppliedCardTheme：把当前卡的主题 token 写进根元素的 --card-* 变量。
 * 只有显式使用 var(--card-*) 的组件会受影响（CardHUD / SceneBar），
 * 因此不会波及整体界面。切换卡或卸载时清理。
 */
export function useAppliedCardTheme(cardId?: string | null): void {
  const theme = useCardTheme(cardId);
  useEffect(() => {
    const root = document.documentElement;
    const applied: string[] = [];
    if (theme) {
      for (const [key, cssVar] of Object.entries(CARD_THEME_VARS)) {
        const value = theme[key];
        if (value) {
          root.style.setProperty(cssVar, value);
          applied.push(cssVar);
        }
      }
    }
    return () => {
      for (const cssVar of applied) root.style.removeProperty(cssVar);
    };
  }, [theme]);
}

/** useCardSdData：订阅某张卡的生图提示词模板（sdData）。 */
export function useCardSdData(cardId?: string | null): string[] {
  const ensure = useCardAssets((s) => s.ensure);
  const data = useCardAssets((s) => (cardId ? s.sdDataByCard[cardId] : undefined));
  useEffect(() => {
    ensure(cardId);
  }, [cardId, ensure]);
  return data ?? EMPTY_ARRAY;
}

const EMPTY_ARRAY: string[] = [];

/** 立绘神情关键词 → 归一化表情键（与 RightRail 的 tachieExpression 一致）。 */
const EXPRESSION_KEYWORDS: Record<string, string[]> = {
  alert: ["alert", "angry", "mad", "guard", "shock", "surprise", "fear", "scared", "worried"],
  calm: ["calm", "neutral", "normal", "default", "quiet", "serious"],
  smile: ["smile", "happy", "joy", "glad", "relieved", "laugh", "cheer"],
  whisper: ["whisper", "blush", "shy", "soft", "moved", "sad", "cry", "sorrow", "gentle"],
};

/**
 * 从卡资源里挑出情绪立绘（type=emotion），按资源名关键词归一到四个表情键。
 * 卡片没有情绪资源时返回空表，调用方回退到默认头像。
 */
export function emotionMapFromAssets(assets: CardAssetView[]): Record<string, string> {
  const out: Record<string, string> = {};
  for (const a of assets) {
    if (a.type !== "emotion" || !a.url) continue;
    const name = a.name.toLowerCase();
    for (const [expr, keywords] of Object.entries(EXPRESSION_KEYWORDS)) {
      if (!out[expr] && keywords.some((k) => name.includes(k))) out[expr] = a.url;
    }
  }
  return out;
}

/** useCardEmotionMap：订阅某张卡的情绪立绘映射（表情键 → 资源 URL）。 */
export function useCardEmotionMap(cardId?: string | null): Record<string, string> {
  const ensure = useCardAssets((s) => s.ensure);
  const assets = useCardAssets((s) => (cardId ? s.typedByCard[cardId] : undefined));
  useEffect(() => {
    ensure(cardId);
  }, [cardId, ensure]);
  return useMemo(() => (assets ? emotionMapFromAssets(assets) : EMPTY), [assets]);
}
