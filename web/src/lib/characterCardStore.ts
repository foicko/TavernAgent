// characterCardStore: 本地持久化角色卡库与头像映射中心
// 确保导入的角色卡在刷新页面后立绘、元数据与会话头像永不丢失。
//
import { parseAttrLine } from "./dossierAttrs";
import { api } from "../app/api";
import type { CardLibrarySummary } from "../app/types";

// 本地存储（localStorage）总配额通常只有 5MB，且写入失败是静默的：
// 一旦超限，要么整份列表丢失、要么头像在刷新后消失。这里的对策是
// ① 不保存可按需重建的大字段（兆级 characterJson）② 写入失败时逐级淘汰并如实上报。

export interface StoredCharacterCard {
  cardId: string;
  characterId: string;
  name: string;
  shortName: string;
  avatar?: string;
  role?: string;
  format?: string;
  tags?: string[];
  creator?: string;
  characterVersion?: string;
  description?: string;
  personality?: string;
  scenario?: string;
  firstMes?: string;
  mesExample?: string;
  systemPrompt?: string;
  postHistoryInstructions?: string;
  creatorNotes?: string;
  nickname?: string;
  characterJson: string;
  importedAt: string;
}

const STORAGE_KEY = "tavernagent_imported_character_cards";
const SESSION_AVATAR_KEY = "tavernagent_session_avatar_map";

// 单卡的 characterJson 预算。超出的卡不再保存原始 JSON：
// 会话创建走的是导入预览里的那份，本地库只需要元数据与头像。
const MAX_STORED_CARD_JSON = 64 * 1024;
// 会话头像映射的条目上限，避免长期使用后无限增长。
const MAX_SESSION_AVATARS = 40;

// 内存单例缓存，提高读取效率
let memoryCardsCache: StoredCharacterCard[] | null = null;
let memorySessionAvatarMap: Record<string, string> | null = null;

function safeGetItem(key: string): string | null {
  try {
    if (typeof window !== "undefined" && window.localStorage) {
      return window.localStorage.getItem(key);
    }
  } catch {
    // ignore
  }
  return null;
}

// safeSetItem 返回是否真正写入成功。配额超限时浏览器会抛 QuotaExceededError，
// 静默吞掉它就等于「卡在内存里能用、刷新后消失」。
function safeSetItem(key: string, value: string): boolean {
  try {
    if (typeof window !== "undefined" && window.localStorage) {
      window.localStorage.setItem(key, value);
      return true;
    }
  } catch {
    // ignore
  }
  return false;
}

export function listImportedCards(): StoredCharacterCard[] {
  if (memoryCardsCache !== null) return memoryCardsCache;
  const raw = safeGetItem(STORAGE_KEY);
  if (!raw) {
    memoryCardsCache = [];
    return [];
  }
  try {
    const list = JSON.parse(raw);
    memoryCardsCache = Array.isArray(list) ? list : [];
  } catch {
    memoryCardsCache = [];
  }
  return memoryCardsCache;
}

export interface SaveCardResult {
  card: StoredCharacterCard;
  /** 因配额不足被淘汰的旧卡片数量。 */
  evicted: number;
  /** 这张卡是否真的写进了本地存储（false 表示仅存在于当前会话内存中）。 */
  persisted: boolean;
}

export function saveImportedCard(card: Partial<StoredCharacterCard> & { name: string; characterJson: string }): SaveCardResult {
  const existingList = listImportedCards();
  const name = card.name.trim();
  const shortName = card.shortName?.trim() || cleanShortName(name);

  // 解析 characterJson 提取更多元数据
  let parsedMeta: Record<string, unknown> = {};
  try {
    parsedMeta = JSON.parse(card.characterJson);
  } catch {
    // ignore
  }

  const rawCharacters = Array.isArray(parsedMeta.characters) ? (parsedMeta.characters[0] as Record<string, unknown>) : null;
  const characterId = card.characterId || (typeof rawCharacters?.characterId === "string" ? rawCharacters.characterId : "") || (typeof parsedMeta.cardId === "string" ? parsedMeta.cardId : "") || ("card_" + hashSimple(name));
  const cardId = card.cardId || (typeof parsedMeta.cardId === "string" ? parsedMeta.cardId : characterId);
  const avatar = card.avatar || (typeof rawCharacters?.avatar === "string" ? rawCharacters.avatar : "") || (typeof parsedMeta.avatar === "string" ? parsedMeta.avatar : undefined);

  const entry: StoredCharacterCard = {
    cardId,
    characterId,
    name,
    shortName,
    avatar,
    role: card.role || (typeof rawCharacters?.description === "string" ? cleanShortRole(rawCharacters.description) : undefined),
    format: card.format || (typeof parsedMeta.spec === "string" ? parsedMeta.spec : "V2/V3"),
    tags: card.tags || (Array.isArray(rawCharacters?.tags) ? rawCharacters.tags as string[] : undefined),
    creator: card.creator || (typeof rawCharacters?.creator === "string" ? rawCharacters.creator : undefined),
    // 卡片 JSON 的 characters[] 用线上命名（character_version 等），读错名字会静默丢字段。
    characterVersion: card.characterVersion || (typeof rawCharacters?.character_version === "string" ? rawCharacters.character_version : undefined),
    description: card.description || (typeof rawCharacters?.description === "string" ? rawCharacters.description : undefined),
    personality: card.personality || (typeof rawCharacters?.personality === "string" ? rawCharacters.personality : undefined),
    scenario: card.scenario || (typeof rawCharacters?.scenario === "string" ? rawCharacters.scenario : undefined),
    // 开场只在卡级存在（角色的 first_mes 不是世界状态字段）。
    firstMes: card.firstMes,
    mesExample: card.mesExample || (typeof rawCharacters?.mes_example === "string" ? rawCharacters.mes_example : undefined),
    systemPrompt: card.systemPrompt || (typeof rawCharacters?.system_prompt === "string" ? rawCharacters.system_prompt : undefined),
    creatorNotes: card.creatorNotes || (typeof rawCharacters?.creator_notes === "string" ? rawCharacters.creator_notes : undefined),
    characterJson: compactCharacterJson(card.characterJson),
    importedAt: card.importedAt || new Date().toISOString(),
  };

  const nextList = [entry, ...existingList.filter((c) => c.characterId !== entry.characterId && c.cardId !== entry.cardId)];
  const { saved, evicted } = persistCards(nextList);
  memoryCardsCache = saved;

  // 新卡永远排在列表首位、淘汰从尾部开始，因此列表非空即代表这张卡已落盘；
  // 被淘汰的旧卡数量单独上报，不与「这张卡有没有存下来」混为一谈。
  return { card: entry, evicted, persisted: saved.length > 0 };
}

/**
 * 从本地角色卡库移除一张卡。
 *
 * 只影响卡片库：会话的角色数据由服务端持有（建会话时已把卡写进模板与初始状态），
 * 所以删卡不会破坏已有故事——只是不再从卡库里显示、头像与角色标签回退到默认值。
 *
 * 返回删除后的列表；卡不存在时按幂等处理（返回原列表），因为"已经没有了"
 * 与"删成功"对调用方是同一个结果。
 */
export function deleteImportedCard(cardId: string): StoredCharacterCard[] {
  const existingList = listImportedCards();
  const nextList = existingList.filter((c) => c.cardId !== cardId);
  if (nextList.length === existingList.length) return existingList;
  const { saved } = persistCards(nextList);
  // 缓存必须同步更新：否则界面在下次整页刷新前仍会渲染已删除的卡。
  memoryCardsCache = saved;
  return saved;
}

/**
 * refreshImportedCards 用服务端卡库覆盖本地缓存。
 *
 * 这是「服务端为真相、localStorage 仅为缓存」的分界点：服务端成功返回时，
 * 本地缓存只是它的一份投影；与服务端已有条目合并（保留上次导入时缓存的
 * 描述/人设等富字段），避免每次刷新都把详情丢掉。网络失败时保留缓存不动，
 * 让离线仍能展示上次已知的卡库。
 */
export async function refreshImportedCards(): Promise<void> {
  const { cards } = await api.listCards();
  const existing = listImportedCards();
  const byId = new Map(existing.map((c) => [c.cardId, c]));
  const merged = (cards ?? []).map((summary) => summaryToStored(summary, byId.get(summary.cardId)));
  const { saved } = persistCards(merged);
  memoryCardsCache = saved;
}

/**
 * deleteImportedCardRemote 先删服务端、再删本地缓存。
 *
 * 顺序很重要：先删服务端保证「删了就是真删了」；服务端失败时本地缓存不动，
 * 界面不会出现一张刷新后又复活的卡。
 */
export async function deleteImportedCardRemote(cardId: string): Promise<void> {
  await api.deleteCard(cardId);
  deleteImportedCard(cardId);
}

/** 把服务端摘要映射成卡片库条目，并尽量保留本地已有的富字段。 */
function summaryToStored(summary: CardLibrarySummary, existing?: StoredCharacterCard): StoredCharacterCard {
  return {
    cardId: summary.cardId,
    characterId: existing?.characterId || summary.cardId,
    name: summary.name,
    shortName: summary.shortName?.trim() || cleanShortName(summary.name),
    avatar: summary.avatar || existing?.avatar,
    format: summary.format || existing?.format,
    role: summary.role || existing?.role,
    description: existing?.description,
    personality: existing?.personality,
    scenario: existing?.scenario,
    firstMes: existing?.firstMes,
    mesExample: existing?.mesExample,
    systemPrompt: existing?.systemPrompt,
    creatorNotes: existing?.creatorNotes,
    tags: existing?.tags,
    creator: existing?.creator,
    characterVersion: existing?.characterVersion,
    nickname: existing?.nickname,
    characterJson: existing?.characterJson ?? "",
    importedAt: summary.createdAt || existing?.importedAt || new Date().toISOString(),
  };
}

/**
 * persistCards 逐级降级写入：配额不足时从最旧的卡片开始淘汰，直到写入成功。
 * 返回真正落盘的列表——内存缓存必须与之一致，否则界面会显示刷新后就消失的卡。
 */
function persistCards(cards: StoredCharacterCard[]): { saved: StoredCharacterCard[]; evicted: number } {
  let list = cards.slice();
  let evicted = 0;
  while (list.length > 0) {
    if (safeSetItem(STORAGE_KEY, JSON.stringify(list))) {
      return { saved: list, evicted };
    }
    if (list.length === 1) break;
    list = list.slice(0, list.length - 1);
    evicted++;
  }
  return { saved: [], evicted };
}

/**
 * compactCharacterJson 去掉与 avatar 字段重复的内嵌头像，并限制整体体积。
 * 超预算时返回空串：宁可少存一份可按需重建的 JSON，也不要让整份角色库写不进去。
 */
function compactCharacterJson(characterJson: string): string {
  if (!characterJson) return "";
  let parsed: unknown;
  try {
    parsed = JSON.parse(characterJson);
  } catch {
    return "";
  }
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return "";
  const obj = parsed as Record<string, unknown>;
  delete obj.avatar;
  if (Array.isArray(obj.characters)) {
    for (const character of obj.characters) {
      if (character && typeof character === "object") {
        delete (character as Record<string, unknown>).avatar;
      }
    }
  }
  const compacted = JSON.stringify(obj);
  return compacted.length <= MAX_STORED_CARD_JSON ? compacted : "";
}

export function findCardByCharacterId(characterId?: string): StoredCharacterCard | undefined {
  if (!characterId) return undefined;
  return listImportedCards().find((c) => c.characterId === characterId || c.cardId === characterId);
}

export function findCardByName(name?: string): StoredCharacterCard | undefined {
  if (!name) return undefined;
  const target = name.trim().toLowerCase();
  if (!target) return undefined;

  const cards = listImportedCards();
  // 1. 精确全等匹配
  const exact = cards.find((c) => c.name.toLowerCase() === target || c.shortName.toLowerCase() === target);
  if (exact) return exact;

  // 2. 标题包含/前缀匹配（如会话标题 "强约™ APP 的冒险" 匹配 shortName "强约™ APP"）
  return cards.find((c) => {
    const sName = c.shortName.toLowerCase();
    const fName = c.name.toLowerCase();
    return (
      (sName.length >= 2 && target.includes(sName)) ||
      (fName.length >= 2 && target.includes(fName)) ||
      (target.length >= 2 && (sName.includes(target) || fName.includes(target)))
    );
  });
}

export function associateSessionWithAvatar(sessionId: string, avatar: string): void {
  if (!sessionId || !avatar) return;
  if (memorySessionAvatarMap === null) {
    try {
      const raw = safeGetItem(SESSION_AVATAR_KEY);
      memorySessionAvatarMap = raw ? JSON.parse(raw) : {};
    } catch {
      memorySessionAvatarMap = {};
    }
  }
  // 重新插入以刷新顺序，超限时淘汰最早的一条。
  const next: Record<string, string> = { [sessionId]: avatar };
  let kept = 1;
  for (const [key, value] of Object.entries(memorySessionAvatarMap ?? {})) {
    if (key === sessionId) continue;
    if (kept >= MAX_SESSION_AVATARS) break;
    next[key] = value;
    kept++;
  }
  memorySessionAvatarMap = next;
  safeSetItem(SESSION_AVATAR_KEY, JSON.stringify(next));
}

export function getSessionAvatar(sessionId?: string): string | undefined {
  if (!sessionId) return undefined;
  if (memorySessionAvatarMap === null) {
    try {
      const raw = safeGetItem(SESSION_AVATAR_KEY);
      memorySessionAvatarMap = raw ? JSON.parse(raw) : {};
    } catch {
      memorySessionAvatarMap = {};
    }
  }
  return memorySessionAvatarMap?.[sessionId];
}

export function cleanShortName(fullName: string): string {
  if (!fullName) return "自定义";
  // 按照常见连接符截取主名："强约™ APP - 世界第一强约交友平台" -> "强约™ APP"
  const parts = fullName.split(/\s*[-—·–_]\s*/);
  return parts[0]?.trim() || fullName;
}

export function cleanShortRole(description?: string): string {
  if (!description) return "导入的角色卡";
  // 逐行挑第一条像人话的：V2 卡常把属性表排在开头，
  // 直接取「清理后的首行」会得到 "[" 或 {"Name": ("Gael")} 这种垃圾标语。
  for (const raw of description.split(/[\r\n]+/)) {
    const line = roleLine(raw);
    if (line) return truncateRole(line);
  }
  // 整段只有标记与属性（例如只写了 {"Name": ("Gael")} 的卡）：
  // 退回占位文案，而不是把 JSON 片段当身份标语糊在铭牌上。
  return "导入的角色卡";
}

// roleLine 把一行描述整理成可读的身份标语；纯标记行与属性行返回空串。
function roleLine(raw: string): string {
  const line = raw
    .replace(/<START>/gi, "")
    .replace(/^#+\s*/, "")
    .replace(/^[>\s*\-–—+•]+/, "")
    .replace(/\[[^\]]*\]/g, "")
    .trim();
  if (!line) return "";
  if (!/[\p{L}\p{N}]/u.test(line)) return ""; // 只剩围括号/标点
  if (parseAttrLine(line)) return ""; // 属性行让给设定面板，不做身份标语
  return line;
}

function truncateRole(text: string): string {
  if (text.length <= 60) return text;
  return text.slice(0, 55) + "…";
}

function hashSimple(s: string): string {
  let h = 0;
  for (let i = 0; i < s.length; i++) h = ((h << 5) - h + s.charCodeAt(i)) | 0;
  return (Math.abs(h) % 0xffffff).toString(16).padStart(6, "0");
}
