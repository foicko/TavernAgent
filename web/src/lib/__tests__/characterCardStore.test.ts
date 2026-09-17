// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from "vitest";

// 浏览器 localStorage 的写入失败是静默的：配额超限就抛异常丢掉整份列表。
// 这里用带配额的假实现把那条路径显式测出来。
function installQuota(limitChars: number): Map<string, string> {
  const store = new Map<string, string>();
  const mock = {
    getItem: (key: string) => (store.has(key) ? store.get(key)! : null),
    setItem: (key: string, value: string) => {
      let total = value.length;
      for (const [k, v] of store) if (k !== key) total += v.length;
      if (total > limitChars) {
        const err = new Error("QuotaExceededError");
        err.name = "QuotaExceededError";
        throw err;
      }
      store.set(key, value);
    },
    removeItem: (key: string) => {
      store.delete(key);
    },
    clear: () => store.clear(),
    key: (i: number) => Array.from(store.keys())[i] ?? null,
    get length() {
      return store.size;
    },
  };
  Object.defineProperty(window, "localStorage", { value: mock, configurable: true, writable: true });
  return store;
}

async function freshStore() {
  vi.resetModules();
  return await import("../characterCardStore");
}

const cardWith = (i: number) => ({
  name: `角色${i}`,
  characterJson: JSON.stringify({ schemaVersion: 2, name: `角色${i}`, description: "x".repeat(1200) }),
});

beforeEach(() => {
  installQuota(5_000_000);
});

describe("saveImportedCard 存储治理", () => {
  it("不把头像在 characterJson 里再存一份", async () => {
    const mod = await freshStore();
    const avatar = "data:image/jpeg;base64," + "A".repeat(5000);
    const result = mod.saveImportedCard({
      name: "图卡",
      avatar,
      characterJson: JSON.stringify({
        schemaVersion: 2,
        name: "图卡",
        avatar,
        characters: [{ name: "图卡", avatar }],
      }),
    });

    expect(result.persisted).toBe(true);
    const stored = mod.listImportedCards()[0];
    expect(stored.avatar).toBe(avatar);
    const parsed = JSON.parse(stored.characterJson);
    expect(parsed.avatar).toBeUndefined();
    expect(parsed.characters[0].avatar).toBeUndefined();
    expect(stored.characterJson.length).toBeLessThan(500);
  });

  it("超大 characterJson 直接不存，而不是让一张卡撑爆整个角色库", async () => {
    const mod = await freshStore();
    const result = mod.saveImportedCard({
      name: "巨型卡",
      characterJson: JSON.stringify({ schemaVersion: 2, name: "巨型卡", description: "x".repeat(200000) }),
    });

    expect(result.persisted).toBe(true);
    const stored = mod.listImportedCards()[0];
    expect(stored.characterJson).toBe("");
    expect(stored.name).toBe("巨型卡");
    expect(stored.shortName).toBe("巨型卡");
  });

  it("配额不足时淘汰最旧的卡片并如实上报", async () => {
    installQuota(4200);
    const mod = await freshStore();
    const results = [0, 1, 2, 3].map((i) => mod.saveImportedCard(cardWith(i)));

    const last = results[results.length - 1];
    expect(last.persisted).toBe(true);
    expect(last.evicted).toBeGreaterThan(0);
    const cards = mod.listImportedCards();
    expect(cards.length).toBeLessThan(4);
    expect(cards[0].name).toBe("角色3");
  });

  it("列表里保留的是真正落盘的内容（内存缓存与存储一致）", async () => {
    installQuota(4200);
    const mod = await freshStore();
    [0, 1, 2, 3].forEach((i) => mod.saveImportedCard(cardWith(i)));

    const persisted = JSON.parse(window.localStorage.getItem("tavernagent_imported_character_cards")!);
    expect(mod.listImportedCards().map((c) => c.cardId)).toEqual(persisted.map((c: { cardId: string }) => c.cardId));
  });
});

describe("会话头像映射", () => {
  it("条目数有上限，超出后淘汰最早的一条", async () => {
    const mod = await freshStore();
    for (let i = 0; i < 50; i++) {
      mod.associateSessionWithAvatar(`session-${i}`, "data:image/jpeg;base64,AAAA");
    }
    const raw = JSON.parse(window.localStorage.getItem("tavernagent_session_avatar_map")!);
    expect(Object.keys(raw)).toHaveLength(40);
    expect(raw["session-0"]).toBeUndefined();
    expect(raw["session-49"]).toBe("data:image/jpeg;base64,AAAA");
    expect(mod.getSessionAvatar("session-49")).toBe("data:image/jpeg;base64,AAAA");
  });
});

describe("cleanShortRole", () => {
  it("跳过属性行，不把「[」或 JSON 片段当成身份标语", async () => {
    const mod = await freshStore();
    const gael = '[\n{"Name": ("Gael")}\n{"Age": ("21")}\n付钱交友的尴尬男孩。';
    expect(mod.cleanShortRole(gael)).toBe("付钱交友的尴尬男孩。");
    expect(mod.cleanShortRole("")).toBe("导入的角色卡");
    expect(mod.cleanShortRole("[\n{\"Name\": (\"Gael\")}\n]")).not.toContain("{");
  });

  it("保留正常首行并截断超长文本", async () => {
    const mod = await freshStore();
    expect(mod.cleanShortRole("酒馆老板娘，脾气不好但心软。")).toBe("酒馆老板娘，脾气不好但心软。");
    expect(mod.cleanShortRole("一".repeat(80)).endsWith("…")).toBe(true);
  });
});

describe("本地库读取卡片 JSON 的线上命名", () => {
  it("不再把 character_version / mes_example 之类的字段读丢", async () => {
    const mod = await freshStore();
    // 形状取自后端 mapSpecData + normalizeCard 的产物（characters[] 是 domain.CharacterInfo）
    const cardJson = JSON.stringify({
      schemaVersion: 2,
      cardId: "native_abc",
      name: "契约角色",
      first_mes: "开场",
      characters: [{
        characterId: "npc_wire",
        name: "契约角色",
        description: "公开人设",
        participant: true,
        mes_example: "示范对白",
        system_prompt: "系统规则",
        creator_notes: "创作者留言",
        character_version: "1.2",
      }],
    });
    const saved = mod.saveImportedCard({
      name: "契约角色",
      characterJson: cardJson,
      firstMes: "开场",
      // 预览响应里的字段（camelCase DTO）
      characterVersion: "1.2",
      mesExample: "示范对白",
      systemPrompt: "系统规则",
      creatorNotes: "创作者留言",
    });

    expect(saved.card.mesExample).toBe("示范对白");
    expect(saved.card.systemPrompt).toBe("系统规则");
    expect(saved.card.creatorNotes).toBe("创作者留言");
    expect(saved.card.characterVersion).toBe("1.2");
    expect(saved.card.firstMes).toBe("开场");
  });

  it("预览未提供时从卡片 JSON 的线上字段兜底", async () => {
    const mod = await freshStore();
    const saved = mod.saveImportedCard({
      name: "兜底卡",
      characterJson: JSON.stringify({
        schemaVersion: 2,
        name: "兜底卡",
        characters: [{
          characterId: "npc_fallback",
          name: "兜底卡",
          description: "人设",
          participant: true,
          mes_example: "卡片里的对白",
          character_version: "9.9",
        }],
      }),
    });
    expect(saved.card.mesExample).toBe("卡片里的对白");
    expect(saved.card.characterVersion).toBe("9.9");
  });
});
