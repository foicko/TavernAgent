// 角色卡模型与中性兜底展示定义。
//
// 应用遵循纯净开源分发规范，默认不携带第三方卡片，全由玩家自由导入自己的 V2/V3 角色卡。
// 当尚未载入卡片时，使用中性 custom 皮肤作为优雅空状态承接。
export interface EntityNode {
  id: string;
  label: string;
  type: "person" | "item" | "place" | "faction" | "danger" | "rule";
  icon: string;
  desc: string;
  affinity: string;
}

export interface MemoryNode {
  id: string;
  category: "episodic" | "inference" | "secret";
  categoryLabel: string;
  turnTag: string;
  turnDesc: string;
  recalled: boolean;
  pinned: boolean;
  hidden: boolean;
  edited: boolean;
  content: string;
  entities: string[];
  valence: string;
  confidence?: number;
}

export interface InventoryAct {
  actionRef?: string;
  key: string;
  label: string;
  text?: string;
  isModal?: boolean;
}

export interface InventoryItemPreset {
  id: string;
  icon: string;
  name: string;
  status: string;
  title: string;
  acts: InventoryAct[];
}

export interface TreeNodePreset {
  id: number | string;
  tag: string;
  desc: string;
  aff: number;
  trust: number;
  alert: number;
  isFork?: boolean;
  isActive?: boolean;
}

export interface MacroPreset {
  label: string;
  text: string;
}

export interface CharacterPreset {
  key: string;
  name: string;
  shortName: string;
  role: string;
  stagePill: string;
  stateBadge: string;
  allianceBadge: string;
  voiceBadge: string;
  avatar: string;
  fullImg: string;
  modalDesc: string;
  breadcrumb: string;
  meters: {
    label1: string;
    val1: string;
    width1: string;
    label2: string;
    val2: string;
    width2: string;
    label3: string;
    val3: string;
    width3: string;
  };
  entities: EntityNode[];
  memories: MemoryNode[];
  inventory: InventoryItemPreset[];
  pledge: {
    title: string;
    status: string;
    text: string;
  };
  treeNodes: TreeNodePreset[];
  telemetryHtml: string;
  inputPlaceholder: string;
  macros: MacroPreset[];
  streamHtml: string;
  prologue: string;
}

export interface LoreEntryPreset {
  id: string;
  title: string;
  keys: string[];
  content: string;
  status: "active" | "always" | "locked";
  alwaysActive: boolean;
}

// 世界书面板展示用的种子条目，与内置角色卡的世界书同源。
export const INITIAL_LORE_ENTRIES: LoreEntryPreset[] = [];

// 中性皮肤：玩家尚未导入角色卡、或会话来自自定义卡时使用。
// 它不是一张「内置角色卡」，只是没有人物设定时的展示兜底。
const neutral: CharacterPreset = {
  key: "custom",
  name: "未选择角色",
  shortName: "未选择",
  role: "请在左侧栏导入角色卡",
  stagePill: "暂无角色",
  stateBadge: "状态：待定",
  allianceBadge: "关系：尚未建立",
  voiceBadge: "叙事：待定",
  avatar: "/assets/original/custom.svg",
  fullImg: "/assets/original/custom.svg",
  modalDesc: "尚未载入角色卡。请点击左侧栏的「+ 导入 / 创建角色卡」，上传 PNG 或 JSON 角色卡开启剧情。",
  breadcrumb: "<span>等待开启冒险</span>",
  meters: {
    label1: "心意眷顾 (Affection)",
    val1: "0",
    width1: "50%",
    label2: "信任沉淀 (Trust)",
    val2: "0",
    width2: "0%",
    label3: "防备戒心 (Alertness)",
    val3: "0",
    width3: "0%",
  },
  entities: [],
  memories: [],
  treeNodes: [],
  inventory: [],
  pledge: { title: "尚未作出约定", status: "等待剧情", text: "故事中达成的承诺会在这里记录。" },
  telemetryHtml: "<span>等待开启冒险</span>",
  inputPlaceholder: "输入你的行动或对白…（Enter 发送，Shift+Enter 换行）",
  macros: [],
  streamHtml: "",
  prologue: "",
};

export const CHARACTER_PRESETS: Record<string, CharacterPreset> = {
  custom: neutral,
};

export const SELECTABLE_PRESET_KEYS: string[] = [];

/** 默认角色的 key（未指定角色时使用）。 */
export const DEFAULT_PRESET_KEY = "custom";

/** 内置角色的展示名，供界面文案使用。 */
export const BUILTIN_CHARACTER_NAME = "";

/**
 * getPresetCardJson 生成原生卡 JSON。
 */
export function getPresetCardJson(charKey: string): string {
  const c = CHARACTER_PRESETS[charKey] ?? CHARACTER_PRESETS[DEFAULT_PRESET_KEY];
  return JSON.stringify({
    schemaVersion: 2,
    cardId: `tavernagent_original_${c.key}_v1`,
    name: c.shortName,
    description: c.modalDesc,
    first_mes: c.prologue,
    characters: [{
      characterId: `npc_${c.key}`,
      name: c.shortName,
      description: c.modalDesc,
      participant: true,
      avatar: c.avatar,
    }],
  }, null, 2);
}
