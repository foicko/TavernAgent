// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SettingsModal } from "../SettingsModal";
import { useSettings } from "../../stores/settingsStore";
import type { ModelInstance, ProviderConfig } from "../../app/types";

const mocks = vi.hoisted(() => ({
  listModels: vi.fn(),
  saveModel: vi.fn(),
  deleteModel: vi.fn(),
  assignSlot: vi.fn(),
  probeModel: vi.fn(),
}));

vi.mock("../../app/api", () => ({ api: mocks }));

function instance(over: Partial<ModelInstance> = {}): ModelInstance {
  return {
    id: "m_deepseek", name: "DeepSeek 官方", kind: "openai-chat",
    baseUrl: "https://api.deepseek.com/v1", model: "deepseek-chat",
    hasApiKey: true, contextWindow: 131072, maxTokens: 4096, temperature: 0.8, ...over,
  };
}

function slots(over: Partial<Record<string, Partial<ProviderConfig>>> = {}): ProviderConfig[] {
  const base: Record<string, ProviderConfig> = {
    primary: { slot: "primary", enabled: true, modelId: "m_deepseek", resolvedModelId: "m_deepseek", model: "deepseek-chat", kind: "openai-chat", baseUrl: "https://api.deepseek.com/v1", hasApiKey: true, contextWindow: 131072, maxTokens: 4096 },
    assist: { slot: "assist", enabled: true, modelId: "", resolvedModelId: "m_deepseek", model: "deepseek-chat", kind: "openai-chat", contextWindow: 131072 },
    reflection: { slot: "reflection", enabled: false, modelId: "", resolvedModelId: "m_deepseek", model: "deepseek-chat", kind: "openai-chat", contextWindow: 131072 },
  };
  return Object.entries({ ...base, ...over }).map(([slot, cfg]) => ({ ...(base[slot] ?? { slot }), ...cfg, slot } as ProviderConfig));
}

const both = () => [
  instance(),
  instance({ id: "m_local", name: "本地 Ollama", model: "qwen2.5:14b", baseUrl: "http://127.0.0.1:11434/v1", contextWindow: 32768 }),
];

beforeEach(() => {
  localStorage.clear();
  vi.resetAllMocks();
  mocks.listModels.mockResolvedValue({ models: [instance()], slots: slots() });
  mocks.saveModel.mockImplementation((input: ModelInstance) => Promise.resolve(instance({ ...input, id: input.id || "m_saved" })));
  mocks.deleteModel.mockResolvedValue({ ok: true });
  mocks.assignSlot.mockImplementation((slot: string, enabled: boolean, modelId: string) =>
    Promise.resolve({ ok: true, providers: slots({ [slot]: { slot, enabled, modelId } }) }));
  mocks.probeModel.mockResolvedValue({ slot: "probe", ok: true, model: "deepseek-chat", latencyMs: 42 });
  useSettings.setState(useSettings.getInitialState(), true);
});

afterEach(cleanup);

describe("SettingsModal · 角色与连接", () => {
  it("shows the three roles with their resolved connection, and the connection list", async () => {
    render(<SettingsModal onClose={() => {}} />);
    await waitFor(() => expect(screen.getByRole("heading", { name: "角色" })).toBeTruthy());

    expect((screen.getByLabelText("对话生成") as HTMLSelectElement).value).toBe("m_deepseek");
    // 非主线角色未显式指派时跟随主线。
    expect((screen.getByLabelText("导演协商") as HTMLSelectElement).value).toBe("");
    expect(screen.getAllByText(/跟随对话生成/).length).toBeGreaterThan(0);
    expect(screen.getByText("已关闭")).toBeTruthy();

    expect(screen.getByRole("heading", { name: "连接" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "编辑连接 DeepSeek 官方" })).toBeTruthy();
  });

  it("never repeats the model name for a migrated connection", async () => {
    // 名称 == 模型名时，副标题只补主机名，不再出现 "deepseek-flash · deepseek-flash"。
    const migrated = instance({ id: "m_mig", name: "deepseek-flash", model: "deepseek-flash" });
    mocks.listModels.mockResolvedValue({ models: [migrated], slots: slots({ primary: { modelId: "m_mig", resolvedModelId: "m_mig" } }) });
    render(<SettingsModal onClose={() => {}} />);
    const row = await screen.findByRole("button", { name: "编辑连接 deepseek-flash" });
    expect(row.textContent).toBe("deepseek-flashapi.deepseek.com对话生成密钥已存");
  });

  it("creates a connection from a provider preset and saves the key", async () => {
    render(<SettingsModal onClose={() => {}} />);
    await waitFor(() => expect(screen.getByText("＋ 新建连接")).toBeTruthy());
    fireEvent.click(screen.getByText("＋ 新建连接"));

    // 预设一键填好协议与地址，名称自动生成（且不等于模型名）。
    fireEvent.click(screen.getByRole("button", { name: "本地 Ollama" }));
    fireEvent.change(screen.getByLabelText("模型名称"), { target: { value: "qwen2.5:14b" } });
    fireEvent.change(screen.getByLabelText("API Key"), { target: { value: "sk-new-secret" } });
    fireEvent.click(screen.getByText("保存连接"));

    await waitFor(() => expect(mocks.saveModel).toHaveBeenCalledTimes(1));
    expect(mocks.saveModel.mock.calls[0][0]).toMatchObject({
      name: "本地 Ollama",
      model: "qwen2.5:14b",
      apiKey: "sk-new-secret",
      baseUrl: "http://127.0.0.1:11434/v1",
      kind: "openai-chat",
    });
    expect(await screen.findByText(/已保存/)).toBeTruthy();
  });

  it("blocks deletion of a connection that a role is using, and explains why", async () => {
    render(<SettingsModal onClose={() => {}} />);
    fireEvent.click(await screen.findByRole("button", { name: "编辑连接 DeepSeek 官方" }));

    const remove = screen.getByRole("button", { name: "删除连接" }) as HTMLButtonElement;
    expect(remove.disabled).toBe(true);
    expect(screen.getByText(/正在被 对话生成 使用/)).toBeTruthy();
  });

  it("surfaces the server refusal when deletion still reaches the backend", async () => {
    mocks.deleteModel.mockRejectedValue(new Error("该模型实例正在被 primary 使用，请先在这些槽位里换一个模型"));
    mocks.listModels.mockResolvedValue({ models: [instance()], slots: slots({ primary: { modelId: "", resolvedModelId: "" } }) });
    render(<SettingsModal onClose={() => {}} />);
    fireEvent.click(await screen.findByRole("button", { name: "编辑连接 DeepSeek 官方" }));
    fireEvent.click(screen.getByRole("button", { name: "删除连接" }));
    fireEvent.click(screen.getByRole("button", { name: "确认删除" }));
    await waitFor(() => expect(screen.getByRole("alert").textContent).toContain("primary"));
  });

  it("assigns another connection to a role and shows the resolved window", async () => {
    mocks.listModels
      .mockResolvedValueOnce({ models: both(), slots: slots() })
      .mockResolvedValue({ models: both(), slots: slots({ primary: { modelId: "m_local", resolvedModelId: "m_local", model: "qwen2.5:14b" } }) });
    render(<SettingsModal onClose={() => {}} />);
    await waitFor(() => expect(screen.getByLabelText("对话生成")).toBeTruthy());

    fireEvent.change(screen.getByLabelText("对话生成"), { target: { value: "m_local" } });
    await waitFor(() => expect(mocks.assignSlot).toHaveBeenCalledWith("primary", true, "m_local"));
    await waitFor(() => expect(screen.getByText(/本地 Ollama · 32k 窗口/)).toBeTruthy());
  });

  it("enables background memory keeping the follow-primary reference", async () => {
    render(<SettingsModal onClose={() => {}} />);
    const toggle = await screen.findByLabelText("启用后台记忆");
    fireEvent.click(toggle);
    await waitFor(() => expect(mocks.assignSlot).toHaveBeenCalledWith("reflection", true, ""));
  });

  it("refuses to save a connection with an emptied number field", async () => {
    render(<SettingsModal onClose={() => {}} />);
    fireEvent.click(await screen.findByRole("button", { name: "编辑连接 DeepSeek 官方" }));
    // 生成参数（窗口/输出/温度/思考强度）现在常显，不需要先展开折叠区。
    fireEvent.change(screen.getByLabelText("上下文窗口"), { target: { value: "" } });
    fireEvent.click(screen.getByText("保存修改"));

    expect(mocks.saveModel).not.toHaveBeenCalled();
    expect(await screen.findByText("请填写")).toBeTruthy();
  });

  it("shows the derived input budget and saves the thinking effort with the connection", async () => {
    render(<SettingsModal onClose={() => {}} />);
    fireEvent.click(await screen.findByRole("button", { name: "编辑连接 DeepSeek 官方" }));

    // 131072 窗口 − 4096 输出 − max(512, 131072/20)=6553 → 约 118k。
    expect(screen.getByText(/输入预算约 118k tokens/)).toBeTruthy();

    fireEvent.change(screen.getByLabelText("思考强度"), { target: { value: "high" } });
    fireEvent.click(screen.getByText("保存修改"));
    await waitFor(() => expect(mocks.saveModel).toHaveBeenCalled());
    expect(mocks.saveModel.mock.calls.at(-1)?.[0]).toMatchObject({ reasoningEffort: "high" });
  });

  it("keeps the writing preferences collapsed with a value summary, and closes from the close button", async () => {
    const onClose = vi.fn();
    render(<SettingsModal onClose={onClose} />);
    expect(await screen.findByRole("heading", { name: "写作偏好" })).toBeTruthy();

    // 默认收起：收起行必须把当前值讲清楚，否则"藏起来"只是让人多点一次。
    const trigger = screen.getByRole("button", { name: /选项与续写/ });
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(trigger.textContent).toContain("选项：关键节点");
    expect(trigger.textContent).toContain("自动续写：关");
    expect(screen.queryByLabelText("截断自动续写")).toBeNull();

    fireEvent.click(trigger);
    fireEvent.click(screen.getByLabelText("选项“填入编辑”模式"));
    expect(useSettings.getState().optionMode).toBe("fill-edit");
    fireEvent.click(screen.getByLabelText("截断自动续写"));
    expect(useSettings.getState().autoContinue).toBe(true);
    // 改动后收起行的摘要要跟着变。
    expect(trigger.textContent).toContain("自动续写：开");

    fireEvent.click(screen.getByLabelText("关闭"));
    expect(onClose).toHaveBeenCalled();
  });
});
