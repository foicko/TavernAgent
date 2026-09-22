import { useCallback, useEffect, useLayoutEffect, useRef, useState, type ReactNode, type RefObject } from "react";
import { createPortal } from "react-dom";
import { useDismiss } from "../lib/useDismiss";
import { useExitTransition } from "../lib/useExitTransition";
import "./Popover.css";

export type PopoverSide = "top" | "bottom" | "left" | "right";
export type PopoverAlign = "start" | "center" | "end";

export interface PopoverPoint {
  x: number;
  y: number;
}

export interface PopoverProps {
  open: boolean;
  onClose: () => void;
  /** 锚定元素；或一个视口坐标点（如鼠标位置的词条浮层）。 */
  anchor: RefObject<HTMLElement | null> | PopoverPoint | null;
  /** 浮层可访问名。 */
  label: string;
  side?: PopoverSide;
  align?: PopoverAlign;
  /** 与锚点的距离，默认 8。 */
  offset?: number;
  /** 打开时把焦点移入第一个可聚焦元素；纯展示浮层（词条卡）传 false。 */
  autoFocus?: boolean;
  /** 关闭后焦点是否回到打开前的元素。 */
  returnFocus?: boolean;
  /** 鼠标移入浮层时的回调（悬停型浮层用来取消延迟关闭）。 */
  onPointerEnter?: () => void;
  onPointerLeave?: () => void;
  width?: number | string;
  className?: string;
  children: ReactNode;
}

const VIEWPORT_GUTTER = 8;
const CLOSE_MS = 150;

interface Placement {
  top: number;
  left: number;
  side: PopoverSide;
}

function anchorRect(anchor: PopoverProps["anchor"]): DOMRect | null {
  if (!anchor) return null;
  if ("current" in anchor) {
    const element = anchor.current;
    return element ? element.getBoundingClientRect() : null;
  }
  return new DOMRect(anchor.x, anchor.y, 0, 0);
}

/** 纯函数：给定锚点矩形与面板尺寸，算出不出视口的定位（空间不够时翻到对侧）。 */
export function placePopover(
  rect: DOMRect,
  panel: { width: number; height: number },
  side: PopoverSide,
  align: PopoverAlign,
  offset: number,
  viewport: { width: number; height: number },
): Placement {
  const fits = (s: PopoverSide) => {
    switch (s) {
      case "top": return rect.top - offset - panel.height >= VIEWPORT_GUTTER;
      case "bottom": return rect.bottom + offset + panel.height <= viewport.height - VIEWPORT_GUTTER;
      case "left": return rect.left - offset - panel.width >= VIEWPORT_GUTTER;
      case "right": return rect.right + offset + panel.width <= viewport.width - VIEWPORT_GUTTER;
    }
  };
  const opposite: Record<PopoverSide, PopoverSide> = { top: "bottom", bottom: "top", left: "right", right: "left" };
  const finalSide = fits(side) || !fits(opposite[side]) ? side : opposite[side];

  let top = 0;
  let left = 0;
  if (finalSide === "top" || finalSide === "bottom") {
    top = finalSide === "top" ? rect.top - offset - panel.height : rect.bottom + offset;
    if (align === "start") left = rect.left;
    else if (align === "end") left = rect.right - panel.width;
    else left = rect.left + rect.width / 2 - panel.width / 2;
  } else {
    left = finalSide === "left" ? rect.left - offset - panel.width : rect.right + offset;
    if (align === "start") top = rect.top;
    else if (align === "end") top = rect.bottom - panel.height;
    else top = rect.top + rect.height / 2 - panel.height / 2;
  }
  left = Math.max(VIEWPORT_GUTTER, Math.min(left, viewport.width - panel.width - VIEWPORT_GUTTER));
  top = Math.max(VIEWPORT_GUTTER, Math.min(top, viewport.height - panel.height - VIEWPORT_GUTTER));
  return { top, left, side: finalSide };
}

/**
 * 锚定浮层（唯一实现）：portal 到 body、视口坐标定位、越界自动翻转、
 * 外点/Esc 关闭（复用 useDismiss）、焦点返回、开 250ms / 关 150ms。
 * 取代 LorePopover / ContextMetricsPopover / InventoryActionBubble / 检定菜单各自的浮层壳。
 */
export function Popover({
  open,
  onClose,
  anchor,
  label,
  side = "bottom",
  align = "start",
  offset = 8,
  autoFocus = true,
  returnFocus = true,
  onPointerEnter,
  onPointerLeave,
  width,
  className = "",
  children,
}: PopoverProps) {
  const panelRef = useRef<HTMLDivElement>(null);
  const [placement, setPlacement] = useState<Placement | null>(null);
  const { mounted, state } = useExitTransition(open, CLOSE_MS);
  const anchorElementRef = anchor && "current" in anchor ? anchor : null;

  const reposition = useCallback(() => {
    const panel = panelRef.current;
    const rect = anchorRect(anchor);
    if (!panel || !rect) return;
    setPlacement(
      placePopover(
        rect,
        { width: panel.offsetWidth, height: panel.offsetHeight },
        side,
        align,
        offset,
        { width: window.innerWidth, height: window.innerHeight },
      ),
    );
  }, [anchor, side, align, offset]);

  useLayoutEffect(() => {
    if (!mounted) {
      setPlacement(null);
      return;
    }
    reposition();
  }, [mounted, reposition, children]);

  useEffect(() => {
    if (!open) return;
    window.addEventListener("resize", reposition);
    window.addEventListener("scroll", reposition, true);
    return () => {
      window.removeEventListener("resize", reposition);
      window.removeEventListener("scroll", reposition, true);
    };
  }, [open, reposition]);

  // 焦点：面板挂载后移入，关闭时回到打开前的元素。
  useEffect(() => {
    if (!open || !mounted) return;
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    if (autoFocus) {
      const first = panelRef.current?.querySelector<HTMLElement>(
        'button:not(:disabled), a[href], input:not(:disabled), textarea:not(:disabled), select:not(:disabled), [tabindex="0"]',
      );
      (first ?? panelRef.current)?.focus();
    }
    return () => {
      if (returnFocus && previous?.isConnected) previous.focus();
    };
  }, [open, mounted, autoFocus, returnFocus]);

  useDismiss({
    enabled: open,
    onDismiss: onClose,
    ignore: anchorElementRef ? [panelRef, anchorElementRef] : [panelRef],
  });

  if (!mounted || typeof document === "undefined") return null;

  const style = placement
    ? { top: placement.top, left: placement.left, width }
    : { top: 0, left: 0, width, visibility: "hidden" as const };

  return createPortal(
    <div
      ref={panelRef}
      role="dialog"
      aria-label={label}
      // 离场过渡期间对辅助技术与指针都不可达：动画照播，但不再算"打开着"。
      aria-hidden={state === "closing" ? true : undefined}
      inert={state === "closing" ? true : undefined}
      tabIndex={-1}
      data-state={state}
      data-side={placement?.side ?? side}
      className={["ui-popover", className].filter(Boolean).join(" ")}
      style={style}
      onPointerEnter={onPointerEnter}
      onPointerLeave={onPointerLeave}
    >
      {children}
    </div>,
    document.body,
  );
}
