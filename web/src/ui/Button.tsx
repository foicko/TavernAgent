import type { ButtonHTMLAttributes, ReactNode } from "react";
import "./Button.css";

export type ButtonVariant = "primary" | "quiet" | "ghost" | "danger";
export type ButtonSize = "sm" | "md";

export interface ButtonProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, "className"> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  /** 提交中：禁用并显示转圈，同时置 aria-busy。 */
  loading?: boolean;
  /** 占满所在容器宽度。 */
  block?: boolean;
  className?: string;
  children?: ReactNode;
}

export function Button({
  variant = "quiet",
  size = "md",
  loading = false,
  block = false,
  className = "",
  disabled,
  type = "button",
  children,
  ...rest
}: ButtonProps) {
  const classes = [
    "ui-btn",
    `ui-btn--${variant}`,
    `ui-btn--${size}`,
    block ? "ui-btn--block" : "",
    className,
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <button
      {...rest}
      type={type}
      className={classes}
      disabled={disabled || loading}
      aria-busy={loading || undefined}
    >
      {loading && <span className="ui-btn__spinner" aria-hidden="true" />}
      {children}
    </button>
  );
}

export interface IconButtonProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, "className"> {
  /** 可访问名，同时作为 title 的默认值（必须给，避免无字图标按钮）。 */
  label: string;
  variant?: ButtonVariant;
  size?: ButtonSize;
  className?: string;
  children?: ReactNode;
}

export function IconButton({
  label,
  variant = "ghost",
  size = "md",
  className = "",
  title,
  type = "button",
  children,
  ...rest
}: IconButtonProps) {
  const classes = ["ui-btn", "ui-icon-btn", `ui-btn--${variant}`, `ui-btn--${size}`, className]
    .filter(Boolean)
    .join(" ");

  return (
    <button {...rest} type={type} className={classes} aria-label={label} title={title ?? label}>
      {children}
    </button>
  );
}
