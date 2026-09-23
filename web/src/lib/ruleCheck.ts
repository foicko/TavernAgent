import type { CheckResult } from "../app/types";

/**
 * TRPG 核心基础属性本地化字典
 */
export interface AttributeMeta {
  name: string;
  abbr: string;
}

const ATTRIBUTE_MAP: Record<string, AttributeMeta> = {
  strength: { name: "力量", abbr: "STR" },
  str: { name: "力量", abbr: "STR" },
  dexterity: { name: "敏捷", abbr: "DEX" },
  dex: { name: "敏捷", abbr: "DEX" },
  constitution: { name: "体质", abbr: "CON" },
  con: { name: "体质", abbr: "CON" },
  intelligence: { name: "智力", abbr: "INT" },
  int: { name: "智力", abbr: "INT" },
  wisdom: { name: "察觉", abbr: "WIS" },
  wis: { name: "察觉", abbr: "WIS" },
  perception: { name: "感知", abbr: "PER" },
  per: { name: "感知", abbr: "PER" },
  charisma: { name: "交涉", abbr: "CHA" },
  cha: { name: "交涉", abbr: "CHA" },
  luck: { name: "运气", abbr: "LCK" },
  lck: { name: "运气", abbr: "LCK" },
  agility: { name: "身手", abbr: "AGI" },
  agi: { name: "身手", abbr: "AGI" },
};

/**
 * 获取属性的中文名与英文缩写
 */
export function getAttributeMeta(attr?: string): AttributeMeta {
  if (!attr) return { name: "通用", abbr: "GEN" };
  const key = attr.toLowerCase().trim();
  return ATTRIBUTE_MAP[key] ?? { name: attr, abbr: attr.slice(0, 3).toUpperCase() };
}

/**
 * 检定成败元数据
 */
export interface OutcomeMeta {
  label: string;
  tone: "crit-up" | "up" | "down" | "crit-down";
  isSuccess: boolean;
  isCrit: boolean;
  badgeClass: string;
  symbol: string;
}

export function getOutcomeMeta(outcome: string): OutcomeMeta {
  switch (outcome) {
    case "critical_success":
      return {
        label: "大成功",
        tone: "crit-up",
        isSuccess: true,
        isCrit: true,
        badgeClass: "outcome-crit-success",
        symbol: "✦",
      };
    case "success":
      return {
        label: "成功",
        tone: "up",
        isSuccess: true,
        isCrit: false,
        badgeClass: "outcome-success",
        symbol: "✓",
      };
    case "critical_failure":
      return {
        label: "大失败",
        tone: "crit-down",
        isSuccess: false,
        isCrit: true,
        badgeClass: "outcome-crit-failure",
        symbol: "✕",
      };
    case "failure":
    default:
      return {
        label: outcome === "failure" ? "失败" : outcome,
        tone: "down",
        isSuccess: false,
        isCrit: false,
        badgeClass: "outcome-failure",
        symbol: "✕",
      };
  }
}

/**
 * 格式化检定算式说明（保留 "d20 = N" 以对齐契约与测试）
 */
export function formatCheckDetail(check: CheckResult, actionLabel?: string): string {
  const attr = getAttributeMeta(check.attribute);
  const title = actionLabel || `${attr.name}检定`;
  const modSign = check.attributeModifier >= 0 ? "+" : "";
  const outcomeMeta = getOutcomeMeta(check.outcome);
  return `${title}（${attr.name}）：d20 = ${check.natural}，修正 ${modSign}${check.attributeModifier}，总值 ${check.total} / DC ${check.dc} · ${outcomeMeta.label}`;
}

/**
 * 格式化行动/检定徽标文案
 * 支持传入可选的行动项，或回退属性/DC/纯引用
 */
export function formatActionCheckBadge(
  action?: { label?: string; actionId?: string; dc?: number } | null,
  fallback?: { attribute?: string; dc?: number; rawRef?: string }
): string {
  if (action) {
    const title = action.label || action.actionId;
    return action.dc !== undefined ? `${title} · DC ${action.dc}` : (title || "规则检定");
  }
  if (fallback?.attribute && fallback.dc !== undefined) {
    return `${getAttributeMeta(fallback.attribute).name}检定 · DC ${fallback.dc}`;
  }
  return fallback?.rawRef || "规则检定";
}

