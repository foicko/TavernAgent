import { describe, it, expect } from "vitest";
import { extractImagePrompt } from "../imagePromptExtractor";
import type { TextBlock } from "../../app/types";

describe("imagePromptExtractor", () => {
  it("extracts visual prompts from story blocks and applies anime style", () => {
    const blocks: TextBlock[] = [
      { kind: "narration", text: "寒风中，她站在酒馆门前，月光洒在银白色的长发上。" },
      { kind: "dialogue", text: "「要进来坐坐吗？」" },
      { kind: "inner_monologue", text: "（心中有些警惕）" },
    ];

    const result = extractImagePrompt({ blocks, characterName: "艾莉丝" }, "anime");
    expect(result.prompt).toContain("Character: 艾莉丝");
    expect(result.prompt).toContain("寒风中，她站在酒馆门前");
    expect(result.prompt).toContain("masterpiece, best quality, anime visual novel CG");
    expect(result.negativePrompt).toContain("worst quality");
    // Should remove parentheses content
    expect(result.prompt).not.toContain("心中有些警惕");
  });

  it("handles empty source gracefully", () => {
    const result = extractImagePrompt({}, "fantasy");
    expect(result.prompt).toContain("masterpiece, dark high fantasy illustration");
    expect(result.negativePrompt).toContain("worst quality");
  });

  it("supports different style presets", () => {
    const resCinematic = extractImagePrompt({ text: "rainy neon city" }, "cinematic");
    expect(resCinematic.prompt).toContain("35mm film still, cinematic lighting");

    const resRealistic = extractImagePrompt({ text: "portrait of an old knight" }, "realistic");
    expect(resRealistic.prompt).toContain("ultra realistic photography");
  });
});
