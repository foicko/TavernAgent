// @vitest-environment jsdom
// 桌面壳桥：窗口跑在 loopback 源上，没有 window.runtime，主题切换必须走同源接口；
// 浏览器部署下这些调用要安静降级，不能抛错打扰读者。
import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ desktopInfo: vi.fn(), setDesktopTheme: vi.fn() }));
vi.mock("../../app/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../../app/api")>();
  return {
    ...actual,
    api: { ...actual.api, desktopInfo: mocks.desktopInfo, setDesktopTheme: mocks.setDesktopTheme },
  };
});

async function freshModule() {
  vi.resetModules();
  return await import("../desktopShell");
}

beforeEach(() => {
  vi.resetAllMocks();
});

describe("applyNativeTheme", () => {
  it("把主题透传给原生窗口", async () => {
    mocks.setDesktopTheme.mockResolvedValue(undefined);
    const { applyNativeTheme } = await freshModule();

    applyNativeTheme("dark");
    applyNativeTheme("light");

    expect(mocks.setDesktopTheme.mock.calls).toEqual([["dark"], ["light"]]);
  });

  it("浏览器部署（接口 404）不抛错", async () => {
    mocks.setDesktopTheme.mockRejectedValue(new Error("404"));
    const { applyNativeTheme } = await freshModule();

    expect(() => applyNativeTheme("dark")).not.toThrow();
    await Promise.resolve();
    expect(mocks.setDesktopTheme).toHaveBeenCalledOnce();
  });
});

describe("detectDesktopShell", () => {
  it("只探测一次并缓存结果", async () => {
    mocks.desktopInfo.mockResolvedValue({ desktop: true });
    const { detectDesktopShell, isDesktopShell } = await freshModule();

    expect(isDesktopShell()).toBe(false); // 尚未探测完成时是安全默认值
    expect(await detectDesktopShell()).toBe(true);
    expect(await detectDesktopShell()).toBe(true);
    expect(mocks.desktopInfo).toHaveBeenCalledOnce();
    expect(isDesktopShell()).toBe(true);
  });

  it("探测失败按浏览器处理，绝不误判成桌面壳", async () => {
    mocks.desktopInfo.mockRejectedValue(new Error("offline"));
    const { detectDesktopShell, isDesktopShell } = await freshModule();

    expect(await detectDesktopShell()).toBe(false);
    expect(isDesktopShell()).toBe(false);
  });
});
