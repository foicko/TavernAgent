// 样式等价性比对（评分文档 P0 项的安全网）。
//
// 把 CSS 解析成"at 规则上下文 + 选择器 + 声明"三元组，按**集合**比对重构前后的
// 构建产物。为什么是集合而不是多重集：拆分/去重的合法结果恰恰是"同一选择器下的
// 同一声明不再重复出现"，多重集必然不相等；而渲染结果只取决于集合。
//
// 用法：
//   node scripts/css-parity.mjs snapshot [--css <file>] [--out <file>]
//   node scripts/css-parity.mjs compare --before <snapshot.json> [--css <file>]
//                                        [--allow <allow.json>] [--out <diff.txt>] [--report]
//
// --css 默认取 web/dist/assets 下最大的 .css（即主样式包，先 pnpm build）。
// compare 默认严格：出现未被 --allow 命中的差异即退出码 1。
//
// 同时导出解析函数供 check.mjs 复用（重复选择器 / !important 计数 / 死类名都基于同一套解析）。
import { readFileSync, writeFileSync, readdirSync, statSync, mkdirSync, existsSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

export const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), "..");
/** 验证产物统一落在仓库根的 output/（.gitignore 已忽略，与 Playwright 证据同级）。 */
export const ARTIFACT_DIR = resolve(ROOT, "..", "output", "css-parity");

// ---------------------------------------------------------------- 扫描器

/** 跳过 `/* ... *\/` 注释，返回其后的下标。 */
function skipComment(css, i) {
  const end = css.indexOf("*/", i + 2);
  return end === -1 ? css.length : end + 2;
}

/** 读一个字符串字面量（含转义），返回 [原文本, 结束下标]。 */
function readString(css, i) {
  const quote = css[i];
  let out = quote;
  let j = i + 1;
  while (j < css.length) {
    const c = css[j];
    if (c === "\\") {
      out += c + (css[j + 1] ?? "");
      j += 2;
      continue;
    }
    out += c;
    j += 1;
    if (c === quote) break;
  }
  return [out, j];
}

/** 找到与 openIndex 处 `{` 配对的 `}`，返回其下标（未闭合则返回末尾）。 */
function matchBrace(css, openIndex) {
  let depth = 0;
  let i = openIndex;
  while (i < css.length) {
    const c = css[i];
    if (c === "/" && css[i + 1] === "*") {
      i = skipComment(css, i);
      continue;
    }
    if (c === '"' || c === "'") {
      [, i] = readString(css, i);
      continue;
    }
    if (c === "{") depth += 1;
    else if (c === "}") {
      depth -= 1;
      if (depth === 0) return i;
    }
    i += 1;
  }
  return css.length;
}

/** 按顶层分隔符切分（忽略字符串、注释与括号内部的同名字符）。 */
function splitTopLevel(text, sep) {
  const out = [];
  let buf = "";
  let depth = 0;
  let i = 0;
  while (i < text.length) {
    const c = text[i];
    if (c === "/" && text[i + 1] === "*") {
      i = skipComment(text, i);
      continue;
    }
    if (c === '"' || c === "'") {
      const [s, ni] = readString(text, i);
      buf += s;
      i = ni;
      continue;
    }
    if (c === "(" || c === "[") depth += 1;
    else if (c === ")" || c === "]") depth -= 1;
    if (c === sep && depth === 0) {
      out.push(buf);
      buf = "";
      i += 1;
      continue;
    }
    buf += c;
    i += 1;
  }
  out.push(buf);
  return out;
}

// ---------------------------------------------------------------- 规范化

/** 只在括号外归一化组合器空格：`:nth-child(2n+1)` 里的 `+` 不能被当成组合器。 */
function normalizeCombinators(selector) {
  let out = "";
  let depth = 0;
  for (let i = 0; i < selector.length; i += 1) {
    const c = selector[i];
    if (c === "(" || c === "[") depth += 1;
    else if (c === ")" || c === "]") depth -= 1;
    if (depth === 0 && (c === ">" || c === "+" || c === "~")) {
      out = `${out.trimEnd()} ${c} `;
      continue;
    }
    out += c;
  }
  return out.replace(/\s+/g, " ").trim();
}

/** 选择器列表：逐项去空白 + 组合器归一并**排序**，因此 `.a, .b` 与 `.b, .a` 等价。 */
export function normalizeSelector(selector) {
  return splitTopLevel(selector, ",")
    .map((s) => normalizeCombinators(s))
    .filter(Boolean)
    .sort()
    .join(", ");
}

/** at 规则前奏：小写化名字并折叠空白，保证 `@media(max-width:768px)` 与展开写法等价。 */
function normalizePrelude(prelude) {
  return prelude
    .replace(/\s+/g, " ")
    .replace(/\s*:\s*/g, ": ")
    .trim()
    .toLowerCase();
}

function findTopLevelColon(decl) {
  let depth = 0;
  for (let i = 0; i < decl.length; i += 1) {
    const c = decl[i];
    if (c === "(" || c === "[") depth += 1;
    else if (c === ")" || c === "]") depth -= 1;
    else if (c === ":" && depth === 0) return i;
  }
  return -1;
}

/** 声明归一化：属性小写（自定义属性大小写敏感，保持原样），值折叠空白与逗号后空格。 */
export function normalizeDecl(decl) {
  const idx = findTopLevelColon(decl);
  if (idx === -1) return "";
  const prop = decl.slice(0, idx).trim();
  if (!prop) return "";
  let value = decl.slice(idx + 1).replace(/\s+/g, " ").trim();
  // content 的值是字符串，动它可能掩盖真实差异。
  if (prop.toLowerCase() !== "content") value = value.replace(/\s*,\s*/g, ", ");
  return `${prop.startsWith("--") ? prop : prop.toLowerCase()}:${value}`;
}

// ---------------------------------------------------------------- 解析

const NESTING_AT_RULES = new Set(["@media", "@supports", "@layer", "@container", "@scope"]);
const OPAQUE_AT_RULES = new Set([
  "@keyframes",
  "@-webkit-keyframes",
  "@-moz-keyframes",
  "@font-face",
  "@page",
  "@property",
  "@counter-style",
  "@font-feature-values",
]);

/**
 * 解析成规则列表：{ context, selector, decls[] }。
 * context 是外层 at 规则前奏链（`A || B`），保证不同媒体查询下的同名选择器互不冲突。
 */
export function parseRules(css, context = [], out = []) {
  let buf = "";
  let i = 0;
  while (i < css.length) {
    const c = css[i];
    if (c === "/" && css[i + 1] === "*") {
      i = skipComment(css, i);
      continue;
    }
    if (c === '"' || c === "'") {
      const [s, ni] = readString(css, i);
      buf += s;
      i = ni;
      continue;
    }
    if (c === "{") {
      const end = matchBrace(css, i);
      const prelude = buf.trim();
      const body = css.slice(i + 1, end);
      buf = "";
      const name = prelude.split(/[\s({]/)[0].toLowerCase();
      if (NESTING_AT_RULES.has(name)) {
        parseRules(body, [...context, normalizePrelude(prelude)], out);
      } else if (OPAQUE_AT_RULES.has(name)) {
        // 关键帧/字体面等整体当一条规则：内部结构不重要，逐字比对即可。
        out.push({
          context: context.join(" || "),
          selector: normalizePrelude(prelude),
          decls: [`body:${normalizePrelude(body)}`],
        });
      } else if (prelude && !prelude.startsWith("@")) {
        out.push({
          context: context.join(" || "),
          selector: normalizeSelector(prelude),
          decls: splitTopLevel(body, ";").map((d) => normalizeDecl(d.trim())).filter(Boolean),
        });
      }
      i = end + 1;
      continue;
    }
    buf += c;
    i += 1;
  }
  return out;
}

/** 选择器 → 出现次数（含 at 规则上下文），用于跨文件重复检测。 */
export function selectorOccurrences(css) {
  const seen = new Map();
  for (const rule of parseRules(css)) {
    if (!rule.selector || rule.selector.startsWith("@")) continue;
    const key = `${rule.context}||${rule.selector}`;
    seen.set(key, (seen.get(key) ?? 0) + 1);
  }
  return seen;
}

/** 全部规则展开成 `context||selector{decl}` 的多重集（值为出现次数）。 */
export function entryMultiset(css) {
  const map = new Map();
  for (const rule of parseRules(css)) {
    for (const decl of rule.decls) {
      const entry = `${rule.context}||${rule.selector}{${decl}}`;
      map.set(entry, (map.get(entry) ?? 0) + 1);
    }
  }
  return map;
}

/** 统计 `!important` 出现次数（门禁用，与解析口径一致：注释与字符串里的不算）。 */
export function countImportant(css) {
  let n = 0;
  let i = 0;
  while (i < css.length) {
    const c = css[i];
    if (c === "/" && css[i + 1] === "*") {
      i = skipComment(css, i);
      continue;
    }
    if (c === '"' || c === "'") {
      [, i] = readString(css, i);
      continue;
    }
    if (c === "!" && /^!\s*important/i.test(css.slice(i, i + 12))) n += 1;
    i += 1;
  }
  return n;
}

// ---------------------------------------------------------------- CLI

function parseArgs(argv) {
  const out = { _: [] };
  for (let i = 0; i < argv.length; i += 1) {
    const a = argv[i];
    if (a.startsWith("--")) {
      const name = a.slice(2);
      const next = argv[i + 1];
      if (next === undefined || next.startsWith("--")) out[name] = true;
      else {
        out[name] = next;
        i += 1;
      }
    } else out._.push(a);
  }
  return out;
}

/** 构建产物里最大的 .css —— 主样式包（先 pnpm build）。 */
export function defaultBuiltCss() {
  const dir = join(ROOT, "dist", "assets");
  const files = readdirSync(dir).filter((f) => f.endsWith(".css"));
  if (files.length === 0) throw new Error(`dist/assets 下没有 .css，请先运行 pnpm build（${dir}）`);
  return files
    .map((f) => join(dir, f))
    .sort((a, b) => statSync(b).size - statSync(a).size)[0];
}

function resolveCss(args) {
  return args.css && args.css !== true ? resolve(args.css) : defaultBuiltCss();
}

function loadAllow(path) {
  if (!path || path === true) return [];
  const raw = JSON.parse(readFileSync(resolve(path), "utf8"));
  return Array.isArray(raw) ? raw : (raw.allow ?? []);
}

function isAllowed(entry, allow) {
  return allow.some((pattern) => entry.includes(pattern));
}

function diffEntries(before, after, allow) {
  const removed = [];
  const added = [];
  for (const [entry, count] of before) {
    const now = after.get(entry) ?? 0;
    if (now < count && !isAllowed(entry, allow)) {
      removed.push({ entry, before: count, after: now });
    }
  }
  for (const [entry, count] of after) {
    const was = before.get(entry) ?? 0;
    if (was < count && !isAllowed(entry, allow)) {
      added.push({ entry, before: was, after: count });
    }
  }
  return { removed, added };
}

function writeTextIfNeeded(path, text) {
  if (!path || path === true) return;
  mkdirSync(dirname(resolve(path)), { recursive: true });
  writeFileSync(resolve(path), text, "utf8");
}

function formatFullDiff({ removed, added }) {
  const lines = [`移除 ${removed.length} 条，新增 ${added.length} 条`];
  for (const item of removed) lines.push(`- [${item.before}→${item.after}] ${item.entry}`);
  for (const item of added) lines.push(`+ [${item.before}→${item.after}] ${item.entry}`);
  return lines.join("\n");
}

function formatDiff({ removed, added }, limit = 40) {
  const lines = [];
  lines.push(`移除 ${removed.length} 条，新增 ${added.length} 条`);
  for (const item of removed.slice(0, limit)) {
    lines.push(`  - [${item.before}→${item.after}] ${item.entry}`);
  }
  if (removed.length > limit) lines.push(`  ...（其余 ${removed.length - limit} 条见 --out 输出）`);
  for (const item of added.slice(0, limit)) {
    lines.push(`  + [${item.before}→${item.after}] ${item.entry}`);
  }
  if (added.length > limit) lines.push(`  ...（其余 ${added.length - limit} 条见 --out 输出）`);
  return lines.join("\n");
}

function run() {
  const args = parseArgs(process.argv.slice(2));
  const command = args._[0];

  if (command === "snapshot") {
    const cssPath = resolveCss(args);
    const css = readFileSync(cssPath, "utf8");
    const multiset = entryMultiset(css);
    const payload = {
      meta: {
        css: cssPath,
        bytes: Buffer.byteLength(css),
        entries: multiset.size,
        declared: [...multiset.values()].reduce((a, b) => a + b, 0),
      },
      entries: Object.fromEntries([...multiset.entries()].sort()),
    };
    const outPath = args.out && args.out !== true ? args.out : join(ARTIFACT_DIR, "baseline.json");
    mkdirSync(dirname(resolve(outPath)), { recursive: true });
    writeFileSync(resolve(outPath), `${JSON.stringify(payload, null, 2)}\n`, "utf8");
    console.log(`✓ 快照 ${payload.meta.entries} 条声明（${payload.meta.declared} 次出现）→ ${outPath}`);
    return 0;
  }

  if (command === "compare") {
    // --before 默认取基线快照，因此重构期的标准动作就是 `pnpm build:parity && pnpm css:parity`。
    const beforePath = args.before && args.before !== true ? resolve(args.before) : join(ARTIFACT_DIR, "baseline.json");
    if (!existsSync(beforePath)) {
      console.error(`✗ 找不到基线快照 ${beforePath}（先用 pnpm css:snapshot 落基线）`);
      return 2;
    }
    const cssPath = resolveCss(args);
    const before = new Map(Object.entries(JSON.parse(readFileSync(beforePath, "utf8")).entries));
    const after = entryMultiset(readFileSync(cssPath, "utf8"));
    const allow = loadAllow(args.allow);
    const diff = diffEntries(before, after, allow);
    const report = formatDiff(diff);
    console.log(report);
    // --out 写**完整**清单：评审要逐条核验差异，控制台只适合看摘要。
    writeTextIfNeeded(args.out, `${formatFullDiff(diff)}\n`);
    if (diff.removed.length === 0 && diff.added.length === 0) {
      console.log(`✓ 声明集合等价（${before.size} 条）`);
      return 0;
    }
    if (args.report) {
      console.log(`⚠️  存在差异（--report 模式，不计为失败）：移除 ${diff.removed.length} / 新增 ${diff.added.length}`);
      return 0;
    }
    console.error(`✗ 声明集合不等价：移除 ${diff.removed.length} / 新增 ${diff.added.length}`);
    return 1;
  }

  console.error("用法：node scripts/css-parity.mjs snapshot|compare [--css <file>] [--out <file>] [--before <file>] [--allow <file>] [--report]");
  return 2;
}

if (process.argv[1] && resolve(process.argv[1]) === resolve(fileURLToPath(import.meta.url))) {
  process.exitCode = run();
}
