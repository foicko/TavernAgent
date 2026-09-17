// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { DossierMarkdown } from "../dossierMarkdown";
import { replaceMacros } from "../characterMacros";

describe("characterMacros", () => {
  it("replaces {{char}} and {{user}} and cleans <START>", () => {
    const raw = "<START>\n{{char}} greets {{user}}. How are you {{user}}?";
    const replaced = replaceMacros(raw, { charName: "Alice", userName: "Bob" });
    expect(replaced).toContain("Alice greets Bob. How are you Bob?");
    expect(replaced).not.toContain("<START>");
    expect(replaced).not.toContain("{{char}}");
    expect(replaced).not.toContain("{{user}}");
  });
});

describe("DossierMarkdown", () => {
  it("renders bracket tags as stylized badges", () => {
    render(
      <DossierMarkdown
        content={"[核心人设]\n强约™APP是一个强制性的、无法卸载的手机软件。\n[叙述者声音与语调]"}
        context={{ charName: "强约", userName: "旅人" }}
      />
    );
    expect(screen.getByText("核心人设")).toBeDefined();
    expect(screen.getByText("叙述者声音与语调")).toBeDefined();
    expect(screen.getByText(/强约™APP是一个强制性的/)).toBeDefined();
  });

  it("replaces macros and renders quotes and headers", () => {
    const { container } = render(
      <DossierMarkdown
        content={"# 人设概述\n> {{char}}对{{user}}说：别想逃。"}
        context={{ charName: "强约", userName: "旅人" }}
      />
    );
    expect(screen.getByText("人设概述")).toBeDefined();
    expect(container.querySelector("blockquote")?.textContent).toContain("强约对旅人说：别想逃。");
  });

  it("renders informal V2 attribute dumps as a key-value grid", () => {
    const { container } = render(
      <DossierMarkdown
        // 三种社区写法混排：JSON 风格、中文括号加号、单行围成一组
        content={[
          '{"Name": ("Gael")}',
          '{"Height": ("171 cm" + "5 feet 8 inches")}',
          '喜欢：（"年长男性" + "赞美"）。',
          '[{{char}}: 物种（"巨人"），年龄（"26岁"）]',
        ].join("\n")}
        context={{ charName: "玛丽", userName: "旅人" }}
      />
    );
    expect(container.querySelectorAll(".dossier-attr-row")).toHaveLength(5);
    expect(screen.getByText("Name")).toBeDefined();
    expect(screen.getByText("171 cm、5 feet 8 inches")).toBeDefined();
    expect(screen.getByText("年长男性、赞美")).toBeDefined();
    // 整行围成的一组：头衔进标签，其余进属性表
    expect(screen.getByText("玛丽")).toBeDefined();
    expect(screen.getByText("物种")).toBeDefined();
    expect(screen.getByText("巨人")).toBeDefined();
  });

  it("keeps prose that merely contains a colon as a paragraph", () => {
    const { container } = render(
      <DossierMarkdown
        content={"背景：{{char}}是水手守卫者的一员，这些女性超级英雄各自代表太阳系中的一个星球或月亮。\n她完全清楚自己在做什么。"}
        context={{ charName: "真琴", userName: "旅人" }}
      />
    );
    expect(container.querySelectorAll(".dossier-attr-row")).toHaveLength(0);
    expect(screen.getByText(/真琴是水手守卫者的一员/)).toBeDefined();
  });
});
