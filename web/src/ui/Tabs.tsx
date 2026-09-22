import { useCallback, useEffect, useLayoutEffect, useRef, type KeyboardEvent, type ReactNode } from "react";
import "./Tabs.css";

export interface TabItem<T extends string = string> {
  value: T;
  label: ReactNode;
  icon?: ReactNode;
  /** 透传到 <button> 上的 id（既有 e2e 依赖 #subtab-btn-*、#btn-mode-*）。 */
  id?: string;
  title?: string;
  disabled?: boolean;
  /** 只在窄屏显示图标时，文字部分加上此类名以便媒体查询隐藏。 */
  labelClassName?: string;
}

export interface TabsProps<T extends string = string> {
  items: ReadonlyArray<TabItem<T>>;
  value: T;
  onChange: (value: T) => void;
  /** tablist 的可访问名。 */
  label: string;
  /** segmented：胶囊轨道 + 滑动高亮块；underline：底部滑动下划线。 */
  variant?: "segmented" | "underline";
  size?: "sm" | "md";
  /** 等宽铺满容器（左栏四页签）；默认按内容宽度。 */
  fill?: boolean;
  className?: string;
}

/**
 * 分段控件 / 页签（唯一实现）：滑动高亮块 250ms 对称过渡（transitions.dev tabs-sliding），
 * roving tabindex + 方向键/Home/End 键盘导航（WAI-ARIA tabs 模式）。
 * 高亮块位置由 useLayoutEffect 量出（支持内容宽度不等的页签），写入 --tab-x / --tab-w。
 */
export function Tabs<T extends string = string>({
  items,
  value,
  onChange,
  label,
  variant = "segmented",
  size = "md",
  fill = false,
  className = "",
}: TabsProps<T>) {
  const listRef = useRef<HTMLDivElement>(null);
  const indicatorRef = useRef<HTMLSpanElement>(null);

  const measure = useCallback(() => {
    const list = listRef.current;
    const indicator = indicatorRef.current;
    if (!list || !indicator) return;
    const active = list.querySelector<HTMLElement>('[role="tab"][aria-selected="true"]');
    if (!active) {
      indicator.style.opacity = "0";
      return;
    }
    indicator.style.opacity = "1";
    indicator.style.setProperty("--tab-x", `${active.offsetLeft}px`);
    indicator.style.setProperty("--tab-w", `${active.offsetWidth}px`);
  }, []);

  useLayoutEffect(measure, [measure, value, items, size, variant, fill]);

  useEffect(() => {
    if (typeof ResizeObserver === "undefined" || !listRef.current) return;
    const observer = new ResizeObserver(() => measure());
    observer.observe(listRef.current);
    return () => observer.disconnect();
  }, [measure]);

  const enabled = items.filter((item) => !item.disabled);

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (enabled.length === 0) return;
    const index = enabled.findIndex((item) => item.value === value);
    let next: TabItem<T> | undefined;
    switch (event.key) {
      case "ArrowRight":
      case "ArrowDown":
        next = enabled[(index + 1) % enabled.length];
        break;
      case "ArrowLeft":
      case "ArrowUp":
        next = enabled[(index - 1 + enabled.length) % enabled.length];
        break;
      case "Home":
        next = enabled[0];
        break;
      case "End":
        next = enabled[enabled.length - 1];
        break;
      default:
        return;
    }
    event.preventDefault();
    if (!next) return;
    onChange(next.value);
    const tabs = Array.from(listRef.current?.querySelectorAll<HTMLElement>('[role="tab"]') ?? []);
    tabs.find((tab) => tab.dataset.value === next.value)?.focus();
  };

  const classes = [
    "ui-tabs",
    `ui-tabs--${variant}`,
    `ui-tabs--${size}`,
    fill ? "ui-tabs--fill" : "",
    className,
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <div ref={listRef} role="tablist" aria-label={label} className={classes} onKeyDown={onKeyDown}>
      <span ref={indicatorRef} className="ui-tabs__indicator" aria-hidden="true" />
      {items.map((item) => {
        const selected = item.value === value;
        return (
          <button
            key={item.value}
            type="button"
            role="tab"
            id={item.id}
            title={item.title}
            data-value={item.value}
            aria-selected={selected}
            tabIndex={selected ? 0 : -1}
            disabled={item.disabled}
            className={["ui-tabs__tab", selected ? "active" : ""].filter(Boolean).join(" ")}
            onClick={() => onChange(item.value)}
          >
            {item.icon ? <span className="ui-tabs__icon" aria-hidden="true">{item.icon}</span> : null}
            <span className={["ui-tabs__label", item.labelClassName].filter(Boolean).join(" ")}>{item.label}</span>
          </button>
        );
      })}
    </div>
  );
}
