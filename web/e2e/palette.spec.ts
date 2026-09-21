// 命令面板验收：Ctrl/Cmd+K 唤起、搜索并执行、Esc 只关自己、窄屏贴底。
import { test, expect, type Page } from "@playwright/test";

const palette = (page: Page) => page.locator(".ui-command-palette");

async function ready(page: Page) {
  await page.goto("/");
  await expect(page.locator(".composer-textarea")).toBeEnabled();
}

test("Ctrl+K 唤起面板，搜索并执行命令", async ({ page }) => {
  await ready(page);

  await page.keyboard.press("Control+k");
  await expect(palette(page)).toBeVisible();
  // 打开即聚焦搜索框，可以直接敲字。
  await page.keyboard.type("墨黑");
  const themeCommand = page.getByTestId("command-view:theme");
  await expect(themeCommand).toBeVisible();
  // 过滤后只剩命中项：别的命令不该还留在列表里。
  await expect(page.getByTestId("command-panel:settings")).toHaveCount(0);

  await page.keyboard.press("Enter");
  await expect(palette(page)).toHaveCount(0);
  await expect
    .poll(() => page.evaluate(() => document.documentElement.getAttribute("data-theme")))
    .toBe("dark");
});

test("Esc 只关面板：下层的世界书弹窗不受影响", async ({ page }) => {
  await ready(page);

  // 先用命令面板打开世界书：顺带验证"执行后自动关闭"。
  await page.keyboard.press("Control+k");
  await page.getByTestId("command-panel:worldbook").click();
  await expect(page.locator("#worldbook-modal")).toBeVisible();
  await expect(palette(page)).toHaveCount(0);

  // 再开面板，Esc 只能关掉最上层这一个。
  await page.keyboard.press("Control+k");
  await expect(palette(page)).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(palette(page)).toHaveCount(0);
  await expect(page.locator("#worldbook-modal")).toBeVisible();
});

test("顶栏入口也能唤起面板", async ({ page }) => {
  await ready(page);
  await page.locator("#command-palette-btn").click();
  await expect(palette(page)).toBeVisible();
  // 点击遮罩关闭
  await page.mouse.click(8, 8);
  await expect(palette(page)).toHaveCount(0);
});

test("窄屏下面板贴底全宽", async ({ page }, info) => {
  test.skip(info.project.name !== "narrow", "只在窄屏 project 断言贴底布局");
  await ready(page);
  await page.keyboard.press("Control+k");
  await expect(palette(page)).toBeVisible();

  const viewport = page.viewportSize();
  const box = await palette(page).boundingBox();
  expect(viewport).not.toBeNull();
  expect(box).not.toBeNull();
  expect(box!.width).toBeGreaterThan(viewport!.width * 0.95);
  // 贴着下沿：面板底边与视口底部基本重合。
  expect(viewport!.height - (box!.y + box!.height)).toBeLessThan(8);
});
