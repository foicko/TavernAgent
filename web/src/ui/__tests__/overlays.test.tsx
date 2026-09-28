// @vitest-environment jsdom
// 新原语的契约测试：页签键盘导航、提示可访问关联、锚定浮层开关、吐司堆叠与离场、头像兜底、卡片语义。
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useRef, useState } from "react";
import { Avatar } from "../Avatar";
import { Card, CardTitle } from "../Card";
import { Kbd } from "../Kbd";
import { Popover, placePopover } from "../Popover";
import { Skeleton, SkeletonText } from "../Skeleton";
import { Tabs } from "../Tabs";
import { ToastStack } from "../Toast";
import { Tooltip } from "../Tooltip";

afterEach(cleanup);

describe("Tabs", () => {
  function Harness({ onChange }: { onChange?: (v: string) => void }) {
    const [value, setValue] = useState("tree");
    return (
      <Tabs
        label="左栏页签"
        value={value}
        onChange={(next) => {
          setValue(next);
          onChange?.(next);
        }}
        items={[
          { value: "tree", label: "因果", id: "subtab-btn-tree" },
          { value: "characters", label: "角色", id: "subtab-btn-characters" },
          { value: "lore", label: "世界书", disabled: true },
          { value: "sessions", label: "会话" },
        ]}
      />
    );
  }

  it("exposes a radiogroup with roving tabindex and passes ids through", () => {
    render(<Harness />);
    expect(screen.getByRole("radiogroup", { name: "左栏页签" })).toBeTruthy();
    const tree = screen.getByRole("radio", { name: "因果" });
    expect(tree.getAttribute("aria-checked")).toBe("true");
    expect(tree.tabIndex).toBe(0);
    expect(screen.getByRole("radio", { name: "角色" }).tabIndex).toBe(-1);
    expect(document.getElementById("subtab-btn-characters")).toBeTruthy();
  });

  it("moves with arrow keys, skips disabled tabs and wraps around", () => {
    const onChange = vi.fn();
    render(<Harness onChange={onChange} />);
    const tree = screen.getByRole("radio", { name: "因果" });
    fireEvent.keyDown(tree, { key: "ArrowRight" });
    expect(onChange).toHaveBeenLastCalledWith("characters");
    fireEvent.keyDown(screen.getByRole("radio", { name: "角色" }), { key: "ArrowRight" });
    // 世界书被禁用，跳到会话
    expect(onChange).toHaveBeenLastCalledWith("sessions");
    fireEvent.keyDown(screen.getByRole("radio", { name: "会话" }), { key: "ArrowRight" });
    expect(onChange).toHaveBeenLastCalledWith("tree");
    fireEvent.keyDown(screen.getByRole("radio", { name: "因果" }), { key: "End" });
    expect(onChange).toHaveBeenLastCalledWith("sessions");
  });

  it("selects on click", () => {
    const onChange = vi.fn();
    render(<Harness onChange={onChange} />);
    fireEvent.click(screen.getByRole("radio", { name: "会话" }));
    expect(onChange).toHaveBeenCalledWith("sessions");
    expect(screen.getByRole("radio", { name: "会话" }).getAttribute("aria-checked")).toBe("true");
  });
});

describe("Tooltip", () => {
  it("links the tooltip text to the trigger and clears the native title", () => {
    render(
      <Tooltip content="切换主题">
        <button type="button" title="原生提示">主题</button>
      </Tooltip>,
    );
    const button = screen.getByRole("button", { name: "主题" });
    const tip = screen.getByRole("tooltip");
    expect(tip.textContent).toBe("切换主题");
    expect(button.getAttribute("aria-describedby")).toBe(tip.id);
    expect(button.getAttribute("title")).toBe("");
  });

  it("renders the child untouched when there is no content", () => {
    render(
      <Tooltip content="">
        <button type="button">主题</button>
      </Tooltip>,
    );
    expect(screen.queryByRole("tooltip")).toBeNull();
    expect(screen.getByRole("button", { name: "主题" }).getAttribute("aria-describedby")).toBeNull();
  });
});

describe("Popover", () => {
  function Harness({ autoFocus = true }: { autoFocus?: boolean }) {
    const [open, setOpen] = useState(false);
    const anchor = useRef<HTMLButtonElement>(null);
    return (
      <>
        <button ref={anchor} type="button" onClick={() => setOpen((v) => !v)}>
          打开
        </button>
        <Popover open={open} onClose={() => setOpen(false)} anchor={anchor} label="上下文占用" autoFocus={autoFocus}>
          <button type="button">内部动作</button>
        </Popover>
        <button type="button">外部</button>
      </>
    );
  }

  it("opens as a labelled dialog, focuses inside, and closes on Escape and outside click", () => {
    vi.useFakeTimers();
    try {
      render(<Harness />);
      fireEvent.click(screen.getByRole("button", { name: "打开" }));
      const dialog = screen.getByRole("dialog", { name: "上下文占用" });
      expect(dialog.getAttribute("data-state")).toBe("open");
      expect(document.activeElement).toBe(screen.getByRole("button", { name: "内部动作" }));

      fireEvent.keyDown(document, { key: "Escape" });
      expect(dialog.getAttribute("data-state")).toBe("closing");
      act(() => {
        vi.advanceTimersByTime(200);
      });
      expect(screen.queryByRole("dialog")).toBeNull();

      fireEvent.click(screen.getByRole("button", { name: "打开" }));
      expect(screen.getByRole("dialog")).toBeTruthy();
      fireEvent.mouseDown(screen.getByRole("button", { name: "外部" }));
      act(() => {
        vi.advanceTimersByTime(200);
      });
      expect(screen.queryByRole("dialog")).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  it("keeps focus on the opener when autoFocus is off", () => {
    render(<Harness autoFocus={false} />);
    const opener = screen.getByRole("button", { name: "打开" });
    opener.focus();
    fireEvent.click(opener);
    expect(screen.getByRole("dialog")).toBeTruthy();
    expect(document.activeElement).toBe(opener);
  });

  it("flips to the opposite side when the preferred side does not fit", () => {
    const rect = new DOMRect(100, 10, 40, 20);
    const below = placePopover(rect, { width: 200, height: 100 }, "top", "start", 8, { width: 800, height: 600 });
    expect(below.side).toBe("bottom");
    expect(below.top).toBe(38);
    const clamped = placePopover(new DOMRect(700, 300, 40, 20), { width: 200, height: 100 }, "bottom", "start", 8, { width: 800, height: 600 });
    expect(clamped.left).toBe(592);
  });
});

describe("ToastStack", () => {
  it("announces toasts politely, keeps the newest on top and animates removal out", () => {
    vi.useFakeTimers();
    try {
      const { rerender } = render(<ToastStack toasts={[{ id: "a", text: "左栏已收起" }, { id: "b", text: "已切换主题" }]} />);
      expect(screen.getByRole("status").getAttribute("aria-live")).toBe("polite");
      const first = screen.getByText("左栏已收起").closest(".ui-toast") as HTMLElement;
      const second = screen.getByText("已切换主题").closest(".ui-toast") as HTMLElement;
      expect(first.style.getPropertyValue("--toast-depth")).toBe("1");
      expect(second.style.getPropertyValue("--toast-depth")).toBe("0");

      rerender(<ToastStack toasts={[{ id: "b", text: "已切换主题" }]} />);
      expect(first.getAttribute("data-state")).toBe("closing");
      act(() => {
        vi.advanceTimersByTime(400);
      });
      expect(screen.queryByText("左栏已收起")).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  it("offers a dismiss button when onDismiss is given", () => {
    const onDismiss = vi.fn();
    render(<ToastStack toasts={[{ id: "a", text: "复制失败", tone: "danger" }]} onDismiss={onDismiss} />);
    fireEvent.click(screen.getByLabelText("关闭通知"));
    expect(onDismiss).toHaveBeenCalledWith("a");
  });
});

describe("Avatar / Skeleton / Card / Kbd", () => {
  it("falls back to the initial when there is no image and keeps the name accessible", () => {
    render(<Avatar name="灯塔向导" />);
    expect(screen.getByRole("img", { name: "灯塔向导" }).textContent).toBe("灯");
  });

  it("renders an image with alt when src is present", () => {
    render(<Avatar name="灯塔向导" src="/a.png" />);
    expect(screen.getByAltText("灯塔向导")).toBeTruthy();
  });

  it("hides skeletons from assistive tech", () => {
    const { container } = render(
      <>
        <Skeleton width={80} />
        <SkeletonText lines={2} />
      </>,
    );
    expect(container.querySelectorAll('[aria-hidden="true"]').length).toBeGreaterThanOrEqual(2);
  });

  it("renders an interactive card as a real button", () => {
    const onClick = vi.fn();
    render(
      <Card interactive onClick={onClick} aria-label="打开档案">
        <CardTitle>角色设定</CardTitle>
      </Card>,
    );
    fireEvent.click(screen.getByRole("button", { name: "打开档案" }));
    expect(onClick).toHaveBeenCalled();
    expect(screen.getByRole("heading", { name: "角色设定" })).toBeTruthy();
  });

  it("renders Kbd as a kbd element", () => {
    render(<Kbd>Ctrl</Kbd>);
    expect(screen.getByText("Ctrl").tagName).toBe("KBD");
  });
});
