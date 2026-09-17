// 极简内联 Markdown 渲染（设计 D2「正文 Markdown 渲染」的最小兑现）。
// 只支持 **粗体** 与 *斜体*——角色扮演叙事正文最常见的两种标记
// （对白强调与 *动作描写*）。
// 刻意不引入 react-markdown（约 40KB gzip，违背"小而美"的体积预算），
// 也不做 HTML 解析（技术契约 §4）：返回纯 React 节点，无注入面。
import type { ReactNode } from "react";

const INLINE_PATTERN = /(\*\*[^*\n]+\*\*|\*[^*\n]+\*)/g;

export function renderInlineMarkdown(text: string): ReactNode[] {
  if (!text) return [];
  return text.split(INLINE_PATTERN).map((part, i) => {
    if (part.startsWith("**") && part.endsWith("**") && part.length > 4) {
      return <strong key={i}>{part.slice(2, -2)}</strong>;
    }
    if (part.startsWith("*") && part.endsWith("*") && part.length > 2) {
      return <em key={i}>{part.slice(1, -1)}</em>;
    }
    return part;
  });
}
