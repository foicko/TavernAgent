# 修复执行计划（FIX_PLAN）

> 配合 `OPTIMIZATION_PLAN.md` 使用。按批次修复，**每批完成后跑门禁并单独 git commit**。
> 门禁：Go `gofmt -l internal cmd`（空）、`go vet ./...`、`go test -count=1 ./...`；Web `pnpm build && pnpm lint && pnpm test`；合规 `python scripts/release_audit.py --strict`、`python -m unittest discover -s scripts/tests`。
> 规则：先 `web/ pnpm build` 再任何 Go 构建；单文件 ~800 行上限；样式只走令牌。

## 批次总览

| 批次 | 主题 | 覆盖 ID | 状态 |
| --- | --- | --- | --- |
| B1 | 后端崩溃与安全红线 | S1, M13, S6, S3, S4, M14, M15 | ✅ 已完成 |
| B2 | 后端正确性与降级 | M1, M2, M3, M4, M6, M8, M11, M12, M17, M21, M22, M23, M24, M29, M30 | ⬜ |
| B3 | 后端性能与鲁棒 | M5, M7, M9, M10, M16, M18, M19, M20, M25, M26, M27, M28 | ⬜ |
| B4 | Web SSE / 状态正确性 | S8, S9, M31, M33, M34, M35, M36, M37, M38, M39, M40, M41, M42, M43, M44, M45 | ⬜ |
| B5 | Web 可访问性与触屏 | S13, S14, S15, S16, S17, M64, M65, M66, M67, M68, M70, M71, M76 | ⬜ |
| B6 | Web 视觉一致性与 UX | M69, M72, M73, M74, M75, M77, M78, M79 | ⬜ |
| B7 | Android 关键缺陷 | S10, S11, S12, M46, M47, M48, M49, M50, M51, M52, M53, M57, M59, M61, M62, M63 | ⬜ |
| B8 | Android 其余与安全 | M54, M55, M56, M58, M60 | ⬜ |
| B9 | 依赖与 CI / 工具链 | M80, M81, M82, M83, M84, M85, M86 | ⬜ |
| B10 | 重构（独立 PR） | R1–R12 | ⬜ |
| B11 | 轻微项清理 | G1–G79 | ⬜ |

## 每批交付要求

1. 代码修复 + 对应回归测试（覆盖该批发现的分支）。
2. 跑全部门禁，全绿后再提交。
3. 提交信息使用 `fix(scope): 主题` / `feat(scope):` / `chore(scope):`，正文列出覆盖的 OPTIMIZATION_PLAN 编号。
4. 提交后更新本文件的「状态」列。

## 备注

- S3 采用「非回环绑定且未显式指定 `-tls` 时自动升级为自签 HTTPS」的保守默认；显式 `-tls off` 仍被尊重（用户明确知情）。
- S6 依赖升级需能访问模块代理；若离线则记录并顺延到可联网时执行。
- 大版本依赖升级（M81–M83）各自独立提交，避免与功能修复混在一起。
