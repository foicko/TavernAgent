// risuImages：把正文里的 RisuAI 图像指令 <img cmd="key"> 渲染为卡内资源图片。
//
// 与 inlineMarkdown 同样刻意不做 HTML 解析：只识别这一种指令形态，
// 命中的 cmd 在资源映射里存在才渲染 <img>，否则原样保留文本。
// 因此没有任意 HTML 注入面。
import { Fragment, type ReactNode } from "react";
import { renderInlineMarkdown } from "./inlineMarkdown";

const IMG_CMD_PATTERN = /<img\s+cmd\s*=\s*"([^"]*)"\s*\/?>/gi;
const NUMERIC_SUFFIX = /^\d+$/;

/**
 * 解析 cmd → 资源 URL。
 *
 * 精确命中优先；否则回退到「同名 + 纯数字后缀」的变体（RisuAI 会为同一表情
 * 导出 1/2/3 等多张，而模型通常只写不带后缀的键）。多张候选按键名排序后
 * 以 cmd 的字符和取模选定，保证同一 cmd 每次渲染稳定（不闪烁）。
 */
export function resolveCardAsset(
  assets: Record<string, string> | undefined,
  cmd: string
): string | undefined {
  if (!assets || !cmd) return undefined;
  const direct = assets[cmd];
  if (direct) return direct;
  const candidates: string[] = [];
  for (const key of Object.keys(assets)) {
    if (key.length > cmd.length && key.startsWith(cmd) && NUMERIC_SUFFIX.test(key.slice(cmd.length))) {
      candidates.push(key);
    }
  }
  if (candidates.length === 0) return undefined;
  candidates.sort();
  let hash = 0;
  for (let i = 0; i < cmd.length; i++) hash = (hash + cmd.charCodeAt(i)) % candidates.length;
  return assets[candidates[hash]];
}

/**
 * 按 <img cmd="..."> 切分文本，命中资源映射的片段渲染为图片，
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
  for (const match of text.matchAll(IMG_CMD_PATTERN)) {
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
