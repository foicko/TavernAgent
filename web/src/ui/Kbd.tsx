import type { ReactNode } from "react";
import "./Kbd.css";

export interface KbdProps {
  children: ReactNode;
  className?: string;
}

/**
 * 键盘按键提示（叶子原语）：等宽小字 + 发丝线底座。
 * 只做呈现，不承担交互；组合键用多个 <Kbd> 并排（`<Kbd>Ctrl</Kbd><Kbd>K</Kbd>`）。
 */
export function Kbd({ children, className = "" }: KbdProps) {
  return <kbd className={["ui-kbd", className].filter(Boolean).join(" ")}>{children}</kbd>;
}
