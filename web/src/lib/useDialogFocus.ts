import { useEffect, useRef, type KeyboardEvent } from "react";

// 同一时刻可能有多个对话框（模态叠模态）：只有最外层关闭时才恢复背景。
let openDialogs = 0;
let savedBodyOverflow = "";

// Every custom dialog uses the same keyboard boundary and restores its opener.
export function useDialogFocus(open: boolean, onClose?: () => void) {
  const ref = useRef<HTMLDivElement>(null);
  const focusable = () => Array.from(ref.current?.querySelectorAll<HTMLElement>(
    'button:not(:disabled), a[href], input:not(:disabled), textarea:not(:disabled), select:not(:disabled), [tabindex="0"]',
  ) ?? []).filter(element => element.tabIndex >= 0 && element.getClientRects().length > 0 && getComputedStyle(element).visibility !== "hidden");

  useEffect(() => {
    if (!open) return;
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const dialog = ref.current;
    if (dialog && !dialog.contains(document.activeElement)) dialog.focus();

    // 背景 inert + 滚动锁：仅靠 aria-modal 无法阻止辅助技术/指针到达背景内容。
    openDialogs += 1;
    const root = document.getElementById("root");
    if (openDialogs === 1) {
      root?.setAttribute("inert", "");
      root?.setAttribute("aria-hidden", "true");
      savedBodyOverflow = document.body.style.overflow;
      document.body.style.overflow = "hidden";
    }
    return () => {
      openDialogs = Math.max(0, openDialogs - 1);
      if (openDialogs === 0) {
        root?.removeAttribute("inert");
        root?.removeAttribute("aria-hidden");
        document.body.style.overflow = savedBodyOverflow;
      }
      if (previous?.isConnected) previous.focus();
    };
  }, [open]);

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      onClose?.();
    } else if (event.key === "Tab") {
      const items = focusable();
      const current = document.activeElement;
      if (!items.length) {
        event.preventDefault();
        ref.current?.focus();
      } else if (event.shiftKey && (current === items[0] || !items.includes(current as HTMLElement))) {
        event.preventDefault();
        items.at(-1)?.focus();
      } else if (!event.shiftKey && (current === items.at(-1) || !items.includes(current as HTMLElement))) {
        event.preventDefault();
        items[0].focus();
      }
    }
  };
  return { ref, tabIndex: -1, onKeyDown, role: "dialog", "aria-modal": true } as const;
}
