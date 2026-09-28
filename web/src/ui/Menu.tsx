import { useCallback, useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent, type ReactNode, type RefObject } from "react";
import { createPortal } from "react-dom";
import { placePopover } from "./Popover";
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
  /** 传入锚点（通常是包裹触发按钮的容器）时，菜单 portal 到 body 并按视口定位，避免被 overflow:hidden 祖先裁切。 */
  anchor?: RefObject<HTMLElement | null>;
  /** portal 模式下把面板节点暴露给调用方，供 useDismiss 的 ignore 使用。 */
  panelRef?: RefObject<HTMLDivElement | null>;
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
  anchor,
  panelRef: externalPanelRef,
  children,
}: MenuProps) {
  const localRef = useRef<HTMLDivElement>(null);
  const panelRef = externalPanelRef ?? localRef;
  const triggerRef = useRef<HTMLElement | null>(null);
  const portal = Boolean(anchor);
  const side = direction === "up" ? "top" : "bottom";
  const popAlign = align === "end" ? "end" : "start";
  const [placement, setPlacement] = useState<{ top: number; left: number; maxHeight: number } | null>(null);

  const reposition = useCallback(() => {
    if (!portal) return;
    const panel = panelRef.current;
    const anchorElement = anchor?.current;
    if (!panel || !anchorElement) return;
    const rect = anchorElement.getBoundingClientRect();
    const viewport = { width: window.innerWidth, height: window.innerHeight };
    const placed = placePopover(rect, { width: panel.offsetWidth, height: panel.offsetHeight }, side, popAlign, 8, viewport);
    // 限制高度：矮屏（横屏手机）也要能滚动到全部选项。
    setPlacement({ top: placed.top, left: placed.left, maxHeight: Math.max(140, Math.min(320, viewport.height - 16)) });
  }, [portal, panelRef, anchor, side, popAlign]);

  useLayoutEffect(() => {
    if (!open) {
      setPlacement(null);
      return;
    }
    reposition();
  }, [open, reposition, children]);

  useEffect(() => {
    if (!open) return;
    window.addEventListener("resize", reposition);
    window.addEventListener("scroll", reposition, true);
    return () => {
      window.removeEventListener("resize", reposition);
      window.removeEventListener("scroll", reposition, true);
    };
  }, [open, reposition]);

  useEffect(() => {
    if (!open) return;
    // 记住触发元素：关闭时把焦点还回去，键盘用户才不会"丢失位置"到 body。
    triggerRef.current = (document.activeElement as HTMLElement | null) ?? null;
    const panel = panelRef.current;
    const items = menuItems(panel);
    const active = items.find((el) => el.getAttribute("aria-checked") === "true");
    (active ?? items[0])?.focus();
    return () => {
      const trigger = triggerRef.current;
      if (!trigger) return;
      const focused = document.activeElement as HTMLElement | null;
      // 仅当焦点仍在菜单内或已落到 body（菜单卸载后）时才归还，避免抢走用户的新焦点。
      if (!focused || focused === document.body || panel?.contains(focused)) {
        trigger.focus?.();
      }
    };
  }, [open, panelRef]);

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

  const panel = (
    <div
      ref={panelRef}
      role="menu"
      aria-label={label}
      className={["ui-menu", portal ? "ui-menu--portal" : "", `ui-menu--${direction}`, `ui-menu--align-${align}`, className].filter(Boolean).join(" ")}
      style={portal ? (placement ? { top: placement.top, left: placement.left, maxHeight: placement.maxHeight } : { top: 0, left: 0, visibility: "hidden" }) : undefined}
      onKeyDown={onKeyDown}
    >
      {header ? <div className="ui-menu__header">{header}</div> : null}
      {children}
      {footer ? <div className="ui-menu__foot">{footer}</div> : null}
    </div>
  );

  if (portal && typeof document !== "undefined") {
    return createPortal(panel, document.body);
  }
  return panel;
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
