import React from "react";
import { Icon } from "../ui/Icon";
import { EmptyState } from "../ui/EmptyState";
import { ListRow } from "../ui/ListRow";
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
        <EmptyState
          size="sm"
          icon={<Icon name="user" size={22} />}
          title="暂无角色卡"
          description="请点击上方「+ 导入 / 创建角色卡」开始"
        />
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
                  useUi.getState().notifyQuiet(`已从卡库移除「${card.name}」`, "success");
                })
                .catch(() => useUi.getState().notifyQuiet(`移除「${card.name}」失败，请稍后重试`, "danger"));
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
          <ListRow
            key={c.key}
            id={`roster-card-${c.key}`}
            className="char-card-entry"
            leading={<img src={c.avatar} alt={c.name} className="char-card-thumb" />}
            title={<span className="char-card-name">{c.name}</span>}
            subtitle={<span className="char-card-desc">{c.role}</span>}
            meta={
              <span className="char-card-badge">
                {isActive ? "当前激活" : count > 0 ? `${count}个存档` : "待起步"}
              </span>
            }
            selected={isActive}
            onSelect={() => {
              void selectCharacter(c.key);
              if (typeof window !== "undefined" && window.innerWidth <= 768) {
                toggleLeftRail();
              }
            }}
            ariaLabel={`切换到角色 ${c.name}`}
          />
        );
      })}
    </div>
  );
};
