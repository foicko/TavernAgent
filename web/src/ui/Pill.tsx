import type { ReactNode } from "react";
import "./Pill.css";

export type PillTone = "neutral" | "accent" | "success" | "warning" | "info" | "danger";

export interface PillProps {
  children: ReactNode;
  tone?: PillTone;
  /** 长文本用：超出宽度省略而不是撑破布局。 */
  truncate?: boolean;
  title?: string;
  className?: string;
}

export function Pill({ children, tone = "neutral", truncate = false, title, className = "" }: PillProps) {
  const classes = ["ui-pill", tone !== "neutral" ? `ui-pill--${tone}` : "", className]
    .filter(Boolean)
    .join(" ");
  return (
    <span className={classes} title={title}>
      <span className={truncate ? "ui-pill__truncate" : undefined}>{children}</span>
    </span>
  );
}

/** 更小的标签，用于列表行内的次要信息。 */
export function Tag({ children, tone = "neutral", title, className = "" }: Omit<PillProps, "truncate">) {
  return (
    <Pill tone={tone} title={title} className={["ui-pill--sm", className].filter(Boolean).join(" ")}>
      {children}
    </Pill>
  );
}

export interface StatusDotProps {
  tone?: PillTone;
  /** 空心点：表示"未连接/占位"而不是某个状态色。 */
  hollow?: boolean;
  title?: string;
  className?: string;
}

export function StatusDot({ tone = "neutral", hollow = false, title, className = "" }: StatusDotProps) {
  const classes = [
    "ui-status-dot",
    tone !== "neutral" ? `ui-status-dot--${tone}` : "",
    hollow ? "ui-status-dot--hollow" : "",
    className,
  ]
    .filter(Boolean)
    .join(" ");
  return <span className={classes} title={title} aria-hidden="true" />;
}
