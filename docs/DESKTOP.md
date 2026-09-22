# 桌面端（Windows · 旧版 Wails 壳）— 已移除

> **状态：已移除（removed）。**
> 旧的 Wails v2 + WebView2 桌面壳（`cmd/tavernagent-desktop/`、`web/src/lib/desktopShell.ts`、
> `scripts/build.ps1 -Desktop` 分支、`go.mod` 中的 wails 依赖）已在 Avalonia 端功能对等后删除。

请使用下一代跨平台桌面端：**Avalonia + Sidecar**（`desktop/`，见
[DESKTOP_AVALONIA.md](DESKTOP_AVALONIA.md)）。

- 运行（开发）：`powershell -ExecutionPolicy Bypass -File scripts/dev-desktop.ps1`
- 打包（自包含 + sidecar 同捆）：`powershell -ExecutionPolicy Bypass -File scripts/publish-desktop.ps1 -Zip`
- 无头服务仍为 `cmd/tavernagent`（`scripts/build.ps1` 不再产出桌面壳）。

`/api/v1/desktop` 与 `/api/v1/desktop/theme` 端点保留在无头服务中（无害且仍被测试覆盖），
仅不再有 Wails 壳消费它们。
