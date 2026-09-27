// uiStore: 管理双模视口、侧栏折叠、主题、立绘差分、弹窗、心境浮动与微交互
import { create } from "zustand";
import { CHARACTER_PRESETS, type CharacterPreset, type InventoryItemPreset } from "../lib/characterPresets";
import { DEFAULT_PRESET_KEY } from "../lib/characterPresets";

export type ViewportMode = "studio" | "reading";
export type LeftSubtab = "tree" | "characters" | "lore" | "sessions";
export type TachieExpression = "alert" | "calm" | "smile" | "whisper";
export type ThemeMode = "light" | "dark" | "violet";

const THEME_STORAGE_KEY = "tavernagent.theme";

// 存储被禁用（隐私模式 / 第三方 iframe）时连访问 window.localStorage 都会抛 SecurityError；
// 主题读取发生在模块加载期，抛出会让整个应用白屏，所以读写都兜底为"不记忆"。
function readSavedTheme(): string | null {
  try {
    return window.localStorage.getItem(THEME_STORAGE_KEY);
  } catch {
    return null;
  }
}

function writeSavedTheme(theme: ThemeMode): void {
  try {
    window.localStorage.setItem(THEME_STORAGE_KEY, theme);
  } catch {
    // localStorage 不可用时不记忆，也不报错
  }
}

/** 首帧主题：用户显式选择优先，否则跟随系统 prefers-color-scheme。 */
export function resolveInitialTheme(): ThemeMode {
  if (typeof window === "undefined") return "light";
  const saved = readSavedTheme();
  if (saved === "light" || saved === "dark" || saved === "violet") return saved;
  return window.matchMedia?.("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

/** 只改 DOM 属性，不动 store（供首帧与系统变化监听复用）。 */
export function applyThemeToDocument(theme: ThemeMode): void {
  if (typeof document === "undefined") return;
  if (theme === "dark") document.documentElement.setAttribute("data-theme", "dark");
  else if (theme === "violet") document.documentElement.setAttribute("data-theme", "violet");
  else document.documentElement.removeAttribute("data-theme");
}

export interface MeterDelta {
  id: string;
  meterIndex: 1 | 2 | 3;
  text: string;
  isPositive: boolean;
}

export interface LorePopoverState {
  visible: boolean;
  title: string;
  desc: string;
  keys: string;
  x: number;
  y: number;
}

export interface InvBubbleState {
  visible: boolean;
  slotId: string;
  item: InventoryItemPreset;
  x: number;
  y: number;
}

interface UiState {
  // 视口模式
  viewMode: ViewportMode;
  savedStateBeforeReading: { leftCollapsed: boolean; rightCollapsed: boolean };
  
  // 侧栏状态
  isLeftRailCollapsed: boolean;
  isRightRailCollapsed: boolean;
  leftTab: LeftSubtab;

  // 主题
  theme: ThemeMode;

  // 当前角色与立绘：null 表示自定义卡会话（显示层落 custom 通用皮肤）。
  activeCharKey: string | null;
  tachieExpression: TachieExpression;

  // 弹窗状态
  fullTachieOpen: boolean;
  worldbookModalOpen: boolean;
  charImportOpen: boolean;
  /** 从卡库直接开新局时携带的 cardId；导入弹窗据此预填而不必重新上传文件。 */
  pendingCardId: string | null;
  blueprintOpen: boolean;
  /** 命令面板（Ctrl/Cmd+K）。它是最上层浮层：Esc 链里排在所有弹窗之前。 */
  paletteOpen: boolean;

  // 互动浮层
  meterDeltas: MeterDelta[];
  lorePopover: LorePopoverState | null;
  invBubble: InvBubbleState | null;
  toastMessage: string | null;

  // 输入框联动
  injectedComposerText: string | null;
  selectionWrapRequest: "quote" | "action" | null;

  // 角色自定义覆盖（如检定后数值改变）
  characterOverrides: Record<string, Partial<CharacterPreset>>;

  // Actions
  toggleLeftRail: () => void;
  toggleRightRail: () => void;
  setLeftRailCollapsed: (collapsed: boolean) => void;
  setRightRailCollapsed: (collapsed: boolean) => void;
  closeAllDrawers: () => void;
  switchLeftTab: (tab: LeftSubtab) => void;
  openLeftTabAndExpand: (tab: LeftSubtab) => void;
  switchViewMode: (mode: ViewportMode) => void;
  toggleTheme: () => void;
  setTheme: (theme: ThemeMode) => void;
  switchActiveCharacter: (key: string) => void;
  setActiveCharKey: (key: string | null) => void;
  setTachieExpression: (expr: TachieExpression) => void;

  // Modals
  setFullTachieOpen: (open: boolean) => void;
  setWorldbookModalOpen: (open: boolean) => void;
  setCharImportOpen: (open: boolean) => void;
  /** openCharImportWithCard 用卡库里的既有卡打开导入/开局弹窗（预填，无需重新上传）。 */
  openCharImportWithCard: (cardId: string) => void;
  setBlueprintOpen: (open: boolean) => void;
  setPaletteOpen: (open: boolean) => void;
  togglePalette: () => void;
  dossierModalOpen: boolean;
  setDossierModalOpen: (open: boolean) => void;
  promisesModalOpen: boolean;
  setPromisesModalOpen: (open: boolean) => void;
  memoryModalOpen: boolean;
  setMemoryModalOpen: (open: boolean) => void;

  // Micro-interactions
  spawnMeterDelta: (meterIndex: 1 | 2 | 3, text: string, isPositive: boolean) => void;
  showLorePop: (title: string, desc: string, keys: string, x: number, y: number) => void;
  hideLorePop: () => void;
  showInvBubble: (slotId: string, item: InventoryItemPreset, x: number, y: number) => void;
  hideInvBubble: () => void;
  notifyQuiet: (msg: string) => void;

  // Composer
  injectComposerText: (text: string) => void;
  clearInjectedText: () => void;
  requestSelectionWrap: (wrap: "quote" | "action") => void;
  clearSelectionWrap: () => void;

  // Update dynamic values for character
  updateCharacterMeters: (charKey: string, metersPatch: Partial<CharacterPreset["meters"]>) => void;
}

export const useUi = create<UiState>((set, get) => ({
  viewMode: "studio",
  savedStateBeforeReading: { leftCollapsed: false, rightCollapsed: false },

  isLeftRailCollapsed: typeof window !== "undefined" ? window.innerWidth < 1024 : false,
  isRightRailCollapsed: typeof window !== "undefined" ? window.innerWidth < 1200 : false,
  leftTab: "tree",

  theme: resolveInitialTheme(),

  activeCharKey: DEFAULT_PRESET_KEY,
  tachieExpression: "alert",

  fullTachieOpen: false,
  worldbookModalOpen: false,
  charImportOpen: false,
  pendingCardId: null,
  blueprintOpen: false,
  paletteOpen: false,
  dossierModalOpen: false,
  promisesModalOpen: false,
  memoryModalOpen: false,

  meterDeltas: [],
  lorePopover: null,
  invBubble: null,
  toastMessage: null,

  injectedComposerText: null,
  selectionWrapRequest: null,
  characterOverrides: {},

  toggleLeftRail: () => {
    if (get().viewMode === "reading") {
      get().notifyQuiet("纯净编年阅读模式下侧栏默认整体隐藏");
      return;
    }
    const next = !get().isLeftRailCollapsed;
    set({ isLeftRailCollapsed: next });
    get().notifyQuiet(next ? "左栏已收起" : "左栏已展开");
  },

  toggleRightRail: () => {
    if (get().viewMode === "reading") {
      get().notifyQuiet("纯净编年阅读模式下侧栏默认整体隐藏");
      return;
    }
    const next = !get().isRightRailCollapsed;
    set({ isRightRailCollapsed: next });
    get().notifyQuiet(next ? "右侧角色立绘与状态已收起" : "右侧角色立绘与状态已展开");
  },

  setLeftRailCollapsed: (collapsed) => set({ isLeftRailCollapsed: collapsed }),
  setRightRailCollapsed: (collapsed) => set({ isRightRailCollapsed: collapsed }),
  closeAllDrawers: () => set({ isLeftRailCollapsed: true, isRightRailCollapsed: true }),

  switchLeftTab: (tab) => set({ leftTab: tab }),

  openLeftTabAndExpand: (tab) => {
    set({ leftTab: tab, isLeftRailCollapsed: false });
  },

  switchViewMode: (mode) => {
    const current = get().viewMode;
    if (mode === current) return;

    if (mode === "reading") {
      set({
        viewMode: "reading",
        savedStateBeforeReading: {
          leftCollapsed: get().isLeftRailCollapsed,
          rightCollapsed: get().isRightRailCollapsed,
        },
      });
      get().notifyQuiet("已进入：纯净编年模式（全视口纯粹文学阅读）");
    } else {
      const saved = get().savedStateBeforeReading;
      set({
        viewMode: "studio",
        isLeftRailCollapsed: saved.leftCollapsed,
        isRightRailCollapsed: saved.rightCollapsed,
      });
      get().notifyQuiet("已返回：跑团工作台（恢复自由双栏与交互视口）");
    }
  },

  toggleTheme: () => {
    const current = get().theme;
    const next: ThemeMode = current === "light" ? "dark" : current === "dark" ? "violet" : "light";
    get().setTheme(next);
    const msg =
      next === "dark"
        ? "已切换为：曜石墨黑主题"
        : next === "violet"
          ? "已切换为：幻夜幽紫主题"
          : "已切换为：乳白纸墨主题";
    get().notifyQuiet(msg);
  },

  setTheme: (theme) => {
    set({ theme });
    applyThemeToDocument(theme);
    if (typeof window !== "undefined") writeSavedTheme(theme);
  },

  switchActiveCharacter: (key) => {
    if (CHARACTER_PRESETS[key]) {
      set({ activeCharKey: key, tachieExpression: "alert" });
      get().notifyQuiet(`已切换角色卡：${CHARACTER_PRESETS[key].name}`);
    }
  },

  setActiveCharKey: (key) => {
    if (key === null) {
      // 自定义卡会话：无匹配预设，显示层走 custom 兜底皮肤。
      set({ activeCharKey: null, tachieExpression: "alert" });
      return;
    }
    if (CHARACTER_PRESETS[key]) {
      set({ activeCharKey: key, tachieExpression: "alert" });
    }
  },

  setTachieExpression: (expr) => set({ tachieExpression: expr }),

  setFullTachieOpen: (open) => set({ fullTachieOpen: open }),
  setWorldbookModalOpen: (open) => set({ worldbookModalOpen: open }),
  setCharImportOpen: (open) => set((s) => ({ charImportOpen: open, pendingCardId: open ? s.pendingCardId : null })),
  openCharImportWithCard: (cardId) => set({ charImportOpen: true, pendingCardId: cardId }),
  setBlueprintOpen: (open) => set({ blueprintOpen: open }),
  setPaletteOpen: (open) => set({ paletteOpen: open }),
  togglePalette: () => set((s) => ({ paletteOpen: !s.paletteOpen })),
  setDossierModalOpen: (open) => set({ dossierModalOpen: open }),
  setPromisesModalOpen: (open) => set({ promisesModalOpen: open }),
  setMemoryModalOpen: (open) => set({ memoryModalOpen: open }),

  spawnMeterDelta: (meterIndex, text, isPositive) => {
    const id = `delta_${Date.now()}_${Math.random()}`;
    const newDelta: MeterDelta = { id, meterIndex, text, isPositive };
    set((state) => ({ meterDeltas: [...state.meterDeltas, newDelta] }));
    setTimeout(() => {
      set((state) => ({
        meterDeltas: state.meterDeltas.filter((d) => d.id !== id),
      }));
    }, 1400);
  },

  showLorePop: (title, desc, keys, x, y) => {
    set({ lorePopover: { visible: true, title, desc, keys, x, y } });
  },

  hideLorePop: () => {
    set({ lorePopover: null });
  },

  showInvBubble: (slotId, item, x, y) => {
    set({ invBubble: { visible: true, slotId, item, x, y } });
  },

  hideInvBubble: () => {
    set({ invBubble: null });
  },

  notifyQuiet: (msg) => {
    set({ toastMessage: msg });
    setTimeout(() => {
      if (get().toastMessage === msg) {
        set({ toastMessage: null });
      }
    }, 2200);
  },

  injectComposerText: (text) => set({ injectedComposerText: text }),
  clearInjectedText: () => set({ injectedComposerText: null }),

  requestSelectionWrap: (wrap) => set({ selectionWrapRequest: wrap }),
  clearSelectionWrap: () => set({ selectionWrapRequest: null }),

  updateCharacterMeters: (charKey, metersPatch) => {
    const curr = get().characterOverrides[charKey]?.meters ?? CHARACTER_PRESETS[charKey]?.meters;
    if (!curr) return;
    set((state) => ({
      characterOverrides: {
        ...state.characterOverrides,
        [charKey]: {
          ...state.characterOverrides[charKey],
          meters: { ...curr, ...metersPatch },
        },
      },
    }));
  },
}));

// 用户未手动选择主题时，跟随系统 prefers-color-scheme 的变化。
if (typeof window !== "undefined" && window.matchMedia) {
  const media = window.matchMedia("(prefers-color-scheme: dark)");
  media.addEventListener("change", (event) => {
    if (readSavedTheme()) return;
    const next: ThemeMode = event.matches ? "dark" : "light";
    useUi.setState({ theme: next });
    applyThemeToDocument(next);
  });
}

// 视口跨入移动抽屉断点（≤768px）时自动收起两侧栏：否则宽屏加载后再变窄/旋转，
// 离屏抽屉会以 transform:translateX(0) 盖住正文且遮罩生效，除顶栏切换外无路可退。
if (typeof window !== "undefined" && window.matchMedia) {
  const mobile = window.matchMedia("(max-width: 768px)");
  const collapseForMobile = (event: MediaQueryListEvent | MediaQueryList) => {
    if (!event.matches) return;
    useUi.setState({ isLeftRailCollapsed: true, isRightRailCollapsed: true });
  };
  mobile.addEventListener("change", collapseForMobile);
  if (mobile.matches) collapseForMobile(mobile);
}
