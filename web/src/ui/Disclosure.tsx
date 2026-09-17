import { useId, useState, type ReactNode } from "react";
import "./Disclosure.css";

export interface DisclosureProps {
  summary: ReactNode;
  children: ReactNode;
  /** 收起时显示在右侧的当前值，例如"温度 0.8 · 窗口 128k"。 */
  value?: ReactNode;
  /** 受控用法；不传则自己管开合。 */
  open?: boolean;
  defaultOpen?: boolean;
  onOpenChange?: (open: boolean) => void;
  className?: string;
}

export function Disclosure({
  summary,
  children,
  value,
  open,
  defaultOpen = false,
  onOpenChange,
  className = "",
}: DisclosureProps) {
  const [innerOpen, setInnerOpen] = useState(defaultOpen);
  const isOpen = open ?? innerOpen;
  const bodyId = useId();

  const toggle = () => {
    const next = !isOpen;
    if (open === undefined) setInnerOpen(next);
    onOpenChange?.(next);
  };

  return (
    <div className={["ui-disclosure", className].filter(Boolean).join(" ")}>
      <button
        type="button"
        className="ui-disclosure__trigger"
        aria-expanded={isOpen}
        aria-controls={bodyId}
        onClick={toggle}
      >
        <span className={["ui-disclosure__caret", isOpen ? "ui-disclosure__caret--open" : ""].filter(Boolean).join(" ")} aria-hidden="true">
          ▶
        </span>
        <span className="ui-disclosure__summary">{summary}</span>
        <span className="ui-disclosure__spacer" />
        {value ? <span className="ui-disclosure__value">{value}</span> : null}
      </button>
      {isOpen ? (
        <div className="ui-disclosure__body" id={bodyId}>
          {children}
        </div>
      ) : null}
    </div>
  );
}
