// 服务商预设：把"新建连接"从 6 个字段降到"选服务商 → 填密钥"。
//
// baseUrl 只需填到版本根（如 https://api.deepseek.com 或 http://127.0.0.1:11434/v1）：
// 三个适配器会各自补全具体路径（/chat/completions、/responses、/v1/messages）。
import type { ModelInstance } from "../app/types";
import { DEFAULT_PROTOCOL } from "./modelLabels";

export interface ProviderPreset {
  id: string;
  label: string;
  kind: string;
  baseUrl: string;
  /** 常见模型名，作为下拉建议（仍可自由输入）。 */
  models: readonly string[];
  /** 密钥从哪里来 / 是否需要密钥。 */
  keyHint?: string;
}

export const PROVIDER_PRESETS: readonly ProviderPreset[] = [
  {
    id: "deepseek",
    label: "DeepSeek 官方",
    kind: "openai-chat",
    baseUrl: "https://api.deepseek.com",
    models: ["deepseek-chat", "deepseek-reasoner"],
    keyHint: "在 platform.deepseek.com 申请",
  },
  {
    id: "openai",
    label: "OpenAI 官方",
    kind: "openai-responses",
    baseUrl: "https://api.openai.com/v1",
    models: ["gpt-4o-mini", "gpt-4o"],
    keyHint: "在 platform.openai.com 申请",
  },
  {
    id: "anthropic",
    label: "Anthropic（Claude）",
    kind: "anthropic-messages",
    baseUrl: "https://api.anthropic.com",
    models: ["claude-sonnet-4-5", "claude-haiku-4-5"],
    keyHint: "在 console.anthropic.com 申请",
  },
  {
    id: "ollama",
    label: "本地 Ollama",
    kind: "openai-chat",
    baseUrl: "http://127.0.0.1:11434/v1",
    models: ["qwen3:8b", "llama3.1:8b"],
    keyHint: "本地服务，无需密钥",
  },
  {
    id: "oneapi",
    label: "本地 OneAPI / NewAPI",
    kind: "openai-chat",
    baseUrl: "http://127.0.0.1:3000/v1",
    models: [],
    keyHint: "填网关里生成的令牌",
  },
  {
    id: "custom",
    label: "自定义（手动填写）",
    kind: DEFAULT_PROTOCOL,
    baseUrl: "",
    models: [],
  },
];

export function findPreset(id: string): ProviderPreset | undefined {
  return PROVIDER_PRESETS.find((preset) => preset.id === id);
}

/** 按协议猜预设，用于回显"这个连接像哪家"。 */
export function presetForKind(kind: string): ProviderPreset | undefined {
  return PROVIDER_PRESETS.find((preset) => preset.kind === kind);
}

/**
 * 套用预设：只覆盖协议 / 地址 / 模型名，不动用户已经填好的密钥。
 * 模型名只在当前为空或仍是上一个预设的建议值时才被覆盖，避免抹掉手填内容。
 */
export function applyPreset(draft: ModelInstance, preset: ProviderPreset, previous?: ProviderPreset): ModelInstance {
  const currentModel = (draft.model ?? "").trim();
  const keepModel = currentModel !== "" && !(previous?.models ?? []).includes(currentModel);
  return {
    ...draft,
    kind: preset.kind,
    baseUrl: preset.baseUrl,
    model: keepModel ? currentModel : (preset.models[0] ?? currentModel),
  };
}

/**
 * 新连接的默认名称建议。刻意不用模型名，避免再次出现"名称 == 模型名"
 * 那种同屏重复（旧界面显示成 deepseek-flash (deepseek-flash)）。
 * 重名时追加主机名消歧。
 */
export function suggestName(preset: ProviderPreset | undefined, baseUrl: string, existing: string[]): string {
  const base = preset && preset.id !== "custom" ? preset.label : "";
  const host = hostOf(baseUrl);
  const candidate = base || host || "新连接";
  if (!existing.includes(candidate)) return candidate;
  const withHost = host ? `${candidate} · ${host}` : candidate;
  if (!existing.includes(withHost)) return withHost;
  let index = 2;
  while (existing.includes(`${withHost} (${index})`)) index++;
  return `${withHost} (${index})`;
}

function hostOf(baseUrl: string): string {
  const raw = (baseUrl ?? "").trim();
  if (!raw) return "";
  try {
    return new URL(/^https?:\/\//i.test(raw) ? raw : `https://${raw}`).host;
  } catch {
    return "";
  }
}
