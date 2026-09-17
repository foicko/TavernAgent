// 模型配置的展示文案唯一来源。
//
// 三条硬规则（对应重构前反复出现的界面问题）：
// 1. 同一个模型名在同屏只出现一次 —— 迁移会把实例名默认写成模型名，
//    拼接 "name · model" 就会得到 "deepseek-flash · deepseek-flash"。
// 2. 界面只说人话：不出现 slot id（primary/assist/reflection）、
//    不出现 "OpenAI 兼容 · Chat Completions" 这类协议全称。
// 3. 永不渲染可疑的长密钥：脱敏值长度异常时自行再截断。
import type { ModelInstance } from "../app/types";

export type Role = "primary" | "assist" | "reflection";

/** 界面顺序：主线角色在前。 */
export const ROLE_ORDER: readonly Role[] = ["primary", "assist", "reflection"];

/** 角色名（给用户看的用途，不是槽位 id）。 */
export const ROLE_LABELS: Record<Role, string> = {
  primary: "对话生成",
  assist: "导演协商",
  reflection: "后台记忆",
};

/** 角色用途说明。 */
export const ROLE_HINTS: Record<Role, string> = {
  primary: "推进剧情时使用",
  assist: "导演模式下与模型协商大纲",
  reflection: "后台抽取记忆、维护摘要",
};

/** 非主角色留空时的含义。 */
export const FOLLOW_PRIMARY = "跟随对话生成";

/** 角色回落：非主线角色未指派时跟随主线。 */
export function isRole(value: string): value is Role {
  return (ROLE_ORDER as readonly string[]).includes(value);
}

export function roleLabel(slot: string): string {
  return isRole(slot) ? ROLE_LABELS[slot] : slot;
}

export function roleHint(slot: string): string {
  return isRole(slot) ? ROLE_HINTS[slot] : "";
}

/** 协议选项：值 → 人话标签。 */
export const PROTOCOL_OPTIONS: ReadonlyArray<{ value: string; label: string }> = [
  { value: "openai-chat", label: "OpenAI 兼容" },
  { value: "openai-responses", label: "OpenAI Responses" },
  { value: "anthropic-messages", label: "Anthropic" },
];

export const DEFAULT_PROTOCOL = "openai-chat";

export function protocolLabel(kind?: string): string {
  if (!kind) return "";
  return PROTOCOL_OPTIONS.find((option) => option.value === kind)?.label ?? kind;
}

/** 思考强度档位：值 → 人话标签（空串 = 默认，不发送任何思考参数）。 */
export const EFFORT_OPTIONS: ReadonlyArray<{ value: string; label: string; hint: string }> = [
  { value: "", label: "默认", hint: "不发送思考参数，完全按供应商自己的默认行为" },
  { value: "low", label: "低", hint: "思考最少，响应最快" },
  { value: "medium", label: "中", hint: "速度与深度的折中" },
  { value: "high", label: "高", hint: "思考最充分，响应最慢" },
];

export function effortLabel(value?: string): string {
  if (!value) return "默认";
  return EFFORT_OPTIONS.find((option) => option.value === value)?.label ?? value;
}

export function effortHint(value?: string): string {
  return EFFORT_OPTIONS.find((option) => option.value === (value ?? ""))?.hint ?? "";
}

/**
 * 派生输入预算（只用于展示）：与后端 compile.go 的 WithBudget 同公式 ——
 * 输入预算 = 窗口 − 输出 − max(512, 窗口/20)。窗口未设置时返回 null。
 */
export function inputBudget(contextWindow?: number | null, maxTokens?: number | null): number | null {
  const window = contextWindow ?? 0;
  if (window <= 0) return null;
  const output = maxTokens && maxTokens > 0 ? maxTokens : 2048;
  const margin = Math.max(512, Math.floor(window / 20));
  return window - output - margin;
}

/** token 数量的口语化写法：1024 以上折算成 k。 */
export function tokenLabel(tokens: number): string {
  if (tokens >= 1024) return `${Math.round(tokens / 1024)}k`;
  return `${tokens}`;
}

/** 从 Base URL 取主机名（用于副标题，避免重复模型名）。 */
export function connectionHost(instance: Pick<ModelInstance, "baseUrl">): string {
  const raw = (instance.baseUrl ?? "").trim();
  if (!raw) return "";
  try {
    const url = new URL(/^https?:\/\//i.test(raw) ? raw : `https://${raw}`);
    return url.host;
  } catch {
    return "";
  }
}

/**
 * 连接标题：优先实例名；名称为空或等于模型名时只保留模型名一份。
 */
export function connectionTitle(instance: Pick<ModelInstance, "name" | "model">): string {
  const name = (instance.name ?? "").trim();
  const model = (instance.model ?? "").trim();
  if (!name) return model || "未命名连接";
  if (model && name.toLowerCase() === model.toLowerCase()) return model;
  return name;
}

/**
 * 连接副标题：补上标题里没有的信息（模型名 / 主机名），并逐段去重。
 * 标题已包含模型名时不重复；主机名与已有片段重合时也不重复。
 */
export function connectionSubtitle(instance: Pick<ModelInstance, "name" | "model" | "baseUrl">): string {
  const title = connectionTitle(instance).toLowerCase();
  const model = (instance.model ?? "").trim();
  const host = connectionHost(instance);
  const parts: string[] = [];

  if (model && model.toLowerCase() !== title && !title.includes(model.toLowerCase())) parts.push(model);
  if (host && host.toLowerCase() !== title && !parts.some((part) => part.toLowerCase().includes(host.toLowerCase()))) {
    parts.push(host);
  }
  return parts.join(" · ");
}

/**
 * 下拉选项文案。重名时（两个连接同名同模型）追加主机名消歧，
 * 保证不会出现两个字面相同的选项。
 */
export function optionLabels(instances: ModelInstance[]): Map<string, string> {
  const base = instances.map((instance) => connectionTitle(instance));
  const counts = new Map<string, number>();
  for (const label of base) counts.set(label, (counts.get(label) ?? 0) + 1);

  const labels = new Map<string, string>();
  instances.forEach((instance, index) => {
    const label = base[index];
    if ((counts.get(label) ?? 0) <= 1) {
      labels.set(instance.id, label);
      return;
    }
    const host = connectionHost(instance);
    labels.set(instance.id, host ? `${label} · ${host}` : `${label} · ${protocolLabel(instance.kind)}`);
  });
  return labels;
}

/** 上下文窗口：按 k 展示；缺省不给"0k"这种噪音。 */
export function windowLabel(contextWindow?: number | null): string {
  if (!contextWindow || contextWindow <= 0) return "窗口未设置";
  if (contextWindow >= 1024 * 1024) return `${Math.round(contextWindow / (1024 * 1024))}M 窗口`;
  return `${Math.round(contextWindow / 1024)}k 窗口`;
}

export function keyStateLabel(hasApiKey?: boolean): string {
  return hasApiKey ? "密钥已存" : "无密钥";
}

/**
 * 密钥提示：只允许出现掩码形态。
 * 正常情况下服务端已脱敏（sk-1…abcd）；若长度异常（例如误传了明文），
 * 这里再截断一次，确保界面永不渲染完整密钥。
 */
export function maskedKeyHint(masked?: string): string {
  const value = (masked ?? "").trim();
  if (!value) return "已保存";
  if (value.includes("…")) return value;
  if (value.length <= 12) return value;
  return `${value.slice(0, 4)}…${value.slice(-4)}`;
}
