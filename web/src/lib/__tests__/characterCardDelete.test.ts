// @vitest-environment jsdom
// 角色卡删除只影响本地卡库：服务端在建立会话时已经把卡写进模板与世界状态，
// 所以删除不能去动任何会话数据，也不能让卡库缓存继续兜着已删的卡。
import { beforeEach, describe, expect, it, vi } from "vitest";

async function freshStore() {
  vi.resetModules();
  return await import("../characterCardStore");
}

const cardOf = (name: string) => ({
  name,
  characterJson: JSON.stringify({ schemaVersion: 2, name, description: "描述" }),
});

beforeEach(() => {
  window.localStorage.clear();
});

describe("deleteImportedCard", () => {
  it("removes only the targeted card and persists the rest", async () => {
    const store = await freshStore();
    store.saveImportedCard(cardOf("甲"));
    const b = store.saveImportedCard(cardOf("乙"));
    store.saveImportedCard(cardOf("丙"));

    const left = store.deleteImportedCard(b.card.cardId);

    expect(left.map((c) => c.name)).toEqual(["丙", "甲"]);
    expect(store.listImportedCards().map((c) => c.name)).toEqual(["丙", "甲"]);
    // 落盘也必须同步：否则整页刷新后已删除的卡会"复活"。
    const raw = JSON.parse(window.localStorage.getItem("tavernagent_imported_character_cards") ?? "[]");
    expect(raw.map((c: { name: string }) => c.name)).toEqual(["丙", "甲"]);
  });

  it("invalidates the in-memory cache so the UI stops rendering it immediately", async () => {
    const store = await freshStore();
    const a = store.saveImportedCard(cardOf("甲"));
    expect(store.listImportedCards()).toHaveLength(1); // 先填充缓存

    store.deleteImportedCard(a.card.cardId);

    expect(store.listImportedCards()).toHaveLength(0);
  });

  it("treats a missing card as already deleted", async () => {
    const store = await freshStore();
    const a = store.saveImportedCard(cardOf("甲"));

    const left = store.deleteImportedCard("card_not_there");

    expect(left.map((c) => c.cardId)).toEqual([a.card.cardId]);
    expect(store.listImportedCards()).toHaveLength(1);
  });

  it("survives an empty library", async () => {
    const store = await freshStore();
    expect(store.deleteImportedCard("card_whatever")).toEqual([]);
  });
});
