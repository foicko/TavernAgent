import React from "react";
import { Icon } from "../ui/Icon";
import { Banner } from "../ui/Banner";
import { EmptyState } from "../ui/EmptyState";
import { ListRow } from "../ui/ListRow";
import { Tabs } from "../ui/Tabs";
import { RowDeleteControl } from "./RowDeleteControl";
import { CHARACTER_PRESETS } from "../lib/characterPresets";
import { storedCardTag, type PresentedCharacter } from "../lib/characterPresentation";
import { findCardByCharacterId, findCardByName, getSessionAvatar } from "../lib/characterCardStore";
import { resolveCharKeyFromSession } from "../stores/storyStore";
import { useUi, type LeftSubtab } from "../stores/uiStore";
import type { SessionView, SessionSummary } from "../app/types";
import type { ToastTone } from "../ui/Toast";

function formatSessionTime(isoString?: string): string {
  if (!isoString) return "最近";
  try {
    const date = new Date(isoString);
    if (isNaN(date.getTime())) return "最近";
    const now = new Date();
    const diffSec = Math.floor((now.getTime() - date.getTime()) / 1000);
    if (diffSec < 60) return "刚刚";
    if (diffSec < 3600) return `${Math.floor(diffSec / 60)} 分钟前`;
    if (diffSec < 86400) return `${Math.floor(diffSec / 3600)} 小时前`;
    if (diffSec < 86400 * 7) return `${Math.floor(diffSec / 86400)} 天前`;
    return `${date.getMonth() + 1}月${date.getDate()}日`;
  } catch {
    return "最近";
  }
}

interface LeftRailSessionsPanelProps {
  sessions: SessionSummary[];
  sessionsError: string | null;
  loadSessions: () => Promise<void>;
  sessionFilter: "all" | "current";
  setSessionFilter: (filter: "all" | "current") => void;
  currentView: SessionView | null;
  pendingSessionId: string | null;
  sessionLoading: boolean;
  activeCharKey: string | null;
  char: PresentedCharacter;
  playerName: string;
  confirmDeleteId: string | null;
  setConfirmDeleteId: (id: string | null) => void;
  openSession: (id: string) => Promise<unknown>;
  deleteSession: (id: string) => Promise<unknown>;
  startNewSessionForCurrentChar: () => Promise<unknown>;
  exportSession: () => Promise<boolean>;
  importSession: (file: File) => Promise<unknown>;
  packError: string | null;
  clearPackError: () => void;
  switchLeftTab: (tab: LeftSubtab) => void;
  setCharImportOpen: (open: boolean) => void;
  toggleLeftRail: () => void;
  notifyQuiet: (message: string, tone?: ToastTone) => void;
  style?: React.CSSProperties;
}

export const LeftRailSessionsPanel: React.FC<LeftRailSessionsPanelProps> = ({
  sessions,
  sessionsError,
  loadSessions,
  sessionFilter,
  setSessionFilter,
  currentView,
  pendingSessionId,
  sessionLoading,
  activeCharKey,
  char,
  playerName,
  confirmDeleteId,
  setConfirmDeleteId,
  openSession,
  deleteSession,
  startNewSessionForCurrentChar,
  exportSession,
  importSession,
  packError,
  clearPackError,
  switchLeftTab,
  setCharImportOpen,
  toggleLeftRail,
  notifyQuiet,
  style,
}) => {
  return (
    <div id="view-sessions" className="rail-view-panel" style={style}>
      {packError ? (
        <Banner tone="error" onDismiss={clearPackError}>
          {packError}
        </Banner>
      ) : null}
      <div className="session-section-header">
        <div className="session-section-title">
          <span>历史故事会话</span>
          <span className="session-total-count">{sessions.length}</span>
        </div>
        <Tabs
          label="会话筛选"
          size="sm"
          fill
          className="session-filter-tabs"
          value={sessionFilter}
          onChange={setSessionFilter}
          items={[
            { value: "all", label: `全部 (${sessions.length})` },
            { value: "current", label: `仅 ${char.shortName}` },
          ]}
        />
      </div>

      {sessionsError && sessions.length === 0 ? (
        <EmptyState
          size="sm"
          icon={<Icon name="alert" size={22} />}
          title="无法加载故事列表"
          description={sessionsError}
          action={
            <button type="button" className="btn-quiet" onClick={() => void loadSessions()}>
              <Icon name="refresh" size={13} /> 重试
            </button>
          }
        />
      ) : sessions.length === 0 ? (
        <EmptyState
          size="sm"
          icon={<Icon name="scroll" size={22} />}
          title="暂无历史故事会话"
          description="点击下方按钮开辟，或直接在底部输入框发言开启全新故事。"
        />
      ) : (
        <div className="session-list-stack">
          {(sessionFilter === "current"
            ? sessions.filter((s) => currentView ? s.characterId === currentView.characterId : resolveCharKeyFromSession(s) === activeCharKey)
            : sessions
          ).map((s) => {
            const isActive = pendingSessionId ? pendingSessionId === s.sessionId : currentView?.sessionId === s.sessionId;
            const charKey = resolveCharKeyFromSession(s);
            const storedCard = findCardByCharacterId(s.characterId) || findCardByName(s.title);
            const sessionAvatar = getSessionAvatar(s.sessionId) || storedCard?.avatar || (currentView?.sessionId === s.sessionId ? (currentView.state?.characters && Object.values(currentView.state.characters).find((c: any) => c.characterId !== "player")?.avatar) : undefined);
            const sPreset = CHARACTER_PRESETS[charKey ?? "custom"] || CHARACTER_PRESETS.custom;
            const resolvedAvatar = sessionAvatar || sPreset.avatar;
            const roleTag = storedCard
              ? (storedCard.shortName || storedCardTag(storedCard, playerName).slice(0, 16))
              : (sPreset.role ? sPreset.role.split("·")[0].trim() : sPreset.shortName);
            return (
              <ListRow
                key={s.sessionId}
                className={`session-entry-card${isActive ? " active" : ""}`}
                leading={
                  <span className="session-avatar-wrapper">
                    <img
                      src={resolvedAvatar}
                      alt={storedCard?.name || sPreset.name}
                      className="session-avatar-img"
                    />
                    {isActive && <span className="session-avatar-indicator" />}
                  </span>
                }
                title={<span className="session-title-text" title={s.title}>{s.title}</span>}
                subtitle={
                  confirmDeleteId === s.sessionId ? (
                    <span className="row-confirm-hint">不可恢复</span>
                  ) : (
                    <span className="session-card-sub-row">
                      <span className="session-role-tag" title={sPreset.name}>{roleTag}</span>
                      <span className="session-time-text">{formatSessionTime(s.createdAt)}</span>
                    </span>
                  )
                }
                meta={
                  confirmDeleteId === s.sessionId ? undefined : (
                    <span className={`session-status-badge ${isActive ? "active" : "archived"}`}>
                      {isActive ? (
                        <>
                          <span className="badge-pulse-dot" />
                          {pendingSessionId === s.sessionId && sessionLoading ? "载入中" : "当前"}
                        </>
                      ) : (
                        "存档"
                      )}
                    </span>
                  )
                }
                selected={isActive}
                onSelect={() => {
                  void openSession(s.sessionId);
                  if (typeof window !== "undefined" && window.innerWidth <= 768) {
                    toggleLeftRail();
                  }
                }}
                ariaLabel={`打开故事 ${s.title}`}
                actions={
                  <RowDeleteControl
                    confirming={confirmDeleteId === s.sessionId}
                    idleLabel={`删除故事 ${s.title}`}
                    confirmLabel="确认删除"
                    confirmTitle="永久删除这个故事（不可恢复）"
                    onAsk={() => setConfirmDeleteId(s.sessionId)}
                    onCancel={() => setConfirmDeleteId(null)}
                    onConfirm={() => {
                      setConfirmDeleteId(null);
                      void deleteSession(s.sessionId).catch((error) => {
                        useUi.getState().notifyQuiet(
                          error instanceof Error && error.message ? error.message : "删除失败，请稍后重试",
                          "danger",
                        );
                      });
                    }}
                  />
                }
              />
            );
          })}
        </div>
      )}
      <div className="session-action-shelf">
        <button
          className="session-btn-primary"
          onClick={() => void startNewSessionForCurrentChar()}
          title={`基于【${char.name}】开辟全新平行故事分支`}
        >
          <Icon name="sparkles" size={14} className="session-primary-icon" />
          <span>与「{char.shortName}」开启新故事</span>
        </button>

        <div className="session-utility-grid">
          <button
            className="session-utility-btn"
            onClick={() => {
              void exportSession().then((ok) => {
                if (ok) notifyQuiet("已导出 .tavernpack 剧情包", "success");
              });
            }}
            disabled={!currentView}
            title="导出当前会话为 .tavernpack 剧情包"
          >
            <Icon name="download" size={13} /> 导出剧本
          </button>

          <label
            className="session-utility-btn"
            title="导入 .tavernpack 或 ZIP 剧本包"
            role="button"
            tabIndex={0}
            aria-label="导入剧本包"
            onKeyDown={(event) => {
              if (event.key === "Enter" || event.key === " ") {
                event.preventDefault();
                event.currentTarget.querySelector<HTMLInputElement>("input[type=file]")?.click();
              }
            }}
          >
            <Icon name="upload" size={13} /> 导入剧本
            <input
              type="file"
              accept=".tavernpack,.zip"
              className="session-import-input"
              onChange={(e) => {
                const file = e.target.files?.[0];
                e.target.value = "";
                if (!file) return;
                void importSession(file)
                  .then(() => notifyQuiet(`已导入剧本包：${file.name}`, "success"))
                  .catch(() => {});
              }}
            />
          </label>
        </div>

        <div className="session-footer-links">
          <button
            type="button"
            className="session-footer-link-btn"
            onClick={() => switchLeftTab("characters")}
            title="前往角色档案库切换人物"
          >
            <Icon name="users" size={13} /> 角色库
          </button>
          <span className="link-divider">·</span>
          <button
            type="button"
            className="session-footer-link-btn"
            onClick={() => setCharImportOpen(true)}
            title="导入新角色卡 PNG 或 JSON"
          >
            <Icon name="plus" size={13} /> 导入角色卡
          </button>
        </div>
      </div>
    </div>
  );
};
