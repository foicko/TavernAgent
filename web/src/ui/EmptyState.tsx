import type { ReactNode } from "react";
import "./EmptyState.css";

export interface EmptyStateProps {
  title: ReactNode;
  description?: ReactNode;
  icon?: ReactNode;
  action?: ReactNode;
  className?: string;
  /** sm 用于侧栏等狭小容器；md（默认）用于弹窗/主区域。 */
  size?: "sm" | "md";
  /** dashed（默认）：虚线卡片；plain：无边框无底色（如导演工作区）。 */
  variant?: "dashed" | "plain";
}

export function EmptyState({
  title,
  description,
  icon,
  action,
  className = "",
  size = "md",
  variant = "dashed",
}: EmptyStateProps) {
  const classes = ["ui-empty", `ui-empty--${variant}`, `ui-empty--${size}`, className].filter(Boolean).join(" ");
  return (
    <div className={classes}>
      {icon ? (
        <span className="ui-empty__icon" aria-hidden="true">
          {icon}
        </span>
      ) : null}
      <span className="ui-empty__title">{title}</span>
      {description ? <span className="ui-empty__description">{description}</span> : null}
      {action ? <div className="ui-empty__action">{action}</div> : null}
    </div>
  );
}
