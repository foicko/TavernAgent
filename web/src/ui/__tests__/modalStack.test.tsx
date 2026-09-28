// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import { render } from "@testing-library/react";
import { Modal } from "../Modal";

describe("Modal + 统一模态栈", () => {
  it("叠层时下层图层整体 inert（含遮罩），栈顶保持可交互", () => {
    render(
      <Modal label="下层弹窗" onClose={() => {}}>
        下层内容
      </Modal>,
    );
    const backdrops = () => Array.from(document.querySelectorAll<HTMLElement>(".ui-modal-backdrop"));
    expect(backdrops()).toHaveLength(1);
    expect(backdrops()[0].hasAttribute("inert")).toBe(false);

    render(
      <Modal label="上层弹窗" onClose={() => {}}>
        上层内容
      </Modal>,
    );
    expect(backdrops()).toHaveLength(2);
    // 下层整层（含遮罩）被 inert，避免点下层遮罩误关；上层可交互。
    expect(backdrops()[0].hasAttribute("inert")).toBe(true);
    expect(backdrops()[1].hasAttribute("inert")).toBe(false);
  });
});
