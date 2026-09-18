// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ThinkingEffortControl } from "../ThinkingEffortControl";
import { useSettings } from "../../stores/settingsStore";
import { useUi } from "../../stores/uiStore";
import type { ModelInstance, ProviderConfig } from "../../app/types";

const mocks = vi.hoisted(() => ({
  listModels: vi.fn(),
  saveModel: vi.fn(),
}));

vi.mock("../../app/api", () => ({ api: mocks }));

const deepseek: ModelInstance = {
  id: "m_deepseek", name: "DeepSeek 官方", kind: "openai-responses",
  baseUrl: "https://api.deepseek.com", model: "deepseek-flash", hasApiKey: true,
  contextWindow: 131072, maxTokens: 4096,
};

function slots(): ProviderConfig[] {
  return [
    { slot: "primary", enabled: true, modelId: "m_deepseek", resolvedModelId: "m_deepseek", model: "deepseek-flash" },
    { slot: "assist", enabled: true, modelId: "", resolvedModelId: "m_deepseek", model: "deepseek-flash" },
    { slot: "reflection", enabled: false, modelId: "", resolvedModelId: "m_deepseek", model: "deepseek-flash" },
  ];
}

function seed(models: ModelInstance[], providers = slots()) {
  useSettings.setState({ ...useSettings.getInitialState(), models, providers, loaded: true }, true);
}

beforeEach(() => {
  vi.resetAllMocks();
  mocks.listModels.mockResolvedValue({ models: [deepseek], slots: slots() });
  mocks.saveModel.mockImplementation((input: ModelInstance) => Promise.resolve({ ...deepseek, ...input }));
  seed([deepseek]);
  useUi.setState({ ...useUi.getInitialState(), notifyQuiet: vi.fn() });
});

afterEach(cleanup);

async function openMenu() {
  render(<ThinkingEffortControl />);
  fireEvent.click(await screen.findByLabelText(/调整思考强度/));
  return screen.findByRole("menu", { name: "选择思考强度" });
}

describe("ThinkingEffortControl · 输入台右下角", () => {
  it("starts on 默认 and offers the four levels", async () => {
    render(<ThinkingEffortControl />);
    expect(await screen.findByText("思考 默认")).toBeTruthy();
    fireEvent.click(screen.getByLabelText(/调整思考强度/));
    await screen.findByRole("menu", { name: "选择思考强度" });
    expect(screen.getAllByRole("menuitemradio")).toHaveLength(4);
    for (const label of ["默认", "低", "中", "高"]) {
      expect(screen.getByRole("menuitemradio", { name: new RegExp(`^${label}`) })).toBeTruthy();
    }
  });

  it("saves the level on the primary connection and reports it", async () => {
    await openMenu();
    fireEvent.click(screen.getByRole("menuitemradio", { name: /^高/ }));
    await waitFor(() => expect(mocks.saveModel).toHaveBeenCalled());
    // 空密钥 = 保留原密钥；改的是连接本身，所以实例 ID 必须带上。
    expect(mocks.saveModel.mock.calls[0]?.[0]).toMatchObject({
      id: "m_deepseek",
      apiKey: "",
      reasoningEffort: "high",
    });
    expect(useUi.getState().notifyQuiet).toHaveBeenCalledWith(expect.stringContaining("思考强度已设为「高」"));
  });

  it("marks the level already in effect and does nothing when re-selecting it", async () => {
    seed([{ ...deepseek, reasoningEffort: "medium" }]);
    await openMenu();
    const current = screen.getByRole("menuitemradio", { name: /^中/ });
    expect(current.getAttribute("aria-checked")).toBe("true");
    fireEvent.click(current);
    await waitFor(() => expect(screen.queryByRole("menu", { name: "选择思考强度" })).toBeNull());
    expect(mocks.saveModel).not.toHaveBeenCalled();
  });

  it("surfaces a failed save instead of pretending the level changed", async () => {
    mocks.saveModel.mockRejectedValue(new Error("模型实例不存在"));
    await openMenu();
    fireEvent.click(screen.getByRole("menuitemradio", { name: /^低/ }));
    await waitFor(() => expect(useUi.getState().notifyQuiet).toHaveBeenCalledWith(expect.stringContaining("设置失败")));
    expect(screen.getByText("思考 默认")).toBeTruthy();
  });

  it("is disabled until a primary connection exists", async () => {
    seed([], []);
    render(<ThinkingEffortControl />);
    const trigger = await screen.findByLabelText(/调整思考强度/);
    expect((trigger as HTMLButtonElement).disabled).toBe(true);
  });

  it("only shows thinking effort options and does not include model settings jump or footer", async () => {
    await openMenu();
    expect(screen.queryByText("打开模型设置…")).toBeNull();
    expect(screen.queryByText(/只影响新回合/)).toBeNull();
    expect(screen.getAllByRole("menuitemradio")).toHaveLength(4);
  });

  it("shows a checkmark for the currently selected effort", async () => {
    seed([{ ...deepseek, reasoningEffort: "medium" }]);
    await openMenu();
    const current = screen.getByRole("menuitemradio", { name: /^中/ });
    expect(current.textContent).toContain("✓");
  });
});
