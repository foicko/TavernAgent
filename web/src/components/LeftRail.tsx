// LeftRail: 左侧多维导航栏 (52px 图标停靠轨 / 224px 完整内容，见 tokens.css 的 --rail-left-*)
import React, { useEffect, useMemo, useState } from "react";
import { useShallow } from "zustand/react/shallow";
import { useUi, type LeftSubtab } from "../stores/uiStore";
import { useStory } from "../stores/storyStore";
import { INITIAL_LORE_ENTRIES } from "../lib/characterPresets";
import { StoryMapModal } from "./StoryMapModal";
import { characterPresentation } from "../lib/characterPresentation";
import { DEFAULT_PLAYER_NAME } from "../lib/characterMacros";
import { truncateText } from "../lib/truncateText";
import { LorebookPanel } from "./LorebookPanel";
import { LeftRailTreePanel, type DisplayTreeNode } from "./LeftRailTreePanel";
import { LeftRailCharactersPanel } from "./LeftRailCharactersPanel";
import { LeftRailSessionsPanel } from "./LeftRailSessionsPanel";
import {
  listImportedCards,
  refreshImportedCards,
} from "../lib/characterCardStore";
import { Icon } from "../ui/Icon";
import { Tabs } from "../ui/Tabs";
import "./LeftRail.css";

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
    sessionsError,
    loadSessions,
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
    packError,
    clearPackError,
  } = useStory(useShallow(s => ({
    sessions: s.sessions, sessionsError: s.sessionsError, loadSessions: s.loadSessions,
    openSession: s.openSession, deleteSession: s.deleteSession, selectCharacter: s.selectCharacter,    startNewSessionForCurrentChar: s.startNewSessionForCurrentChar, view: s.view, hud: s.hud,
    viewAt: s.viewAt, forkFrom: s.forkFrom, regenerate: s.regenerate,
    exportSession: s.exportSession, importSession: s.importSession,
    sessionLoading: s.sessionLoading, pendingSessionId: s.pendingSessionId,
    packError: s.packError, clearPackError: s.clearPackError,
  })));

  const char = characterPresentation(activeCharKey, currentView);
  const playerName = currentView?.state?.characters?.["player"]?.name || DEFAULT_PLAYER_NAME;
  const [probeNodeId, setProbeNodeId] = useState<string | null>(null);
  const [storyMapOpen, setStoryMapOpen] = useState(false);
  const [branchConfirmId, setBranchConfirmId] = useState<string | null>(null);
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);
  const [confirmCardId, setConfirmCardId] = useState<string | null>(null);
  const [importedCards, setImportedCards] = useState(() => listImportedCards());
  const [loreFilter, setLoreFilter] = useState("");
  const [sessionFilter, setSessionFilter] = useState<"all" | "current">("all");

  useEffect(() => {
    let alive = true;
    void refreshImportedCards()
      .then(() => { if (alive) setImportedCards(listImportedCards()); })
      .catch(() => {});
    return () => { alive = false; };
  }, []);

  useEffect(() => { setImportedCards(listImportedCards()); }, [charImportOpen]);

  const liveRel = hud?.relationships
    ? hud.relationships[`npc_${activeCharKey}`] ||
      hud.relationships[activeCharKey ?? ""] ||
      Object.values(hud.relationships)[0]
    : undefined;

  const hasBackendNodes = !!(currentView && currentView.nodes && currentView.nodes.length > 0);
  // Parsing every node's contentJson is O(n); memoize so unrelated re-renders
  // (e.g. typing in the composer) don't repeat it.
  const displayTreeNodes: DisplayTreeNode[] = useMemo(() => {
    if (hasBackendNodes && currentView?.nodes) {
      return currentView.nodes.map((n, idx) => {
        let desc = "剧情节点";
        const isOpening = n.kind === "root";
        try {
          const parsed = JSON.parse(n.contentJson);
          if (isOpening) {
            desc = parsed.openingText || "命运起点 · 酒馆初见";
          } else {
            const firstBlock = parsed.blocks?.[0]?.text;
            desc = parsed.inputText
              ? `玩家: 「${truncateText(String(parsed.inputText), 36)}」`
              : typeof firstBlock === "string" && firstBlock ? truncateText(firstBlock, 40) : "文学演义片段";
          }
        } catch {
          desc = isOpening ? "命运起点" : "文学演义片段";
        }

        const isHead = n.nodeId === currentView.branch.headNodeId;
        const isViewing = n.nodeId === currentView.viewNodeId;
        const tag = isOpening ? "命运起点" : `回合 #${n.turnNumber ?? idx}`;

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
      });
    }
    return (currentView ? [] : char.treeNodes).map((n) => ({
      id: String(n.id),
      tag: n.tag,
      desc: n.desc,
      isActive: !!n.isActive,
      isViewing: false,
      isFork: !!n.isFork,
      aff: n.aff,
      trust: n.trust,
      alert: n.alert,
      canRegenerate: !n.tag.includes("起点"),
    }));
  }, [hasBackendNodes, currentView, liveRel, char.treeNodes]);

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
      {storyMapOpen && currentView && (
        <StoryMapModal
          key={currentView.sessionId}
          view={currentView}
          onClose={() => setStoryMapOpen(false)}
        />
      )}

      {/* Collapsed Icon View (Dock Mode) */}
      <div className="rail-collapsed-view">
        <button className="collapsed-icon-btn" onClick={toggleLeftRail} title="展开左侧栏 (Ctrl+[)">
          <Icon name="chevronRight" size={16} />
        </button>
        <div className="rail-dock-divider" />
        {([
          ["tree", "因果分支树", "leaf"],
          ["characters", "角色档案库", "user"],
          ["lore", "世界书词条", "book"],
          ["sessions", "剧本会话史", "clock"],
        ] as Array<[LeftSubtab, string, "leaf" | "user" | "book" | "clock"]>).map(([tab, label, icon]) => (
          <div
            key={tab}
            className={`rail-dock-btn-wrap ${leftTab === tab ? "active" : ""}`}
            title={`${label} (Alt+${tab === "tree" ? 1 : tab === "characters" ? 2 : tab === "lore" ? 3 : 4})`}
          >
            <button
              className={`collapsed-icon-btn ${leftTab === tab ? "active" : ""}`}
              onClick={() => openLeftTabAndExpand(tab)}
              aria-label={label}
            >
              <Icon name={icon} size={15} />
            </button>
          </div>
        ))}
      </div>

      {/* Full Expanded View */}
      <div className="rail-full-view">
        <div className="rail-tab-header">
          <Tabs
            label="左侧栏维度切换"
            className="rail-subtabs"
            size="sm"
            fill
            value={leftTab}
            onChange={(next) => switchLeftTab(next)}
            items={[
              { value: "tree", id: "subtab-btn-tree", label: "脉络" },
              { value: "characters", id: "subtab-btn-characters", label: "角色" },
              { value: "lore", id: "subtab-btn-lore", label: "世界" },
              { value: "sessions", id: "subtab-btn-sessions", label: "会话" },
            ]}
          />
          <button
            className="rail-collapse-handle-btn"
            onClick={toggleLeftRail}
            title="收起左侧栏 (Ctrl+[)"
            aria-label="收起左侧栏"
          >
            <Icon name="chevronLeft" size={15} />
          </button>
        </div>

        <div className="rail-scroll-body">
          {/* 1. 因果分支树 */}
          <LeftRailTreePanel
            style={{ display: leftTab === "tree" ? "flex" : "none" }}
            currentView={currentView}
            hasBackendNodes={hasBackendNodes}
            displayTreeNodes={displayTreeNodes}
            probeNodeId={probeNodeId}
            setProbeNodeId={setProbeNodeId}
            branchConfirmId={branchConfirmId}
            setBranchConfirmId={setBranchConfirmId}
            forkFrom={forkFrom}
            viewAt={viewAt}
            regenerate={regenerate}
            notifyQuiet={notifyQuiet}
            liveRel={liveRel}
            onOpenStoryMap={() => setStoryMapOpen(true)}
          />

          {/* 2. 角色库 */}
          <LeftRailCharactersPanel
            style={{ display: leftTab === "characters" ? "flex" : "none" }}
            importedCards={importedCards}
            setImportedCards={setImportedCards}
            currentView={currentView}
            sessions={sessions}
            playerName={playerName}
            confirmCardId={confirmCardId}
            setConfirmCardId={setConfirmCardId}
            activeCharKey={activeCharKey}
            selectCharacter={selectCharacter}
            setCharImportOpen={setCharImportOpen}
            toggleLeftRail={toggleLeftRail}
          />

          {/* 3. 世界书 */}
          <div
            id="view-lore"
            className="rail-view-panel"
            style={{ display: leftTab === "lore" ? "flex" : "none" }}
          >
            {currentView ? (
              leftTab === "lore" && <LorebookPanel key={currentView.sessionId} view={currentView} />
            ) : (
              <>
                <div className="lore-search-row">
                  <input
                    type="text"
                    className="lore-search-input"
                    aria-label="检索世界书设定"
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
                      <span className={`lore-toggle-pill ${item.status === "active" || item.status === "always" ? "active" : ""}`}>
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
              </>
            )}
          </div>

          {/* 4. 会话库 */}
          <LeftRailSessionsPanel
            style={{ display: leftTab === "sessions" ? "flex" : "none" }}
            sessions={sessions}
            sessionsError={sessionsError}
            loadSessions={loadSessions}
            sessionFilter={sessionFilter}
            setSessionFilter={setSessionFilter}
            currentView={currentView}
            pendingSessionId={pendingSessionId}
            sessionLoading={sessionLoading}
            activeCharKey={activeCharKey}
            char={char}
            playerName={playerName}
            confirmDeleteId={confirmDeleteId}
            setConfirmDeleteId={setConfirmDeleteId}
            openSession={openSession}
            deleteSession={deleteSession}
            startNewSessionForCurrentChar={startNewSessionForCurrentChar}
            exportSession={exportSession}
            importSession={importSession}
            packError={packError}
            clearPackError={clearPackError}
            switchLeftTab={switchLeftTab}
            setCharImportOpen={setCharImportOpen}
            toggleLeftRail={toggleLeftRail}
            notifyQuiet={notifyQuiet}
          />
        </div>
      </div>
    </aside>
  );
};
