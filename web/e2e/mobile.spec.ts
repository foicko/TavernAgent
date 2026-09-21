// 手机视口验收：三栏改为贴底抽屉、触控目标不小于 44px、不出现横向溢出。
// 显式设置视口而不依赖 project 配置，这样三个宽度区间（手机 / 平板 / 桌面）在同一条
// 用例里都能被断言到，481~768px 那段也不会因为项目只有 390 与 1440 而失去覆盖。
import { test, expect, type Page } from "@playwright/test";

async function ready(page: Page, width: number, height: number) {
  await page.setViewportSize({ width, height });
  await page.goto("/");
  await expect(page.locator(".composer-textarea")).toBeEnabled();
}

/** 打开左栏（窄视口下默认是收起的抽屉）。 */
async function openLeftRail(page: Page) {
  await page.getByLabel("剧本与角色目录").click();
  await expect(page.locator(".rail-left")).toBeInViewport();
}

test("390px：左右栏是从底部升起的贴底抽屉", async ({ page }) => {
  await ready(page, 390, 844);
  const viewport = page.viewportSize()!;

  await openLeftRail(page);
  const box = await page.locator(".rail-left").boundingBox();
  expect(box).not.toBeNull();
  // 贴底全宽、且不高于 75% 视口（留出正文可见高度）
  expect(box!.width).toBeGreaterThan(viewport.width * 0.98);
  expect(viewport.height - (box!.y + box!.height)).toBeLessThan(3);
  expect(box!.height).toBeLessThanOrEqual(viewport.height * 0.76);

  // 点遮罩关闭，再打开右栏确认同一套行为
  await page.locator(".mobile-drawer-backdrop").click({ position: { x: 10, y: 10 } });
  await expect(page.locator(".rail-left")).not.toBeInViewport();

  await page.getByLabel("角色立绘与状态").click();
  const right = await page.locator(".rail-right").boundingBox();
  expect(right).not.toBeNull();
  expect(viewport.height - (right!.y + right!.height)).toBeLessThan(3);
});

test("390px：触控目标不小于 44px，且不出现横向溢出", async ({ page }) => {
  await ready(page, 390, 844);

  for (const label of ["剧本与角色目录", "角色立绘与状态"]) {
    const box = await page.getByLabel(label).boundingBox();
    expect(box, `${label} 应有可见的命中区`).not.toBeNull();
    expect(box!.width, `${label} 宽度`).toBeGreaterThanOrEqual(44);
    expect(box!.height, `${label} 高度`).toBeGreaterThanOrEqual(44);
  }

  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
  expect(overflow, "手机端不应出现横向滚动").toBeLessThanOrEqual(1);
});

test("700px（平板区间）仍是侧滑抽屉，不受手机贴底规则影响", async ({ page }) => {
  await ready(page, 700, 900);
  const viewport = page.viewportSize()!;

  await openLeftRail(page);
  const box = await page.locator(".rail-left").boundingBox();
  expect(box).not.toBeNull();
  expect(box!.x, "贴左沿").toBeLessThan(3);
  expect(box!.height, "占满高度").toBeGreaterThan(viewport.height * 0.9);
});

test("1200px（桌面）三栏并排，抽屉规则完全不生效", async ({ page }) => {
  await ready(page, 1200, 900);
  const left = await page.locator(".rail-left").boundingBox();
  const right = await page.locator(".rail-right").boundingBox();
  expect(left).not.toBeNull();
  expect(right).not.toBeNull();
  // 左右栏各自贴住视口两侧，中间留给主舞台
  expect(left!.x).toBeLessThan(3);
  expect(right!.x + right!.width).toBeGreaterThan(page.viewportSize()!.width - 3);
});
