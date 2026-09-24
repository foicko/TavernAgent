// CardHUD：把服务端归一化的卡片 UI 标记渲染为原生状态面板/场景标签。
//
// 数据来自 TurnContent.uiHints（已提交回合）或正文里的标准标记（实时流）。
// 只做展示，不解析任意 HTML。
import { Fragment } from "react";
import type { UIHint } from "../app/types";
import "./CardHUD.css";

export function CardHUD({ hint }: { hint: UIHint }) {
  if (hint.kind === "scene") {
    if (!hint.text) return null;
    return (
      <div className="card-scene-chip">
        <span className="card-scene-label">场景</span>
        <span className="card-scene-title">{hint.text}</span>
      </div>
    );
  }
  if (hint.kind === "hud" && hint.values) {
    const entries = Object.entries(hint.values);
    if (entries.length === 0) return null;
    return (
      <div className="card-hud">
        <div className="card-hud-title">状态</div>
        <dl className="card-hud-grid">
          {entries.map(([k, v]) => (
            <Fragment key={k}>
              <dt>{k}</dt>
              <dd>{v}</dd>
            </Fragment>
          ))}
        </dl>
      </div>
    );
  }
  return null;
}

/** 渲染一组已提交回合的 UI 提示（场景在前，状态面板在后）。 */
export function CardHUDStack({ hints }: { hints?: UIHint[] }) {
  if (!hints || hints.length === 0) return null;
  const visible = hints.filter((h) => (h.kind === "scene" && h.text) || (h.kind === "hud" && h.values));
  if (visible.length === 0) return null;
  return (
    <div className="card-hud-stack">
      {visible.map((h, i) => (
        <CardHUD key={i} hint={h} />
      ))}
    </div>
  );
}
