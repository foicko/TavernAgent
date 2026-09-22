import { useState } from "react";
import "./Avatar.css";

export interface AvatarProps {
  src?: string | null;
  /** 名字：用于 alt 与无图时的首字兜底。 */
  name: string;
  size?: number;
  shape?: "circle" | "rounded";
  className?: string;
}

/** 头像：图片加载失败或缺省时回落为首字母/首字。 */
export function Avatar({ src, name, size = 32, shape = "circle", className = "" }: AvatarProps) {
  const [failed, setFailed] = useState(false);
  const showImage = !!src && !failed;
  const initial = name.trim().charAt(0).toUpperCase() || "?";
  const style = { width: size, height: size, fontSize: Math.max(10, Math.round(size * 0.42)) };
  return (
    <span
      className={["ui-avatar", `ui-avatar--${shape}`, className].filter(Boolean).join(" ")}
      style={style}
      role={showImage ? undefined : "img"}
      aria-label={showImage ? undefined : name}
    >
      {showImage ? (
        <img className="ui-avatar__img" src={src ?? undefined} alt={name} onError={() => setFailed(true)} />
      ) : (
        <span className="ui-avatar__initial" aria-hidden="true">{initial}</span>
      )}
    </span>
  );
}
