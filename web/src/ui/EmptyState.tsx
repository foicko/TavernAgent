import type { ReactNode } from "react";
import "./EmptyState.css";

export interface EmptyStateProps {
  title: ReactNode;
  description?: ReactNode;
  icon?: ReactNode;
  action?: ReactNode;
  className?: string;
}

export function EmptyState({ title, description, icon, action, className = "" }: EmptyStateProps) {
  return (
    <div className={["ui-empty", className].filter(Boolean).join(" ")}>
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
