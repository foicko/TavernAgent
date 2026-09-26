// truncateText：按码点截断展示文本，只在真正截断时追加省略号。
//
// 直接用 String.prototype.slice 会按 UTF-16 码元切，可能把 emoji 等代理对劈成半个
// 乱码字符；而且无条件拼 "..." 会给本来就很短的文本加上多余的省略号。
export function truncateText(text: string, maxChars: number, ellipsis = "…"): string {
  const chars = Array.from(text);
  if (chars.length <= maxChars) return text;
  return chars.slice(0, maxChars).join("") + ellipsis;
}
