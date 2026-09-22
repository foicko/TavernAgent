import { cloneElement, isValidElement, useId, type ReactElement, type ReactNode } from "react";
import "./Tooltip.css";

export type TooltipSide = "top" | "bottom" | "left" | "right";

export interface TooltipProps {
  /** 提示文案；为空时原样渲染子元素（方便按条件加提示）。 */
  content: ReactNode;
  side?: TooltipSide;
  /** 触发元素：单个可聚焦元素（按钮、链接）。 */
  children: ReactElement;
  className?: string;
}

/**
 * 轻量提示（纯 CSS）：hover / focus-within 显示，80ms 意图延迟进入、即时离开
 * （transitions.dev 17-tooltip）。用 aria-describedby 把文案挂到触发元素上，
 * 并清掉子元素的 title，避免原生提示与之重叠。
 */
export function Tooltip({ content, side = "top", children, className = "" }: TooltipProps) {
  const id = useId();
  if (content === null || content === undefined || content === "") return children;

  const child = isValidElement<{ "aria-describedby"?: string; title?: string }>(children)
    ? cloneElement(children, {
        "aria-describedby": [children.props["aria-describedby"], id].filter(Boolean).join(" "),
        title: "",
      })
    : children;

  return (
    <span className={["ui-tooltip-anchor", className].filter(Boolean).join(" ")}>
      {child}
      <span role="tooltip" id={id} className="ui-tooltip" data-side={side}>
        {content}
      </span>
    </span>
  );
}
