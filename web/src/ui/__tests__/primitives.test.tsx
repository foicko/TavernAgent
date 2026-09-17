// @vitest-environment jsdom
// 原语层的契约测试：这些行为（可访问名、焦点边界、键盘）是各页面共同依赖的地基。
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useState } from "react";
import { Button, IconButton } from "../Button";
import { Disclosure } from "../Disclosure";
import { Field, NumberInput, Select, TextInput } from "../Field";
import { ListRow } from "../ListRow";
import { Menu, MenuItem, MenuSeparator } from "../Menu";
import { Modal } from "../Modal";
import { Switch } from "../Switch";

afterEach(cleanup);

describe("Field 与输入控件", () => {
  it("auto-wires the label to its control so getByLabelText works without ids", () => {
    render(
      <Field label="模型名称" hint="例如 deepseek-chat">
        <TextInput />
      </Field>,
    );
    const input = screen.getByLabelText("模型名称");
    expect(input.tagName).toBe("INPUT");
    // hint 走 aria-describedby，不并入可访问名
    expect(input.getAttribute("aria-describedby")).toBeTruthy();
    expect(screen.getByText("例如 deepseek-chat")).toBeTruthy();
  });

  it("shows a field error as an alert and links it to the control", () => {
    render(
      <Field label="接口地址" error="地址格式不对">
        <TextInput />
      </Field>,
    );
    const input = screen.getByLabelText("接口地址");
    expect(screen.getByRole("alert").textContent).toBe("地址格式不对");
    expect(input.getAttribute("aria-describedby")).toBe(screen.getByRole("alert").id);
  });

  it("keeps Field contexts apart when several fields are on one page", () => {
    render(
      <>
        <Field label="名称">
          <TextInput placeholder="a" />
        </Field>
        <Field label="协议">
          <Select>
            <option value="openai-chat">OpenAI 兼容</option>
          </Select>
        </Field>
        <Field label="温度">
          <NumberInput value={0.8} onValueChange={() => {}} />
        </Field>
      </>,
    );
    expect(screen.getByLabelText("名称").getAttribute("placeholder")).toBe("a");
    expect((screen.getByLabelText("协议") as HTMLSelectElement).tagName).toBe("SELECT");
    expect((screen.getByLabelText("温度") as HTMLInputElement).value).toBe("0.8");
  });

  it("lets NumberInput be cleared instead of collapsing to zero", () => {
    const seen: Array<number | undefined> = [];
    function Harness() {
      const [value, setValue] = useState<number | undefined>(4096);
      return (
        <Field label="最大输出">
          <NumberInput
            value={value}
            onValueChange={(next) => {
              seen.push(next);
              setValue(next);
            }}
          />
        </Field>
      );
    }
    render(<Harness />);
    const input = screen.getByLabelText("最大输出") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "" } });
    expect(input.value).toBe("");
    expect(seen.at(-1)).toBeUndefined();
    fireEvent.change(input, { target: { value: "8192" } });
    expect(seen.at(-1)).toBe(8192);
  });

  it("ignores non-numeric typing in NumberInput", () => {
    const seen: Array<number | undefined> = [];
    render(
      <Field label="温度">
        <NumberInput value={0.8} onValueChange={(next) => seen.push(next)} />
      </Field>,
    );
    const input = screen.getByLabelText("温度") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "abc" } });
    expect(input.value).toBe("0.8");
    expect(seen).toHaveLength(0);
  });
});

describe("Button", () => {
  it("exposes loading as disabled + aria-busy", () => {
    render(
      <Button variant="primary" loading>
        保存
      </Button>,
    );
    const button = screen.getByRole("button", { name: "保存" });
    expect((button as HTMLButtonElement).disabled).toBe(true);
    expect(button.getAttribute("aria-busy")).toBe("true");
  });

  it("gives icon buttons an accessible name from label", () => {
    render(<IconButton label="关闭">✕</IconButton>);
    const button = screen.getByLabelText("关闭");
    expect(button.getAttribute("title")).toBe("关闭");
  });
});

describe("Switch", () => {
  it("uses the label as the accessible name and the hint as its description", () => {
    render(<Switch label="启用后台记忆" hint="后台抽取记忆" checked={false} onChange={() => {}} />);
    const input = screen.getByLabelText("启用后台记忆");
    expect(input.getAttribute("aria-describedby")).toBe(screen.getByText("后台抽取记忆").id);
  });

  it("reports the new checked state", () => {
    const onChange = vi.fn();
    render(<Switch label="截断自动续写" checked={false} onChange={onChange} />);
    fireEvent.click(screen.getByLabelText("截断自动续写"));
    expect(onChange).toHaveBeenCalledWith(true);
  });
});

describe("Disclosure", () => {
  it("hides its body until expanded and reports aria-expanded", () => {
    render(
      <Disclosure summary="高级参数" value="温度 0.8">
        <span>温度输入</span>
      </Disclosure>,
    );
    const trigger = screen.getByRole("button", { name: /高级参数/ });
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByText("温度输入")).toBeNull();
    fireEvent.click(trigger);
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByText("温度输入")).toBeTruthy();
  });

  it("starts expanded when defaultOpen is set", () => {
    render(
      <Disclosure summary="高级参数" defaultOpen>
        <span>温度输入</span>
      </Disclosure>,
    );
    expect(screen.getByText("温度输入")).toBeTruthy();
  });
});

describe("ListRow", () => {
  it("keeps row actions as siblings so no button is nested in another", () => {
    render(
      <ListRow
        title="DeepSeek 官方"
        subtitle="deepseek-chat · api.deepseek.com"
        meta={<span>密钥已存</span>}
        actions={<button type="button">编辑</button>}
        onSelect={() => {}}
        ariaLabel="编辑连接 DeepSeek 官方"
      />,
    );
    expect(screen.getByRole("button", { name: "编辑连接 DeepSeek 官方" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "编辑" })).toBeTruthy();
    expect(document.querySelectorAll("button button")).toHaveLength(0);
  });

  it("marks the selected row with aria-current", () => {
    render(<ListRow title="A" selected onSelect={() => {}} />);
    expect(screen.getByRole("button", { name: "A" }).getAttribute("aria-current")).toBe("true");
  });
});

describe("Menu", () => {
  function Harness() {
    const [open, setOpen] = useState(true);
    return (
      <Menu open={open} label="选择主线模型" onClose={() => setOpen(false)} footer="对新回合生效">
        <MenuItem title="A" description="第一个" onSelect={() => {}} />
        <MenuItem title="B" description="第二个" onSelect={() => {}} />
        <MenuSeparator />
        <MenuItem title="C" onSelect={() => {}} />
      </Menu>
    );
  }

  it("focuses the first item and moves with arrow keys", () => {
    render(<Harness />);
    const items = screen.getAllByRole("menuitem");
    expect(document.activeElement).toBe(items[0]);
    fireEvent.keyDown(items[0], { key: "ArrowDown" });
    expect(document.activeElement).toBe(items[1]);
    fireEvent.keyDown(items[1], { key: "End" });
    expect(document.activeElement).toBe(items.at(-1));
    fireEvent.keyDown(items.at(-1)!, { key: "Home" });
    expect(document.activeElement).toBe(items[0]);
    fireEvent.keyDown(items[0], { key: "ArrowUp" });
    expect(document.activeElement).toBe(items[0]);
  });

  it("closes on Escape and on Tab", () => {
    render(<Harness />);
    fireEvent.keyDown(screen.getAllByRole("menuitem")[0], { key: "Escape" });
    expect(screen.queryByRole("menu")).toBeNull();

    cleanup();
    render(<Harness />);
    fireEvent.keyDown(screen.getAllByRole("menuitem")[0], { key: "Tab" });
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("carries the radio semantics for a single-choice list", () => {
    render(
      <Menu open label="选择主线模型" onClose={() => {}}>
        <MenuItem radio selected title="A" onSelect={() => {}} />
        <MenuItem radio title="B" onSelect={() => {}} />
      </Menu>,
    );
    expect(screen.getByRole("menuitemradio", { name: /^A/ }).getAttribute("aria-checked")).toBe("true");
    expect(screen.getByRole("menuitemradio", { name: /^B/ }).getAttribute("aria-checked")).toBe("false");
  });
});

describe("Modal", () => {
  it("exposes an accessible dialog and closes on Escape and backdrop click", () => {
    const onClose = vi.fn();
    render(
      <Modal id="settings-modal" label="模型设置" title="模型设置" onClose={onClose}>
        <span>正文</span>
      </Modal>,
    );
    expect(screen.getByRole("dialog", { name: "模型设置" })).toBeTruthy();
    expect(screen.getByText("正文")).toBeTruthy();

    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);

    fireEvent.click(document.getElementById("settings-modal")!);
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  it("does not close when the click lands inside the dialog", () => {
    const onClose = vi.fn();
    render(
      <Modal label="模型设置" onClose={onClose}>
        <span>正文</span>
      </Modal>,
    );
    fireEvent.click(screen.getByText("正文"));
    expect(onClose).not.toHaveBeenCalled();
  });

  it("renders nothing when closed", () => {
    render(
      <Modal open={false} label="模型设置" onClose={() => {}}>
        <span>正文</span>
      </Modal>,
    );
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("ships a close button labelled 关闭", () => {
    const onClose = vi.fn();
    render(
      <Modal label="模型设置" onClose={onClose}>
        <span>正文</span>
      </Modal>,
    );
    fireEvent.click(screen.getByLabelText("关闭"));
    expect(onClose).toHaveBeenCalled();
  });
});
