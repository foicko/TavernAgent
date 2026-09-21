import { test, expect, type Page } from "@playwright/test";
import { readFileSync } from "node:fs";
const fixture = JSON.parse(readFileSync(new URL("../../scripts/fixtures/quality_story_card.json", import.meta.url), "utf8"));

async function seed(page: Page, title: string) {
  const created = await page.request.post("/api/v1/sessions", { data: {
    title, characterJson: JSON.stringify({ ...fixture, cardId: `modelcfg_${title}_${Date.now()}` }),
    playerName: "测试旅人", openingVariantId: "harbor",
  } });
  expect(created.status()).toBe(201);
  return await created.json();
}

// 离线可跑：连接指向一个不存在的本地端口，只用于验证"配置链路"，不产生真实调用。
async function createConnection(page: Page, name: string, model: string) {
  const res = await page.request.post("/api/v1/config/models", { data: {
    name, kind: "openai-chat", baseUrl: "http://127.0.0.1:9/v1", model,
    contextWindow: 32768, maxTokens: 2048, temperature: 0.7,
  } });
  expect(res.status()).toBe(200);
  return await res.json() as { id: string; name: string };
}

async function primaryModelId(page: Page): Promise<string> {
  const body = await (await page.request.get("/api/v1/config/provider")).json();
  const primary = (body.providers ?? []).find((p: { slot: string }) => p.slot === "primary");
  return primary?.modelId ?? "";
}

async function connectionIds(page: Page): Promise<string[]> {
  const body = await (await page.request.get("/api/v1/config/models")).json();
  return (body.models ?? []).map((m: { id: string }) => m.id);
}

async function resetConfig(page: Page) {
  await page.request.put("/api/v1/config/provider", { data: { slot: "primary", enabled: false, modelId: "" } });
  for (const id of await connectionIds(page)) {
    const res = await page.request.delete(`/api/v1/config/models/${id}`);
    expect(res.status()).toBe(200);
  }
}

test.describe("模型配置：角色优先", () => {
  test.beforeEach(async ({ page }) => {
    await resetConfig(page);
  });

  test.afterEach(async ({ page }) => {
    await resetConfig(page);
  });

  test("连接只配一次、角色只选连接、输入台一键切主线", async ({ page }) => {
    await seed(page, "模型配置验收");
    await page.goto("/");
    await expect(page.locator(".composer-textarea")).toBeEnabled();

    // 1) 建两条连接，主线指向第一条。
    const deepseek = await createConnection(page, "验收 · DeepSeek", "deepseek-chat");
    const local = await createConnection(page, "验收 · 本地", "qwen2.5:14b");
    const assign = await page.request.put("/api/v1/config/provider", { data: { slot: "primary", enabled: true, modelId: deepseek.id } });
    expect(assign.status()).toBe(200);

    // 2) 输入台的模型控件显示当前连接，切换后按钮与服务端同步变化。
    await page.reload();
    const switcher = page.getByLabel("切换主线模型");
    await expect(switcher).toContainText("验收 · DeepSeek");
    await switcher.click();
    await page.getByRole("menuitemradio", { name: /验收 · 本地/ }).click();
    await expect(switcher).toContainText("验收 · 本地");
    await expect.poll(() => primaryModelId(page)).toBe(local.id);

    // 3) 切换只影响新回合：菜单里有明确说明。
    await switcher.click();
    await expect(page.getByText("切换对新回合生效，正在生成的这一段不变。")).toBeVisible();
    await page.keyboard.press("Escape");

    // 4) 上下文控件是仪表（百分比），不再冒充第二个模型选择器。
    const meter = page.getByLabel("实时上下文统计");
    await expect(meter).toContainText("%");
    await expect(meter).not.toContainText("验收 · 本地");

    // 5) 设置页：三个角色 + 连接清单；角色选择显示解析后的连接与窗口。
    await page.locator("#settings-btn").click();
    const dialog = page.getByRole("dialog", { name: "模型设置" });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole("heading", { name: "角色" })).toBeVisible();
    await expect(dialog.getByLabel("对话生成")).toHaveValue(local.id);
    await expect(dialog.getByText(/验收 · 本地 · 32k 窗口/)).toBeVisible();

    // 6) 被角色使用的连接不能在界面上删除，并就地说明原因。
    await dialog.getByRole("button", { name: "编辑连接 验收 · 本地" }).click();
    await expect(dialog.getByRole("button", { name: "删除连接" })).toBeDisabled();
    await expect(dialog.getByText(/正在被 对话生成 使用/)).toBeVisible();

    // 7) 用服务商预设新建一条连接：预填协议与地址，只需补模型名与密钥。
    await dialog.getByRole("button", { name: "返回列表" }).click();
    await dialog.getByRole("button", { name: "＋ 新建连接" }).click();
    await dialog.getByRole("button", { name: "本地 Ollama" }).click();
    await expect(dialog.getByLabel("接口地址")).toHaveValue("http://127.0.0.1:11434/v1");
    await dialog.getByLabel("模型名称").fill("qwen3:8b");
    await dialog.getByLabel("API Key").fill("sk-e2e-secret");
    await dialog.getByRole("button", { name: "保存连接" }).click();
    await expect(dialog.getByText(/已保存「本地 Ollama」/)).toBeVisible();

    // 8) 还原：清空配置，避免影响后续用例（服务端在整轮之间共享）。
    await dialog.getByRole("button", { name: "返回列表" }).click();
    await page.keyboard.press("Escape");
    await resetConfig(page);
  });

  test("生成参数常显：窗口/输出/温度/思考强度，且密钥永不回显明文", async ({ page }) => {
    await seed(page, "模型配置脱敏");
    const created = await createConnection(page, "验收 · 脱敏", "deepseek-chat");
    await page.request.put("/api/v1/config/provider", { data: { slot: "primary", enabled: true, modelId: created.id } });
    await page.goto("/");
    await page.locator("#settings-btn").click();

    const dialog = page.getByRole("dialog", { name: "模型设置" });
    await dialog.getByRole("button", { name: "编辑连接 验收 · 脱敏" }).click();

    // 服务端没存密钥时给出中性提示，而不是渲染一段长字符串。
    await expect(dialog.getByText("还没有保存密钥")).toBeVisible();
    await expect(dialog.getByLabel("API Key")).toHaveAttribute("type", "password");

    // 生成参数不再折叠：窗口/输出/温度/思考强度一次看全，并显示派生出的输入预算。
    await expect(dialog.getByLabel("上下文窗口")).toHaveValue("32768");
    await expect(dialog.getByLabel("最大输出")).toHaveValue("2048");
    await expect(dialog.getByLabel("温度")).toHaveValue("0.7");
    await expect(dialog.getByLabel("思考强度")).toHaveValue("");
    // 32768 − 2048 − max(512, 32768/20)=1638 → 29082 → 约 28k。
    await expect(dialog.getByText(/输入预算约 28k tokens/)).toBeVisible();

    await page.keyboard.press("Escape");
    await resetConfig(page);
  });

  test("输入台右下角调思考强度：立刻生效、服务端落盘、设置页回显", async ({ page }) => {
    await seed(page, "思考强度验收");
    const created = await createConnection(page, "验收 · 思考", "deepseek-chat");
    await page.request.put("/api/v1/config/provider", { data: { slot: "primary", enabled: true, modelId: created.id } });
    await page.goto("/");
    await expect(page.locator(".composer-textarea")).toBeEnabled();

    // 1) 默认档：不发任何思考参数。
    const effort = page.getByLabel(/调整思考强度/);
    await expect(effort).toContainText("思考 默认");

    // 2) 选"高" → 按钮立刻更新。
    await effort.click();
    await page.getByRole("menuitemradio", { name: /^高/ }).click();
    await expect(effort).toContainText("思考 高");

    // 3) 落在这条连接上（不是只改了前端内存）。
    await expect
      .poll(async () => {
        const body = await (await page.request.get("/api/v1/config/models")).json();
        const model = (body.models ?? []).find((m: { id: string }) => m.id === created.id);
        return model?.reasoningEffort ?? "";
      })
      .toBe("high");

    // 4) 设置页看到同一个值。
    await page.locator("#settings-btn").click();
    const dialog = page.getByRole("dialog", { name: "模型设置" });
    await dialog.getByRole("button", { name: "编辑连接 验收 · 思考" }).click();
    await expect(dialog.getByLabel("思考强度")).toHaveValue("high");
    await page.keyboard.press("Escape");

    // 5) 改回默认再过一遍（共享服务端，不留状态给后续用例）。
    await effort.click();
    await page.getByRole("menuitemradio", { name: /^默认/ }).click();
    await expect(effort).toContainText("思考 默认");
    await resetConfig(page);
  });
});
