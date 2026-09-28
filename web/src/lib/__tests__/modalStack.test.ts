// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { closeTopLayer, isLayerOpen, layerCount, popLayer, pushLayer, type ModalLayer } from "../modalStack";

const pushed: number[] = [];
function push(layer: ModalLayer): number {
  const id = pushLayer(layer);
  pushed.push(id);
  return id;
}

function inertRecorder() {
  const states: boolean[] = [];
  return { states, setInert: (value: boolean) => states.push(value) };
}

describe("modalStack", () => {
  beforeEach(() => {
    document.body.innerHTML = '<div id="root"></div>';
    document.body.style.overflow = "";
  });

  afterEach(() => {
    pushed.splice(0).forEach(popLayer);
  });

  it("只让栈顶可交互，栈非空时应用根 inert", () => {
    const a = inertRecorder();
    const b = inertRecorder();
    const ida = push({ setInert: a.setInert });
    expect(document.getElementById("root")!.hasAttribute("inert")).toBe(true);
    expect(a.states.at(-1)).toBe(false); // 单个图层时它就是栈顶

    const idb = push({ setInert: b.setInert });
    expect(a.states.at(-1)).toBe(true); // 被压到下层
    expect(b.states.at(-1)).toBe(false); // 新栈顶

    popLayer(idb);
    expect(a.states.at(-1)).toBe(false); // 重新成为可交互顶层

    popLayer(ida);
    expect(document.getElementById("root")!.hasAttribute("inert")).toBe(false);
    expect(document.getElementById("root")!.hasAttribute("aria-hidden")).toBe(false);
  });

  it("叠层期间只锁一次滚动，全部关闭后恢复原值", () => {
    document.body.style.overflow = "auto";
    const ida = push({ setInert: () => {} });
    const idb = push({ setInert: () => {} });
    expect(document.body.style.overflow).toBe("hidden");

    popLayer(idb);
    expect(document.body.style.overflow).toBe("hidden"); // 还有一层

    popLayer(ida);
    expect(document.body.style.overflow).toBe("auto"); // 恢复入栈前的值
  });

  it("closeTopLayer 只关栈顶，且尊重阻塞式图层", () => {
    const closeA = vi.fn();
    const ida = push({ setInert: () => {}, onClose: closeA });
    const idb = push({ setInert: () => {} }); // 阻塞式：没有 onClose

    expect(closeTopLayer()).toBe(false); // 栈顶不可关
    expect(closeA).not.toHaveBeenCalled();

    popLayer(idb);
    expect(closeTopLayer()).toBe(true);
    expect(closeA).toHaveBeenCalledTimes(1);

    popLayer(ida);
    expect(closeTopLayer()).toBe(false);
    expect(isLayerOpen()).toBe(false);
    expect(layerCount()).toBe(0);
  });
});
