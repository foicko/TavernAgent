// 删除功能验收：故事删除（不可撤销、两步确认）与角色卡删除（只影响本地卡库）。
import { test, expect, type Page } from "@playwright/test";
import { readFileSync } from "node:fs";
const fixture = JSON.parse(readFileSync(new URL("../../scripts/fixtures/quality_story_card.json", import.meta.url), "utf8"));

async function seed(page: Page, title: string, cardId: string) {
  const created = await page.request.post("/api/v1/sessions", { data: {
    title, characterJson: JSON.stringify({ ...fixture, cardId }),
    playerName: "测试旅人", openingVariantId: "harbor",
  } });
  expect(created.status()).toBe(201);
  return await created.json() as { sessionId: string; branchId: string };
}

// seedCard 把卡片真正放进**服务端**卡库，并返回服务端认可的 cardId。
//
// 这里不再 poke localStorage：卡库已是服务端真相，本地那份只是离线缓存，而且会被
// 服务端列表覆盖——只写 localStorage 的卡片根本不会出现在名册里（旧写法就是这么
// 失效的）。返回服务端给的 ID 而不是回传入参：ID 可能由内容派生，测试不该猜它。
async function seedCard(page: Page, cardId: string): Promise<string> {
  const created = await page.request.post("/api/v1/cards", {
    data: JSON.stringify({ ...fixture, cardId }),
    headers: { "Content-Type": "application/json" },
  });
  expect(created.status()).toBe(200);
  return (await created.json() as { cardId: string }).cardId;
}

async function sessionIds(page: Page): Promise<string[]> {
  const body = await (await page.request.get("/api/v1/sessions")).json();
  return (body.sessions ?? []).map((s: { sessionId: string }) => s.sessionId);
}

// 窄屏时左栏是抽屉：点开故事会把抽屉收起来，所以每次交互前都重新确保这段子页可见。
async function openTab(page: Page, projectName: string, tab: "sessions" | "characters") {
  const id = tab === "sessions" ? "#subtab-btn-sessions" : "#subtab-btn-characters";
  if (projectName === "narrow" && !(await page.locator(id).isVisible())) {
    await page.getByLabel("剧本与角色目录").click();
  }
  await page.locator(id).click();
}

// 标题必须唯一：e2e 的浏览器服务端在所有 project 之间共享同一份数据目录，
// 同名会话会让"这一行"的断言同时命中多行。
const unique = (projectName: string) => `${projectName}-${Date.now()}-${Math.floor(Math.random() * 1000)}`;

test("删除故事：两步确认后服务端与列表同时消失", async ({ page }, info) => {
  const tag = unique(info.project.name);
  const firstTitle = `待删故事 ${tag}`;
  const secondTitle = `保留故事 ${tag}`;
  const first = await seed(page, firstTitle, `del_a_${tag}`);
  const second = await seed(page, secondTitle, `del_b_${tag}`);
  await page.goto("/");
  await expect(page.locator(".composer-textarea")).toBeEnabled();
  await openTab(page, info.project.name, "sessions");

  const findCard = () => page.locator(".session-entry-card", { hasText: firstTitle }).first();
  await expect(findCard()).toBeVisible();

  // 第一次点击只是"问一次"，不能已经删掉。
  await findCard().getByRole("button", { name: `删除故事 ${firstTitle}` }).click();
  await expect(findCard().getByRole("button", { name: "确认删除" })).toBeVisible();
  expect(await sessionIds(page)).toContain(first.sessionId);

  // 可以反悔：取消后回到初始态，故事仍在。
  await findCard().getByRole("button", { name: "取消" }).click();
  await expect(findCard().getByRole("button", { name: `删除故事 ${firstTitle}` })).toBeVisible();
  expect(await sessionIds(page)).toContain(first.sessionId);

  // 确认后：服务端 404、列表里消失、另一条故事不受影响。
  await findCard().getByRole("button", { name: `删除故事 ${firstTitle}` }).click();
  await findCard().getByRole("button", { name: "确认删除" }).click();
  await expect.poll(async () => (await page.request.get(`/api/v1/sessions/${first.sessionId}`)).status()).toBe(404);
  await openTab(page, info.project.name, "sessions");
  await expect(page.locator(".session-entry-card", { hasText: firstTitle })).toHaveCount(0);
  expect(await sessionIds(page)).toContain(second.sessionId);
  await expect(page.locator(".session-entry-card", { hasText: secondTitle })).toHaveCount(1);

  // 收尾：数据目录是共享的，别把播种的会话留下。
  expect((await page.request.delete(`/api/v1/sessions/${second.sessionId}`)).status()).toBe(200);
});

test("删除角色卡：只从卡库移除，已有存档照常打开", async ({ page }, info) => {
  const tag = unique(info.project.name);
  const cardId = await seedCard(page, `relcard_${tag}`);
  const sessionTitle = `卡库里的故事 ${cardId}`;
  const session = await seed(page, sessionTitle, cardId);
  await page.goto("/");
  await expect(page.locator(".composer-textarea")).toBeEnabled();
  await openTab(page, info.project.name, "characters");

  const card = page.locator(`#roster-card-${cardId}`);
  await expect(card).toBeVisible();

  // 两步确认：先问，再确认。
  await card.getByRole("button", { name: /删除角色卡/ }).click();
  await expect(card.getByText(/从卡库移除/)).toBeVisible();
  await card.getByRole("button", { name: "确认移除" }).click();
  await expect(page.locator(`#roster-card-${cardId}`)).toHaveCount(0);

  // 卡没了但故事还在：会话列表与打开都正常（服务端持有角色数据）。
  await openTab(page, info.project.name, "sessions");
  await expect(page.locator(".session-entry-card", { hasText: sessionTitle })).toHaveCount(1);
  await page.locator(".session-entry-card", { hasText: sessionTitle }).first().click();
  await expect(page.locator(".composer-textarea")).toBeEnabled();
  expect((await page.request.get(`/api/v1/sessions/${session.sessionId}`)).status()).toBe(200);

  expect((await page.request.delete(`/api/v1/sessions/${session.sessionId}`)).status()).toBe(200);
});

test("删除故事：服务端拒绝时如实报错，不假装成功", async ({ page }, info) => {
  const title = `已被别处删除的故事 ${unique(info.project.name)}`;
  const gone = await seed(page, title, `ghost_${Date.now()}`);
  await page.goto("/");
  await expect(page.locator(".composer-textarea")).toBeEnabled();
  await openTab(page, info.project.name, "sessions");
  await expect(page.locator(".session-entry-card", { hasText: title })).toHaveCount(1);

  // 绕过界面先删掉：界面上的那一行此时已是幽灵行。
  expect((await page.request.delete(`/api/v1/sessions/${gone.sessionId}`)).status()).toBe(200);

  const card = page.locator(".session-entry-card", { hasText: title }).first();
  await card.getByRole("button", { name: `删除故事 ${title}` }).click();
  await card.getByRole("button", { name: "确认删除" }).click();

  // 报出服务端给的原因，并把幽灵行清掉（列表与服务端对齐）。
  await expect(page.getByText(/会话不存在|删除失败/)).toBeVisible();
  await expect(page.locator(".session-entry-card", { hasText: title })).toHaveCount(0);
});
