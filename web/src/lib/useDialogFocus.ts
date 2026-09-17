import { useEffect, useRef, type KeyboardEvent } from "react";

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
    return () => { if (previous?.isConnected) previous.focus(); };
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
