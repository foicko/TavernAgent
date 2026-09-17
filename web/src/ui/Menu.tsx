import { useEffect, useRef, type KeyboardEvent, type ReactNode } from "react";
import "./Menu.css";

export interface MenuProps {
  open: boolean;
  /** 菜单可访问名（aria-label）。 */
  label: string;
  onClose: () => void;
  /** 面板内的可见标题（可选），例如"选择 TRPG 技能检定动作"。 */
  header?: ReactNode;
  /** 相对触发按钮的方向：输入台向上，设置页向下。 */
  direction?: "up" | "down";
  align?: "start" | "end";
  footer?: ReactNode;
  className?: string;
  children: ReactNode;
}

/** 取面板内可聚焦的菜单项（模块级，避免进入 effect 依赖）。 */
function menuItems(panel: HTMLDivElement | null): HTMLElement[] {
  return Array.from(panel?.querySelectorAll<HTMLElement>('[role^="menuitem"]:not(:disabled)') ?? []);
}

/** 下拉面板：role="menu" + 方向键/Home/End/Esc/Tab 键盘行为。 */
export function Menu({
  open,
  label,
  onClose,
  header,
  direction = "down",
  align = "start",
  footer,
  className = "",
  children,
}: MenuProps) {
  const panelRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    menuItems(panelRef.current)[0]?.focus();
  }, [open]);

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    const nodes = menuItems(panelRef.current);
    const current = document.activeElement as HTMLElement | null;
    const index = current ? nodes.indexOf(current) : -1;

    switch (event.key) {
      case "ArrowDown":
        event.preventDefault();
        nodes[Math.min(nodes.length - 1, index + 1)]?.focus();
        break;
      case "ArrowUp":
        event.preventDefault();
        nodes[Math.max(0, index - 1)]?.focus();
        break;
      case "Home":
        event.preventDefault();
        nodes[0]?.focus();
        break;
      case "End":
        event.preventDefault();
        nodes.at(-1)?.focus();
        break;
      case "Escape":
        event.preventDefault();
        event.stopPropagation();
        onClose();
        break;
      case "Tab":
        onClose();
        break;
      default:
        break;
    }
  };

  if (!open) return null;

  return (
    <div
      ref={panelRef}
      role="menu"
      aria-label={label}
      className={["ui-menu", `ui-menu--${direction}`, `ui-menu--align-${align}`, className].filter(Boolean).join(" ")}
      onKeyDown={onKeyDown}
    >
      {header ? <div className="ui-menu__header">{header}</div> : null}
      {children}
      {footer ? <div className="ui-menu__foot">{footer}</div> : null}
    </div>
  );
}

export interface MenuItemProps {
  title: ReactNode;
  description?: ReactNode;
  /** 行尾补充信息（标签、快捷键等）。 */
  meta?: ReactNode;
  onSelect: () => void;
  /** 单选语义：加 aria-checked 并配合 selected 呈现选中态。 */
  radio?: boolean;
  selected?: boolean;
  disabled?: boolean;
  className?: string;
}

export function MenuItem({
  title,
  description,
  meta,
  onSelect,
  radio = false,
  selected = false,
  disabled = false,
  className = "",
}: MenuItemProps) {
  return (
    <button
      type="button"
      role={radio ? "menuitemradio" : "menuitem"}
      aria-checked={radio ? selected : undefined}
      disabled={disabled}
      className={["ui-menu__item", selected ? "ui-menu__item--active" : "", className].filter(Boolean).join(" ")}
      onClick={onSelect}
    >
      <span className="ui-menu__text">
        <span className="ui-menu__item-title">{title}</span>
        {description ? <span className="ui-menu__item-desc">{description}</span> : null}
      </span>
      {meta ? <span className="ui-menu__item-meta">{meta}</span> : null}
    </button>
  );
}

export function MenuSeparator() {
  return <div className="ui-menu__sep" role="separator" />;
}
