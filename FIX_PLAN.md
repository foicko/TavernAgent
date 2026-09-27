# 修复执行计划（FIX_PLAN）

> 配合 `OPTIMIZATION_PLAN.md` 使用。按批次修复，**每批完成后跑门禁并单独 git commit**。
> 门禁：Go `gofmt -l internal cmd`（空）、`go vet ./...`、`go test -count=1 ./...`；Web `pnpm build && pnpm lint && pnpm test`；合规 `python scripts/release_audit.py --strict`、`python -m unittest discover -s scripts/tests`。
> 规则：先 `web/ pnpm build` 再任何 Go 构建；单文件 ~800 行上限；样式只走令牌。

## 批次总览

| 批次 | 主题 | 覆盖 ID | 状态 |
| --- | --- | --- | --- |
| B1 | 后端崩溃与安全红线 | S1, M13, S6, S3, S4, M14, M15 | ✅ 已完成 |
| B2 | 后端正确性与降级 | M6, M12, M21, M24, M26 | ✅ 已完成 |
| B3 | 后端性能与鲁棒 | M5, M7, M9, M10, M16, M18, M19, M20, M25, M26, M27, M28 | ⬜ |
| B4 | Web SSE / 状态正确性（核心） | S8, M34, M44, M45 | ✅ 已完成 |
| B4b | Web 状态/持久化其余项 | S9, M31, M33, M36, M37, M38, M39, M40, M41, M42, M43 | ⬜ |
| B5 | Web 可访问性与触屏（核心） | S13, S14, S15, S16, S17, M76 | ✅ 已完成 |
| B5b | Web a11y/触控其余项 | M64, M65, M66, M67, M68, M70, M71 | ⬜ |
| B6 | Web 视觉一致性与 UX | M69, M72, M73, M74, M75, M77, M78, M79 | ⬜ |
| B7 | Android 关键缺陷（核心） | S10, S12, M48, M49, M50 | ✅ 已完成 |
| B7b | Android 关键其余项 | S11, M46, M47, M51, M52, M53, M57, M59, M61, M62, M63 | ⬜ |
| B8 | Android 其余与安全 | M54, M55, M56, M58, M60 | ⬜ |
| B9 | 依赖与 CI / 工具链（基础） | M80, M85, M86, G79 | ✅ 已完成 |
| B9b | 大版本依赖升级与许可证门禁 | M81, M82, M83, M84 | ⬜ |
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

## 已复核并调整的条目

- **M11（TTS `[轻叹]` 插入）**：复核 `web/src/lib/ttsDirector.ts:225`，Go 端口与 TS 参考实现逐字一致（仅在前置风格前缀存在时插入）。**非缺陷，不改**。
- **M30（`DeleteSession` 把 `awaiting_continuation` 当非忙碌）**：`session_delete_test.go:226` 明确断言"暂停中的草稿不算在途，允许删除"，属**有意设计**。如需改动应先讨论语义，暂不动。
- **M22（导演 activate 草稿存在性）**：加 `RowsAffected` 校验会破坏 `TestDirectorRestartRecoveryAndProjectionReplay`（允许无草稿行时 activate）。正确做法是 `INSERT … ON CONFLICT` 的 upsert 语义，需单独设计，**顺延到 B2b**。
- **M1/M2（上下文存储错误 fail-closed）**：需要改 `loadSessionContext` 签名并贯穿调用链，属结构性改动，**顺延到 B2b/R10**。
- **M3（秘密宏展开）**：模板 `Content` 参与 `CardIdentity` 校验，直接持久化展开后的卡 JSON 会破坏导入/导出身份校验；正确做法是在加载期展开，需单独设计，**顺延到 B2b**。
- **M4（规则前置条件默认属性）**：改默认值会改变既有规则语义，需产品决策，**顺延**。
- **M17/M23/M29（SQLite 读写连接纪律）**：并发/锁语义改动风险较高，归入 **B2b** 单独处理并配压测。
- **M35（会话头像映射重复写盘）**：复核发现原守卫"命中首位即返回"会在一次重排后立即短路，**并非每次渲染都写**；`characterCardStore.test.ts:146` 已固化该意图。**非缺陷，不改**。
