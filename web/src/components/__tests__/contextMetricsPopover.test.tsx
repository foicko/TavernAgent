// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { ContextMetricsPopover } from "../ContextMetricsPopover";
import { useSettings } from "../../stores/settingsStore";
import { useStory } from "../../stores/storyStore";
import { estimateTokens, formatTokenCount, calculateContextMetrics } from "../../lib/tokenEstimator";
import { CHARACTER_PRESETS } from "../../lib/characterPresets";

describe("tokenEstimator unit tests", () => {
  it("estimates CJK and Latin tokens accurately", () => {
    expect(estimateTokens("")).toBe(0);
    expect(estimateTokens("你好世界")).toBe(4);
    expect(estimateTokens("Hello World")).toBeGreaterThan(1);
    expect(estimateTokens("你好 World")).toBeGreaterThanOrEqual(3);
  });

  it("formats token counts properly (123.4K, 1M, 64K, 850)", () => {
    expect(formatTokenCount(850)).toBe("850");
    expect(formatTokenCount(65536)).toBe("64K");
    expect(formatTokenCount(123400)).toBe("123.4K");
    expect(formatTokenCount(1048576)).toBe("1M");
  });

  it("calculates context metrics matching the design specifications", () => {
    const metrics = calculateContextMetrics({
      char: {
        ...CHARACTER_PRESETS.custom,
        name: "测试角色",
        modalDesc: "一个测试角色描述",
      },
      messages: [
        {
          id: "m1",
          role: "user",
          inputText: "你好，请问你是谁？",
          blocks: [],
          options: [],
        },
        {
          id: "m2",
          role: "assistant",
          blocks: [{ kind: "narration", text: "我是你的虚拟冒险伙伴。" }],
          options: [],
        },
      ],
      inputText: "我们在哪里？",
      primaryConfig: {
        slot: "primary",
        enabled: true,
        kind: "openai-chat",
        baseUrl: "https://api.deepseek.com",
        model: "deepseek-chat",
        contextWindow: 65536,
      },
    });

    expect(metrics.maxContextWindow).toBe(65536);
    expect(metrics.totalUsedTokens).toBeGreaterThan(200);
    expect(metrics.percentage).toBeGreaterThan(0);
    expect(metrics.cacheHitRate).toBeGreaterThan(50);
    // 模型名只如实来自配置，不再按 baseUrl 猜供应商（旧实现会拼出 "DeepSeek/deepseek-chat"）。
    expect(metrics.modelName).toBe("deepseek-chat");
    expect(metrics.turnCount).toBe(2);
  });

  it("does not invent a provider name when nothing is configured", () => {
    const metrics = calculateContextMetrics({ char: CHARACTER_PRESETS.custom, messages: [], inputText: "" });
    expect(metrics.modelName).toBe("");
  });
});

describe("ContextMetricsPopover UI", () => {
  beforeEach(() => {
    useSettings.setState({
      providers: [
        {
          slot: "primary",
          enabled: true,
          kind: "openai-chat",
          baseUrl: "https://api.deepseek.com",
          model: "deepseek-chat",
          contextWindow: 65536,
        },
      ],
      settingsOpen: false,
    });
    useStory.setState({
      messages: [],
    });
  });

  afterEach(cleanup);

  it("renders the trigger as a meter (percentage + donut), not as a second model selector", () => {
    render(<ContextMetricsPopover inputText="输入草稿测试" />);

    const triggerBtn = screen.getByRole("button", { name: "实时上下文统计" });
    expect(triggerBtn.textContent).toMatch(/%/);
    expect(triggerBtn.textContent).not.toContain("deepseek-chat");
  });

  it("opens popover on click and shows the window breakdown and cache rate", () => {
    render(<ContextMetricsPopover inputText="输入草稿测试" />);

    const triggerBtn = screen.getByRole("button", { name: "实时上下文统计" });
    fireEvent.click(triggerBtn);

    // Popover content（界面文案统一为中文）
    expect(screen.getByText("上下文窗口")).toBeTruthy();
    expect(screen.getByText("平均前缀缓存命中")).toBeTruthy();
    expect(screen.getByText(/主线模型：deepseek-chat/)).toBeTruthy();
    expect(screen.getByText("人设基底与规则")).toBeTruthy();
    expect(screen.getByText("当前输入框草稿")).toBeTruthy();

    // Settings entry button
    const settingsBtn = screen.getByRole("button", { name: /切换模型与上下文配额/ });
    expect(settingsBtn).toBeTruthy();
    fireEvent.click(settingsBtn);

    expect(useSettings.getState().settingsOpen).toBe(true);
  });

  it("closes popover on Escape key", () => {
    render(<ContextMetricsPopover inputText="" />);

    const triggerBtn = screen.getByRole("button", { name: "实时上下文统计" });
    fireEvent.click(triggerBtn);
    expect(screen.getByText("上下文窗口")).toBeTruthy();

    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByText("上下文窗口")).toBeNull();
  });
});
