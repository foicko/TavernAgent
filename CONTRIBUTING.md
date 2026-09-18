# 参与开发

项目使用 Go 1.26.6、Node.js 24 和 pnpm 11.9.0。请先阅读 [项目现状与待办](docs/PROJECT_STATUS.md) 和 [运行维护说明](docs/OPERATIONS.md)。

1. 在独立分支上修改，测试使用临时数据目录。不要使用日常故事库或把模型凭据提交到仓库。
2. 前端运行 `pnpm install --frozen-lockfile`、`pnpm typecheck`、`pnpm lint`、`pnpm test` 和 `pnpm build`。`web/dist` 是生成物、不入库，但被 Go 编译期嵌入，所以第 2 步必须先于第 3 步完成。
3. 根目录运行 `go test -count=1 ./...`、`go vet ./...` 和 `golangci-lint run`。Linux 还需 `go test -race -count=1 ./...`。
4. 浏览器测试前先构建嵌入最新前端的 `build/browser/tavernagent`（Windows 加 `.exe`），再在 `web` 运行 `pnpm exec playwright install chromium` 和 `pnpm test:browser`。测试服务使用 18891 端口及新建数据目录。
5. 修改事件、状态转换、迁移或异步归属时，补充能够复现原问题的回归。保留失败和复测证据。

提交说明应描述触发问题的操作、修复后的行为及验证结果。数据迁移必须递增版本、提供一致性备份并拒绝旧程序打开新版本库。历史事件不可原地改写；新故事素材须原创或具有明确授权。

真实模型验收由 `scripts/release_eval.py` 和 `scripts/gemini_meter.py` 负责。普通 CI 不调用付费模型。发布资格以对应源码、二进制和调用台账的验收报告为准。

贡献将按项目 MIT 许可证提供；第三方代码和素材保留原许可证与署名。
