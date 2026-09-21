import { useEffect, useState } from "react";
import { useStory, resolveCharKeyFromSession } from "./stores/storyStore";
import { chooseStartupSession, lastSessionId } from "./stores/sessionMemory";
import { useSettings } from "./stores/settingsStore";
import { useUi } from "./stores/uiStore";
import { api, onAuthRequired } from "./app/api";
import { applyNativeTheme } from "./lib/desktopShell";
import { buildCommands } from "./lib/commandRegistry";
import { CommandPalette } from "./ui/CommandPalette";

import { TopHeader } from "./components/TopHeader";
import { LeftRail } from "./components/LeftRail";
import { RightRail } from "./components/RightRail";
import { NarrativeStream } from "./components/NarrativeStream";
import { ComposerShelf } from "./components/ComposerShelf";

import { LorePopover } from "./components/LorePopover";
import { InventoryActionBubble } from "./components/InventoryActionBubble";

import { FullTachieModal } from "./components/FullTachieModal";
import { WorldbookModal } from "./components/WorldbookModal";
import { CharacterImportModal } from "./components/CharacterImportModal";
import { MindGraphModal } from "./components/MindGraphModal";
import { SettingsModal } from "./components/SettingsModal";
import { AuthPairingModal } from "./components/AuthPairingModal";

export default function App() {
  const loadSessions = useStory((s) => s.loadSessions);
  const settingsOpen = useSettings((s) => s.settingsOpen);
  const closeSettings = useSettings((s) => s.closeSettings);
  const [authPairingOpen, setAuthPairingOpen] = useState(false);

  const {
    viewMode,
    theme,
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
    mindGraphOpen,
    setMindGraphOpen,
    paletteOpen,
    setPaletteOpen,
    toastMessage,
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

  // 让原生标题栏跟随应用主题（浏览器下是空操作）。
  useEffect(() => {
    applyNativeTheme(theme);
  }, [theme]);

  // 同步侧栏折叠与阅读模式类至 document.body (与 styles.css 契约完全吻合)
  useEffect(() => {
    if (isLeftRailCollapsed) document.body.classList.add("rail-left-collapsed");
    else document.body.classList.remove("rail-left-collapsed");

    if (isRightRailCollapsed) document.body.classList.add("rail-right-collapsed");
    else document.body.classList.remove("rail-right-collapsed");

    if (viewMode === "reading") document.body.classList.add("mode-reading");
    else document.body.classList.remove("mode-reading");
  }, [isLeftRailCollapsed, isRightRailCollapsed, viewMode]);

  // 全局快捷键支持: Ctrl+K (命令面板), Ctrl+[ (左栏), Ctrl+] (右栏), Esc (自上层向下关闭)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
        // 浏览器里 Ctrl+K 是"跳到地址栏"，必须拦下来才轮得到我们。
        e.preventDefault();
        setPaletteOpen(!useUi.getState().paletteOpen);
      } else if ((e.ctrlKey || e.metaKey) && e.key === "[") {
        e.preventDefault();
        toggleLeftRail();
      } else if ((e.ctrlKey || e.metaKey) && e.key === "]") {
        e.preventDefault();
        toggleRightRail();
      } else if (e.key === "Escape") {
        if (paletteOpen) {
          setPaletteOpen(false);
        } else if (mindGraphOpen) {
          setMindGraphOpen(false);
        } else if (fullTachieOpen) {
          setFullTachieOpen(false);
        } else if (worldbookModalOpen) {
          setWorldbookModalOpen(false);
        } else if (charImportOpen) {
          setCharImportOpen(false);
        } else if (settingsOpen) {
          closeSettings();
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
    paletteOpen,
    setPaletteOpen,
    mindGraphOpen,
    setMindGraphOpen,
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
          <NarrativeStream />
          {viewMode === "studio" && <ComposerShelf />}
        </main>

        {/* 右侧立绘与心境遥测 HUD */}
        <RightRail />
      </div>

      {/* 移动端侧栏抽屉遮罩层 */}
      <div
        className={`mobile-drawer-backdrop ${(!isLeftRailCollapsed || !isRightRailCollapsed) ? "active" : ""}`}
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
      <MindGraphModal />
      {settingsOpen && <SettingsModal onClose={closeSettings} />}
      <AuthPairingModal
        open={authPairingOpen}
        onSuccess={() => {
          setAuthPairingOpen(false);
          void loadSessions();
        }}
        onClose={() => setAuthPairingOpen(false)}
      />

      {/* 轻量微通知吐司 */}
      {toastMessage && (
        <div className="quiet-toast-notification" role="status">
          {toastMessage}
        </div>
      )}
    </div>
  );
}
