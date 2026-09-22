// TopHeader: 顶栏中枢（品牌标题、场景面包屑、双模切换与主题切换、当前生效模型指示）
import React from "react";
import { useUi } from "../stores/uiStore";
import { useStory } from "../stores/storyStore";
import { useSettings, activePrimaryInstance } from "../stores/settingsStore";
import { CHARACTER_PRESETS } from "../lib/characterPresets";
import { connectionTitle } from "../lib/modelLabels";
import { Icon } from "../ui/Icon";
import { Tabs } from "../ui/Tabs";
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
          <Icon name="menu" size={16} className="rail-toggle-glyph" />
        </button>

        <span className="brand-title">TAVERN AGENT</span>
        {view ? (
          <div className="scene-breadcrumb">
            <Icon name="book" size={14} className="breadcrumb-icon" />
            <span className="breadcrumb-title">{view.title}</span>
            <span className="quiet-state-pill breadcrumb-branch">
              <Icon name="leaf" size={12} /> {view.branch?.name || "main"}
            </span>
          </div>
        ) : (
          <div className="scene-breadcrumb" dangerouslySetInnerHTML={{ __html: char.breadcrumb }} />
        )}
      </div>

      {/* Mode Selector: 跑团工作台 ⇄ 纯净编年 */}
      <Tabs
        label="视图模式"
        className="view-mode-tabs"
        size="sm"
        value={viewMode}
        onChange={(next) => switchViewMode(next)}
        items={[
          {
            value: "studio",
            id: "btn-mode-studio",
            title: "跑团工作台模式",
            icon: <Icon name="compass" size={14} />,
            label: "工作台",
            labelClassName: "tab-text",
          },
          {
            value: "reading",
            id: "btn-mode-reading",
            title: "纯净文学编年模式",
            icon: <Icon name="book" size={14} />,
            label: "纯净编年",
            labelClassName: "tab-text",
          },
        ]}
      />

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
            <Icon name="command" size={13} />
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
          <Icon name="user" size={15} className="rail-toggle-glyph" />
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
          <Icon name="sliders" size={13} className="header-model-glyph mobile-only" />
          <span className="header-btn-text header-active-model-name">{activeModelName}</span>
        </button>

        {/* Monochrome Theme Switcher (乳白纸墨 vs 墨黑幽夜) */}
        <button className="btn-quiet" onClick={toggleTheme} id="theme-btn" title="切换乳白 / 纯黑底色">
          <span id="theme-icon" style={{ display: "inline-flex" }}>
            <Icon name={theme === "dark" ? "sun" : "moon"} size={14} />
          </span>
          <span id="theme-text" className="header-btn-text">{theme === "dark" ? "乳白纸墨" : "墨黑夜色"}</span>
        </button>
      </div>
    </header>
  );
};
