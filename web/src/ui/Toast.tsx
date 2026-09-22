import { useMemo, type CSSProperties, type ReactNode } from "react";
import { useExitList } from "../lib/useExitTransition";
import { Icon } from "./Icon";
import { IconButton } from "./Button";
import "./Toast.css";

export type ToastTone = "neutral" | "success" | "warning" | "danger";

export interface ToastItem {
  id: string;
  text: ReactNode;
  tone?: ToastTone;
}

export interface ToastStackProps {
  toasts: ReadonlyArray<ToastItem>;
  /** 传了才显示关闭按钮（键盘用户需要）。 */
  onDismiss?: (id: string) => void;
  /** 同屏最多显示条数，多的只保留最新。默认 3。 */
  max?: number;
  className?: string;
}

const CLOSE_MS = 350;
const keyOf = (toast: ToastItem) => toast.id;

/**
 * 吐司堆叠（纯呈现，状态由调用方持有）：底部升起，新的在最前、旧的向后缩退
 * （beui animated-toast-stack / transitions.dev 22-toast：进 350ms 慢、出 350ms 淡）。
 * role="status" + aria-live 让读屏播报但不打断。
 */
export function ToastStack({ toasts, onDismiss, max = 3, className = "" }: ToastStackProps) {
  const visible = useMemo(() => toasts.slice(-max), [toasts, max]);
  const entries = useExitList(visible, keyOf, CLOSE_MS);
  if (entries.length === 0) return null;
  // 最新的一条（数组末尾）深度为 0，越早的条目深度越大、缩退越多。
  const openKeys = entries.filter((entry) => entry.state === "open").map((entry) => entry.key);

  return (
    <div className={["ui-toast-stack", className].filter(Boolean).join(" ")} role="status" aria-live="polite">
      {entries.map((entry) => {
        const position = openKeys.indexOf(entry.key);
        const depth = position === -1 ? 0 : openKeys.length - 1 - position;
        const tone = entry.item.tone ?? "neutral";
        return (
          <div
            key={entry.key}
            className={["ui-toast", tone !== "neutral" ? `ui-toast--${tone}` : ""].filter(Boolean).join(" ")}
            data-state={entry.state}
            style={{ "--toast-depth": depth } as CSSProperties}
          >
            {tone !== "neutral" ? (
              <span className="ui-toast__icon" aria-hidden="true">
                <Icon name={tone === "success" ? "check" : "alert"} size={13} />
              </span>
            ) : null}
            <span className="ui-toast__text">{entry.item.text}</span>
            {onDismiss ? (
              <IconButton label="关闭通知" size="sm" className="ui-toast__close" onClick={() => onDismiss(entry.item.id)}>
                <Icon name="close" size={12} />
              </IconButton>
            ) : null}
          </div>
        );
      })}
    </div>
  );
}
