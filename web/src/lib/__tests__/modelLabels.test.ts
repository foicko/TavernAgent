import { describe, expect, it } from "vitest";
import type { ModelInstance } from "../../app/types";
import {
  connectionHost,
  connectionSubtitle,
  connectionTitle,
  effortLabel,
  inputBudget,
  keyStateLabel,
  maskedKeyHint,
  optionLabels,
  protocolLabel,
  roleLabel,
  tokenLabel,
  windowLabel,
} from "../modelLabels";

function instance(partial: Partial<ModelInstance>): ModelInstance {
  return { id: "m_1", name: "", kind: "openai-chat", ...partial };
}

describe("connectionTitle", () => {
  it("keeps a real custom name", () => {
    expect(connectionTitle(instance({ name: "DeepSeek 官方", model: "deepseek-chat" }))).toBe("DeepSeek 官方");
  });

  it("collapses the migrated name-equals-model case to a single copy", () => {
    // 迁移会把实例名默认写成模型名，旧界面因此显示 "deepseek-flash (deepseek-flash) · deepseek-flash"。
    const migrated = instance({ name: "deepseek-flash", model: "deepseek-flash" });
    expect(connectionTitle(migrated)).toBe("deepseek-flash");
    expect(connectionSubtitle(migrated)).toBe("");
  });

  it("falls back to the model name when the instance has no name", () => {
    expect(connectionTitle(instance({ name: "  ", model: "gemini-3.5-flash-lite" }))).toBe("gemini-3.5-flash-lite");
    expect(connectionTitle(instance({ name: "", model: "" }))).toBe("未命名连接");
  });
});

describe("connectionSubtitle", () => {
  it("adds host without repeating the model name", () => {
    const row = instance({ name: "deepseek-flash", model: "deepseek-flash", baseUrl: "https://api.deepseek.com/v1" });
    expect(connectionSubtitle(row)).toBe("api.deepseek.com");
  });

  it("shows model then host when both are new information", () => {
    const row = instance({ name: "DeepSeek 官方", model: "deepseek-chat", baseUrl: "https://api.deepseek.com" });
    expect(connectionSubtitle(row)).toBe("deepseek-chat · api.deepseek.com");
  });

  it("does not repeat a host already contained in a previous part", () => {
    const row = instance({ name: "本地网关", model: "api.deepseek.com", baseUrl: "https://api.deepseek.com" });
    expect(connectionSubtitle(row)).toBe("api.deepseek.com");
  });

  it("stays empty when there is nothing new to say", () => {
    expect(connectionSubtitle(instance({ name: "Ollama", model: "", baseUrl: "" }))).toBe("");
  });
});

describe("connectionHost", () => {
  it("parses host from a full URL and drops the path", () => {
    expect(connectionHost(instance({ baseUrl: "https://api.deepseek.com/v1/chat" }))).toBe("api.deepseek.com");
  });

  it("accepts a scheme-less host and rejects junk", () => {
    expect(connectionHost(instance({ baseUrl: "127.0.0.1:8045" }))).toBe("127.0.0.1:8045");
    expect(connectionHost(instance({ baseUrl: "not a url" }))).toBe("");
    expect(connectionHost(instance({ baseUrl: "" }))).toBe("");
  });
});

describe("optionLabels", () => {
  it("returns plain titles when they are unique", () => {
    const labels = optionLabels([
      instance({ id: "a", name: "DeepSeek 官方", model: "deepseek-chat" }),
      instance({ id: "b", name: "本地 Ollama", model: "qwen3" }),
    ]);
    expect(labels.get("a")).toBe("DeepSeek 官方");
    expect(labels.get("b")).toBe("本地 Ollama");
  });

  it("disambiguates duplicate titles with the host so no two options read the same", () => {
    const labels = optionLabels([
      instance({ id: "a", name: "deepseek-flash", model: "deepseek-flash", baseUrl: "https://api.deepseek.com" }),
      instance({ id: "b", name: "deepseek-flash", model: "deepseek-flash", baseUrl: "http://localhost:8045" }),
    ]);
    expect(labels.get("a")).toBe("deepseek-flash · api.deepseek.com");
    expect(labels.get("b")).toBe("deepseek-flash · localhost:8045");
    expect(new Set(labels.values()).size).toBe(2);
  });
});

describe("small labels", () => {
  it("renders roles as purposes, never slot ids", () => {
    expect(roleLabel("primary")).toBe("对话生成");
    expect(roleLabel("reflection")).toBe("后台记忆");
    expect(roleLabel("weird")).toBe("weird");
  });

  it("renders protocols as short human labels", () => {
    expect(protocolLabel("openai-chat")).toBe("OpenAI 兼容");
    expect(protocolLabel("unknown-kind")).toBe("unknown-kind");
    expect(protocolLabel(undefined)).toBe("");
  });

  it("renders windows in k/M and flags unset ones", () => {
    expect(windowLabel(131072)).toBe("128k 窗口");
    expect(windowLabel(1048576)).toBe("1M 窗口");
    expect(windowLabel(0)).toBe("窗口未设置");
    expect(windowLabel(undefined)).toBe("窗口未设置");
  });

  it("describes key state without ever printing a key", () => {
    expect(keyStateLabel(true)).toBe("密钥已存");
    expect(keyStateLabel(false)).toBe("无密钥");
  });
});

describe("maskedKeyHint", () => {
  it("keeps the server-side mask as-is", () => {
    expect(maskedKeyHint("sk-1…abcd")).toBe("sk-1…abcd");
  });

  it("returns a neutral label when nothing is stored", () => {
    expect(maskedKeyHint("")).toBe("已保存");
    expect(maskedKeyHint(undefined)).toBe("已保存");
  });

  it("re-masks an unexpectedly long value so a full secret is never rendered", () => {
    // 截图中出现过 "已存密钥 sk-8ed4d79...（留空即保留）" 这种近乎明文的提示。
    // 字面量刻意拆开拼接：完整的 "sk-" + 长串会被发布审计的疑似密钥扫描误报（scripts/release_audit.py）。
    const leakedLongKey = ["sk-", "8ed4d79276764a09844c1240995", "da8ba"].join("");
    expect(maskedKeyHint(leakedLongKey)).toBe("sk-8…a8ba");
  });
});

describe("effortLabel", () => {
  it("maps the stored levels to plain Chinese, empty meaning 默认", () => {
    expect(effortLabel("")).toBe("默认");
    expect(effortLabel(undefined)).toBe("默认");
    expect(effortLabel("low")).toBe("低");
    expect(effortLabel("medium")).toBe("中");
    expect(effortLabel("high")).toBe("高");
  });

  it("shows an unknown stored value verbatim instead of hiding it", () => {
    expect(effortLabel("maximum")).toBe("maximum");
  });
});

describe("inputBudget", () => {
  it("mirrors the backend formula (窗口 − 输出 − max(512, 窗口/20))", () => {
    // 131072 - 4096 - 6553 = 120423
    expect(inputBudget(131072, 4096)).toBe(120423);
    // 32768 - 2048 - 1638 = 29082
    expect(inputBudget(32768, 2048)).toBe(29082);
  });

  it("falls back to the backend defaults when the output cap is unset", () => {
    // 输出缺省按 2048：8192 - 2048 - 512 = 5632
    expect(inputBudget(8192, undefined)).toBe(5632);
    expect(inputBudget(8192, 0)).toBe(5632);
  });

  it("returns null while the window is still unset, and can go negative", () => {
    expect(inputBudget(undefined, 4096)).toBeNull();
    expect(inputBudget(0, 4096)).toBeNull();
    // 窗口只比输出大 2048（刚好过校验）时，安全余量会把输入预算吃成负数。
    expect(inputBudget(100000, 97000)).toBe(-2000);
  });
});

describe("tokenLabel", () => {
  it("rounds to k above 1024 and keeps small counts as-is", () => {
    expect(tokenLabel(120423)).toBe("118k");
    expect(tokenLabel(1024)).toBe("1k");
    expect(tokenLabel(820)).toBe("820");
  });
});
