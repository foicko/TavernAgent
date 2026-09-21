// 轻量门禁检查（D6 工程门禁的轻量化版本）：
// 1) CSS token 契约：styles/tokens.css 必须声明设计令牌，且 styles.css 必须 import 它
// 2) 组件层级：web/src/components 只允许 import 自 app/stores/lib/ui/react，禁止跨层反向依赖
// 3) 原语层级：web/src/ui 是叶子层，只允许依赖 react 与 ../lib（禁止依赖 stores/app/components）
// 4) CSS 预算与去重：文件行数、!important、跨文件重复选择器、死类名四项**棘轮**门禁
//
// 棘轮的含义：基线（scripts/css-baseline.json）记录"已经存在的债"，只允许变小不允许变大；
// 新文件/新条目按零容忍处理。收紧基线用 `node scripts/check.mjs --update-css-baseline`，
// 该动作会打印出被放宽的条目，便于评审时发现"顺手把上限调高"。
import { readFileSync, readdirSync, writeFileSync, existsSync } from "node:fs";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { collectCssMetrics } from "./css-classes.mjs";

const root = join(fileURLToPath(new URL(".", import.meta.url)), "..");
const tokensPath = join(root, "src", "styles", "tokens.css");
const stylesPath = join(root, "src", "styles.css");

// ---- 1) CSS token 契约 ----
// 令牌契约：唯一来源是 src/styles/tokens.css。
const requiredTokens = [
  // 兼容别名（历史样式沿用）
  "--bg", "--bg-soft", "--surface", "--border", "--text", "--muted",
  "--accent", "--danger", "--rr", "--r-m", "--r-l", "--font", "--ease",
  // 表面 / 边框 / 文本 / 强调
  "--bg-base", "--bg-canvas", "--bg-surface", "--bg-subtle", "--bg-elevated",
  "--border-light", "--border-mid", "--border-dark",
  "--text-main", "--text-muted", "--text-dim",
  "--accent-solid", "--accent-contrast",
  // 圆角 / 间距 / 字号 / 阴影 / 层级
  "--radius-sm", "--radius-md", "--radius-lg", "--radius-full",
  "--space-1", "--space-2", "--space-3", "--space-4", "--space-5", "--space-6",
  "--fs-xs", "--fs-sm", "--fs-md", "--fs-lg", "--fs-xl",
  "--shadow-sm", "--shadow-md", "--shadow-float",
  "--z-dropdown", "--z-modal", "--z-toast",
  // 字体族
  "--font-display", "--font-body", "--font-narrative",
];

let tokensCss = "";
try {
  tokensCss = readFileSync(tokensPath, "utf8");
} catch {
  console.error("✗ 找不到 src/styles/tokens.css");
  process.exit(1);
}

// 只认"声明"（--name:），避免把 var(--name) 的引用当成定义。
const declared = new Set([...tokensCss.matchAll(/(--[a-zA-Z0-9-]+)\s*:/g)].map((m) => m[1]));
const missing = requiredTokens.filter((t) => !declared.has(t));
if (missing.length > 0) {
  console.error(`✗ tokens.css 缺少 token 声明: ${missing.join(", ")}`);
  process.exitCode = 1;
} else {
  console.log(`✓ CSS token 契约通过（${declared.size} 个令牌声明）`);
}

let stylesCss = "";
try {
  stylesCss = readFileSync(stylesPath, "utf8");
} catch {
  console.error("✗ 找不到 styles.css");
  process.exit(1);
}
if (!/@import\s+["']\.\/styles\/tokens\.css["']/.test(stylesCss)) {
  console.error("✗ styles.css 未 import ./styles/tokens.css，令牌不会进入产物");
  process.exitCode = 1;
}

// ---- 2) 组件层级检查 ----
const compDir = join(root, "src", "components");
const allowedPrefixes = ["react", "zustand/react/shallow", "../app/", "../stores/", "../lib/", "../ui/", "../styles", "./"];
let layerOk = true;
for (const f of readdirSync(compDir)) {
  if (!f.endsWith(".tsx") && !f.endsWith(".ts")) continue;
  const src = readFileSync(join(compDir, f), "utf8");
  for (const line of src.split("\n")) {
    const m = line.match(/from\s+["']([^"']+)["']/);
    if (!m) continue;
    const spec = m[1];
    if (!allowedPrefixes.some((p) => spec.startsWith(p))) {
      console.error(`✗ ${f} 跨层导入 ${spec}（components 只允许 app/stores/lib/ui/react）`);
      layerOk = false;
    }
  }
}
if (!layerOk) process.exitCode = 1;
else console.log("✓ 组件层级检查通过");

// ---- 3) 原语层级检查（ui 为叶子层） ----
const uiDir = join(root, "src", "ui");
const uiAllowed = ["react", "../lib/", "./"];
let uiOk = true;
for (const f of readdirSync(uiDir)) {
  if (!f.endsWith(".tsx") && !f.endsWith(".ts")) continue;
  const src = readFileSync(join(uiDir, f), "utf8");
  for (const line of src.split("\n")) {
    const m = line.match(/from\s+["']([^"']+)["']/);
    if (!m) continue;
    const spec = m[1];
    if (!uiAllowed.some((p) => spec.startsWith(p))) {
      console.error(`✗ ui/${f} 跨层导入 ${spec}（ui 只允许 react / ../lib / 同级）`);
      uiOk = false;
    }
  }
}
if (!uiOk) process.exitCode = 1;
else console.log("✓ 原语层级检查通过");

// ---- 4) CSS 预算与去重（棘轮门禁） ----

/** 单文件行数上限：与 Go 侧架构门禁的 MAX_FILE_LINES=800 保持一致。 */
const MAX_FILE_LINES = 800;
/** 基线：只记录"已经存在的债"，见文件头注释。 */
const baselinePath = join(root, "scripts", "css-baseline.json");

const metrics = collectCssMetrics(root);
const updateBaseline = process.argv.includes("--update-css-baseline");

if (updateBaseline) {
  const previous = existsSync(baselinePath)
    ? JSON.parse(readFileSync(baselinePath, "utf8"))
    : { fileLines: {}, important: {}, duplicates: [], deadClasses: [] };
  const next = {
    $comment: "由 node scripts/check.mjs --update-css-baseline 生成；只允许收紧，放宽会在输出中打印。",
    maxFileLines: MAX_FILE_LINES,
    // 只记录超过上限的文件：上限内的文件不需要豁免。
    fileLines: Object.fromEntries(
      Object.entries(metrics.lineCounts).filter(([, n]) => n > MAX_FILE_LINES).sort(),
    ),
    important: Object.fromEntries(Object.entries(metrics.important).sort()),
    duplicates: metrics.duplicates,
    deadClasses: metrics.deadClasses,
  };
  const relaxed = [];
  // 遍历两边键的并集：只看旧键会漏掉"新增的文件里带了多少 !important"。
  for (const path of new Set([...Object.keys(previous.fileLines ?? {}), ...Object.keys(next.fileLines)])) {
    const was = previous.fileLines?.[path] ?? 0;
    const now = next.fileLines[path] ?? 0;
    if (now > was) relaxed.push(`fileLines ${path}: ${was} → ${now}`);
  }
  const importantTotal = (table) => Object.values(table ?? {}).reduce((a, b) => a + b, 0);
  for (const path of new Set([...Object.keys(previous.important ?? {}), ...Object.keys(next.important)])) {
    const was = previous.important?.[path] ?? 0;
    const now = next.important[path] ?? 0;
    if (now > was) relaxed.push(`important ${path}: ${was} → ${now}`);
  }
  for (const key of next.duplicates) {
    if (!(previous.duplicates ?? []).includes(key)) relaxed.push(`duplicate ${key}`);
  }
  for (const cls of next.deadClasses) {
    if (!(previous.deadClasses ?? []).includes(cls)) relaxed.push(`deadClass .${cls}`);
  }
  writeFileSync(baselinePath, `${JSON.stringify(next, null, 2)}\n`, "utf8");
  console.log(`✓ CSS 基线已更新：${relative(root, baselinePath)}`);
  console.log(
    `  !important 总量：${importantTotal(previous.important)} → ${importantTotal(next.important)}；` +
      `跨文件重复选择器：${(previous.duplicates ?? []).length} → ${next.duplicates.length}；` +
      `死类名：${(previous.deadClasses ?? []).length} → ${next.deadClasses.length}`,
  );
  if (relaxed.length > 0) {
    console.error(`⚠️  基线被放宽 ${relaxed.length} 项（评审时必须解释原因）：`);
    for (const line of relaxed) console.error(`   + ${line}`);
  }
  process.exit(0);
}

const baseline = JSON.parse(readFileSync(baselinePath, "utf8"));
let cssOk = true;
const stale = [];

// 4.1 行数棘轮：上限是硬顶，基线只豁免"改动前就已超限"的文件。
for (const [path, lines] of Object.entries(metrics.lineCounts)) {
  const allowed = Math.max(MAX_FILE_LINES, baseline.fileLines?.[path] ?? 0);
  if (lines > allowed) {
    console.error(`✗ ${path} 有 ${lines} 行，超过上限 ${allowed}（拆分成多个文件，或先说明为何放宽基线）`);
    cssOk = false;
  } else if (lines <= MAX_FILE_LINES && baseline.fileLines?.[path]) {
    stale.push(`fileLines ${path} 已回落到上限内，可从基线移除`);
  }
}

// 4.2 !important 棘轮：新文件零容忍；存量文件只允许变少。
for (const [path, count] of Object.entries(metrics.important)) {
  const allowed = baseline.important?.[path] ?? 0;
  if (count > allowed) {
    console.error(`✗ ${path} 新增 !important（${allowed} → ${count}）：请改用更具体的选择器或令牌`);
    cssOk = false;
  } else if (count < allowed) {
    stale.push(`important ${path}: ${allowed} → ${count}`);
  }
}

// 4.3 跨文件重复选择器棘轮：同一选择器在多个文件中重复声明会让层叠顺序难以推理。
const baselineDuplicates = new Set(baseline.duplicates ?? []);
for (const key of metrics.duplicates) {
  if (!baselineDuplicates.has(key)) {
    const files = [...metrics.selectorFiles.get(key)];
    console.error(`✗ 选择器重复声明于多个文件：${key}（${files.join(" , ")}）`);
    cssOk = false;
  }
}
for (const key of baselineDuplicates) {
  if (!metrics.duplicates.includes(key)) stale.push(`duplicate ${key} 已消除`);
}

// 4.4 死类名棘轮：样式文件里定义、但源码里没有任何引用的类名。
const baselineDead = new Set(baseline.deadClasses ?? []);
for (const cls of metrics.deadClasses) {
  if (!baselineDead.has(cls)) {
    const files = [...metrics.classFiles.get(cls)];
    console.error(
      `✗ 死类名 .${cls}（定义于 ${files.join(" , ")}，src 下无引用）：` +
        "删掉它，或加入 css-baseline.json 的 deadClasses 并说明是否为动态拼接",
    );
    cssOk = false;
  }
}
for (const cls of baselineDead) {
  if (!metrics.deadClasses.includes(cls)) stale.push(`deadClass .${cls} 已消除`);
}

if (cssOk) {
  console.log(
    `✓ CSS 预算检查通过（${Object.keys(metrics.lineCounts).length} 个文件，` +
      `${Object.values(metrics.important).reduce((a, b) => a + b, 0)} 处 !important，` +
      `${metrics.duplicates.length} 个跨文件重复选择器，${metrics.deadClasses.length} 个已知死类名）`,
  );
} else {
  process.exitCode = 1;
}
if (stale.length > 0) {
  console.log(`提示：${stale.length} 项债已偿还，运行 node scripts/check.mjs --update-css-baseline 收紧基线`);
}
