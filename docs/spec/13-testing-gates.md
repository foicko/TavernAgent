# 13 测试与 CI 门禁

> 决策：**契约测试 + 安全边界测试必跑**。
> 测试策略总则（来自 `AGENTS.md`）：单元测试**只保留守卫安全边界与不可逆路径**的用例；纯领域规则/分词器/协议解析/UI 原语不再单测覆盖。契约测试是本规范的**明确例外**，必须存在。

## 1. 必须存在的测试

| 类别 | 覆盖 | 位置 |
|---|---|---|
| 契约测试（SSE 事件名） | 三端一致 | `internal/protocol/events_contract_test.go`、`web/src/app/__tests__/sseEventsContract.test.ts`、生成 Kotlin 测试 |
| 契约测试（卡片标记） | 三端一致 | `internal/cardmarkers/contract_test.go`、`web/src/lib/__tests__/cardMarkersContract.test.ts`、生成 Kotlin 测试 |
| 契约测试（DTO 字段） | 三端一致（**待建**） | 见 [`06-dto.md`](06-dto.md) |
| 安全边界 | 鉴权/来源、Host 防线、SSRF/端点守卫、路径穿越、数据锁、凭据脱敏、迁移与提交原子性、状态哈希/重放顺序、秘密不泄漏、恢复幂等 | `internal/**` 的 `*_test.go` |
| 合规/防漂移 | 发布审计、文档防漂移、前端指纹 | `scripts/tests/*` |
| 浏览器 e2e | 关键用户流 | `web/e2e`（playwright） |

## 2. CI 门禁清单（`.github/workflows/quality.yml`，全部 blocking）

### Job `quality`

- 前端：`pnpm install --frozen-lockfile` → `typecheck` → `lint` → `test` → `build` → `audit --audit-level=high`
- 第三方声明：`python scripts/license_notices.py` + `git diff --exit-code -- THIRD_PARTY_NOTICES.md LICENSES`
- Go：`gofmt -l internal cmd` 为空 → `go test -race -count=1 ./...` → `go vet ./...` → `go build ./...` → `python scripts/layering_check.py` → `gen_marker_contract.py --check` → `gen_sse_contract.py --check`
- `golangci-lint`（版本锁定 `v2.12.2`）
- `govulncheck@v1.1.4 ./...` → `python -m unittest discover -s scripts/tests -v` → `python scripts/release_audit.py --strict`
- 浏览器测试：构建二进制 → `pnpm test:browser`
- 上传 `browser-evidence`（非阻塞）

### Job `android`

- `./gradlew :app:testDebugUnitTest :app:assembleDebug --no-daemon`

### Job `artifact-runtime`

- `needs: quality`；5 平台矩阵跑 `python scripts/release.py --target <os/arch> --smoke`

## 3. 本地必跑（提交前）

```
# 根目录（先构建前端嵌入物！）
cd web; pnpm install --frozen-lockfile; pnpm typecheck; pnpm lint; pnpm test; pnpm build; cd ..
gofmt -l internal cmd; go vet ./...; go test -count=1 ./...
golangci-lint run
python scripts/layering_check.py
python scripts/gen_marker_contract.py --check
python scripts/gen_sse_contract.py --check
python -m unittest discover -s scripts/tests
python scripts/release_audit.py --strict
# Android
cd android; ./gradlew :app:testDebugUnitTest :app:assembleDebug
```

## 4. 规则

- **新增测试前先问**：它守卫安全边界或不可逆行为吗？是契约测试吗？都不是就不写。
- 契约源变更后，**生成物必须同提交更新**，`--check` 必须通过（[`14-docs-sync.md`](14-docs-sync.md)）。
- 有意改视觉 → `pnpm css:snapshot`；纯结构改动 → `pnpm build:parity && pnpm css:parity`；收紧基线 → `node scripts/check.mjs --update-css-baseline`（放宽项会被打印并要求解释）。
- 单文件超 800 行且被基线豁免的文件**不得继续加行**，应拆分。

## 5. 差距（必须收敛）

- **DTO 契约测试未入 CI**：`testdata/contract/api.json` 未建，Go/Kotlin 无断言，Web 只有手写 fixture。目标：建成后把 `gen_api_contract.py --check` 加入 Go 门禁步骤。
- **错误码枚举未入 CI**：错误码跨端共享后应加契约测试（[`04-errors.md`](04-errors.md)）。
- **Android 分层无检查**：见 [`12-layering.md`](12-layering.md) §4。
