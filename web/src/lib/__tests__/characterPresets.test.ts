// 内置角色卡的契约测试。
//
// 全应用只保留一个内置角色，它的卡面同时是"卡片能力示例"：
// 一旦有人随手改动结构（改回复合名、去掉方括号属性组、字段名换回 camelCase、
// 开场 variantId 变空），这些功能会静默退化——所以在纯函数层把契约钉住。
import { describe, expect, it } from "vitest";
import fixture from "../../../../scripts/fixtures/builtin_character_card.json";
import {
  BUILTIN_CHARACTER_NAME,
  CHARACTER_PRESETS,
  DEFAULT_PRESET_KEY,
  SELECTABLE_PRESET_KEYS,
  getPresetCardJson,
} from "../characterPresets";

const card = fixture as any;
const VALID_EFFECTS = ["item_grant", "item_transfer", "item_consume", "promise_settle", "scene_propose", "milestone_propose"];

describe("内置角色集合", () => {
  it("无预设内置角色，纯净启动并提供中性自定义兜底", () => {
    expect(SELECTABLE_PRESET_KEYS).toEqual([]);
    expect(SELECTABLE_PRESET_KEYS).not.toContain("custom");
    // custom 仍作为"没有人物设定"的展示兜底存在，但它不是一张角色卡。
    expect(Object.keys(CHARACTER_PRESETS)).toEqual(["custom"]);
    expect(CHARACTER_PRESETS.custom.role).toContain("角色卡");
    expect(DEFAULT_PRESET_KEY).toBe("custom");
    expect(BUILTIN_CHARACTER_NAME).toBe("");
  });
});

describe("标准卡结构契约", () => {
  it("卡级 name 是短名：{{char}} 展开后落在正文里是角色称呼，不是复合展示名", () => {
    expect(card.name).not.toContain("·");
    expect(card.characters[0].name).toBe(card.name);
  });

  it("属性块用方括号围成一组，人设面板才能排成属性表", () => {
    const attrLine = card.description.split("\n").find((line: string) => line.includes("年龄"));
    expect(attrLine?.startsWith("[")).toBe(true);
    expect(attrLine?.endsWith("]")).toBe(true);
  });

  it("使用 {{char}}/{{user}} 宏，交给会话创建时展开", () => {
    expect(card.description).toContain("{{char}}");
    expect(card.first_mes).toContain("{{char}}");
  });

  it("角色级多词字段用线上命名（与后端 domain.CharacterInfo 一致）", () => {
    const character = card.characters[0];
    for (const key of ["mes_example", "system_prompt", "post_history_instructions", "creator_notes", "character_version"]) {
      expect(character[key], `缺少线上字段 ${key}`).toBeTruthy();
    }
    for (const camel of ["mesExample", "systemPrompt", "postHistoryInstructions", "creatorNotes", "characterVersion", "firstMes"]) {
      expect(character[camel], `不应再出现 camelCase 字段 ${camel}`).toBeUndefined();
    }
  });

  it("开场变体 id 非空且唯一，标题符合后端编号规则（默认开场占 a 位，备选从 b 起）", () => {
    const openings = card.openingVariants;
    expect(openings.length).toBeGreaterThanOrEqual(3);
    const ids = openings.map((o: { variantId: string }) => o.variantId);
    expect(new Set(ids).size).toBe(ids.length);
    expect(ids.every((id: string) => id.length > 0)).toBe(true);
    expect(openings[0].title).toBe("默认开场");
    expect(openings.slice(1).map((o: { title: string }) => o.title)).toEqual(["开场b", "开场c"]);
    expect(openings.every((o: { text: string }) => o.text.trim().length > 0)).toBe(true);
  });

  it("世界书条目带关键词与内容，便于按关键词命中注入", () => {
    expect(card.lorebookRefs).toHaveLength(1);
    const entries = card.lorebookRefs[0].entries;
    expect(entries.length).toBeGreaterThanOrEqual(3);
    for (const entry of entries) {
      expect(entry.entryId).toBeTruthy();
      expect(entry.keys.length).toBeGreaterThan(0);
      expect(entry.content.trim().length).toBeGreaterThan(10);
      expect(entry.enabled).toBe(true);
    }
  });

  it("规则动作结构合法，且效果类型在允许集合内", () => {
    const actions = card.rules.actions;
    expect(Object.keys(actions).length).toBeGreaterThan(0);
    for (const [key, rule] of Object.entries(actions) as Array<[string, Record<string, unknown>]>) {
      expect(rule.actionId).toBe(key);
      expect(typeof rule.attribute).toBe("string");
      expect(Number(rule.dc)).toBeGreaterThanOrEqual(1);
      expect(Number(rule.dc)).toBeLessThanOrEqual(100);
      const consequences = rule.consequences as Record<string, Array<{ type: string; payload: unknown }>>;
      expect(Object.keys(consequences).length).toBeGreaterThan(0);
      for (const effects of Object.values(consequences)) {
        for (const effect of effects) expect(VALID_EFFECTS).toContain(effect.type);
      }
    }
  });

  it("规则动作消耗的物品在初始状态里存在，数量足够", () => {
    const action = Object.values(card.rules.actions)[0] as { consequences: { success: Array<{ payload: { itemId: string; quantity: number } }> } };
    const consumed = action.consequences.success[0].payload;
    const item = card.initialState.items.find((i: { instanceId: string }) => i.instanceId === consumed.itemId);
    expect(item, "规则引用了初始状态里不存在的物品").toBeTruthy();
    expect(item.quantity).toBeGreaterThanOrEqual(consumed.quantity);
  });

  it("序列化后不残留 undefined 字段名", () => {
    expect(getPresetCardJson(DEFAULT_PRESET_KEY)).not.toContain("undefined");
  });

  it("中性占位的卡 JSON 仍然可解析（不因可选字段缺失而崩）", () => {
    const neutral = JSON.parse(getPresetCardJson("custom"));
    expect(neutral.schemaVersion).toBe(2);
    expect(neutral.cardId).toContain("tavernagent_original_custom");
    expect(Array.isArray(neutral.characters)).toBe(true);
  });
});

describe("脚本 fixture 结构规范", () => {
  it("scripts/fixtures/builtin_character_card.json 包含完整的规范卡片字段", () => {
    expect(fixture.schemaVersion).toBe(2);
    expect(fixture.cardId).toBe("tavernagent_original_chixia_v1");
  });
});
