// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// 模拟隐私模式 / 被禁用的存储：连访问 window.localStorage 都抛 SecurityError
describe("uiStore 主题在存储不可用时", () => {
  const ownDescriptor = Object.getOwnPropertyDescriptor(window, "localStorage");

  beforeEach(() => {
    vi.resetModules();
    Object.defineProperty(window, "localStorage", {
      configurable: true,
      get() {
        throw new DOMException("storage disabled", "SecurityError");
      },
    });
  });

  afterEach(() => {
    if (ownDescriptor) Object.defineProperty(window, "localStorage", ownDescriptor);
    else Reflect.deleteProperty(window, "localStorage");
  });

  it("模块加载与首帧主题解析不抛错", async () => {
    const mod = await import("../uiStore");
    expect(mod.resolveInitialTheme()).toMatch(/^(light|dark)$/);
    expect(mod.useUi.getState().theme).toMatch(/^(light|dark)$/);
  });

  it("切换主题照常生效，只是不记忆", async () => {
    const { useUi } = await import("../uiStore");
    expect(() => useUi.getState().setTheme("dark")).not.toThrow();
    expect(useUi.getState().theme).toBe("dark");
  });
});
