import { describe, expect, it } from "vitest";
import { formatActionCheckBadge, formatCheckDetail, getAttributeMeta, getOutcomeMeta } from "../ruleCheck";
import type { CheckResult } from "../../app/types";

describe("ruleCheck utility", () => {
  it("translates known attributes and falls back gracefully", () => {
    expect(getAttributeMeta("strength")).toEqual({ name: "力量", abbr: "STR" });
    expect(getAttributeMeta("dexterity")).toEqual({ name: "敏捷", abbr: "DEX" });
    expect(getAttributeMeta("wisdom")).toEqual({ name: "察觉", abbr: "WIS" });
    expect(getAttributeMeta("charisma")).toEqual({ name: "交涉", abbr: "CHA" });
    expect(getAttributeMeta("luck")).toEqual({ name: "运气", abbr: "LCK" });
    expect(getAttributeMeta("unknown_attr")).toEqual({ name: "unknown_attr", abbr: "UNK" });
    expect(getAttributeMeta(undefined)).toEqual({ name: "通用", abbr: "GEN" });
  });

  it("extracts outcome metadata with crit distinctions", () => {
    const critSuccess = getOutcomeMeta("critical_success");
    expect(critSuccess.isSuccess).toBe(true);
    expect(critSuccess.isCrit).toBe(true);
    expect(critSuccess.label).toBe("大成功");
    expect(critSuccess.tone).toBe("crit-up");

    const success = getOutcomeMeta("success");
    expect(success.isSuccess).toBe(true);
    expect(success.isCrit).toBe(false);
    expect(success.label).toBe("成功");

    const failure = getOutcomeMeta("failure");
    expect(failure.isSuccess).toBe(false);
    expect(failure.isCrit).toBe(false);
    expect(failure.label).toBe("失败");

    const critFailure = getOutcomeMeta("critical_failure");
    expect(critFailure.isSuccess).toBe(false);
    expect(critFailure.isCrit).toBe(true);
    expect(critFailure.label).toBe("大失败");
    expect(critFailure.tone).toBe("crit-down");
  });

  it("formats formula detail string containing d20 breakdown", () => {
    const check: CheckResult = {
      rollId: "r1",
      actionId: "check.strength",
      attribute: "strength",
      attributeModifier: 3,
      natural: 15,
      total: 18,
      dc: 14,
      outcome: "success",
    };
    const detail = formatCheckDetail(check, "破门而入");
    expect(detail).toContain("破门而入（力量）");
    expect(detail).toContain("d20 = 15");
    expect(detail).toContain("修正 +3");
    expect(detail).toContain("总值 18 / DC 14");
    expect(detail).toContain("成功");
  });

  it("formats action check badge with label, DC and fallbacks", () => {
    // 带有 label 和 dc 的行动
    expect(
      formatActionCheckBadge({ actionId: "search", label: "勘察四周", dc: 15 })
    ).toBe("勘察四周 · DC 15");

    // 缺少 label 时使用 actionId
    expect(
      formatActionCheckBadge({ actionId: "act_stealth", dc: 12 })
    ).toBe("act_stealth · DC 12");

    // 缺少 dc 时仅展示 label
    expect(
      formatActionCheckBadge({ actionId: "act_talk", label: "交谈" })
    ).toBe("交谈");

    // 回退到属性与 DC
    expect(
      formatActionCheckBadge(null, { attribute: "dexterity", dc: 14 })
    ).toBe("敏捷检定 · DC 14");

    // 回退到 rawRef 标识
    expect(
      formatActionCheckBadge(null, { rawRef: "act_custom" })
    ).toBe("act_custom");

    // 全空回退默认文案
    expect(formatActionCheckBadge(null)).toBe("规则检定");
  });
});
