import type { CSSProperties } from "react";
import "./Skeleton.css";

export interface SkeletonProps {
  width?: number | string;
  height?: number | string;
  /** 圆形占位（头像）。 */
  circle?: boolean;
  radius?: number | string;
  className?: string;
}

/** 单块骨架：柔和脉动（transitions.dev 14-skeleton-reveal 的占位阶段）。 */
export function Skeleton({ width, height = 12, circle = false, radius, className = "" }: SkeletonProps) {
  const style: CSSProperties = {
    width: circle ? height : width,
    height,
    borderRadius: circle ? "50%" : radius,
  };
  return <span className={["ui-skeleton", className].filter(Boolean).join(" ")} style={style} aria-hidden="true" />;
}

export interface SkeletonTextProps {
  /** 行数，默认 3；最后一行短一些。 */
  lines?: number;
  className?: string;
}

/** 多行文本骨架。 */
export function SkeletonText({ lines = 3, className = "" }: SkeletonTextProps) {
  return (
    <span className={["ui-skeleton-text", className].filter(Boolean).join(" ")} aria-hidden="true">
      {Array.from({ length: lines }, (_, index) => (
        <Skeleton key={index} width={index === lines - 1 && lines > 1 ? "62%" : "100%"} height={11} />
      ))}
    </span>
  );
}
