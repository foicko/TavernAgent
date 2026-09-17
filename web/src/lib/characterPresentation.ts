import type { SessionView } from "../app/types";
import { CHARACTER_PRESETS, type CharacterPreset } from "./characterPresets";
import { DEFAULT_PLAYER_NAME, replaceMacros } from "./characterMacros";
import {
  findCardByCharacterId,
  findCardByName,
  getSessionAvatar,
  associateSessionWithAvatar,
  cleanShortName,
  cleanShortRole,
} from "./characterCardStore";

export interface CharacterDossier {
  description: string;
  personality: string;
  scenario: string;
  mesExample: string;
  systemPrompt: string;
  postHistoryInstructions: string;
  creatorNotes: string;
  tags: string[];
  creator: string;
  version: string;
  nickname: string;
}

export interface PresentedCharacter extends CharacterPreset {
  dossier: CharacterDossier;
  isCustom: boolean;
}

export function characterPresentation(
  key: string | null,
  view: (Pick<SessionView, "title" | "state"> & { sessionId?: string; characterId?: string }) | null,
): PresentedCharacter {
  const isBuiltInKey = !!key && key !== "custom" && !!CHARACTER_PRESETS[key];
  const preset = CHARACTER_PRESETS[key ?? "custom"] || CHARACTER_PRESETS.custom;

  const emptyDossier: CharacterDossier = {
    description: preset.modalDesc || "",
    personality: "",
    scenario: "",
    mesExample: "",
    systemPrompt: "",
    postHistoryInstructions: "",
    creatorNotes: "",
    tags: [],
    creator: "",
    version: "",
    nickname: "",
  };

  if (!view) {
    return { ...preset, dossier: emptyDossier, isCustom: !isBuiltInKey };
  }

  const characters = Object.values(view.state?.characters ?? {}).filter((c) => c.characterId !== "player");
  const character = characters.find((c) => c.participant) || characters[0];

  // 尝试在本地收录的角色卡库中检索（按 characterId 或 标题/名称）
  const stored =
    findCardByCharacterId(view.characterId) ||
    findCardByCharacterId(character?.characterId) ||
    findCardByName(character?.name || view.title);

  // 头像优先级：角色自身携带 > 会话关联头像 > 本地库缓存 > 预设回退
  const resolvedAvatar =
    character?.avatar ||
    stored?.avatar ||
    (view.sessionId ? getSessionAvatar(view.sessionId) : undefined) ||
    preset.avatar;

  // 若有会话 ID 且解析到 Data URL 头像，缓存关联
  if (view.sessionId && resolvedAvatar && resolvedAvatar.startsWith("data:")) {
    associateSessionWithAvatar(view.sessionId, resolvedAvatar);
  }

  const fullName = character?.name || stored?.name || view.title;
  const shortName = character?.nickname || stored?.shortName || cleanShortName(fullName);
  const playerName = view.state?.characters?.["player"]?.name || DEFAULT_PLAYER_NAME;

  // 铭牌标签：如果是内置官方预设则保留，如果是自定义角色卡则显示真实卡片格式或标签
  let stagePill = preset.stagePill;
  if (!isBuiltInKey) {
    if (stored?.format) {
      stagePill = `${stored.format.toUpperCase()} 角色卡`;
    } else if (character?.tags && character.tags.length > 0) {
      stagePill = character.tags.slice(0, 2).join(" · ");
    } else {
      stagePill = "角色档案 · 动态叙事";
    }
  }

  // 角色定位/身份标语：避免塞入整篇万字提示词与系统格式标记
  let role = preset.role;
  if (character?.description) {
    role = cleanShortRole(character.description);
  } else if (stored?.role) {
    role = cleanShortRole(stored.role);
  }
  // 身份标语取自卡片描述，同样带 {{char}}/{{user}}。在这里统一展开一次，
  // 所有消费方（铭牌、立绘灯箱、开场头、角色库）就不必各自记得替换。
  role = replaceMacros(role, { charName: shortName || fullName, userName: playerName });

  const dossier: CharacterDossier = {
    description: character?.description || stored?.description || preset.modalDesc || "",
    personality: character?.personality || stored?.personality || "",
    scenario: character?.scenario || stored?.scenario || "",
    // 线上（V2 兼容）命名：与世界状态、卡片 JSON、剧情包一致，见 types.ts 的说明。
    mesExample: character?.mes_example || stored?.mesExample || "",
    systemPrompt: character?.system_prompt || stored?.systemPrompt || "",
    postHistoryInstructions: character?.post_history_instructions || "",
    creatorNotes: character?.creator_notes || stored?.creatorNotes || "",
    tags: character?.tags || stored?.tags || [],
    creator: character?.creator || stored?.creator || "",
    version: character?.character_version || stored?.characterVersion || "",
    nickname: character?.nickname || stored?.nickname || "",
  };

  return {
    ...preset,
    name: fullName,
    shortName,
    role,
    stagePill,
    avatar: resolvedAvatar,
    fullImg: resolvedAvatar,
    dossier,
    isCustom: !isBuiltInKey,
  };
}

/**
 * storedCardTag 是本地角色库里一张卡的展示标签（身份标语）。
 * 角色库列表不经过会话视图，因此拿不到 playerName——由调用方传入当前会话的玩家名，
 * 缺失时回退到通用称呼。宏在这里展开，调用方不必各自记得替换。
 */
export function storedCardTag(card: { name: string; shortName?: string; role?: string }, playerName?: string): string {
  return replaceMacros(card.role || "导入的角色卡", {
    charName: card.shortName || card.name,
    userName: playerName || DEFAULT_PLAYER_NAME,
  });
}
