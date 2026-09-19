// @vitest-environment jsdom
// 回合依据条：把「为什么这样演」摊给读者看。
//
// 这里锁的重点不是排版，而是**只用服务端已提交的数据**：变化摘要、检定、记忆快照
// 都由节点内容带下来，前端不做任何推断或补全——依据一旦是猜的就不再是依据。
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { TurnEvidence } from "../TurnEvidence";
import type { StoryMessage } from "../../stores/storyTypes";

function message(overrides: Partial<StoryMessage> = {}): StoryMessage {
  return { id: "n1", role: "assistant", blocks: [], options: [], ...overrides };
}

afterEach(cleanup);

describe("TurnEvidence", () => {
  it("没有可展示的依据时什么都不渲染", () => {
    const { container } = render(<TurnEvidence msg={message({ inputText: "我走过去。" })} />);
    expect(container.querySelector(".turn-evidence")).toBeNull();
  });

  it("结果先看得见：变化与检定以常驻筹码呈现", () => {
    render(<TurnEvidence msg={message({
      checks: [{ rollId: "r1", actionId: "check.charisma", attribute: "charisma", attributeModifier: 2, natural: 14, total: 16, dc: 12, outcome: "success" }],
      changes: [
        { kind: "relationship", label: "艾琳娜 · 好感", text: "+2（28 → 30）", delta: 2 },
        { kind: "item", label: "物品", text: "消耗「铜钥匙」×1", delta: -1 },
      ],
    })} />);

    expect(screen.getByText("艾琳娜 · 好感")).toBeTruthy();
    expect(screen.getByText("+2（28 → 30）")).toBeTruthy();
    // 检定常驻显示的是结果，不是骰值细节（细节在展开里）。
    expect(screen.getByText("成功")).toBeTruthy();
    expect(screen.getByText("消耗「铜钥匙」×1")).toBeTruthy();
    // 展开前不展示明细。
    expect(screen.queryByText(/d20 = 14/)).toBeNull();
  });

  it("展开后给出明细：检定依据、状态变化与记忆快照", async () => {
    const user = userEvent.setup();
    render(<TurnEvidence msg={message({
      checks: [{ rollId: "r1", actionId: "check.charisma", attribute: "charisma", attributeModifier: 2, natural: 14, total: 16, dc: 12, outcome: "success" }],
      changes: [{ kind: "secret", label: "秘密", text: "解锁「她的旧名」" }],
      injectedMemories: [
        { memoryId: "m1", text: "她怕冷，总把炉子烧得很旺。", kind: "observed" },
        { memoryId: "m2", text: "她提到过北方有一座钟楼。", kind: "reported" },
      ],
    })} />);

    const toggle = screen.getByRole("button", { name: /展开依据/ });
    expect(toggle.textContent).toContain("1 次检定");
    expect(toggle.textContent).toContain("2 条记忆");
    await user.click(toggle);

    expect(screen.getByText(/d20 = 14/)).toBeTruthy();
    expect(screen.getByText("解锁「她的旧名」")).toBeTruthy();
    expect(screen.getByText("她怕冷，总把炉子烧得很旺。")).toBeTruthy();
    expect(screen.getByText("见证")).toBeTruthy();
    // 记忆没被用上时，读者要能想到"可能是没召回"，而不是以为角色失忆。
    expect(screen.getByText(/也可能是这条记忆没被召回/)).toBeTruthy();
  });

  it("玩家注记常驻显示，并与结果区分开", () => {
    render(<TurnEvidence msg={message({
      inputNote: "这次让 NPC 先开口。",
      changes: [{ kind: "scene", label: "场景", text: "钟楼二层" }],
    })} />);

    expect(screen.getByText("本轮注记")).toBeTruthy();
    expect(screen.getByText("这次让 NPC 先开口。")).toBeTruthy();
    expect(screen.getByText("钟楼二层")).toBeTruthy();
  });
});
