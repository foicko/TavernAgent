import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  timeout: 60_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  workers: 1,
  retries: 0,
  outputDir: "../output/playwright/results",
  reporter: [["list"], ["html", { outputFolder: "../output/playwright/report", open: "never" }], ["json", { outputFile: "../output/playwright/results.json" }]],
  use: { baseURL: "http://127.0.0.1:18891", trace: "retain-on-failure", screenshot: "only-on-failure" },
  projects: [
    { name: "desktop", use: { ...devices["Desktop Chrome"], viewport: { width: 1440, height: 1000 } } },
    { name: "narrow", use: { ...devices["Desktop Chrome"], viewport: { width: 390, height: 844 } } },
  ],
  webServer: { command: "python ../scripts/browser_server.py", url: "http://127.0.0.1:18891/healthz", reuseExistingServer: false, timeout: 45_000 },
});
