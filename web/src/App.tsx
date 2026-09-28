import { useEffect, useState } from "react";
import { useStory, resolveCharKeyFromSession } from "./stores/storyStore";
import { chooseStartupSession, lastSessionId } from "./stores/sessionMemory";
import { useSettings } from "./stores/settingsStore";
import { useUi } from "./stores/uiStore";
import { api, onAuthRequired } from "./app/api";
import { buildCommands } from "./lib/commandRegistry";
import { isModalOpen } from "./lib/modalStack";
import { CommandPalette } from "./ui/CommandPalette";
import { ToastStack } from "./ui/Toast";

import { TopHeader } from "./components/TopHeader";
import { LeftRail } from "./components/LeftRail";
import { RightRail } from "./components/RightRail";
import { NarrativeStream } from "./components/NarrativeStream";
import { ComposerShelf } from "./components/ComposerShelf";
import { FirstRunBanner } from "./components/FirstRunBanner";

import { LorePopover } from "./components/LorePopover";
import { InventoryActionBubble } from "./components/InventoryActionBubble";

import { FullTachieModal } from "./components/FullTachieModal";
import { WorldbookModal } from "./components/WorldbookModal";
import { CharacterImportModal } from "./components/CharacterImportModal";
import { SettingsModal } from "./components/SettingsModal";
import { AuthPairingModal } from "./components/AuthPairingModal";
import type { LeftSubtab } from "./stores/uiStore";

// 用 KeyboardEvent.code 而不是 key：Alt+数字在部分键盘布局下 key 会变成符号。
const LEFT_TAB_KEYS: Record<string, LeftSubtab> = {
  Digit1: "tree", Digit2: "characters", Digit3: "lore", Digit4: "sessions",
};

export default function App() {
  const loadSessions = useStory((s) => s.loadSessions);
  const settingsOpen = useSettings((s) => s.settingsOpen);
  const closeSettings = useSettings((s) => s.closeSettings);
  const [authPairingOpen, setAuthPairingOpen] = useState(false);

  const {
    viewMode,
    switchViewMode,
    isLeftRailCollapsed,
    isRightRailCollapsed,
    toggleLeftRail,
    toggleRightRail,
    fullTachieOpen,
    setFullTachieOpen,
    worldbookModalOpen,
    setWorldbookModalOpen,
    charImportOpen,
    setCharImportOpen,
    paletteOpen,
    setPaletteOpen,
    toasts,
    dismissToast,
  } = useUi();

  // 监听局域网配对授权需求
  useEffect(() => {
    return onAuthRequired(() => {
      setAuthPairingOpen(true);
    });
  }, []);

  useEffect(() => {
    void api
      .authStatus()
      .then((st) => {
        if (st.required && !st.authenticated) {
          setAuthPairingOpen(true);
        }
      })
      .catch(() => {});
  }, []);

  // 初始化加载后端会话列表并激活上次打开的故事（刷新后不跳到列表第一条）
  useEffect(() => {
    void loadSessions()
      .then(async () => {
        const current = useStory.getState();
        if (!current.view && current.sessions.length > 0) {
          const activeCharKey = useUi.getState().activeCharKey; // null = 自定义卡
          const target = chooseStartupSession(
            current.sessions,
            lastSessionId(),
            (s) => resolveCharKeyFromSession(s) === activeCharKey,
          );
          if (target) await current.openSession(target.sessionId);
        }
      })
      .catch(() => {
        // 若无后端连接，平滑降级为离线真实角色剧场
      });
  }, [loadSessions]);

  // 同步侧栏折叠与阅读模式类至 document.body (与 styles.css 契约完全吻合)
  useEffect(() => {
    if (isLeftRailCollapsed) document.body.classList.add("rail-left-collapsed");
    else document.body.classList.remove("rail-left-collapsed");

    if (isRightRailCollapsed) document.body.classList.add("rail-right-collapsed");
    else document.body.classList.remove("rail-right-collapsed");

    if (viewMode === "reading") document.body.classList.add("mode-reading");
    else document.body.classList.remove("mode-reading");
  }, [isLeftRailCollapsed, isRightRailCollapsed, viewMode]);

  // 全局快捷键：
  // Ctrl+K 命令面板 · Ctrl+[ / Ctrl+] 左右栏 · Ctrl+, 设置 · Ctrl+Shift+L 主题
  // Alt+1..4 左栏标签（不用 Ctrl+数字：浏览器用它切换标签页）· Esc 自上层向下关闭
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const mod = e.ctrlKey || e.metaKey;
      if (mod && e.key.toLowerCase() === "k") {
        // 浏览器里 Ctrl+K 是"跳到地址栏"，必须拦下来才轮得到我们。
        e.preventDefault();
        // 已打开时按键关闭；否则只在没有其它模态时才开，避免模态叠模态。
        if (useUi.getState().paletteOpen) setPaletteOpen(false);
        else if (!isModalOpen()) setPaletteOpen(true);
      } else if (mod && e.key === "[") {
        e.preventDefault();
        toggleLeftRail();
      } else if (mod && e.key === "]") {
        e.preventDefault();
        toggleRightRail();
      } else if (mod && e.key === ",") {
        e.preventDefault();
        if (!isModalOpen()) useSettings.getState().openSettings();
      } else if (mod && e.shiftKey && e.key.toLowerCase() === "l") {
        e.preventDefault();
        useUi.getState().toggleTheme();
      } else if (e.altKey && !mod && LEFT_TAB_KEYS[e.code]) {
        e.preventDefault();
        useUi.getState().openLeftTabAndExpand(LEFT_TAB_KEYS[e.code]);
      } else if (e.key === "Escape") {
        // 抽屉遮罩仅在 ≤768px 生效；桌面端 Esc 不应收起侧栏。
        const drawerOpen = (window.matchMedia?.("(max-width: 768px)").matches ?? false)
          && (!isLeftRailCollapsed || !isRightRailCollapsed);
        if (paletteOpen) {
          setPaletteOpen(false);
        } else if (fullTachieOpen) {
          setFullTachieOpen(false);
        } else if (worldbookModalOpen) {
          setWorldbookModalOpen(false);
        } else if (charImportOpen) {
          setCharImportOpen(false);
        } else if (settingsOpen) {
          closeSettings();
        } else if (drawerOpen) {
          if (!isLeftRailCollapsed) toggleLeftRail();
          if (!isRightRailCollapsed) toggleRightRail();
        } else if (viewMode === "reading") {
          switchViewMode("studio");
        }
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [
    toggleLeftRail,
    toggleRightRail,
    isLeftRailCollapsed,
    isRightRailCollapsed,
    paletteOpen,
    setPaletteOpen,
    fullTachieOpen,
    setFullTachieOpen,
    worldbookModalOpen,
    setWorldbookModalOpen,
    charImportOpen,
    setCharImportOpen,
    settingsOpen,
    closeSettings,
    viewMode,
    switchViewMode,
  ]);

  return (
    <div className={`app-root ${viewMode === "reading" ? "mode-reading" : ""}`}>
      {/* 极简顶栏 (纯净编年模式下自动隐藏或淡化) */}
      <TopHeader />

      {/* 双模驱动主工作区 */}
      <div className="workspace-layout" id="workspace">
        {/* 左侧多维导航栏 */}
        <LeftRail />

        {/* 中轴剧场主舞台 (故事流 + 底部输入中枢) */}
        <main className="stage-main">
          {viewMode === "studio" && <FirstRunBanner />}
          <NarrativeStream />
          {viewMode === "studio" && <ComposerShelf />}
        </main>

        {/* 右侧立绘与心境遥测 HUD */}
        <RightRail />
      </div>

      {/* 移动端侧栏抽屉遮罩层 */}
      <div
        className={`mobile-drawer-backdrop ${(!isLeftRailCollapsed || !isRightRailCollapsed) ? "active" : ""}`}
        role="presentation"
        aria-hidden="true"
        onClick={() => {
          if (!isLeftRailCollapsed) toggleLeftRail();
          if (!isRightRailCollapsed) toggleRightRail();
        }}
      />

      {/* 微交互悬浮浮层 */}
      <LorePopover />
      <InventoryActionBubble />

      {/* 全局命令面板：只在打开时构建命令清单（构建会读各 store 的当前快照）。 */}
      {paletteOpen && <CommandPalette commands={buildCommands()} onClose={() => setPaletteOpen(false)} />}

      {/* 弹窗集合 */}
      <FullTachieModal />
      <WorldbookModal />
      <CharacterImportModal />
      {settingsOpen && <SettingsModal onClose={closeSettings} />}
      <AuthPairingModal
        open={authPairingOpen}
        onSuccess={() => {
          setAuthPairingOpen(false);
          void loadSessions();
        }}
        onClose={() => setAuthPairingOpen(false)}
      />

      {/* 轻量微通知吐司（设计系统实现：色调 / 可关闭 / 堆叠 / aria-live） */}
      <ToastStack toasts={toasts} onDismiss={dismissToast} />
    </div>
  );
}
