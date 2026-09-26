// risuImages：把正文里的 RisuAI 图像指令渲染为卡内资源图片。
//
// 与 inlineMarkdown 同样刻意不做 HTML 解析：只识别下面几种固定的指令形态，
// 命中的键在资源映射里存在才渲染 <img>，否则原样保留文本。
// 因此没有任意 HTML 注入面。
//
// 兼容的写法（均来自真实 RisuAI / charx 角色卡）：
//   <img cmd="key">           经典写法
//   <img="key">               RisuAI 旧式写法
//   <img_obj="key">           物件/道具图
//   <imgBG="key">             背景图
//   <img src="key" rel="…">   带附加属性的写法（只取 src）
//
// 属性名与引号外的其余属性都会被跳过，只提取第一个双引号值作为资源键。
import { Fragment, type ReactNode } from "react";
import { renderInlineMarkdown } from "./inlineMarkdown";

// IMG_TAG_SOURCE 同时被 cardMarkers.tsx 复用来拼装完整标记正则，避免两份语法实现。
export const IMG_TAG_SOURCE = `<img\\s*[-\\w]*\\s*=\\s*"([^"]{0,400})"[^>]*>`;
const IMG_TAG_PATTERN = new RegExp(IMG_TAG_SOURCE, "gi");
const NUMERIC_SUFFIX = /^\d+$/;
// 只剥离「已知图片扩展名」，避免把资源名里本就有意义的一段（如 nami.flustered）
// 误当作扩展名剥掉。
const IMAGE_EXT = /\.(png|jpe?g|webp|gif|avif|bmp|svg|apng|jfif)$/;

function stripImageExt(lower: string): string {
  return lower.replace(IMAGE_EXT, "");
}

/**
 * 解析资源键 → 资源 URL。
 *
 * 依次尝试（命中即返回）：
 *   1. 精确命中；
 *   2. 忽略大小写命中；
 *   3. 「去掉图片扩展名后同名」——覆盖三种真实命名：
 *      · 资源带扩展名而标记不带（`main` ↔ `main.png`）
 *      · 标记带扩展名而资源不带（`main.png` ↔ `main`）
 *      · 标记与资源扩展名不同（`robe.default` ↔ `robe.default.avif`）
 *   4. 在其基础上再回退「纯数字后缀」变体：`dressed_happy` ↔ `dressed_happy1.png`。
 *
 * 多张候选按键名排序后以键的字符和取模选定，保证同一键每次渲染稳定（不闪烁）。
 */
export function resolveCardAsset(
  assets: Record<string, string> | undefined,
  raw: string
): string | undefined {
  if (!assets || !raw) return undefined;
  const key = raw.trim();
  if (!key) return undefined;
  const direct = assets[key];
  if (direct) return direct;

  const norm = key.toLowerCase();
  const normStem = stripImageExt(norm);
  let hit: string | undefined;
  const suffix: string[] = [];
  for (const name of Object.keys(assets)) {
    const lower = name.toLowerCase();
    if (lower === norm) {
      hit = name;
      continue;
    }
    const stem = stripImageExt(lower);
    if (stem === norm || stem === normStem) {
      // 记录首个候选即可（同一键命中多张时按资源顺序稳定取第一张）。
      if (!hit) hit = name;
      continue;
    }
    if (stem.length > normStem.length && stem.startsWith(normStem) && NUMERIC_SUFFIX.test(stem.slice(normStem.length))) {
      suffix.push(name);
    }
  }
  if (hit) return assets[hit];
  if (suffix.length === 0) return undefined;
  suffix.sort();
  let hash = 0;
  for (let i = 0; i < key.length; i++) hash = (hash + key.charCodeAt(i)) % suffix.length;
  return assets[suffix[hash]];
}

/**
 * 按 RisuAI 图像指令切分文本，命中资源映射的片段渲染为图片，
 * 其余片段走常规内联 Markdown。
 */
export function renderWithCardImages(
  text: string,
  assets?: Record<string, string>
): ReactNode[] {
  if (!text) return [];
  if (!assets || Object.keys(assets).length === 0) return renderInlineMarkdown(text);

  const nodes: ReactNode[] = [];
  let last = 0;
  let index = 0;
  for (const match of text.matchAll(IMG_TAG_PATTERN)) {
    const start = match.index ?? 0;
    if (start > last) {
      nodes.push(
        <Fragment key={`md-${index++}`}>{renderInlineMarkdown(text.slice(last, start))}</Fragment>
      );
    }
    const cmd = (match[1] ?? "").trim();
    const url = resolveCardAsset(assets, cmd);
    if (url) {
      nodes.push(
        <img
          key={`img-${index++}`}
          className="story-inline-image"
          src={url}
          alt={cmd}
          loading="lazy"
        />
      );
    } else {
      nodes.push(match[0]);
    }
    last = start + match[0].length;
  }
  if (last < text.length) {
    nodes.push(
      <Fragment key={`md-${index++}`}>{renderInlineMarkdown(text.slice(last))}</Fragment>
    );
  }
  return nodes;
}
