import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useUi } from "../uiStore";

// M69：轻提示改走 ToastStack，需保证「堆叠而非替换」「色调传递」「可关闭」。
describe("uiStore 轻提示堆叠", () => {
  beforeEach(() => {
    useUi.setState({ toasts: [] });
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("多条提示并存而不是相互替换，并保留色调", () => {
    useUi.getState().notifyQuiet("左栏已收起");
    useUi.getState().notifyQuiet("导入失败：磁盘已满", "danger");
    const toasts = useUi.getState().toasts;
    expect(toasts).toHaveLength(2);
    expect(toasts[0]).toMatchObject({ text: "左栏已收起", tone: "neutral" });
    expect(toasts[1]).toMatchObject({ text: "导入失败：磁盘已满", tone: "danger" });
  });

  it("到期自动移除", () => {
    useUi.getState().notifyQuiet("提示");
    expect(useUi.getState().toasts).toHaveLength(1);
    vi.advanceTimersByTime(2400);
    expect(useUi.getState().toasts).toHaveLength(0);
  });

  it("手动关闭后不再被残留定时器影响", () => {
    useUi.getState().notifyQuiet("提示");
    const id = useUi.getState().toasts[0].id;
    useUi.getState().dismissToast(id);
    expect(useUi.getState().toasts).toHaveLength(0);
    vi.advanceTimersByTime(5000);
    expect(useUi.getState().toasts).toHaveLength(0);
  });
});
