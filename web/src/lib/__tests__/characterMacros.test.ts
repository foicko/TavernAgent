import { describe, expect, it } from "vitest";
import { replaceMacros } from "../characterMacros";

describe("replaceMacros", () => {
  it("expands identity macros", () => {
    expect(replaceMacros("{{char}} 看向 {{user}}", { charName: "阿尔", userName: "旅人" })).toBe("阿尔 看向 旅人");
  });

  it("resolves {{getvar::key}} from session variables", () => {
    const out = replaceMacros("语言：{{getvar::lang}}，场景 {{ getvar::scene }}", {
      charName: "阿尔",
      variables: { lang: "KO", scene: "3" },
    });
    expect(out).toBe("语言：KO，场景 3");
  });

  it("leaves missing variables empty", () => {
    expect(replaceMacros("{{getvar::missing}}!", { charName: "阿尔", variables: {} })).toBe("!");
    expect(replaceMacros("{{getvar::missing}}!", { charName: "阿尔" })).toBe("!");
  });
});
