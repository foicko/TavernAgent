import { test, expect, type Page } from "@playwright/test";
import { readFileSync } from "node:fs";
const fixture = JSON.parse(readFileSync(new URL("../../scripts/fixtures/quality_story_card.json", import.meta.url), "utf8"));

async function seed(page: Page, title: string) {
  const created = await page.request.post("/api/v1/sessions", { data: {
    title, characterJson: JSON.stringify({ ...fixture, cardId: `recovery_${title}_${Date.now()}` }),
    playerName: "测试旅人", openingVariantId: "harbor",
  } });
  expect(created.status()).toBe(201);
  return await created.json();
}

async function openDirectory(page: Page, info: { project: { name: string } }) {
  if (info.project.name === "narrow") await page.getByLabel("剧本与角色目录").click();
}

async function closeDrawer(page: Page, info: { project: { name: string } }) {
  // 窄屏下抽屉会盖住正文区：操作正文里的控件前必须先收起。
  if (info.project.name === "narrow") await page.getByTitle("收起目录").click();
}

async function openStory(page: Page, info: { project: { name: string } }, title: string) {
  await openDirectory(page, info);
  await page.locator("#subtab-btn-sessions").click();
  await page.locator(".session-entry-card", { hasText: title }).first().click();
  await expect(page.locator(".breadcrumb-title")).toHaveText(title);
}

// 刷新页面必须回到读者上次打开的故事：列表第一条往往是最新创建/更新的另一个故事，
// 早期实现会用它兜底，读者正在写的故事因此"消失"。
test("刷新后回到读者上次打开的故事，而不是列表第一条", async ({ page }, info) => {
  await seed(page, "甲故事");
  await seed(page, "乙故事");
  await page.goto("/");
  await openStory(page, info, "甲故事");
  // 打开甲之后新建丙：丙成为列表第一条，刷新时不得抢走视角。
  await seed(page, "丙故事");
  await page.reload();
  await expect(page.locator(".breadcrumb-title")).toHaveText("甲故事");
  await expect(page.locator(".composer-textarea")).toBeEnabled();
});

// 历史导航失败必须回滚落点并给出可见错误：旧实现会把 viewNodeId 留在
// "不在当前视图里的节点"上，输入台被当成只读历史锁死，而异常被 void 吞掉。
test("历史导航失败时回滚位置并保持输入台可用", async ({ page }, info) => {
  await seed(page, "回滚故事");
  await page.goto("/");
  await expect(page.locator(".breadcrumb-title")).toHaveText("回滚故事");
  const input = page.locator(".composer-textarea");
  await input.fill("我查看海港地图，询问灯塔的方向。");
  await input.press("Enter");
  await expect(page.locator(".story-turn")).toHaveCount(2);
  await expect(input).toBeEnabled();

  // 从分支树进入历史快照（只读）。
  await openDirectory(page, info);
  await page.locator("#subtab-btn-tree").click();
  await page.locator(".branch-timeline-tree .node-turn-tag").first().click();
  await page.getByTitle("切换主窗口视角至该历史快照").first().click();
  await closeDrawer(page, info);
  await expect(page.getByText("正在浏览历史快照节点", { exact: false })).toBeVisible();
  await expect(input).toBeDisabled();

  // 让随后的视图读取失败一次。
  let failed = false;
  await page.route("**/api/v1/sessions/**", async route => {
    const url = route.request().url();
    const isViewRead = route.request().method() === "GET" && !url.includes("/events") && !url.includes("/memories") && !url.includes("/graph") && !url.includes("/lorebook");
    if (!failed && isViewRead) {
      failed = true;
      return route.fulfill({ status: 500, contentType: "application/json",
        body: JSON.stringify({ code: "STORAGE_UNAVAILABLE", message: "temporary failure", retryable: true }) });
    }
    return route.continue();
  });

  await page.getByRole("button", { name: "返回最新进度" }).click();
  await expect(page.getByText("已还原到原位置", { exact: false })).toBeVisible();
  // 落点没有被切走：仍在同一个历史快照里，界面保持一致（而不是半切换状态）。
  await expect(page.getByText("正在浏览历史快照节点", { exact: false })).toBeVisible();
  await expect(input).toBeDisabled();

  // 故障恢复后重试成功：回到最新进度，输入台重新可用。
  await page.unroute("**/api/v1/sessions/**");
  await page.getByRole("button", { name: "返回最新进度" }).click();
  await expect(page.getByText("正在浏览历史快照节点", { exact: false })).toHaveCount(0);
  await expect(input).toBeEnabled();
});

// 生成失败时，错误卡要让长错误文本自己换行，而不是把重试按钮挤扁。
// 旧实现的按钮是 flex 子项、默认可收缩，且没有 white-space: nowrap：供应商原文一长，
// 它就被压到只剩两个字宽而竖排（"重"/"试" 分两行）。CSS 四项门禁查不出这类布局回归。
test("生成失败时重试按钮不被长错误文本挤扁", async ({ page }) => {
  await seed(page, "错误卡故事");
  await page.goto("/");
  await expect(page.locator(".breadcrumb-title")).toHaveText("错误卡故事");

  await page.route("**/api/v1/sessions/**/turns", route => route.fulfill({
    status: 503,
    contentType: "application/json",
    body: JSON.stringify({
      code: "PROVIDER_UNAVAILABLE",
      retryable: true,
      message: "供应商返回 503 (overloaded_error): No available accounts: Token acquisition timeout (5s) - system too busy or deadlock detected",
    }),
  }));

  const input = page.locator(".composer-textarea");
  await input.fill("我查看海港地图，询问灯塔的方向。");
  await input.press("Enter");

  const retry = page.locator(".error-turn__retry");
  await expect(retry).toBeVisible();
  // 竖排时高度会翻倍：用"内容不溢出自身高度"钉住单行。
  expect(await retry.evaluate(el => {
    const node = el as HTMLElement;
    return node.scrollHeight <= node.clientHeight + 1;
  })).toBe(true);
  const box = await retry.boundingBox();
  expect(box?.width ?? 0).toBeGreaterThan(30);
});

// 刷新后必须重放已提交的正文与选项：断线期间的完整块靠 durable 事件回放，
// 正在写的那一块靠 GET /turns/{id} 的 draft 快照（服务端与 store 各有单测钉住）。
test("刷新后正文与选项完整重放", async ({ page }, info) => {
  await seed(page, "重放故事");
  await page.goto("/");
  await expect(page.locator(".breadcrumb-title")).toHaveText("重放故事");
  const input = page.locator(".composer-textarea");
  await input.fill("我把怀表放回口袋，等她开口。");
  await input.press("Enter");
  await expect(page.locator(".story-turn")).toHaveCount(2);
  await expect(input).toBeEnabled();
  const narration = await page.locator(".story-turn").last().innerText();

  await page.reload();
  await expect(page.locator(".breadcrumb-title")).toHaveText("重放故事");
  await expect(page.locator(".story-turn")).toHaveCount(2);
  await expect(page.locator(".story-turn").last()).toContainText(narration.slice(0, 12));
  await expect(input).toBeEnabled();
  await expect(page.locator(".active-streaming-turn")).toHaveCount(0);
});
