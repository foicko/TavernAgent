// dossierAttrs: V2/V3 角色卡里常见的「非正式属性表」识别与归一。
//
// 社区卡把结构化属性写成各种土办法：
//   {"Name": ("Gael")}                      —— 方括号内逐行 JSON 风格
//   喜欢：（"年长男性" + "赞美" + "浪漫"）。      —— 中文括号 + 加号连接
//   姓名：西娜·韦尔德                          —— 普通键值行
//   [{{char}}: 物种（"巨人"），年龄（"26岁"）]   —— 单行围成一组
// 直接当普通段落渲染就是一大堵文字墙，这里把它们归一成键值对，
// 交给渲染层排成属性表。

export interface AttrPair {
  key: string;
  value: string;
}

// 键的字符上限：超过这个长度就当成正文句子里的冒号，而不是属性名。
const MAX_ATTR_KEY = 12;
// 值的字符上限：再长就不是属性值，而是需要整段阅读的叙述。
// 只用于③这种无标记的键值行；①②带引号的语法本身已足够明确，不受长度限制。
const MAX_ATTR_VALUE = 30;

// 键由字母/数字/常见连接符组成，长度受 MAX_ATTR_KEY 约束。
const PLAIN_KEY_VALUE_RE = new RegExp(
  `^([\\p{L}\\p{N}·・_\\-/ ]{1,${MAX_ATTR_KEY}})[：:]\\s*(\\S.*)$`,
  "u",
);

// parseAttrLine 尝试把一行解析成属性键值对；不是属性行时返回 null。
export function parseAttrLine(raw: string): AttrPair | null {
  const line = raw.trim();
  if (!line) return null;

  return parseJsonStyleAttr(line) ?? parseParenStyleAttr(line) ?? parsePlainKeyValue(line);
}

// ① {"Key": ("值" + "值")}
// 收尾花括号允许重复：社区卡里手写多一个 } 的情况很常见。
// 值按行锚定后贪婪匹配：值本身可能再嵌括号（如 ("盖尔（Gael） loves to ...")），
// 不能在第一个 ) 处截断。
function parseJsonStyleAttr(line: string): AttrPair | null {
  // 键与冒号是这类行最稳定的特征，先取键。
  const keyMatch = /^\{\s*"([^"]{1,32})"\s*:/.exec(line);
  if (!keyMatch) return null;
  // 规范写法：{"Key": ("值" + "值")}。贪婪匹配右括号，值内嵌套的括号不算结束符。
  const strict = /^\{\s*"([^"]{1,32})"\s*:\s*\(([\s\S]*)\)\s*\}*\s*$/.exec(line);
  // 社区卡常漏写收尾的 ) 或 }（整行以值结尾）：不闭合也按属性行收下，
  // 否则这行会掉回正文，长句直接糊在面板上。
  const loose = /^\{\s*"([^"]{1,32})"\s*:\s*\(?\s*([\s\S]*?)\s*\)?\s*\}*\s*$/.exec(line);
  const raw = strict?.[2] ?? loose?.[2];
  if (raw === undefined) return null;
  const value = normalizeAttrValue(raw);
  if (!value) return null;
  return { key: keyMatch[1].trim(), value };
}

// isContainerLine 判断一行是否只是属性表的容器符号（[ ] { }）。
// 社区卡常把整张属性表包在方括号里，容器符号单独成行时不该渲染成段落。
export function isContainerLine(line: string): boolean {
  const trimmed = line.trim();
  return trimmed.length > 0 && trimmed.length <= 2 && /^[[\]{}()（）]+$/.test(trimmed);
}

// ② Key：（"值" + "值"）。 / Key（"值"）—— 冒号可有可无（整行围成一组时通常省略）
function parseParenStyleAttr(line: string): AttrPair | null {
  // 与①同一处理：按行锚定 + 贪婪，值内部的括号不算结束符。
  const m = /^([^：:()（）[\]\n]{1,16})([：:])?\s*[（(]([\s\S]*)[）)]\s*[。.]?$/.exec(line);
  if (!m) return null;
  const key = m[1].trim();
  const rawValue = m[3];
  // 没有冒号时只认「带引号的值」：否则「她说（笑）。」这类叙述会被误判成属性。
  if (!m[2] && !/^\s*["“'‘「『]/.test(rawValue)) return null;
  const value = normalizeAttrValue(rawValue);
  if (!key || !value) return null;
  return { key, value };
}

// ③ Key：值
function parsePlainKeyValue(line: string): AttrPair | null {
  const m = PLAIN_KEY_VALUE_RE.exec(line);
  if (!m) return null;
  const key = m[1].trim();
  const value = m[2].trim();
  if (!key || !value) return null;
  if (value.length > MAX_ATTR_VALUE) return null;
  // 对白行（「她说：「你好」」）不是属性：值是引号/对白括号开头时让给正文渲染。
  if (/^[「『“"']/.test(value)) return null;
  return { key, value };
}

// ④ [头衔: k（"v"），k2（"v2"）] —— 单个围成一组属性，拆成头衔 + 多个键值对。
export function parseBracketAttrGroup(raw: string): { head: string; pairs: AttrPair[] } | null {
  const line = raw.trim();
  const m = /^\[([^：:[\]]{0,24})[：:]\s*(.+)\]$/.exec(line);
  if (!m) return null;
  const head = m[1].trim();
  const pairs: AttrPair[] = [];
  for (const segment of splitAttrSegments(m[2])) {
    const pair = parseParenStyleAttr(segment) ?? parsePlainKeyValue(segment);
    if (!pair) return null;
    pairs.push(pair);
  }
  if (pairs.length === 0) return null;
  return { head, pairs };
}

// splitAttrSegments 按逗号切分「k（"v"），k2（"v2"）」形式的分组；
// 逗号可能出现在括号内的值里，因此按括号深度切分而不是直接 split。
function splitAttrSegments(body: string): string[] {
  const out: string[] = [];
  let depth = 0;
  let current = "";
  for (const ch of body) {
    if (ch === "（" || ch === "(" || ch === "「") depth++;
    if (ch === "）" || ch === ")" || ch === "」") depth = Math.max(0, depth - 1);
    if ((ch === "，" || ch === ",") && depth === 0) {
      if (current.trim()) out.push(current.trim());
      current = "";
      continue;
    }
    current += ch;
  }
  if (current.trim()) out.push(current.trim());
  return out;
}

// normalizeAttrValue 把 '("a" + "b")' 这类值拆成可读的顿号列表，去掉引号与括号。
export function normalizeAttrValue(raw: string): string {
  const parts = raw
    .split(/\s*[+＋]\s*/)
    .map(stripValueWrapper)
    .filter(Boolean);
  if (parts.length === 0) return stripValueWrapper(raw);
  return parts.join("、");
}

// stripValueWrapper 去掉值最外层的引号与配对包围号。
// 必须是「配对」才剥：值内部的括号是内容（高个子（1.8 米）），
// 无脑裁剪会把内容吃掉一半。引号与括号会交替包裹（`（"a"）`），
// 因此循环剥到稳定为止。
function stripValueWrapper(part: string): string {
  let value = part.trim();
  for (let round = 0; round < 4; round++) {
    const before = value;
    value = value.replace(/^["“”'‘’「」『』]+/, "").replace(/["“”'‘’「」『』]+$/, "").trim();
    const opens = (value.match(/[（(]/g) ?? []).length;
    const closes = (value.match(/[）)]/g) ?? []).length;
    if (opens > closes && /^[（(]/.test(value)) {
      value = value.slice(1).trim();
    }
    if (closes > opens && /[）)]$/.test(value)) {
      value = value.slice(0, -1).trim();
    }
    if (value === before) break;
  }
  return value;
}
