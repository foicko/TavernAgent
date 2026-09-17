import { describe, expect, it } from "vitest";
import { isContainerLine, normalizeAttrValue, parseAttrLine, parseBracketAttrGroup } from "../dossierAttrs";

describe("parseAttrLine", () => {
  it("recognizes the JSON-ish attribute dump used by many English cards", () => {
    expect(parseAttrLine('{"Name": ("Gael")}')).toEqual({ key: "Name", value: "Gael" });
    expect(parseAttrLine('{"Age": ("21")}')).toEqual({ key: "Age", value: "21" });
    expect(parseAttrLine('   {"Species": ("Human")}  ')).toEqual({ key: "Species", value: "Human" });
  });

  it("joins plus-separated quoted values into a readable list", () => {
    expect(parseAttrLine('{"Height": ("171 cm" + "5 feet 8 inches")}')).toEqual({
      key: "Height",
      value: "171 cm、5 feet 8 inches",
    });
    // 中文卡用全角括号与加号
    expect(parseAttrLine('喜欢：（"年长男性" + "赞美" + "调情" + "浪漫" + "{{user}}"）。')).toEqual({
      key: "喜欢",
      value: "年长男性、赞美、调情、浪漫、{{user}}",
    });
  });

  it("recognizes plain key-value lines", () => {
    expect(parseAttrLine("姓名：西娜·韦尔德")).toEqual({ key: "姓名", value: "西娜·韦尔德" });
    expect(parseAttrLine("年龄: 24岁")).toEqual({ key: "年龄", value: "24岁" });
    expect(parseAttrLine("### 性格")).toBeNull();
  });

  it("leaves prose and dialogue to normal paragraph rendering", () => {
    // 长值不是属性，而是需要整段阅读的叙述
    const longProse = "背景：{{char}}是水手守卫者的一员，这些女性超级英雄各自代表太阳系中的一个星球或月亮，高中时因身材高大而闻名。";
    expect(parseAttrLine(longProse)).toBeNull();
    // 对白行不能变成键值对
    expect(parseAttrLine("她说：「你好。」")).toBeNull();
    expect(parseAttrLine("{{char}}: “别想逃。”")).toBeNull();
    expect(parseAttrLine("她完全清楚自己在做什么，但仍然每次都不例外地重蹈覆辙。")).toBeNull();
    expect(parseAttrLine("")).toBeNull();
  });
});

describe("parseBracketAttrGroup", () => {
  it("splits a single-line bracket group into a head and pairs", () => {
    const line = '[{{char}}: 物种（"巨人"），年龄（"26岁"），性取向（"异性恋"），服装（"无"）]';
    const group = parseBracketAttrGroup(line);
    expect(group?.head).toBe("{{char}}");
    expect(group?.pairs).toEqual([
      { key: "物种", value: "巨人" },
      { key: "年龄", value: "26岁" },
      { key: "性取向", value: "异性恋" },
      { key: "服装", value: "无" },
    ]);
  });

  it("does not split on commas nested inside values", () => {
    const group = parseBracketAttrGroup('[玛丽: 个性（"天真，温柔，抱着你"），服装（"无"）]');
    expect(group?.pairs).toEqual([
      { key: "个性", value: "天真，温柔，抱着你" },
      { key: "服装", value: "无" },
    ]);
  });

  it("ignores plain bracketed section labels", () => {
    expect(parseBracketAttrGroup("[核心人设]")).toBeNull();
    expect(parseBracketAttrGroup("[叙述者声音与语调]")).toBeNull();
    // 不是属性的内容整组放弃，交给原来的标签渲染
    expect(parseBracketAttrGroup("[系统: 这里是一段并不符合属性语法的说明文字]")).toBeNull();
  });
});

describe("normalizeAttrValue", () => {
  it("strips quotes and parentheses", () => {
    expect(normalizeAttrValue('"巨人"')).toBe("巨人");
    expect(normalizeAttrValue('（"a" + "b"）')).toBe("a、b");
    expect(normalizeAttrValue("")).toBe("");
  });
});

describe("容器行与容错", () => {
  it("多写一个收尾花括号的属性行仍被识别", () => {
    // 真实卡片里的手写笔误：{"Genitalia": (...)}} —— 之前会漏成正文
    expect(parseAttrLine('{"Genitalia": ("Virgin" + "average length, but thick penis" + "Hairless")}}')).toEqual({
      key: "Genitalia",
      value: "Virgin、average length, but thick penis、Hairless",
    });
  });

  it("属性表容器符号单独成行时被识别为噪声", () => {
    for (const line of ["[", "]", "[]", "{", "}", "（"]) {
      expect(isContainerLine(line)).toBe(true);
    }
    for (const line of ["[核心人设]", "正文内容", "]"]) {
      if (line === "]") continue;
      expect(isContainerLine(line)).toBe(false);
    }
  });
});

describe("值内嵌套括号", () => {
  it("不会在值内部的括号处提前截断", () => {
    // 真实卡片：值里带中文括号的地道名字
    expect(parseAttrLine('{"Behavior": ("盖尔（Gael） 喜欢被人追捧")}')).toEqual({
      key: "Behavior",
      value: "盖尔（Gael） 喜欢被人追捧",
    });
    expect(parseAttrLine('外形：（"高个子（1.8 米）" + "短发"）')).toEqual({
      key: "外形",
      value: "高个子（1.8 米）、短发",
    });
  });
});

describe("漏写收尾括号的属性行", () => {
  it("整行以值结尾时仍按属性行收下", () => {
    // 真实卡片：{"Behavior": ("..." 整行没有闭合的 )}，之前会掉回正文当成身份标语
    const line = [
      '{"Behavior": ("{{char}} loves to have conversation about video games,',
      'anime, his sexuality, but feels socially awkward around people."',
    ].join(" ");
    const pair = parseAttrLine(line);
    expect(pair?.key).toBe("Behavior");
    expect(pair?.value.startsWith("{{char}} loves to have conversation")).toBe(true);
    expect(pair?.value.endsWith("around people.")).toBe(true);
  });
});
