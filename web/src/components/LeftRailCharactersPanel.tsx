import React from "react";
import { Icon } from "../ui/Icon";
import { CharacterCardRow } from "./CharacterCardRow";
import { CHARACTER_PRESETS, SELECTABLE_PRESET_KEYS } from "../lib/characterPresets";
import { openCard } from "../lib/cardLaunch";
import { listImportedCards, deleteImportedCardRemote, type StoredCharacterCard } from "../lib/characterCardStore";
import { useUi } from "../stores/uiStore";
import { resolveCharKeyFromSession } from "../stores/storyStore";
import type { SessionView, SessionSummary } from "../app/types";

interface LeftRailCharactersPanelProps {
  importedCards: StoredCharacterCard[];
  setImportedCards: (cards: StoredCharacterCard[]) => void;
  currentView: SessionView | null;
  sessions: SessionSummary[];
  playerName: string;
  confirmCardId: string | null;
  setConfirmCardId: (id: string | null) => void;
  activeCharKey: string | null;
  selectCharacter: (key: string) => Promise<unknown>;
  setCharImportOpen: (open: boolean) => void;
  toggleLeftRail: () => void;
  activateOnKey: (run: () => void) => (event: React.KeyboardEvent) => void;
  style?: React.CSSProperties;
}

export const LeftRailCharactersPanel: React.FC<LeftRailCharactersPanelProps> = ({
  importedCards,
  setImportedCards,
  currentView,
  sessions,
  playerName,
  confirmCardId,
  setConfirmCardId,
  activeCharKey,
  selectCharacter,
  setCharImportOpen,
  toggleLeftRail,
  activateOnKey,
  style,
}) => {
  const totalCards = SELECTABLE_PRESET_KEYS.length + importedCards.length;

  return (
    <div id="view-characters" className="rail-view-panel" style={style}>
      <div className="roster-actions-bar">
        <button
          className="btn-primary-send roster-import-btn"
          onClick={() => setCharImportOpen(true)}
        >
          + 导入 / 创建角色卡
        </button>
      </div>

      <div className="tree-lead-label">
        <span>已收录角色卡 ({totalCards})</span>
        <span className="tree-lead-hint">点击直接切换主角</span>
      </div>

      {totalCards === 0 && (
        <div className="hud-empty-card" style={{ padding: "16px 12px", textAlign: "center" }}>
          <div style={{ marginBottom: "6px", color: "var(--text-dim)" }}><Icon name="user" size={22} /></div>
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
              openCard(card);
              if (typeof window !== "undefined" && window.innerWidth <= 768) {
                toggleLeftRail();
              }
            }}
            onAskDelete={() => setConfirmCardId(card.cardId)}
            onCancelDelete={() => setConfirmCardId(null)}
            onConfirmDelete={() => {
              setConfirmCardId(null);
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
            role="button"
            tabIndex={0}
            aria-pressed={isActive}
            aria-label={`切换到角色 ${c.name}`}
            onClick={() => {
              void selectCharacter(c.key);
              if (typeof window !== "undefined" && window.innerWidth <= 768) {
                toggleLeftRail();
              }
            }}
            onKeyDown={activateOnKey(() => {
              void selectCharacter(c.key);
              if (typeof window !== "undefined" && window.innerWidth <= 768) {
                toggleLeftRail();
              }
            })}
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
    </div>
  );
};
