// CharacterCardRow：角色卡库中的一行（含行内两步删除）。
//
// 从 LeftRail 抽出来：一是"一张卡 + 行尾删除"自成一个展示单元，二是 LeftRail 已经
// 触到架构门禁的文件行数上限。所有类名与 id 原样保留，样式仍在 styles.css/LeftRail.css。
import { storedCardTag } from "../lib/characterPresentation";
import type { StoredCharacterCard } from "../lib/characterCardStore";
import { RowDeleteControl } from "./RowDeleteControl";
import "./CharacterCardRow.css";

export interface CharacterCardRowProps {
  card: StoredCharacterCard;
  /** 是否是当前主角的卡。 */
  selected: boolean;
  /** 该卡名下已有多少存档（删除时用来提示"不影响已有故事"）。 */
  sessionCount: number;
  playerName: string;
  confirming: boolean;
  onOpen: () => void;
  onAskDelete: () => void;
  onCancelDelete: () => void;
  onConfirmDelete: () => void;
}

export function CharacterCardRow({
  card,
  selected,
  sessionCount,
  playerName,
  confirming,
  onOpen,
  onAskDelete,
  onCancelDelete,
  onConfirmDelete,
}: CharacterCardRowProps) {
  const badge = selected
    ? "当前主角"
    : sessionCount > 0
      ? `${sessionCount}个存档`
      : card.format
        ? card.format.toUpperCase()
        : "已收录";

  return (
    <div
      className={`char-card-entry ${selected ? "active" : ""} ${confirming ? "confirming" : ""}`}
      id={`roster-card-${card.cardId}`}
      onClick={() => {
        if (confirming) return; // 确认态下避免误触切换主角
        onOpen();
      }}
    >
      {card.avatar ? (
        <img src={card.avatar} alt={card.name} className="char-card-thumb" />
      ) : (
        <div className="char-card-thumb char-card-thumb--fallback" aria-hidden="true">
          🎭
        </div>
      )}
      <div className="char-card-info">
        <div className="char-card-name-row">
          <span className="char-card-name" title={card.name}>{card.shortName || card.name}</span>
          {/* 确认态下让位给按钮：徽标会把它挤成竖排 */}
          {!confirming ? <span className="char-card-badge">{badge}</span> : null}
        </div>
        {confirming ? (
          <span className="row-confirm-hint">
            从卡库移除{sessionCount > 0 ? `；${sessionCount} 个存档仍可继续游玩` : ""}
          </span>
        ) : (
          <span className="char-card-desc">{storedCardTag(card, playerName)}</span>
        )}
      </div>
      <RowDeleteControl
        confirming={confirming}
        idleLabel={`删除角色卡 ${card.name}`}
        confirmLabel="确认移除"
        confirmTitle="从角色卡库移除（不影响已有存档）"
        onAsk={onAskDelete}
        onCancel={onCancelDelete}
        onConfirm={onConfirmDelete}
      />
    </div>
  );
}
