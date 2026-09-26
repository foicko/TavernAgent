import { describe, expect, it } from "vitest";
import { truncateText } from "../truncateText";

describe("truncateText", () => {
  it("短文本原样返回，不追加省略号", () => {
    expect(truncateText("出发去森林", 36)).toBe("出发去森林");
    expect(truncateText("", 5)).toBe("");
  });

  it("超长时截断并追加省略号", () => {
    expect(truncateText("abcdef", 3)).toBe("abc…");
    expect(truncateText("abcdef", 3, "...")).toBe("abc...");
  });

  it("按码点截断，不劈开代理对", () => {
    const text = "😀😀😀😀";
    expect(truncateText(text, 2)).toBe("😀😀…");
    // 按码元切会在第 3 个码元处劈开第二个 emoji
    expect(truncateText("a😀b", 2)).toBe("a😀…");
  });
});
