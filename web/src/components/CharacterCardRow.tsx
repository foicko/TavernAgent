// CharacterCardRow：角色卡库中的一行（含行内两步删除）。
//
// 从 LeftRail 抽出来：一是"一张卡 + 行尾删除"自成一个展示单元，二是 LeftRail 已经
// 触到架构门禁的文件行数上限。所有类名与 id 原样保留，样式仍在 styles.css/LeftRail.css。
import { storedCardTag } from "../lib/characterPresentation";
import type { StoredCharacterCard } from "../lib/characterCardStore";
import { Icon } from "../ui/Icon";
import { ListRow } from "../ui/ListRow";
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
    <ListRow
      id={`roster-card-${card.cardId}`}
      className={`char-card-entry${confirming ? " confirming" : ""}`}
      leading={
        card.avatar ? (
          <img src={card.avatar} alt={card.name} className="char-card-thumb" />
        ) : (
          <span className="char-card-thumb char-card-thumb--fallback" aria-hidden="true">
            <Icon name="mask" size={18} />
          </span>
        )
      }
      title={<span className="char-card-name" title={card.name}>{card.shortName || card.name}</span>}
      subtitle={
        confirming ? (
          <span className="row-confirm-hint">
            从卡库移除{sessionCount > 0 ? `；${sessionCount} 个存档仍可继续游玩` : ""}
          </span>
        ) : (
          <span className="char-card-desc">{storedCardTag(card, playerName)}</span>
        )
      }
      meta={confirming ? undefined : <span className="char-card-badge">{badge}</span>}
      selected={selected}
      disabled={confirming}
      onSelect={onOpen}
      ariaLabel={`打开角色卡 ${card.name}`}
      actions={
        <RowDeleteControl
          confirming={confirming}
          idleLabel={`删除角色卡 ${card.name}`}
          confirmLabel="确认移除"
          confirmTitle="从角色卡库移除（不影响已有存档）"
          onAsk={onAskDelete}
          onCancel={onCancelDelete}
          onConfirm={onConfirmDelete}
        />
      }
    />
  );
}
