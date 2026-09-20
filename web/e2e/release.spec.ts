import { test, expect, type Page } from "@playwright/test";
import { readFileSync } from "node:fs";
const fixture = JSON.parse(readFileSync(new URL("../../scripts/fixtures/quality_story_card.json", import.meta.url), "utf8"));

async function seed(page: Page, title: string) {
  const created = await page.request.post("/api/v1/sessions", { data: {
    title, characterJson: JSON.stringify({ ...fixture, cardId: `browser_${title}_${Date.now()}` }),
    playerName: "测试旅人", openingVariantId: "harbor",
  } });
  expect(created.status()).toBe(201);
  const result = await created.json() as { sessionId: string; rootNodeId: string };
  await page.addInitScript((id) => {
    try { window.localStorage.setItem("tavernagent.lastSessionId", id); } catch {}
  }, result.sessionId);
  await page.goto("/");
  await expect(page.locator(".breadcrumb-title")).toHaveText(title);
  return result;
}

// 离线可跑：指向一个不存在的本地端口，只验证配置界面，不产生真实调用。
async function createConnection(page: Page, name: string, model: string) {
  const res = await page.request.post("/api/v1/config/models", { data: {
    name, kind: "openai-chat", baseUrl: "http://127.0.0.1:9/v1", model, contextWindow: 32768, maxTokens: 2048,
  } });
  expect(res.status()).toBe(200);
  return await res.json() as { id: string; name: string };
}

test("character import, identity, opening and keyboard story operation", async ({ page }, info) => {
  await page.goto("/");
  // The directory drawer is explicitly accessible on narrow screens.
  if (info.project.name === "narrow") await page.getByLabel("剧本与角色目录").click();
  await page.locator("#subtab-btn-characters").click();
  await page.getByRole("button", { name: "+ 导入 / 创建角色卡", exact: true }).click();
  await page.locator("#char-card-file-input").setInputFiles({ name: "original.json", mimeType: "application/json", buffer: Buffer.from(JSON.stringify({ ...fixture, cardId: `browser_import_${Date.now()}` })) });
  await page.getByLabel("玩家姓名", { exact: true }).fill("林舟");
  await page.getByRole("combobox", { name: "选择开场", exact: true }).selectOption("harbor");
  await page.getByRole("button", { name: "开启冒险", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  const input = page.locator(".composer-textarea");
  await input.fill("我查看海港地图，询问灯塔的方向。");
  await input.press("Enter");
  await expect(page.locator(".player-speech-text").last()).toContainText("我查看海港地图");
  await expect(page.locator(".active-streaming-turn")).toHaveCount(0);
  await expect(input).toBeEnabled();
  await expect(page.locator(".story-turn")).toHaveCount(2);
  await page.screenshot({ path: `../output/playwright/${info.project.name}-story.png`, fullPage: true });
});

test("settings mask stored credentials and support keyboard dismissal", async ({ page, request }, info) => {
  // 快捷预设会把供应商指派到 primary 槽位并落库，而服务端在整轮用例间共享：
  // 不还原的话，后续用例会绕过 mock 直接打真实模型（表现为 401 生成受阻）。
  const snapshot = await (await request.get("/api/v1/config/provider")).json();
  const enabledSlots = (state: { providers?: Array<{ slot: string; enabled: boolean }> }) =>
    (state.providers ?? []).filter((provider) => provider.enabled).map((provider) => provider.slot).sort().join(",");
  const restoreConfig = async () => {
    const target = (snapshot.providers ?? []) as Array<{ slot: string; enabled: boolean; modelId?: string }>;
    if (target.length === 0) {
      // 原本没有落库配置：显式关闭 primary，让 -provider mock 的兜底重新生效。
      await request.put("/api/v1/config/provider", { data: { slot: "primary", enabled: false, modelId: "" } });
    } else {
      // 槽位只接受 enabled + modelId；连接信息属于模型实例。
      for (const provider of target) {
        await request.put("/api/v1/config/provider", { data: { slot: provider.slot, enabled: provider.enabled, modelId: provider.modelId ?? "" } });
      }
    }
    return enabledSlots(await (await request.get("/api/v1/config/provider")).json());
  };
  try {
    await seed(page, "配置验收");
    const connection = await createConnection(page, "配置验收 · 掩码", "deepseek-chat");
    await page.locator("#settings-btn").click();
    const dialog = page.getByRole("dialog", { name: "模型设置" });
    await expect(dialog).toBeFocused();
    await page.keyboard.press("Shift+Tab");
    expect(await dialog.evaluate(element => element.contains(document.activeElement))).toBe(true);
    await page.keyboard.press("Tab");
    expect(await dialog.evaluate(element => element.contains(document.activeElement))).toBe(true);
    await expect(dialog).toBeVisible();
    // 打开连接表单：模型名来自已存配置，密钥只以 password 输入框存在（不回显明文）。
    await dialog.getByRole("button", { name: "编辑连接 配置验收 · 掩码" }).click();
    await expect(page.getByLabel("模型名称", { exact: true })).not.toHaveValue("");
    await expect(page.getByLabel("API Key", { exact: true })).toHaveAttribute("type", "password");
    await page.keyboard.press("Escape");
    await expect(page.locator("#settings-modal")).toHaveCount(0);
    await expect(page.locator("#settings-btn")).toBeFocused();
    const stored = await page.evaluate(() => Object.entries(localStorage));
    expect(stored.every(([key, value]) => key !== "tavernagent_configured_models" && !/"apiKey"\s*:/.test(value))).toBe(true);
    await page.locator("#theme-btn").click();
    await expect.poll(() => page.locator(".opening-turn .dialogue-turn-wrap").evaluate(element => {
      const luminance = (rgb: string) => {
        const channels = (rgb.match(/[\d.]+/g) ?? []).slice(0, 3).map(Number).map(value => {
          const channel = value / 255;
          return channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4;
        });
        return channels[0] * 0.2126 + channels[1] * 0.7152 + channels[2] * 0.0722;
      };
      const foreground = luminance(getComputedStyle(element.querySelector(".dialogue-content-text")!).color);
      const background = luminance(getComputedStyle(element).backgroundColor);
      return (Math.max(foreground, background) + 0.05) / (Math.min(foreground, background) + 0.05);
    })).toBeGreaterThanOrEqual(4.5);
    await page.screenshot({ path: `../output/playwright/${info.project.name}-theme.png`, fullPage: true, animations: "disabled" });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    const cleanup = await page.request.delete(`/api/v1/config/models/${connection.id}`);
    expect(cleanup.status()).toBe(200);
  } finally {
    // 快捷预设是异步落库的：若弹窗期间发出的写入还在路上，单次还原会被它覆盖，
    // 因此轮询到配置确实回到原样为止。
    await expect.poll(restoreConfig).toBe(enabledSlots(snapshot));
  }
});

test("Chinese composition never submits on IME confirmation; network retry preserves input", async ({ page }) => {
  await seed(page, "中文与重试验收");
  const input = page.locator(".composer-textarea");
  await input.fill("正在组合中文");
  let requests = 0;
  page.on("request", request => { if (request.method() === "POST" && /\/turns$/.test(request.url())) requests++; });
  await input.dispatchEvent("compositionstart");
  await input.dispatchEvent("keydown", { key: "Enter", code: "Enter", isComposing: true, keyCode: 229 });
  await input.dispatchEvent("compositionend");
  await expect(input).toHaveValue("正在组合中文");
  expect(requests).toBe(0);
  let failures = 0;
  await page.route("**/api/v1/sessions/*/branches/*/turns", async route => {
    if (failures++ === 0) await route.abort("connectionfailed");
    else await route.continue();
  });
  await input.fill("断线后只提交一次");
  await input.press("Enter");
  await expect(page.getByText(/生成受阻/)).toBeVisible();
  await page.getByRole("button", { name: /重试/ }).last().click();
  await expect(page.getByText(/生成受阻/)).toHaveCount(0);
  await expect(page.locator(".player-speech-text").last()).toContainText("断线后只提交一次");
  await expect(page.locator(".active-streaming-turn")).toHaveCount(0);
  await expect(input).toBeEnabled();
  expect(requests).toBe(2);
});

test("LAN boundary contracts and invalid inputs", async ({ request }) => {
  const denied = await request.post("/api/v1/sessions", { headers: { Origin: "https://untrusted.example" }, data: {} });
  expect(denied.status()).toBe(403);
  const malformed = await request.post("/api/v1/cards/import", { data: "invalid card", headers: { "Content-Type": "application/json" } });
  expect(malformed.status()).toBe(422);
  const wrong = await request.post("/api/v1/auth/pair", { data: { pin: "000000" } });
  expect(wrong.status()).toBe(401);
  const paired = await request.post("/api/v1/auth/pair", { data: { pin: "482619" } });
  expect(paired.status()).toBe(200);
  expect((await paired.json()).token).toBeTruthy();
});

test("story graph opens immutable history and the memory graph restores focus", async ({ page }, info) => {
  const seeded = await seed(page, "图谱与历史验收");
  const input = page.locator(".composer-textarea");
  for (const text of ["我查看灯塔。", "我在港口停下脚步。"]) {
    await input.fill(text);
    await input.press("Enter");
    await expect(page.locator(".player-speech-text").last()).toHaveText(text);
    await expect(page.locator(".active-streaming-turn")).toHaveCount(0);
  }
  // 回合是异步受理（202）的：对白气泡是乐观渲染，出现得比落库更早。
  // 若此时就打开图谱，节点还没长出来，断言会随机失败——先以服务端图谱为准。
  await expect.poll(async () => {
    const res = await page.request.get(`/api/v1/sessions/${seeded.sessionId}/graph?nodeId=${seeded.rootNodeId}`);
    if (!res.ok()) return -1;
    return ((await res.json()).nodes ?? []).length;
  }).toBe(3);
  if (info.project.name === "narrow") await page.getByLabel("剧本与角色目录").click();
  await page.locator("#subtab-btn-tree").click();
  await page.getByRole("button", { name: "打开剧情分支图" }).click();
  const graph = page.getByRole("dialog", { name: "剧情分支图", exact: true });
  await expect(graph.locator(".story-map-node")).toHaveCount(3);
  await graph.getByRole("button", { name: /故事起点/ }).click();
  await graph.getByRole("button", { name: "查看历史快照" }).click();
  await expect(graph).toHaveCount(0);
  if (info.project.name === "narrow") await page.getByTitle("收起目录", { exact: true }).click();
  await expect(page.locator(".history-read-banner")).toBeVisible();
  await expect(input).toBeDisabled();
  await page.getByRole("button", { name: "返回最新进度" }).click();
  await expect(page.locator(".history-read-banner")).toHaveCount(0);
  await expect(input).toBeEnabled();
  if (info.project.name === "narrow") await page.getByLabel("角色立绘与状态").click();
  // 星图入口现在位于**认知记忆库弹窗**里（不再挂在右栏）：必须先把记忆库打开。
  // 弹窗会先于星图关掉（触发按钮随之卸载），所以关闭星图后焦点回到记忆库的入口磁贴。
  const memoryTile = page.getByTitle("点击查看并修订亲历观察、推测记忆与心智星图");
  await memoryTile.click();
  await expect(page.getByRole("dialog", { name: "认知记忆库" })).toBeVisible();
  await page.getByTitle("全景心智星图预览").click();
  await expect(page.getByRole("dialog", { name: "心智与实体星图" })).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog", { name: "心智与实体星图" })).toHaveCount(0);
  await expect(memoryTile).toBeFocused();
});

test("metered Gemini story package renders long history and retained facts", async ({ page }, info) => {
  const archive = process.env.TAVERNAGENT_REAL_STORY_PACK;
  test.skip(!archive, "Set TAVERNAGENT_REAL_STORY_PACK to an exported, metered Gemini story for release evidence");
  const imported = await page.request.post("/api/v1/sessions/import", {
    data: readFileSync(archive!), headers: { "Content-Type": "application/zip" },
  });
  expect(imported.status()).toBe(201);
  const session = await imported.json();
  await page.goto("/");
  const view = await page.request.get(`/api/v1/sessions/${session.sessionId}`);
  const data = await view.json();
  await expect(page.locator(".breadcrumb-title")).toHaveText(data.title);
  await expect.poll(() => page.locator(".story-turn").count()).toBeGreaterThanOrEqual(41);
  const latest = page.locator(".story-turn").last();
  await expect(latest).toContainText("北码头");
  await expect(latest).toContainText("蓝色");
  await expect(page.locator(".composer-textarea")).toBeEnabled();
  await latest.scrollIntoViewIfNeeded();
  await page.screenshot({ path: `../output/playwright/${info.project.name}-gemini-story.png`, fullPage: true, animations: "disabled" });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
});
