// 类名使用情况分析：回答"样式文件里定义的类名，源码里到底还有没有人用"。
//
// 这是死类名门禁的核心。它必须比"在源码里搜字符串"更聪明一点：
// BEM 修饰类几乎都是拼出来的（`ui-btn--${variant}`），源码里根本不存在
// `ui-btn--primary` 这个字符串，只按字面量查会把它们全部误判成死类名，
// 于是门禁逼着人去删掉**正在生效**的样式——比不检查更糟。
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { parseRules, countImportant } from "./css-parity.mjs";

/** 递归列出目录下的 CSS 文件（绝对路径）。 */
export function listCssFiles(dir, acc = []) {
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) listCssFiles(full, acc);
    else if (name.endsWith(".css")) acc.push(full);
  }
  return acc;
}

/** 递归列出目录下的 ts/tsx 文件（绝对路径）。 */
export function listSourceFiles(dir, acc = []) {
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) listSourceFiles(full, acc);
    else if (name.endsWith(".ts") || name.endsWith(".tsx")) acc.push(full);
  }
  return acc;
}

// 正则字面量的合法"前一个字符"。刻意排除 `<`：JSX 的闭合标签 `</p>` 会紧跟 `<`，
// 一旦把 `</` 当成正则开头，就会一路吞到行尾，把 `</p> : <div className="x">` 整段吃掉
// （`director-columns` 这个在用的类名就是这么被误报成死类名的）。
const REGEX_PRECEDERS = new Set(["(", ",", "=", ":", "[", "!", "&", "|", "?", "{", ";", "+", "-", "*", "%", "~", "^", ">"]);
const REGEX_KEYWORDS = new Set(["return", "typeof", "case", "in", "of", "new", "delete", "void", "instanceof", "do", "else", "yield", "await"]);

/**
 * 判断 `text[i]` 处的 `/` 是否为正则字面量开头。
 *
 * 不做这一步会踩到真实的坑：`dossierMarkdown.tsx` 里有一个处理行内代码的
 * `INLINE_TOKEN_REGEX`，字符类中带反引号。扫描器一旦把它当普通代码，就会在
 * 反引号处进入"模板模式"并一路吞到文件末尾——整个文件的类名全被判成死类名。
 */
function startsRegex(text, i) {
  let j = i - 1;
  while (j >= 0 && /\s/.test(text[j])) {
    if (text[j] === "\n") return true;
    j -= 1;
  }
  if (j < 0) return true;
  if (REGEX_PRECEDERS.has(text[j])) return true;
  const word = /([A-Za-z_$][\w$]*)$/.exec(text.slice(Math.max(0, j - 10), j + 1));
  return word ? REGEX_KEYWORDS.has(word[1]) : false;
}

/** 跳过正则字面量（含字符类与转义），返回收尾 `/` 与 flags 之后的下标。 */
function skipRegex(text, start) {
  let i = start + 1;
  let inClass = false;
  while (i < text.length) {
    const c = text[i];
    if (c === "\\") {
      i += 2;
      continue;
    }
    if (c === "\n") return i;
    if (c === "[") inClass = true;
    else if (c === "]") inClass = false;
    else if (c === "/" && !inClass) {
      i += 1;
      while (i < text.length && /[a-z]/i.test(text[i])) i += 1;
      return i;
    }
    i += 1;
  }
  return i;
}

/**
 * 把源码切成"模板静态片段"与"其余代码"两部分。
 *
 * 必须自建扫描器而不是用正则：反引号里的 `"` 会让朴素的引号配对错位，一旦错位，
 * 后面所有字符串字面量都会配上错误的对，条件类名（`isPinned ? "pinned" : ""`）
 * 就会被误判成死类名。注释直接丢掉——注释里写到的类名不是引用。
 */
export function splitSource(text) {
  const plain = [];
  const templates = [];
  const stack = [{ kind: "code", depth: 0 }];
  let buf = "";
  let quote = "";
  const flush = () => {
    if (buf.trim()) templates.push(buf);
    buf = "";
  };

  for (let i = 0; i < text.length; i += 1) {
    const c = text[i];
    const top = stack[stack.length - 1];

    if (quote) {
      plain.push(c);
      if (c === "\\") {
        plain.push(text[i + 1] ?? "");
        i += 1;
        continue;
      }
      if (c === quote) quote = "";
      continue;
    }

    if (top.kind === "template") {
      if (c === "\\") {
        buf += c + (text[i + 1] ?? "");
        i += 1;
        continue;
      }
      if (c === "`") {
        flush();
        stack.pop();
        continue;
      }
      if (c === "$" && text[i + 1] === "{") {
        flush();
        stack.push({ kind: "expr", depth: 1 });
        plain.push(" ");
        i += 1;
        continue;
      }
      buf += c;
      continue;
    }

    if (c === "/" && text[i + 1] === "*") {
      const end = text.indexOf("*/", i + 2);
      i = end === -1 ? text.length : end + 1;
      continue;
    }
    if (c === "/" && text[i + 1] === "/") {
      const nl = text.indexOf("\n", i + 1);
      i = nl === -1 ? text.length : nl;
      continue;
    }
    if (c === "/" && startsRegex(text, i)) {
      i = skipRegex(text, i) - 1;
      plain.push(" ");
      continue;
    }
    if (c === '"' || c === "'") {
      quote = c;
      plain.push(c);
      continue;
    }
    if (c === "`") {
      stack.push({ kind: "template" });
      continue;
    }
    if (top.kind === "expr") {
      if (c === "{") top.depth += 1;
      else if (c === "}") {
        top.depth -= 1;
        if (top.depth === 0) stack.pop();
        plain.push(c);
        continue;
      }
    }
    plain.push(c);
  }
  flush();
  return { plain: plain.join(""), templates: templates.join(" ") };
}

/**
 * 源码里"可能用到"的类名，两种写法都要认：
 *
 * 1. 字面量：`className="a b"`、`closest(".lore-term")`、`classList.add("is-active")`。
 * 2. 模板静态片段：按空白切开作为**前缀**登记，类名以任一前缀开头即视为被引用
 *    （`ui-btn--` 覆盖 `ui-btn--primary` / `ui-btn--danger` / ...）。
 */
export function referencedClasses(text) {
  const { plain, templates } = splitSource(text);
  const exact = new Set();
  for (const m of plain.matchAll(/["']([^"'\n]*)["']/g)) {
    const body = m[1];
    for (const dotted of body.matchAll(/\.(-?[A-Za-z_][\w-]*)/g)) exact.add(dotted[1]);
    const trimmed = body.trim();
    if (/^[A-Za-z_][\w-]*(\s+[A-Za-z_][\w-]*)*$/.test(trimmed)) {
      for (const token of trimmed.split(/\s+/)) exact.add(token);
    }
  }
  for (const dotted of templates.matchAll(/\.(-?[A-Za-z_][\w-]*)/g)) exact.add(dotted[1]);
  const prefixes = new Set();
  for (const fragment of templates.split(/\s+/)) {
    if (/^[A-Za-z_][\w-]*$/.test(fragment) && fragment.length >= 3) prefixes.add(fragment);
  }
  // 兜底：代码部分占比过低，说明扫描被某个未识别的字面量带偏了。
  // 此时退化成"文中出现的词都算引用"——宁可漏报死类名，也不能误报后逼着人删掉在生效的样式。
  const fallback = new Set();
  if (plain.length < text.length * 0.5) {
    for (const m of text.matchAll(/[A-Za-z_-][\w-]{2,}/g)) fallback.add(m[0]);
  }
  return { exact, prefixes, fallback };
}

/** 从选择器里抽出类名（`.a.b:hover` → a, b）。 */
export function classesOf(selector) {
  return [...selector.matchAll(/\.(-?[A-Za-z_][\w-]*)/g)].map((m) => m[1]);
}

/**
 * 采集全部 CSS 指标：行数、!important、跨文件重复选择器、死类名。
 * 返回的 map 同时带上"类名 → 定义于哪些文件""选择器 → 出现在哪些文件"，供门禁打印证据。
 */
export function collectCssMetrics(root) {
  const metrics = { lineCounts: {}, important: {}, duplicates: [], deadClasses: [] };
  const selectorFiles = new Map();
  const classFiles = new Map();
  for (const full of listCssFiles(join(root, "src"))) {
    const path = relative(root, full).split("\\").join("/");
    const css = readFileSync(full, "utf8");
    metrics.lineCounts[path] = css.split("\n").length;
    const important = countImportant(css);
    if (important > 0) metrics.important[path] = important;
    for (const rule of parseRules(css)) {
      if (!rule.selector || rule.selector.startsWith("@")) continue;
      const key = `${rule.context}||${rule.selector}`;
      if (!selectorFiles.has(key)) selectorFiles.set(key, new Set());
      selectorFiles.get(key).add(path);
      for (const cls of classesOf(rule.selector)) {
        if (!classFiles.has(cls)) classFiles.set(cls, new Set());
        classFiles.get(cls).add(path);
      }
    }
  }
  for (const [key, files] of selectorFiles) {
    if (files.size > 1) metrics.duplicates.push(key);
  }
  metrics.duplicates.sort();
  // 逐文件独立扫描再合并：拼成一整段文本会让一个文件里未闭合的引号/反引号
  // 把后续所有文件都吞掉（状态机串味），那样报出来的"死类名"就不可信了。
  const exact = new Set();
  const prefixes = new Set();
  for (const file of listSourceFiles(join(root, "src"))) {
    const found = referencedClasses(readFileSync(file, "utf8"));
    for (const name of found.exact) exact.add(name);
    for (const name of found.prefixes) prefixes.add(name);
    for (const name of found.fallback ?? []) exact.add(name);
  }
  const prefixList = [...prefixes];
  metrics.deadClasses = [...classFiles.keys()]
    .filter((cls) => !exact.has(cls) && !prefixList.some((p) => cls.startsWith(p)))
    .sort();
  metrics.classFiles = classFiles;
  metrics.selectorFiles = selectorFiles;
  metrics.referenced = { exact, prefixes };
  return metrics;
}
