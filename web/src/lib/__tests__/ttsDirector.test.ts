// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import {
  stripAudioTags,
  detectVoiceArchetype,
  buildDirectorInstruction,
  enrichDialogueWithAudioTags,
} from "../ttsDirector";

describe("ttsDirector", () => {
  describe("stripAudioTags", () => {
    it("strips English and Chinese parenthesized style tags", () => {
      const input = "(冷峻)你想带我走？（轻蔑微讽）凭你也配。";
      expect(stripAudioTags(input)).toBe("你想带我走？凭你也配。");
    });

    it("strips audio action tags in square and full-width brackets", () => {
      const input = "[深呼吸]外头狂风大作……[停顿片刻]快走！【冷笑】";
      expect(stripAudioTags(input)).toBe("外头狂风大作……快走！");
    });

    it("handles mixed tags and extra spaces cleanly", () => {
      const input = "(冰冷低音) [深呼吸] 别靠近我…… [停顿片刻]";
      expect(stripAudioTags(input)).toBe("别靠近我……");
    });

    it("returns empty string if input is empty", () => {
      expect(stripAudioTags("")).toBe("");
    });
  });

  describe("detectVoiceArchetype", () => {
    it("detects young female voice for girl/apprentice/princess keywords", () => {
      const archetype = detectVoiceArchetype({
        characterName: "林小月",
        characterRole: "初入江湖的剑宗小师妹",
        characterPersona: "性格单纯天真，容易害羞",
      });
      expect(archetype).toContain("年轻女声");
      expect(archetype).toContain("清脆");
    });

    it("detects young male voice for boy/youth/prince keywords", () => {
      const archetype = detectVoiceArchetype({
        characterName: "艾伦",
        characterRole: "见习少年剑士",
        characterPersona: "热血冲动，憧憬冒险",
      });
      expect(archetype).toContain("清爽年轻男声");
      expect(archetype).toContain("朝气");
    });

    it("detects mature female voice for queen/matriarch keywords", () => {
      const archetype = detectVoiceArchetype({
        characterName: "岑当家",
        characterRole: "商会女主人",
        characterPersona: "手段雷厉风行，不怒自威的御姐",
      });
      expect(archetype).toContain("成熟女声");
      expect(archetype).toContain("从容");
    });

    it("detects mature male voice for elder/master keywords", () => {
      const archetype = detectVoiceArchetype({
        characterName: "玄空",
        characterRole: "隐世宗师",
        characterPersona: "白发老者，道骨仙风",
      });
      expect(archetype).toContain("成熟沉稳男声");
    });
  });

  describe("buildDirectorInstruction", () => {
    it("builds dynamic instruction using real character, scene and live mood", () => {
      const instruction = buildDirectorInstruction({
        characterName: "艾伦",
        characterRole: "少年剑士",
        characterPersona: "性格直率开朗，富有正义感",
        sceneLocation: "晨曦酒馆二楼吧台",
        sceneStage: "初次相遇",
        moodText: "兴奋难耐，充满期待",
        moodCode: "happy",
        relationship: { affection: 75, alertness: 10, trust: 80 },
      });

      expect(instruction).toContain("【角色】艾伦（少年剑士）。");
      expect(instruction).toContain("清爽年轻男声");
      expect(instruction).toContain("【场景】晨曦酒馆二楼吧台，阶段：初次相遇。");
      expect(instruction).toContain("【心境】兴奋难耐，充满期待。");
      expect(instruction).toContain("【指导】");
      expect(instruction).toContain("轻快笑意");
      expect(instruction).toContain("温情与信任");
    });

    it("adapts vocal guidance for fear and panic", () => {
      const instruction = buildDirectorInstruction({
        characterName: "小月",
        moodText: "瑟瑟发抖，极为惶恐",
        moodCode: "fear",
      });

      expect(instruction).toContain("【心境】瑟瑟发抖，极为惶恐。");
      expect(instruction).toContain("细微颤音与慌乱");
      expect(instruction).toContain("呼吸浅急");
    });

    it("adapts vocal guidance for anger and rage", () => {
      const instruction = buildDirectorInstruction({
        characterName: "雷恩",
        moodText: "怒不可遏",
        moodCode: "angry",
      });

      expect(instruction).toContain("【心境】怒不可遏。");
      expect(instruction).toContain("发声紧绷");
      expect(instruction).toContain("锐利度");
    });
  });

  describe("enrichDialogueWithAudioTags", () => {
    it("injects dynamic style prefix matching angry live mood", () => {
      const enriched = enrichDialogueWithAudioTags("你为什么要骗我？！", {
        moodCode: "angry",
        moodText: "暴怒质问",
      });
      expect(enriched).toBe("(愤懑质问)你为什么要骗我？！");
    });

    it("injects dynamic style prefix matching shy live mood", () => {
      const enriched = enrichDialogueWithAudioTags("那个……谢谢你刚才护着我。", {
        moodCode: "shy",
        moodText: "脸红心跳",
      });
      expect(enriched).toBe("(羞赧轻声)那个……[停顿片刻]谢谢你刚才护着我。");
    });

    it("does not duplicate style prefix if dialogue already has one", () => {
      const enriched = enrichDialogueWithAudioTags("(冷酷)滚开。", {
        moodCode: "angry",
      });
      expect(enriched).toBe("(冷酷)滚开。");
    });

    it("enriches laugh according to mood", () => {
      const angryLine = enrichDialogueWithAudioTags("呵，你以为你能赢？", {
        moodCode: "angry",
      });
      expect(angryLine).toContain("[冷哼]呵");

      const happyLine = enrichDialogueWithAudioTags("呵，你这人真有意思！", {
        moodCode: "happy",
      });
      expect(happyLine).toContain("[轻笑]呵");
    });
  });
});
