// TopHeader: 顶栏中枢（品牌标题、场景面包屑、双模切换与主题切换、当前生效模型指示）
import React from "react";
import { useUi } from "../stores/uiStore";
import { useStory } from "../stores/storyStore";
import { useSettings, activePrimaryInstance } from "../stores/settingsStore";
import { CHARACTER_PRESETS } from "../lib/characterPresets";
import { connectionTitle } from "../lib/modelLabels";
import "./TopHeader.css";

export const TopHeader: React.FC = () => {
  const {
    viewMode,
    switchViewMode,
    theme,
    toggleTheme,
    activeCharKey,
    toggleLeftRail,
    toggleRightRail,
    isLeftRailCollapsed,
    isRightRailCollapsed,
    togglePalette,
  } = useUi();
  const view = useStory((s) => s.view);
  const openSettings = useSettings((s) => s.openSettings);
  const providers = useSettings((s) => s.providers);
  const models = useSettings((s) => s.models);
  const loaded = useSettings((s) => s.loaded);
  const loadSettings = useSettings((s) => s.load);

  React.useEffect(() => {
    if (!loaded) void loadSettings();
  }, [loaded, loadSettings]);

  const primaryInstance = activePrimaryInstance(providers, models);
  const activeModelName = primaryInstance ? connectionTitle(primaryInstance) : "未配置模型";
  const char = CHARACTER_PRESETS[activeCharKey ?? "custom"] || CHARACTER_PRESETS.custom;

  return (
    <header className="top-header">
      <div className="brand-group">
        {/* 移动端/平板快速呼出左侧导航栏（剧本/角色/分支） */}
        <button
          className={`btn-quiet mobile-rail-toggle mobile-only ${!isLeftRailCollapsed ? "active" : ""}`}
          onClick={toggleLeftRail}
          aria-label="剧本与角色目录"
          title="展开/收起左栏 (会话/角色/分支)"
        >
          <span className="rail-toggle-glyph">☰</span>
        </button>

        <span className="brand-title">TAVERN AGENT</span>
        {view ? (
          <div className="scene-breadcrumb">
            <span className="breadcrumb-icon">📖</span>
            <span className="breadcrumb-title">{view.title}</span>
            <span className="quiet-state-pill breadcrumb-branch">
              🌿 {view.branch?.name || "main"}
            </span>
          </div>
        ) : (
          <div className="scene-breadcrumb" dangerouslySetInnerHTML={{ __html: char.breadcrumb }} />
        )}
      </div>

      {/* Mode Selector: 跑团工作台 ⇄ 纯净编年 */}
      <nav className="view-mode-tabs">
        <button
          className={`view-tab-btn ${viewMode === "studio" ? "active" : ""}`}
          id="btn-mode-studio"
          onClick={() => switchViewMode("studio")}
          title="跑团工作台模式"
        >
          <span className="tab-icon">✦</span>
          <span className="tab-text"> 工作台</span>
        </button>
        <button
          className={`view-tab-btn ${viewMode === "reading" ? "active" : ""}`}
          id="btn-mode-reading"
          onClick={() => switchViewMode("reading")}
          title="纯净文学编年模式"
        >
          <span className="tab-icon">📖</span>
          <span className="tab-text"> 纯净编年</span>
        </button>
      </nav>

      <div className="header-actions">
        {/* 命令面板入口：键盘用户走 Ctrl/Cmd+K，这里给鼠标与触屏一个可发现的入口 */}
        <button
          className="btn-quiet header-command-trigger"
          onClick={togglePalette}
          id="command-palette-btn"
          title="打开命令面板 (Ctrl+K)"
          aria-label="命令面板"
        >
          <span className="command-trigger-glyph" aria-hidden="true">
            ⌘
          </span>
          <span className="header-btn-text">命令</span>
          <kbd className="command-trigger-key header-btn-text">Ctrl K</kbd>
        </button>

        {/* 移动端/平板快速呼出右侧角色立绘与状态 */}
        <button
          className={`btn-quiet mobile-rail-toggle mobile-only ${!isRightRailCollapsed ? "active" : ""}`}
          onClick={toggleRightRail}
          aria-label="角色立绘与状态"
          title="展开/收起右侧角色立绘与心境"
        >
          <span className="rail-toggle-glyph rail-toggle-glyph--emoji">🎭</span>
        </button>

        {/* 当前主线模型入口：显示连接名，点击打开模型设置 */}
        <button
          className="btn-quiet header-active-model-pill"
          onClick={openSettings}
          id="settings-btn"
          title={`当前主线：${activeModelName} · 点击打开模型设置`}
          aria-label="模型设置"
        >
          <span className={`active-model-indicator-dot ${primaryInstance ? "active" : "idle"}`} />
          <span className="header-model-glyph mobile-only" aria-hidden="true">⚙</span>
          <span className="header-btn-text header-active-model-name">{activeModelName}</span>
        </button>

        {/* Monochrome Theme Switcher (乳白纸墨 vs 墨黑幽夜) */}
        <button className="btn-quiet" onClick={toggleTheme} id="theme-btn" title="切换乳白 / 纯黑底色">
          <span id="theme-icon">{theme === "dark" ? "◑" : "◐"}</span>
          <span id="theme-text" className="header-btn-text">{theme === "dark" ? "乳白纸墨" : "墨黑夜色"}</span>
        </button>
      </div>
    </header>
  );
};
