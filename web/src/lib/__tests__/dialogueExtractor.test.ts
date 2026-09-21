import { describe, it, expect } from "vitest";
import { extractTTSContent } from "../dialogueExtractor";
import type { TextBlock } from "../../app/types";

describe("dialogueExtractor", () => {
  it("filters out narration and inner monologue when dialogueOnly is true with blocks", () => {
    const blocks: TextBlock[] = [
      { kind: "narration", text: "寒风呼啸，推开了酒馆厚重的橡木门。" },
      { kind: "dialogue", speakerId: "char", text: "欢迎来到风舵酒馆，要喝点什么？" },
      { kind: "inner_monologue", text: "（打量着眼前的旅行者，看起来来头不小。）" },
      { kind: "dialogue", speakerId: "char", text: "我们有刚酿好的黑麦啤酒。" },
      { kind: "narration", text: "她递过来一份泛黄的羊皮纸菜单。" },
    ];

    const result = extractTTSContent({ blocks }, { dialogueOnly: true });
    expect(result.hasDialogue).toBe(true);
    expect(result.text).toBe("欢迎来到风舵酒馆，要喝点什么？\n我们有刚酿好的黑麦啤酒。");
    expect(result.text).not.toContain("寒风呼啸");
    expect(result.text).not.toContain("羊皮纸菜单");
    expect(result.text).not.toContain("打量着眼前的旅行者");
  });

  it("returns empty dialogue when blocks only contain narration", () => {
    const blocks: TextBlock[] = [
      { kind: "narration", text: "四周一片寂静，只有壁炉里的柴火噼啪作响。" },
      { kind: "narration", text: "窗外的大雪依然下个不停。" },
    ];

    const result = extractTTSContent({ blocks }, { dialogueOnly: true });
    expect(result.hasDialogue).toBe(false);
    expect(result.text).toBe("");
    expect(result.originalText).toContain("四周一片寂静");
  });

  it("returns all text when dialogueOnly is false with blocks", () => {
    const blocks: TextBlock[] = [
      { kind: "narration", text: "周围很安静。" },
      { kind: "dialogue", text: "你好！" },
    ];

    const result = extractTTSContent({ blocks }, { dialogueOnly: false });
    expect(result.hasDialogue).toBe(true);
    expect(result.text).toBe("周围很安静。\n你好！");
  });

  it("extracts dialogues from raw text using corner quotes and double quotes", () => {
    const rawText = `夜幕降临了。
「你好，陌生人。」一个声音从阴影中传来。
风吹落了树叶。
“请问你有火柴吗？”他又问了一句。
周围重归寂静。`;

    const result = extractTTSContent({ text: rawText }, { dialogueOnly: true });
    expect(result.hasDialogue).toBe(true);
    expect(result.text).toBe("你好，陌生人。\n请问你有火柴吗？");
    expect(result.text).not.toContain("夜幕降临了");
  });

  it("returns empty dialogue when raw text has no quotes and dialogueOnly is true", () => {
    const rawText = `纯粹的环境描写与心理旁白，没有任何人在说话。`;
    const result = extractTTSContent({ text: rawText }, { dialogueOnly: true });
    expect(result.hasDialogue).toBe(false);
    expect(result.text).toBe("");
  });

  it("returns full text when dialogueOnly is false for raw text", () => {
    const rawText = `旁白描述与任何内容。`;
    const result = extractTTSContent({ text: rawText }, { dialogueOnly: false });
    expect(result.hasDialogue).toBe(true);
    expect(result.text).toBe(rawText);
  });
});
