import { useEffect, useRef, type RefObject } from "react";

export interface DismissOptions {
  enabled: boolean;
  onDismiss: () => void;
  /** 额外视为"内部"的节点（例如触发按钮），点击它们不算外部点击。 */
  ignore?: Array<RefObject<HTMLElement | null>>;
}

/**
 * 浮层关闭的统一实现：点击外部或按 Esc 关闭。
 * 取代各组件里重复的 mousedown/keydown 订阅。
 */
export function useDismiss({ enabled, onDismiss, ignore = [] }: DismissOptions) {
  const ignoreRef = useRef(ignore);
  ignoreRef.current = ignore;

  useEffect(() => {
    if (!enabled) return;
    const onPointer = (event: MouseEvent) => {
      const target = event.target as Node;
      const inside = ignoreRef.current.some((ref) => ref.current?.contains(target));
      if (!inside) onDismiss();
    };
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.stopPropagation();
        onDismiss();
      }
    };
    document.addEventListener("mousedown", onPointer);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onPointer);
      document.removeEventListener("keydown", onKey);
    };
  }, [enabled, onDismiss]);
}
