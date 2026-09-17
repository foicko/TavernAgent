import type { ReactNode } from "react";
import { IconButton } from "./Button";
import "./Banner.css";

export type BannerTone = "info" | "ok" | "warn" | "error";

const ICONS: Record<BannerTone, string> = {
  info: "💡",
  ok: "✓",
  warn: "!",
  error: "!",
};

export interface BannerProps {
  children: ReactNode;
  tone?: BannerTone;
  /** 传了才显示关闭按钮。 */
  onDismiss?: () => void;
  className?: string;
}

/**
 * 行内提示条。error 用 role="alert" 让读屏立即播报，其余用 role="status"，
 * 与既有测试对 role="alert" 的断言保持一致。
 */
export function Banner({ children, tone = "info", onDismiss, className = "" }: BannerProps) {
  return (
    <div
      className={["ui-banner", `ui-banner--${tone}`, className].filter(Boolean).join(" ")}
      role={tone === "error" ? "alert" : "status"}
    >
      <span className="ui-banner__icon" aria-hidden="true">
        {ICONS[tone]}
      </span>
      <div className="ui-banner__body">{children}</div>
      {onDismiss ? (
        <IconButton label="关闭提示" size="sm" className="ui-banner__dismiss" onClick={onDismiss}>
          ✕
        </IconButton>
      ) : null}
    </div>
  );
}
