// RowDeleteControl：行内两步删除控件。
//
// 未确认时只显示一个图标按钮；确认后换成"确认删除 / 取消"。放在行尾而不是塞进行内
// 主按钮里，是为了不改变行的点击语义；所有按钮都阻止冒泡，否则点删除会连带触发
// 切换故事 / 切换主角。
//
// 两个列表（角色卡库、故事列表）共用同一个控件，避免各写一套确认逻辑。
import type { MouseEvent } from "react";
import "./RowDeleteControl.css";

export interface RowDeleteControlProps {
  confirming: boolean;
  /** 未确认态按钮的可访问名（例如「删除故事 某某」）。 */
  idleLabel: string;
  confirmLabel: string;
  /** 悬停提示：说清后果（是否可恢复）。 */
  confirmTitle: string;
  onAsk: () => void;
  onCancel: () => void;
  onConfirm: () => void;
}

export function RowDeleteControl({
  confirming,
  idleLabel,
  confirmLabel,
  confirmTitle,
  onAsk,
  onCancel,
  onConfirm,
}: RowDeleteControlProps) {
  const stop = (event: MouseEvent, action: () => void) => {
    event.stopPropagation();
    action();
  };

  return (
    <div className="row-delete-actions">
      {confirming ? (
        <>
          <button
            type="button"
            className="row-delete-btn row-delete-btn--confirm"
            title={confirmTitle}
            onClick={(event) => stop(event, onConfirm)}
          >
            {confirmLabel}
          </button>
          <button
            type="button"
            className="row-delete-btn"
            onClick={(event) => stop(event, onCancel)}
          >
            取消
          </button>
        </>
      ) : (
        <button
          type="button"
          className="row-delete-btn row-delete-btn--icon"
          aria-label={idleLabel}
          title={idleLabel}
          onClick={(event) => stop(event, onAsk)}
        >
          🗑
        </button>
      )}
    </div>
  );
}
