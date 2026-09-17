// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ModelQuickSwitch } from "../ModelQuickSwitch";
import { useSettings } from "../../stores/settingsStore";
import { useUi } from "../../stores/uiStore";
import type { ModelInstance, ProviderConfig } from "../../app/types";

const mocks = vi.hoisted(() => ({
  listModels: vi.fn(),
  assignSlot: vi.fn(),
}));

vi.mock("../../app/api", () => ({ api: mocks }));

const deepseek: ModelInstance = { id: "m_deepseek", name: "DeepSeek 官方", kind: "openai-chat", baseUrl: "https://api.deepseek.com/v1", model: "deepseek-chat", hasApiKey: true, contextWindow: 131072 };
const local: ModelInstance = { id: "m_local", name: "本地 Ollama", kind: "openai-chat", baseUrl: "http://127.0.0.1:11434/v1", model: "qwen2.5:14b", hasApiKey: false, contextWindow: 32768 };

function slots(primaryModelId = "m_deepseek"): ProviderConfig[] {
  return [
    { slot: "primary", enabled: true, modelId: primaryModelId, resolvedModelId: primaryModelId, model: "deepseek-chat" },
    { slot: "assist", enabled: true, modelId: "", resolvedModelId: primaryModelId, model: "deepseek-chat" },
    { slot: "reflection", enabled: true, modelId: "", resolvedModelId: primaryModelId, model: "deepseek-chat" },
  ];
}

beforeEach(() => {
  vi.resetAllMocks();
  mocks.listModels.mockResolvedValue({ models: [deepseek, local], slots: slots() });
  mocks.assignSlot.mockImplementation((_slot: string, _enabled: boolean, modelId: string) =>
    Promise.resolve({ ok: true, providers: slots(modelId) }));
  // 应用里目录由顶栏/设置页加载；这里直接预置成"已加载"，避免测试依赖加载时序。
  useSettings.setState({ ...useSettings.getInitialState(), models: [deepseek, local], providers: slots(), loaded: true }, true);
  useUi.setState({ ...useUi.getInitialState(), notifyQuiet: vi.fn() });
});

afterEach(cleanup);

describe("ModelQuickSwitch · 目录自加载", () => {
  it("loads the catalog itself when nothing has loaded it yet", async () => {
    useSettings.setState({ ...useSettings.getInitialState(), models: [], providers: [], loaded: false }, true);
    render(<ModelQuickSwitch />);
    await waitFor(() => expect(mocks.listModels).toHaveBeenCalled());
    expect(await screen.findByText("DeepSeek 官方")).toBeTruthy();
  });
});

async function openMenu() {
  render(<ModelQuickSwitch />);
  fireEvent.click(await screen.findByLabelText("切换主线模型"));
  return screen.findByRole("menu", { name: "选择主线模型" });
}

describe("ModelQuickSwitch · 输入台快捷切换", () => {
  it("shows the current primary instance on the button", async () => {
    render(<ModelQuickSwitch />);
    expect(await screen.findByText("DeepSeek 官方")).toBeTruthy();
    expect(screen.getByLabelText("切换主线模型").getAttribute("aria-expanded")).toBe("false");
  });

  it("switches the primary slot and only reports the new instance", async () => {
    const menu = await openMenu();
    expect(menu.textContent).toContain("切换对新回合生效");
    fireEvent.click(screen.getByRole("menuitemradio", { name: /本地 Ollama/ }));
    await waitFor(() => expect(mocks.assignSlot).toHaveBeenCalledWith("primary", true, "m_local"));
    expect(useUi.getState().notifyQuiet).toHaveBeenCalledWith(expect.stringContaining("本地 Ollama"));
  });

  it("marks the current instance and does nothing when re-selecting it", async () => {
    await openMenu();
    const current = screen.getByRole("menuitemradio", { name: /DeepSeek 官方/ });
    expect(current.getAttribute("aria-checked")).toBe("true");
    fireEvent.click(current);
    await waitFor(() => expect(screen.queryByRole("menu", { name: "选择主线模型" })).toBeNull());
    expect(mocks.assignSlot).not.toHaveBeenCalled();
  });

  it("surfaces a failed switch instead of silently showing the new name", async () => {
    mocks.assignSlot.mockRejectedValue(new Error("模型实例不存在"));
    await openMenu();
    fireEvent.click(screen.getByRole("menuitemradio", { name: /本地 Ollama/ }));
    await waitFor(() => expect(useUi.getState().notifyQuiet).toHaveBeenCalledWith(expect.stringContaining("切换失败")));
    expect(useUi.getState().notifyQuiet).toHaveBeenCalledWith(expect.stringContaining("模型实例不存在"));
  });

  it("points to the settings page when no instance exists yet", async () => {
    mocks.listModels.mockResolvedValue({ models: [], slots: [] });
    useSettings.setState({ models: [], providers: [], loaded: true });
    await openMenu();
    expect(screen.getByText("还没有连接")).toBeTruthy();
    fireEvent.click(screen.getByText("打开模型设置…"));
    expect(useSettings.getState().settingsOpen).toBe(true);
  });

  it("shows a migrated instance name only once", async () => {
    // 迁移会把实例名默认写成模型名，旧界面因此显示 "deepseek-flash (deepseek-flash)"。
    const migrated: ModelInstance = {
      id: "m_migrated",
      name: "deepseek-flash",
      kind: "openai-chat",
      baseUrl: "https://api.deepseek.com",
      model: "deepseek-flash",
      hasApiKey: true,
      contextWindow: 65536,
    };
    useSettings.setState({ models: [migrated], providers: slots("m_migrated"), loaded: true });
    await openMenu();
    const item = screen.getByRole("menuitemradio", { name: /deepseek-flash/ });
    expect(item.textContent).toBe("deepseek-flashapi.deepseek.com✓主线");
  });
});
