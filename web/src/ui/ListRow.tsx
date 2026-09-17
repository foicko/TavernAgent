import type { ReactNode } from "react";
import "./ListRow.css";

export interface ListRowProps {
  title: ReactNode;
  subtitle?: ReactNode;
  /** 行内次要信息（标签/状态点），显示在标题右侧。 */
  meta?: ReactNode;
  /** 行尾操作按钮；与整行按钮并列，不会嵌套。 */
  actions?: ReactNode;
  selected?: boolean;
  disabled?: boolean;
  onSelect?: () => void;
  /** 覆盖可访问名（默认取 title 的文本）。 */
  ariaLabel?: string;
  className?: string;
}

export function ListRow({
  title,
  subtitle,
  meta,
  actions,
  selected = false,
  disabled = false,
  onSelect,
  ariaLabel,
  className = "",
}: ListRowProps) {
  const classes = [
    "ui-list-row",
    selected ? "ui-list-row--selected" : "",
    disabled ? "ui-list-row--disabled" : "",
    className,
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <div className={classes}>
      <button
        type="button"
        className="ui-list-row__main"
        aria-label={ariaLabel}
        aria-current={selected ? "true" : undefined}
        disabled={disabled || !onSelect}
        onClick={onSelect}
      >
        <span className="ui-list-row__text">
          <span className="ui-list-row__title">{title}</span>
          {subtitle ? <span className="ui-list-row__subtitle">{subtitle}</span> : null}
        </span>
        {meta ? <span className="ui-list-row__meta">{meta}</span> : null}
      </button>
      {actions ? <div className="ui-list-row__actions">{actions}</div> : null}
    </div>
  );
}
