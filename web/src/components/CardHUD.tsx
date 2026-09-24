// CardHUD：把服务端归一化的卡片 UI 标记渲染为原生状态面板/场景标签。
//
// 数据来自 TurnContent.uiHints（已提交回合）或正文里的标准标记（实时流）。
// 只做展示，不解析任意 HTML。
import { Fragment } from "react";
import type { UIHint } from "../app/types";
import "./CardHUD.css";

/** 把 "n/m" 或 "n%" 解析为 0–1 的进度；否则只返回文本。 */
export function parseMeter(raw: string): { text: string; fraction?: number } {
  const s = raw.trim();
  const slash = /^(-?\d+(?:\.\d+)?)\s*\/\s*(\d+(?:\.\d+)?)$/.exec(s);
  if (slash) {
    const d = Number(slash[2]);
    if (d > 0) return { text: s, fraction: Math.min(1, Math.max(0, Number(slash[1]) / d)) };
  }
  const pct = /^(-?\d+(?:\.\d+)?)\s*%$/.exec(s);
  if (pct) return { text: s, fraction: Math.min(1, Math.max(0, Number(pct[1]) / 100)) };
  return { text: s };
}

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
          {entries.map(([k, v]) => {
            const meter = parseMeter(v);
            return (
              <Fragment key={k}>
                <dt>{k}</dt>
                <dd>
                  <span className="card-hud-value">{meter.text}</span>
                  {meter.fraction !== undefined && (
                    <span className="card-hud-meter">
                      <span
                        className="card-hud-meter-fill"
                        style={{ width: `${Math.round(meter.fraction * 100)}%` }}
                      />
                    </span>
                  )}
                </dd>
              </Fragment>
            );
          })}
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
