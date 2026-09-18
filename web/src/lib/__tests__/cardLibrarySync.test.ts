// @vitest-environment jsdom
// 卡库真相在服务端：本地 localStorage 只是缓存。这里验证"服务端返回值覆盖缓存"
// 与"删除先走服务端"两条边界，以及网络失败时缓存不被清空。
import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ listCards: vi.fn(), deleteCard: vi.fn() }));
vi.mock("../../app/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../../app/api")>();
  return { ...actual, api: { ...actual.api, listCards: mocks.listCards, deleteCard: mocks.deleteCard } };
});

async function freshStore() {
  vi.resetModules();
  return await import("../characterCardStore");
}

const richCard = (cardId: string, name: string) => ({
  cardId,
  name,
  role: "本地富字段",
  description: "完整人设，只有导入时才知道",
  characterJson: JSON.stringify({ schemaVersion: 2, cardId, name }),
});

beforeEach(() => {
  window.localStorage.clear();
  vi.resetAllMocks();
});

describe("refreshImportedCards", () => {
  it("overwrites the cache from the server and keeps locally known rich fields", async () => {
    const store = await freshStore();
    store.saveImportedCard(richCard("c1", "旧名"));
    mocks.listCards.mockResolvedValue({
      cards: [
        { cardId: "c1", name: "新名", shortName: "新", role: "新身份" },
        { cardId: "c2", name: "乙" },
      ],
    });

    await store.refreshImportedCards();

    const cards = store.listImportedCards();
    expect(cards.map((c) => c.cardId)).toEqual(["c1", "c2"]);
    const c1 = cards.find((c) => c.cardId === "c1")!;
    expect(c1.name).toBe("新名");
    expect(c1.role).toBe("新身份");
    // 服务端摘要是权威；描述这类不在摘要里的富字段从缓存保留。
    expect(c1.description).toBe("完整人设，只有导入时才知道");
    // 缓存必须与真正落盘的一致（刷新后不"复活"别的卡）。
    const persisted = JSON.parse(window.localStorage.getItem("tavernagent_imported_character_cards") ?? "[]");
    expect(persisted.map((c: { cardId: string }) => c.cardId)).toEqual(["c1", "c2"]);
  });

  it("keeps the cached library when the server is unreachable", async () => {
    const store = await freshStore();
    store.saveImportedCard(richCard("c1", "离线卡"));
    mocks.listCards.mockRejectedValue(new Error("offline"));

    await expect(store.refreshImportedCards()).rejects.toThrow("offline");
    expect(store.listImportedCards().map((c) => c.cardId)).toEqual(["c1"]);
  });
});

describe("deleteImportedCardRemote", () => {
  it("deletes on the server first, then drops the cache entry", async () => {
    const store = await freshStore();
    store.saveImportedCard(richCard("c1", "甲"));
    store.saveImportedCard(richCard("c2", "乙"));
    mocks.deleteCard.mockResolvedValue({ ok: true });

    await store.deleteImportedCardRemote("c1");

    expect(mocks.deleteCard).toHaveBeenCalledWith("c1");
    expect(store.listImportedCards().map((c) => c.cardId)).toEqual(["c2"]);
  });

  it("keeps the cache untouched when the server delete fails", async () => {
    const store = await freshStore();
    store.saveImportedCard(richCard("c1", "甲"));
    mocks.deleteCard.mockRejectedValue(new Error("boom"));

    await expect(store.deleteImportedCardRemote("c1")).rejects.toThrow("boom");
    expect(store.listImportedCards().map((c) => c.cardId)).toEqual(["c1"]);
  });
});
