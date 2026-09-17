import { expect, it } from "vitest";
import type { SessionView } from "../../app/types";
import { characterPresentation, storedCardTag } from "../characterPresentation";
import { CHARACTER_PRESETS } from "../characterPresets";

it("a custom character named like a preset retains its own presentation and identity", () => {
  const view = { characterId: "imported-elena", title: "测试", state: { characters: {
    player: { characterId: "player", name: "旅人", participant: true },
    keeper: { characterId: "keeper", name: "Elena", participant: true, description: "灯塔守望者" },
  }, relationships: {}, items: {}, promises: {}, moods: {} } } satisfies Pick<SessionView, "characterId" | "title" | "state">;
  const presented = characterPresentation("custom", view);
  expect(presented.name).toBe("Elena");
  expect(presented.role).toBe("灯塔守望者");
  expect(presented.key).toBe("custom");
  expect(presented.avatar).toBe(CHARACTER_PRESETS.custom.avatar);
  expect(view.characterId).toBe("imported-elena");
});

it("resolves embedded data URL avatar and extracts V2/V3 structured dossier fields", () => {
  const customAvatar = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==";
  const view = {
    characterId: "char-qiangyue",
    title: "强约™ APP - 致命交友约会体验",
    state: {
      characters: {
        player: { characterId: "player", name: "玩家", participant: true },
        app: {
          characterId: "app",
          name: "强约™ APP - 致命交友约会体验",
          nickname: "强约™ APP",
          description: "[叙述者声音与语调] [核心人设] 强约™APP是一个强制性的、无法卸载的、高风险的手机约会软件...",
          personality: "病娇、严密监控、喜怒无常",
          scenario: "深夜空荡的公寓，手机屏幕突然亮起...",
          avatar: customAvatar,
          tags: ["病娇", "AI", "悬疑"],
          creator: "测试创作者",
          participant: true,
        },
      },
      relationships: {},
      items: {},
      promises: {},
      moods: {},
    },
  } satisfies Pick<SessionView, "characterId" | "title" | "state">;

  const presented = characterPresentation("custom", view);
  expect(presented.avatar).toBe(customAvatar);
  expect(presented.shortName).toBe("强约™ APP");
  expect(presented.role).not.toContain("[叙述者声音与语调]"); // short role cleaned
  expect(presented.role.length).toBeLessThan(100);
  expect(presented.dossier.personality).toBe("病娇、严密监控、喜怒无常");
  expect(presented.dossier.scenario).toBe("深夜空荡的公寓，手机屏幕突然亮起...");
  expect(presented.dossier.tags).toEqual(["病娇", "AI", "悬疑"]);
  expect(presented.dossier.creator).toBe("测试创作者");
});

// 身份标语来自卡片描述，里面几乎必然带 {{char}}/{{user}}：
// 它必须在本层展开一次，否则每个消费方（铭牌、立绘灯箱、开场头、角色库）都会漏。
it("expands card macros in the identity tag for every consumer", () => {
  const view = {
    characterId: "char-mary",
    title: "玛丽 的冒险",
    state: {
      characters: {
        player: { characterId: "player", name: "旅人", participant: true },
        npc: { characterId: "npc", name: "玛丽", participant: true, description: "{{char}}从不让{{user}}离开。{{char}}完全赤裸。" },
      },
      relationships: {}, items: {}, promises: {}, moods: {},
    },
  } satisfies Pick<SessionView, "characterId" | "title" | "state">;

  const presented = characterPresentation("custom", view);
  expect(presented.role).not.toContain("{{");
  expect(presented.role).toContain("玛丽从不让旅人离开");
});

it("expands macros for the local card library tags too", () => {
  expect(storedCardTag({ name: "盖尔", shortName: "盖尔", role: "{{char}}很害羞，需要{{user}}陪伴。" }, "旅人"))
    .toBe("盖尔很害羞，需要旅人陪伴。");
  // 库里没有描述时给出占位文案，而不是空白
  expect(storedCardTag({ name: "无描述" })).toBe("导入的角色卡");
});

// 世界状态的 characters[] 用线上（V2 兼容）命名下发。前端若按 camelCase 读，
// 人设面板的「对话范例 / 系统规则 / 创作者说明」会永远为空——这条用例锁住该回归。
it("reads the wire field names so the dossier tabs are never silently empty", () => {
  const view = {
    characterId: "char-wire",
    title: "契约验收",
    state: {
      characters: {
        player: { characterId: "player", name: "旅人", participant: true },
        npc: {
          characterId: "npc_wire",
          name: "契约角色",
          participant: true,
          description: "公开人设",
          personality: "毒舌",
          scenario: "雨夜酒馆",
          mes_example: ["<START>", "{{user}}: 打烊了吗", "{{char}}: 打烊了。"].join("\n"),
          system_prompt: "保持角色口吻",
          post_history_instructions: "结尾要留悬念",
          creator_notes: "写给中文玩家",
          character_version: "1.2",
          tags: ["契约"],
          creator: "作者",
        },
      },
      relationships: {}, items: {}, promises: {}, moods: {},
    },
  } satisfies Pick<SessionView, "characterId" | "title" | "state">;

  const { dossier } = characterPresentation("custom", view);
  expect(dossier.mesExample).toContain("打烊了吗");
  expect(dossier.systemPrompt).toBe("保持角色口吻");
  expect(dossier.postHistoryInstructions).toBe("结尾要留悬念");
  expect(dossier.creatorNotes).toBe("写给中文玩家");
  expect(dossier.version).toBe("1.2");
});
