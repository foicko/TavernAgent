import { useEffect, useRef, type KeyboardEvent } from "react";
import { popLayer, pushLayer } from "./modalStack";

// 图层根：从对话框元素向上找到 portal 的顶层容器（通常是 .ui-modal-backdrop，
// 它是 document.body 的直接子节点）。置 inert 必须作用于它——只 inert 内层
// 对话框，遮罩仍可点击，会误关下层。
function layerRoot(element: HTMLElement | null): HTMLElement | null {
  let node = element;
  while (node && node.parentElement && node.parentElement !== document.body) {
    node = node.parentElement;
  }
  return node;
}

// 所有自定义对话框共用同一键盘边界（Tab 循环 + Esc 关闭），并把自身注册进
// lib/modalStack：栈顶之外的应用内容与更下层对话框都会被 inert。
export function useDialogFocus(open: boolean, onClose?: () => void) {
  const ref = useRef<HTMLDivElement>(null);
  // onClose 通过 ref 取最新值：注册 effect 只依赖 open，避免每次渲染都重新进出栈
  // （那会导致焦点被反复重设）。
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;

  const focusable = () => Array.from(ref.current?.querySelectorAll<HTMLElement>(
    'button:not(:disabled), a[href], input:not(:disabled), textarea:not(:disabled), select:not(:disabled), [tabindex="0"]',
  ) ?? []).filter(element => element.tabIndex >= 0 && element.getClientRects().length > 0 && getComputedStyle(element).visibility !== "hidden");

  useEffect(() => {
    if (!open) return;
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const dialog = ref.current;
    if (dialog && !dialog.contains(document.activeElement)) dialog.focus();

    const layerId = pushLayer({
      onClose: () => onCloseRef.current?.(),
      setInert: (inert) => {
        const element = layerRoot(ref.current);
        if (!element) return;
        if (inert) {
          element.setAttribute("inert", "");
          element.setAttribute("aria-hidden", "true");
        } else {
          element.removeAttribute("inert");
          element.removeAttribute("aria-hidden");
        }
      },
    });

    return () => {
      popLayer(layerId);
      if (previous?.isConnected) previous.focus();
    };
  }, [open]);

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    // 内层控件已经处理掉这次按键时不再插手（例如命令面板自己吃掉 Tab）。
    if (event.defaultPrevented) return;
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      onCloseRef.current?.();
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
