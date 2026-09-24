import { describe, expect, it } from "vitest";
import { describeCapabilities } from "../cardCapabilities";

describe("describeCapabilities", () => {
  it("空特性返回空串", () => {
    expect(describeCapabilities(undefined)).toBe("");
    expect(describeCapabilities({})).toBe("");
  });

  it("汇总模块/主题/变量/标记/资源", () => {
    const text = describeCapabilities({
      hasModule: true,
      hasBackgroundHTML: true,
      hasDefaultVariables: true,
      markerFamilies: ["CeresStatus"],
      imageAssetCount: 189,
      audioAssetCount: 3,
      lorebookEntries: 17,
      lowLevelAccess: true,
    });
    expect(text).toContain("内嵌模块（不执行）");
    expect(text).toContain("自定义主题");
    expect(text).toContain("状态标记：CeresStatus");
    expect(text).toContain("图片 189");
    expect(text).toContain("音频 3");
    expect(text).toContain("世界书 17");
    expect(text).toContain("⚠ 低层访问");
  });

  it("只有图片指令时显示图片指令", () => {
    expect(describeCapabilities({ imgCmd: true })).toContain("图片指令");
  });
});
