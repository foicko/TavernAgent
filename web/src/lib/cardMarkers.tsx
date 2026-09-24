// cardMarkers：把正文里的卡片 UI 标记渲染为原生组件。
//
// 与 risuImages 一样刻意不做 HTML 解析：只识别固定几种标记形态，命中才渲染，
// 否则原样保留文本。因此没有任意 HTML 注入面。
//
// 支持的标记（与 internal/cardmarkers、desktop CardMarkerParser 保持一致）：
//   <img cmd="key"> / [IMG:key]   图片（卡内资源）
//   [HUD:label=value|…]           状态面板
//   [SCENE:title]                 场景
//   [VAR:key=value|…]             会话变量（正文里不展示）
//
// 转义：`[[` 表示字面量 `[`。
import { Fragment, type ReactNode } from "react";
import { renderInlineMarkdown } from "./inlineMarkdown";
import { resolveCardAsset } from "./risuImages";
import { CardHUD } from "../components/CardHUD";
import type { UIHint } from "../app/types";

const MARKER_RE = /<img\s+cmd\s*=\s*"([^"]*)"\s*\/?>|\[(IMG|HUD|SCENE|VAR):([^\[\]\r\n]*)\]/gi;
const ESCAPE_SENTINEL = "\u0000LBRACK\u0000";

interface Segment {
  kind: "text" | "image" | "hud" | "scene";
  text?: string; // text / 未解析标记的原文
  url?: string; // image
  alt?: string; // image
  hint?: UIHint; // hud / scene
}

function parsePairs(body: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const part of body.split("|")) {
    const eq = part.indexOf("=");
    if (eq < 0) continue;
    const k = part.slice(0, eq).trim();
    const v = part.slice(eq + 1).trim();
    if (k) out[k] = v;
  }
  return out;
}

/** 把正文切成有序片段（文本/图片/状态/场景）。状态类标记从文本中剥离。 */
export function parseCardMarkers(text: string, assets?: Record<string, string>): Segment[] {
  const protectedText = text.split("[[").join(ESCAPE_SENTINEL);
  const segs: Segment[] = [];
  let last = 0;
  const pushText = (s: string) => {
    if (!s) return;
    const t = s.split(ESCAPE_SENTINEL).join("[");
    // 合并相邻文本段：标记被剥离/原样保留时会产生多个文本片，合成一段更干净。
    const last = segs[segs.length - 1];
    if (last && last.kind === "text") last.text = (last.text ?? "") + t;
    else segs.push({ kind: "text", text: t });
  };
  for (const m of protectedText.matchAll(MARKER_RE)) {
    const start = m.index ?? 0;
    if (start > last) pushText(protectedText.slice(last, start));
    if (m[1] !== undefined) {
      const key = m[1].trim();
      const url = resolveCardAsset(assets, key);
      if (url) segs.push({ kind: "image", url, alt: key });
      else pushText(m[0]);
    } else {
      const tag = (m[2] ?? "").toUpperCase();
      const body = (m[3] ?? "").trim();
      if (tag === "IMG") {
        const url = resolveCardAsset(assets, body);
        if (url) segs.push({ kind: "image", url, alt: body });
        else pushText(m[0]);
      } else if (tag === "HUD") {
        const values = parsePairs(body);
        if (Object.keys(values).length > 0) segs.push({ kind: "hud", hint: { kind: "hud", values } });
        else pushText(m[0]);
      } else if (tag === "SCENE") {
        if (body) segs.push({ kind: "scene", hint: { kind: "scene", text: body } });
        else pushText(m[0]);
      }
      // VAR：状态标记，正文里不展示。
    }
    last = start + m[0].length;
  }
  if (last < protectedText.length) pushText(protectedText.slice(last));
  return segs;
}

/** 渲染正文：文本走内联 Markdown，图片/状态/场景走原生组件。 */
export function renderCardMarkers(text: string, assets?: Record<string, string>): ReactNode[] {
  if (!text) return [];
  const segs = parseCardMarkers(text, assets);
  if (segs.length === 1 && segs[0].kind === "text") return renderInlineMarkdown(segs[0].text ?? "");
  const nodes: ReactNode[] = [];
  let i = 0;
  for (const s of segs) {
    if (s.kind === "text") {
      nodes.push(<Fragment key={`md-${i++}`}>{renderInlineMarkdown(s.text ?? "")}</Fragment>);
    } else if (s.kind === "image") {
      nodes.push(
        <img key={`img-${i++}`} className="story-inline-image" src={s.url} alt={s.alt} loading="lazy" />
      );
    } else if (s.hint) {
      nodes.push(<CardHUD key={`hud-${i++}`} hint={s.hint} />);
    }
  }
  return nodes;
}
