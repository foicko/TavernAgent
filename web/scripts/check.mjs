// 轻量门禁检查（D6 工程门禁的轻量化版本）：
// 1) CSS token 契约：styles/tokens.css 必须声明设计令牌，且 styles.css 必须 import 它
// 2) 组件层级：web/src/components 只允许 import 自 app/stores/lib/ui/react，禁止跨层反向依赖
// 3) 原语层级：web/src/ui 是叶子层，只允许依赖 react 与 ../lib（禁止依赖 stores/app/components）
import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

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
