// LeftRail: 左侧多维导航栏 (48px 图标停靠轨 / 270px 完整内容)
import React, { useEffect, useState } from "react";
import { useShallow } from "zustand/react/shallow";
import { useUi } from "../stores/uiStore";
import { useStory, resolveCharKeyFromSession } from "../stores/storyStore";
import { CHARACTER_PRESETS, INITIAL_LORE_ENTRIES, SELECTABLE_PRESET_KEYS } from "../lib/characterPresets";
import { StoryMapModal, StoryOutlineSection } from "./StoryMapModal";
import { characterPresentation, storedCardTag } from "../lib/characterPresentation";
import { DEFAULT_PLAYER_NAME } from "../lib/characterMacros";
import { LorebookPanel } from "./LorebookPanel";
import { CharacterCardRow } from "./CharacterCardRow";
import { RowDeleteControl } from "./RowDeleteControl";
import {
  listImportedCards,
  findCardByCharacterId,
  findCardByName,
  getSessionAvatar,
  refreshImportedCards,
  deleteImportedCardRemote,
} from "../lib/characterCardStore";
import { openCard } from "../lib/cardLaunch";
import "./LeftRail.css";

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

export const LeftRail: React.FC = () => {
  const {
    toggleLeftRail,
    leftTab,
    switchLeftTab,
    openLeftTabAndExpand,
    activeCharKey,
    charImportOpen,
    setCharImportOpen,
    setWorldbookModalOpen,
    notifyQuiet,
  } = useUi();

  const {
    sessions,
    openSession,
    deleteSession,
    selectCharacter,
    startNewSessionForCurrentChar,
    view: currentView,
    hud,
    viewAt,
    forkFrom,
    regenerate,
    exportSession,
    importSession,
    sessionLoading,
    pendingSessionId,
  } = useStory(useShallow(s => ({
    sessions: s.sessions, openSession: s.openSession, deleteSession: s.deleteSession, selectCharacter: s.selectCharacter,
    startNewSessionForCurrentChar: s.startNewSessionForCurrentChar, view: s.view, hud: s.hud,
    viewAt: s.viewAt, forkFrom: s.forkFrom, regenerate: s.regenerate,
    exportSession: s.exportSession, importSession: s.importSession,
    sessionLoading: s.sessionLoading, pendingSessionId: s.pendingSessionId,
  })));

  const char = characterPresentation(activeCharKey, currentView);
  // 角色库/会话列表里的身份标语同样来自卡片描述，宏在这一层展开。
  const playerName = currentView?.state?.characters?.["player"]?.name || DEFAULT_PLAYER_NAME;
  const [probeNodeId, setProbeNodeId] = useState<string | null>(null);
  const [storyMapOpen, setStoryMapOpen] = useState(false);
  const [branchConfirmId, setBranchConfirmId] = useState<string | null>(null);
  // 删除是不可撤销的：两个列表都走"先问一次、再确认"的两步式，且确认态下不触发
  // 行本身的点击（切换故事/切换主角）。
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);
  const [confirmCardId, setConfirmCardId] = useState<string | null>(null);
  // 卡库真相在服务端：本地缓存只负责渲染。挂载时拉取一次服务端卡库，
  // 成功后重读缓存；失败则保留缓存（离线仍能展示上次已知的卡）。
  const [importedCards, setImportedCards] = useState(() => listImportedCards());
  useEffect(() => {
    let alive = true;
    void refreshImportedCards()
      .then(() => { if (alive) setImportedCards(listImportedCards()); })
      .catch(() => { /* 离线：继续用本地缓存 */ });
    return () => { alive = false; };
  }, []);
  // 导入弹窗关闭后重读：新导入的卡已写入缓存，需要反映到列表。
  useEffect(() => { setImportedCards(listImportedCards()); }, [charImportOpen]);
  const [loreFilter, setLoreFilter] = useState("");
  const [sessionFilter, setSessionFilter] = useState<"all" | "current">("all");

  const liveRel = hud?.relationships
    ? hud.relationships[`npc_${activeCharKey}`] ||
      hud.relationships[activeCharKey ?? ""] ||
      Object.values(hud.relationships)[0]
    : undefined;

  const hasBackendNodes = !!(currentView && currentView.nodes && currentView.nodes.length > 0);
  const displayTreeNodes = hasBackendNodes
    ? currentView.nodes!.map((n, idx) => {
        let desc = "剧情节点";
        const isOpening = n.kind === "root";
        try {
          const parsed = JSON.parse(n.contentJson);
          if (isOpening) {
            desc = parsed.openingText || "命运起点 · 酒馆初见";
          } else {
            desc = parsed.inputText
              ? `玩家: 「${parsed.inputText.slice(0, 36)}...」`
              : parsed.blocks?.[0]?.text?.slice(0, 40) || "文学演义片段";
          }
        } catch {
          desc = isOpening ? "命运起点" : "文学演义片段";
        }

        const isHead = n.nodeId === currentView.branch.headNodeId;
        const isViewing = n.nodeId === currentView.viewNodeId;
        const tag = isOpening ? "命运起点" : `回合 #${n.turnNumber ?? idx}`;

        // 真实量表指标：从状态快照中获取，不再使用线性拟合数字
        const nodeAff = liveRel?.affection ?? 0;
        const nodeTrust = liveRel?.trust ?? 0;
        const nodeAlert = liveRel?.alertness ?? 0;

        return {
          id: n.nodeId,
          tag,
          desc,
          isActive: isHead,
          isViewing,
          isFork: false,
          aff: nodeAff,
          trust: nodeTrust,
          alert: nodeAlert,
          canRegenerate: n.kind === "turn",
        };
      })
    : (currentView ? [] : char.treeNodes).map((n) => ({
        id: String(n.id),
        tag: n.tag,
        desc: n.desc,
        isActive: n.isActive,
        isViewing: false,
        isFork: n.isFork,
        aff: n.aff,
        trust: n.trust,
        alert: n.alert,
        canRegenerate: !n.tag.includes("起点"),
      }));

  const filteredLore = INITIAL_LORE_ENTRIES.filter((e) => {
    if (!loreFilter) return true;
    const q = loreFilter.toLowerCase();
    return (
      e.title.toLowerCase().includes(q) ||
      e.keys.some((k) => k.toLowerCase().includes(q)) ||
      e.content.toLowerCase().includes(q)
    );
  });

  return (
    <aside className="rail-left" id="rail-left">
      {storyMapOpen && currentView && <StoryMapModal key={currentView.sessionId} view={currentView} onClose={() => setStoryMapOpen(false)} />}
      {/* Collapsed Icon View (Dock Mode) */}
      <div className="rail-collapsed-view">
        <button className="collapsed-icon-btn" onClick={toggleLeftRail} title="展开左侧栏 (Ctrl+[)">
          ›
        </button>
        <div className="rail-dock-divider" />
        <div
          className={`collapsed-icon-btn ${leftTab === "tree" ? "active" : ""}`}
          id="dock-icon-tree"
          title="因果分支树 (点击展开)"
          onClick={() => openLeftTabAndExpand("tree")}
        >
          枝
        </div>
        <div
          className={`collapsed-icon-btn ${leftTab === "characters" ? "active" : ""}`}
          id="dock-icon-characters"
          title="角色档案库 (点击展开)"
          onClick={() => openLeftTabAndExpand("characters")}
        >
          人
        </div>
        <div
          className={`collapsed-icon-btn ${leftTab === "lore" ? "active" : ""}`}
          id="dock-icon-lore"
          title="世界书词条 (点击展开)"
          onClick={() => openLeftTabAndExpand("lore")}
        >
          典
        </div>
        <div
          className={`collapsed-icon-btn ${leftTab === "sessions" ? "active" : ""}`}
          id="dock-icon-sessions"
          title="剧本会话史 (点击展开)"
          onClick={() => openLeftTabAndExpand("sessions")}
        >
          卷
        </div>
      </div>

      {/* Full Left Rail Content */}
      <div className="rail-full-view">
        {/* 移动端抽屉顶栏关闭按钮 */}
        <div className="mobile-drawer-header mobile-only">
          <span className="mobile-drawer-title">
            <span>📚</span> 剧本与角色
          </span>
          <button className="modal-close-btn" onClick={toggleLeftRail} title="收起目录">
            ✕
          </button>
        </div>

        <div className="rail-tab-header">
          <button
            className={`rail-subtab-btn ${leftTab === "tree" ? "active" : ""}`}
            id="subtab-btn-tree"
            onClick={() => switchLeftTab("tree")}
          >
            因果分支
          </button>
          <button
            className={`rail-subtab-btn ${leftTab === "characters" ? "active" : ""}`}
            id="subtab-btn-characters"
            onClick={() => switchLeftTab("characters")}
          >
            角色库
          </button>
          <button
            className={`rail-subtab-btn ${leftTab === "lore" ? "active" : ""}`}
            id="subtab-btn-lore"
            onClick={() => switchLeftTab("lore")}
          >
            世界书
          </button>
          <button
            className={`rail-subtab-btn ${leftTab === "sessions" ? "active" : ""}`}
            id="subtab-btn-sessions"
            onClick={() => switchLeftTab("sessions")}
          >
            会话
          </button>
          <button
            className="rail-collapse-handle-btn"
            onClick={toggleLeftRail}
            title="收起左侧栏 (Ctrl+[)"
          >
            ‹
          </button>
        </div>

        <div className="rail-scroll-body">
          {/* 1. 因果分支树 (Branch Tree) */}
          <div
            className="branch-timeline-tree rail-view-panel"
            id="view-tree"
            style={{ "--lr-panel-display": leftTab === "tree" ? "block" : "none" } as React.CSSProperties}
          >
            {currentView && <><button className="story-map-open btn-quiet" onClick={() => setStoryMapOpen(true)}>打开剧情分支图</button><StoryOutlineSection view={currentView} /></>}
            <div className="tree-lead-label">
              <span>分支拓扑 (Branch Graph)</span>
              <span className="tree-lead-hint">可点击节点回溯</span>
            </div>

            <div className="tree-node-stack">
              {displayTreeNodes.length > 0 && <div className="tree-vertical-line" />}

              {displayTreeNodes.length === 0 ? (
                <div className="tree-empty-card">
                  <div className="tree-empty-icon">🌱</div>
                  <div className="tree-empty-title">
                    因果树未播种
                  </div>
                  <div className="tree-empty-desc">
                    推进第一轮对话后，大模型将在此自动生长因果拓扑与历史快照。
                  </div>
                </div>
              ) : (
                displayTreeNodes.map((node) => {
                  const isOpen = probeNodeId === node.id;
                  const isConfirming = branchConfirmId === node.id;
                  return (
                    <div
                      key={node.id}
                      className={`node-item ${node.isActive ? "active" : ""} ${node.isViewing ? "viewing" : ""} ${isOpen ? "opened" : ""} ${node.isFork ? "fork" : ""}`}
                      onClick={() => {
                        setProbeNodeId(isOpen ? null : node.id);
                        setBranchConfirmId(null);
                      }}
                    >
                      <div className="node-turn-tag">
                        <div className="node-tag-group">
                          <span className="node-tag-text">{node.tag}</span>
                          {node.isViewing && <span className="node-badge-viewing">当前视角</span>}
                        </div>
                        {node.isActive ? (
                          <span className="node-state-pill active">活跃中 ⚡</span>
                        ) : node.isFork ? (
                          <span className="node-state-pill fork">已封存分支</span>
                        ) : (
                          <span className="node-state-pill">快照</span>
                        )}
                      </div>

                      <div className="node-turn-desc" title={node.desc}>
                        {node.desc}
                      </div>

                      {node.isViewing && liveRel && <div className="node-mini-metrics">
                        <span className="metric-chip aff" title="好感度">
                          <span>好感</span>
                          <b>{node.aff > 0 ? `+${node.aff}` : node.aff}</b>
                        </span>
                        <span className="metric-chip trust" title="信任度">
                          <span>信任</span>
                          <b>{node.trust}%</b>
                        </span>
                        <span className="metric-chip alert" title="戒备度">
                          <span>戒备</span>
                          <b>{node.alert}%</b>
                        </span>
                      </div>}

                      {isOpen && (
                        <div className="branch-probe-card" onClick={(e) => e.stopPropagation()}>
                          <div className="probe-head">
                            <span>快照控制</span>
                            <button
                              className="probe-close-btn"
                              onClick={() => {
                                setProbeNodeId(null);
                                setBranchConfirmId(null);
                              }}
                              title="收起选项"
                            >
                              ✕
                            </button>
                          </div>

                          {isConfirming ? (
                            <div className="probe-fork-confirm-box">
                              <div className="probe-fork-confirm-text">
                                🌿 确定从<strong>【{node.tag}】</strong>派生新平行因果分支？
                              </div>
                              <div className="probe-fork-confirm-actions">
                                <button
                                  className="probe-btn-confirm"
                                  onClick={() => {
                                    if (hasBackendNodes) {
                                      void forkFrom(node.id, `平行分支_${node.tag}`);
                                    } else {
                                      notifyQuiet("请先开启故事，再开辟分支。");
                                    }
                                    setBranchConfirmId(null);
                                    setProbeNodeId(null);
                                  }}
                                >
                                  ✓ 确认开辟
                                </button>
                                <button
                                  className="probe-btn-cancel"
                                  onClick={() => setBranchConfirmId(null)}
                                >
                                  取消
                                </button>
                              </div>
                            </div>
                          ) : (
                            <div className="probe-actions-stack">
                              {node.isViewing ? (
                                <div className="probe-viewing-badge">
                                  <span>👁️ 当前主窗口正在显示此节点</span>
                                </div>
                              ) : (
                                <button
                                  className="probe-btn-primary"
                                  onClick={() => {
                                    if (hasBackendNodes) {
                                      void viewAt(node.id);
                                    } else {
                                      notifyQuiet("请先开启故事，再查看历史快照。");
                                    }
                                    setProbeNodeId(null);
                                  }}
                                  title="切换主窗口视角至该历史快照"
                                >
                                  <span>👁️</span>
                                  <span>回溯定位至此快照</span>
                                </button>
                              )}

                              <div
                                className="probe-secondary-grid probe-secondary-grid--dynamic"
                                style={{ "--lr-probe-cols": node.canRegenerate && hasBackendNodes ? "1fr 1fr" : "1fr" } as React.CSSProperties}
                              >
                                <button
                                  className="probe-btn-sub"
                                  onClick={() => setBranchConfirmId(node.id)}
                                  title="从该快照分叉新时间线"
                                >
                                  <span>🌿</span>
                                  <span>开辟分支</span>
                                </button>
                                {node.canRegenerate && hasBackendNodes && (
                                  <button
                                    className="probe-btn-sub"
                                    onClick={() => {
                                      void regenerate(node.id);
                                      notifyQuiet(`🔄 正在重新生成【${node.tag}】的变体回复... ⚡`);
                                      setProbeNodeId(null);
                                    }}
                                    title="在兄弟节点上重跑大模型变体回复"
                                  >
                                    <span>🔄</span>
                                    <span>重生变体</span>
                                  </button>
                                )}
                              </div>
                            </div>
                          )}
                        </div>
                      )}
                    </div>
                  );
                }))}
            </div>
          </div>

          {/* 2. 角色卡库与添加/选择 (Characters Roster) */}
          <div
            id="view-characters"
            className="rail-view-panel"
            style={{ "--lr-panel-display": leftTab === "characters" ? "flex" : "none" } as React.CSSProperties}
          >
            <div className="roster-actions-bar">
              <button
                className="btn-primary-send roster-import-btn"
                onClick={() => setCharImportOpen(true)}
              >
                + 导入 / 创建角色卡
              </button>
            </div>

            {(() => {
              const totalCards = SELECTABLE_PRESET_KEYS.length + importedCards.length;
              return (
                <>
                  <div className="tree-lead-label">
                    <span>已收录角色卡 ({totalCards})</span>
                    <span className="tree-lead-hint">点击直接切换主角</span>
                  </div>

                  {totalCards === 0 && (
                    <div className="hud-empty-card" style={{ padding: "16px 12px", textAlign: "center" }}>
                      <div style={{ fontSize: "20px", marginBottom: "6px" }}>🎭</div>
                      <div style={{ fontSize: "12px", color: "var(--text-dim)" }}>暂无角色卡，请点击上方「+ 导入 / 创建角色卡」开始</div>
                    </div>
                  )}

                  {/* 用户已收录/导入的角色卡 */}
                  {importedCards.map((card) => {
                    const isSelected = currentView?.characterId === card.characterId || (currentView && card.cardId === currentView.characterId);
                    const count = sessions.filter(s => s.characterId === card.characterId || s.characterId === card.cardId).length;
                    return (
                      <CharacterCardRow
                        key={card.cardId}
                        card={card}
                        selected={!!isSelected}
                        sessionCount={count}
                        playerName={playerName}
                        confirming={confirmCardId === card.cardId}
                        onOpen={() => {
                          // 与命令面板共用同一套判定（有存档直接进故事，没有则预填开局弹窗）。
                          openCard(card);
                          if (typeof window !== "undefined" && window.innerWidth <= 768) {
                            toggleLeftRail();
                          }
                        }}
                        onAskDelete={() => setConfirmCardId(card.cardId)}
                        onCancelDelete={() => setConfirmCardId(null)}
                        onConfirmDelete={() => {
                          setConfirmCardId(null);
                          // 先删服务端再删缓存；失败时提示且缓存不动，避免出现“刷新后复活”的卡。
                          void deleteImportedCardRemote(card.cardId)
                            .then(() => {
                              setImportedCards(listImportedCards());
                              useUi.getState().notifyQuiet(`已从卡库移除「${card.name}」`);
                            })
                            .catch(() => useUi.getState().notifyQuiet(`移除「${card.name}」失败，请稍后重试`));
                        }}
                      />
                    );
                  })}

                  {/* 内置官方预设角色 */}
                  {SELECTABLE_PRESET_KEYS.map((presetKey) => {
                    const c = CHARACTER_PRESETS[presetKey];
                    const isActive = c.key === activeCharKey;
                    const count = sessions.filter((s) => resolveCharKeyFromSession(s) === c.key).length;
                    return (
                      <div
                        key={c.key}
                        className={`char-card-entry ${isActive ? "active" : ""}`}
                        id={`roster-card-${c.key}`}
                        onClick={() => {
                          void selectCharacter(c.key);
                          if (typeof window !== "undefined" && window.innerWidth <= 768) {
                            toggleLeftRail();
                          }
                        }}
                      >
                        <img src={c.avatar} alt={c.name} className="char-card-thumb" />
                        <div className="char-card-info">
                          <div className="char-card-name-row">
                            <span className="char-card-name">{c.name}</span>
                            <span className="char-card-badge">
                              {isActive ? "当前激活" : count > 0 ? `${count}个存档` : "待起步"}
                            </span>
                          </div>
                          <span className="char-card-desc">{c.role}</span>
                        </div>
                      </div>
                    );
                  })}
                </>
              );
            })()}
          </div>

          {/* 3. 世界书查看与管理 (Worldbook / Lorebook) */}
          <div
            id="view-lore"
            className="rail-view-panel"
            style={{ "--lr-panel-display": leftTab === "lore" ? "flex" : "none" } as React.CSSProperties}
          >
            {currentView ? (leftTab === "lore" && <LorebookPanel key={currentView.sessionId} view={currentView} />) : <>
            <div className="lore-search-row">
              <input
                type="text"
                className="lore-search-input"
                placeholder="检索世界书设定（支持触发词）..."
                value={loreFilter}
                onChange={(e) => setLoreFilter(e.target.value)}
              />
            </div>

            <div className="lore-lead-row">
              <span className="tree-lead-label">示例世界书词条 ({filteredLore.length})</span>
              <button
                className="btn-quiet lore-sample-btn"
                onClick={() => setWorldbookModalOpen(true)}
              >
                查看示例
              </button>
            </div>

            {filteredLore.map((item) => (
              <div
                key={item.id}
                className="lore-entry-card"
                onClick={() => setWorldbookModalOpen(true)}
              >
                <div className="lore-entry-top">
                  <span className="lore-entry-title">{item.title}</span>
                  <span className={`lore-toggle-pill ${item.status === "active" ? "active" : item.status === "always" ? "active" : ""}`}>
                    {item.status === "always" ? "常驻示例" : item.status === "active" ? "触发示例" : "条件示例"}
                  </span>
                </div>
                <div className="lore-keys-row">
                  {item.keys.map((k) => (
                    <span key={k} className="lore-key-tag">
                      {k}
                    </span>
                  ))}
                </div>
                <div className="lore-entry-snippet">{item.content}</div>
              </div>
            ))}
            </>}
          </div>

          {/* 4. 剧本会话库 (Sessions List) */}
          <div
            id="view-sessions"
            className="rail-view-panel"
            style={{ "--lr-panel-display": leftTab === "sessions" ? "flex" : "none" } as React.CSSProperties}
          >
            <div className="session-section-header">
              <div className="session-section-title">
                <span>历史故事会话</span>
                <span className="session-total-count">{sessions.length}</span>
              </div>
              <div className="session-segmented-control">
                <button
                  className={`session-segment-btn ${sessionFilter === "all" ? "active" : ""}`}
                  onClick={() => setSessionFilter("all")}
                >
                  全部 ({sessions.length})
                </button>
                <button
                  className={`session-segment-btn ${sessionFilter === "current" ? "active" : ""}`}
                  onClick={() => setSessionFilter("current")}
                >
                  仅 {char.shortName}
                </button>
              </div>
            </div>

            {sessions.length === 0 ? (
              <div className="session-empty-state">
                <div className="empty-icon">📜</div>
                <div className="empty-title">暂无历史故事会话</div>
                <div className="empty-desc">
                  点击下方按钮开辟，或直接在底部输入框发言开启全新故事。
                </div>
              </div>
            ) : (
              <div className="session-list-stack">
                {(sessionFilter === "current"
                  ? sessions.filter((s) => currentView ? s.characterId === currentView.characterId : resolveCharKeyFromSession(s) === activeCharKey)
                  : sessions
                ).map((s) => {
                  const isActive = pendingSessionId ? pendingSessionId === s.sessionId : currentView?.sessionId === s.sessionId;
                  const charKey = resolveCharKeyFromSession(s);
                  const storedCard = findCardByCharacterId(s.characterId) || findCardByName(s.title);
                  const sessionAvatar = getSessionAvatar(s.sessionId) || storedCard?.avatar || (currentView?.sessionId === s.sessionId ? (currentView.state?.characters && Object.values(currentView.state.characters).find(c => c.characterId !== "player")?.avatar) : undefined);
                  const sPreset = CHARACTER_PRESETS[charKey ?? "custom"] || CHARACTER_PRESETS.custom;
                  const resolvedAvatar = sessionAvatar || sPreset.avatar;
                  const roleTag = storedCard
                    ? (storedCard.shortName || storedCardTag(storedCard, playerName).slice(0, 16))
                    : (sPreset.role ? sPreset.role.split("·")[0].trim() : sPreset.shortName);
                  return (
                    <div
                      key={s.sessionId}
                      className={`session-entry-card ${isActive ? "active" : ""}`}
                      onClick={() => {
                        void openSession(s.sessionId);
                        if (typeof window !== "undefined" && window.innerWidth <= 768) {
                          toggleLeftRail();
                        }
                      }}
                    >
                      <div className="session-card-main">
                        <div className="session-avatar-wrapper">
                          <img
                            src={resolvedAvatar}
                            alt={storedCard?.name || sPreset.name}
                            className="session-avatar-img"
                          />
                          {isActive && <span className="session-avatar-indicator" />}
                        </div>

                        <div className="session-card-content">
                          <div className="session-card-header-row">
                            <span className="session-title-text" title={s.title}>
                              {s.title}
                            </span>
                            {/* 确认态收起徽标：它会把标题挤成省略号 */}
                            {confirmDeleteId === s.sessionId ? null : (
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
                            )}
                          </div>

                          {/* 确认态下让位给按钮：角色/时间这一行暂时收起 */}
                          {confirmDeleteId === s.sessionId ? (
                            <span className="row-confirm-hint">不可恢复</span>
                          ) : (
                            <div className="session-card-sub-row">
                              <span className="session-role-tag" title={sPreset.name}>
                                {roleTag}
                              </span>
                              <span className="session-time-text">
                                {formatSessionTime(s.createdAt)}
                              </span>
                            </div>
                          )}
                        </div>

                        {/* 行尾删除控件：与内容同处一行，两步确认后才真正删除 */}
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
                              );
                            });
                          }}
                        />
                      </div>
                    </div>
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
                <span className="session-primary-icon">✨</span>
                <span>与「{char.shortName}」开启新故事</span>
              </button>

              <div className="session-utility-grid">
                <button
                  className="session-utility-btn"
                  onClick={() => {
                    void exportSession();
                    notifyQuiet("正在打包导出 .tavernpack 会话剧本... 📦");
                  }}
                  disabled={!currentView}
                  title="导出当前会话为 .tavernpack 剧情包"
                >
                  <span>📦 导出剧本</span>
                </button>

                <label
                  className="session-utility-btn"
                  title="导入 .tavernpack 或 ZIP 剧本包"
                >
                  <span>📥 导入剧本</span>
                  <input
                    type="file"
                    accept=".tavernpack,.zip"
                    className="session-import-input"
                    onChange={(e) => {
                      const file = e.target.files?.[0];
                      if (file) {
                        void importSession(file).catch(() => {});
                        notifyQuiet(`正在导入剧本包：${file.name}... ⏳`);
                      }
                      e.target.value = "";
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
                  👥 角色库
                </button>
                <span className="link-divider">·</span>
                <button
                  type="button"
                  className="session-footer-link-btn"
                  onClick={() => setCharImportOpen(true)}
                  title="导入新角色卡 PNG 或 JSON"
                >
                  ➕ 导入角色卡
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </aside>
  );
};

