import { useEffect, useRef, useState } from "react";

export type ExitState = "open" | "closing";

/**
 * 关闭态过渡：`open` 变为 false 后先进入 `closing`，等 `durationMs` 再真正卸载，
 * 让弹窗/浮层/吐司有机会播放"关"的动画（transitions.dev：关比开更快更安静）。
 *
 * 用法：
 *   const { mounted, state } = useExitTransition(open, 150);
 *   if (!mounted) return null;
 *   return <div data-state={state}>…</div>;
 */
export function useExitTransition(open: boolean, durationMs: number) {
  const [mounted, setMounted] = useState(open);
  const [state, setState] = useState<ExitState>("open");
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (timer.current) {
      clearTimeout(timer.current);
      timer.current = null;
    }
    if (open) {
      setMounted(true);
      setState("open");
      return;
    }
    if (!mounted) return;
    setState("closing");
    timer.current = setTimeout(() => {
      setMounted(false);
      timer.current = null;
    }, Math.max(0, durationMs));
    return () => {
      if (timer.current) clearTimeout(timer.current);
    };
    // mounted 只在首帧参与判断，故意不放进依赖：否则 setMounted(false) 会再触发一轮。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, durationMs]);

  return { mounted, state } as const;
}

export interface ExitListEntry<T> {
  item: T;
  state: ExitState;
}

/**
 * 列表版：条目从 `items` 里消失后仍保留 `durationMs`，并标记为 `closing`。
 * 用于吐司堆叠、可移除的行等需要"离场动画"的列表。
 */
export function useExitList<T>(items: readonly T[], keyOf: (item: T) => string, durationMs: number) {
  const [entries, setEntries] = useState<Array<ExitListEntry<T> & { key: string }>>(() =>
    items.map((item) => ({ item, key: keyOf(item), state: "open" as const })),
  );
  const timers = useRef(new Map<string, ReturnType<typeof setTimeout>>());

  useEffect(() => {
    const liveKeys = new Set(items.map(keyOf));
    setEntries((prev) => {
      const byKey = new Map(prev.map((entry) => [entry.key, entry]));
      const next: Array<ExitListEntry<T> & { key: string }> = [];
      // 现存条目按传入顺序排，保留已有的 closing 条目在原位之后。
      for (const item of items) {
        const key = keyOf(item);
        const existing = byKey.get(key);
        if (existing?.state === "closing") {
          const timer = timers.current.get(key);
          if (timer) clearTimeout(timer);
          timers.current.delete(key);
        }
        next.push({ item, key, state: "open" });
        byKey.delete(key);
      }
      for (const [key, entry] of byKey) {
        if (!liveKeys.has(key)) {
          next.push({ ...entry, state: "closing" });
          if (!timers.current.has(key)) {
            timers.current.set(
              key,
              setTimeout(() => {
                timers.current.delete(key);
                setEntries((current) => current.filter((e) => e.key !== key));
              }, Math.max(0, durationMs)),
            );
          }
        }
      }
      // 传入的 items 数组常常是每次渲染新建的；若内容未变必须返回原引用，
      // 否则 setEntries 每次都触发新渲染，effect 依赖 items 会无限循环。
      const unchanged =
        next.length === prev.length &&
        next.every(
          (entry, i) =>
            entry.key === prev[i].key && entry.state === prev[i].state && entry.item === prev[i].item,
        );
      return unchanged ? prev : next;
    });
    // keyOf 视为稳定函数（调用方传模块级函数或 useCallback）。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [items, durationMs]);

  useEffect(() => {
    const map = timers.current;
    return () => {
      for (const timer of map.values()) clearTimeout(timer);
      map.clear();
    };
  }, []);

  return entries;
}
