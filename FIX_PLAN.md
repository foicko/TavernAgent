# 修复执行计划（FIX_PLAN）

> 配合 `OPTIMIZATION_PLAN.md` 使用。按批次修复，**每批完成后跑门禁并单独 git commit**。
> 门禁：Go `gofmt -l internal cmd`（空）、`go vet ./...`、`go test -count=1 ./...`；Web `pnpm build && pnpm lint && pnpm test`；合规 `python scripts/release_audit.py --strict`、`python -m unittest discover -s scripts/tests`。
> 规则：先 `web/ pnpm build` 再任何 Go 构建；单文件 ~800 行上限；样式只走令牌。

## 批次总览

| 批次 | 主题 | 覆盖 ID | 状态 |
| --- | --- | --- | --- |
| B1 | 后端崩溃与安全红线 | S1, M13, S6, S3, S4, M14, M15 | ✅ 已完成 |
| B1b | 后端安全补遗（注入/SSRF/许可） | S2, S5, S7 | ✅ 已完成 |
| B2 | 后端正确性与降级 | M6, M12, M21, M24, M26 | ✅ 已完成 |
| B2b | 后端结构性/并发（需设计） | M1, M2, M3, M4, M17, M22, M23, M29 | ⬜ |
| B3 | 后端性能与鲁棒（部分） | M10, M25, M28 | ✅ 已完成 |
| B3b | 后端性能与鲁棒（其余） | M7, M18 | ✅ 已完成 |
| B3c | 后端性能与鲁棒（余下） | M5, M9, M16, M19, M20, M27 | ⬜ |
| B4 | Web SSE / 状态正确性（核心） | S8, M34, M44, M45 | ✅ 已完成 |
| B4b | Web 状态/持久化其余项 | S9, M31, M33, M37, M38, M39, M40 | ✅ 已完成 |
| B4c | Web 状态清理其余项 | M41, M42, M43 | ✅ 已完成 |
| B4d | Web 会话列表错误态 | M36 | ⬜ |
| B5 | Web 可访问性与触屏（核心） | S13, S14, S15, S16, S17, M76 | ✅ 已完成 |
| B5b | Web a11y/触控其余项 | M64, M65, M66, M67, M68, M70, M71 | ⬜ |
| B6 | Web 视觉一致性与 UX | M69, M72, M73, M74, M75, M77, M78, M79 | ⬜ |
| B7 | Android 关键缺陷（核心） | S10, S12, M48, M49, M50 | ✅ 已完成 |
| B7b | Android 关键其余项 | S11, M51, M57（禁用备份面） | ✅ 已完成 |
| B7c | Android 其余关键项 | M46, M47, M52, M53, M57（Keystore 加密）, M59, M61, M62, M63 | ⬜ |
| B8 | Android 其余与安全 | M54, M55, M56, M58, M60 | ⬜ |
| B9 | 依赖与 CI / 工具链（基础） | M80, M85, M86, G79 | ✅ 已完成 |
| B9b | 大版本依赖升级与许可证门禁 | M81, M82, M83, M84 | ⬜ |
| B10 | 重构（独立 PR） | R1–R12 | ⬜ |
| B11 | 轻微项清理 | G1–G78 | ⬜ |

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

---

## 继续修复计划（详细）

### 执行顺序与依赖

```
B1b（3 个严重安全） ──┐
B4b（含 S9 严重）    ──┤
B7b（含 S11 严重）   ──┼──▶ B2b（后端结构性/并发，需设计）
B3（后端性能/鲁棒）  ──┤
B5b + B6（Web a11y/视觉）┘
                              └──▶ B8（Android 其余）
                                   B9b（大版本依赖升级，独立 PR）
                                   B10（重构，独立 PR）
                                   B11（轻微项，批量）
```
- **无相互依赖**：B1b、B4b、B5b、B6、B7b、B8 可任意顺序。
- **B2b 需先定设计**：M1/M2（fail-closed）、M3（秘密宏）、M4（规则默认属性）、M17/M23/M29（读写连接纪律）。
- **B9b 必须独立提交**：每个大版本升级一个 PR，避免与功能修复混提。

### B1b · 后端安全补遗（3 项严重）

| ID | 目标 | 涉及文件 | 方法 | 风险/测试 |
| --- | --- | --- | --- | --- |
| S2 | 边界标签中和大小写不敏感 + 导演讨论摘要纳入保护 | `internal/context/external_content.go:96`、`internal/context/prompts.go:714-736` | `neutralizeBoundaryTags` 改大小写不敏感（`(?i)` 或逐字符小写匹配）；`buildPlanningMessages` 的摘要/祖先文本走 `externalContentBlock`/中和 | 新增注入用例：`<EXTERNAL_CONTENT>`、`</external_content>`、planning 路径 |
| S5 | 阻断 SSRF 到云元数据/链路本地地址 | `internal/adapters/image/image.go`、`tts/tts.go`、`embedding/openai.go`，复用 `config.validateEndpoint`（`internal/adapters/config/file.go:625`） | 新增共享 `providerutil.ValidateEndpointURL`：阻断 `169.254.0.0/16`、`fd00:ec2::254`、`100.100.100.200`、`metadata.*`；保留公网与回环（本地 LLM 常用）；三个适配器在发请求前调用 | 需产品确认是否也阻断任意私网；新增单测覆盖各地址 |
| S7 | 处理 `reference-components/` 许可证 | `reference-components/`（383 文件，~3MB）、`.gitignore`、`scripts/release_audit.py` | **待决策**：移出跟踪（保留本地）或补齐上游许可证 | 移出后需确认 `showcase/`、`reference-components/` 不再被引用 |

### B4b · Web 状态/持久化（含 S9 严重）

| ID | 目标 | 方法 |
| --- | --- | --- |
| S9 | 已加载历史不被 `reloadView` 丢弃 | 新增 `olderMessages`/`oldestTurnId` 状态；`loadOlder` 累加、`reloadView` 按节点 id 去重合并；切会话/分支时重置（`beginNavigation`/`switchBranch`/`emptyStory`） |
| M31 | 长请求超时按调用可配 | `http`/`binaryRequest` 增加 `timeoutMs`；生图/合成/整理记忆用 120s，元数据用 30s |
| M33/M40 | 持久化脱敏与限容 | 三个 settings store 加 `partialize` 排除密钥；`imageSettingsStore` 排除 `turnIllustrations`；各 store 加 `version`/`migrate` |
| M36/M41 | 错误态可见 | `sessionSlice` 增 `sessionsError`/loading；`getTTSVoices` 上抛或返回错误供设置页重试 |
| M37/M38/M39 | 并发/回滚正确性 | director 讨论加忙碌守卫；`navigateView` 回滚补齐草稿态；`branchBusy` 用单调 seq |
| M42/M43 | 状态清理 | `beginNavigation` 清 `pendingSubmission`；`uiStore` 定时器/handler 可拆除 |

风险/测试：`storyIsolation`（41 项）、`turnRecovery`、`characterCardStore`；新增"loadOlder 后 reload 保留历史""超时可配"用例。

### B7b · Android 关键其余（含 S11 严重）

| ID | 目标 | 方法 |
| --- | --- | --- |
| S11 | 加载更早消息前置而非替换 | `applyView`/`reload` 在 `before != null` 时合并窗口（保留旧 messages，合并 nodes/oldestTurnId/hasMore） |
| M46 | 主线程 JSON 解码 | `applyView` 的 `StoryMessages.fromView` 与 worldState 解析移入 `withContext(Dispatchers.Default)` |
| M47 | 失败回合重试语义 | 终态失败时重发新回合（新幂等键），保留 `pendingInput` |
| M51/M52 | 前台服务与通知 | 保存并取消 `stopJob`；完成通知单一 owner + 渠道只建一次 + 前台抑制 |
| M53 | 注册表/思考缓存驱逐 | 终态事件驱逐 `TurnStreamRegistry` 条目；thinking 映射限容 |
| M57 | token 存储与明文流量 | Keystore 包装存储；`allowBackup=false`/`dataExtractionRules`；`networkSecurityConfig` 仅放行已配置主机 |
| M59/M61/M62/M63 | 幂等/取消/失败码 | 记忆补丁键跨重试复用；重试复用原始并发基线；`runCatching` 重抛 `CancellationException`；透传 `failureCode`/`failureMessage` |

风险：本机无 SDK，全部改动需由新增 Android CI 编译验证；`ApiClientMockServerTest` 等 51 项单测为回归基线。

### B2b · 后端结构性/并发（需设计，风险较高）

| ID | 目标 | 方法 | 备注 |
| --- | --- | --- | --- |
| M1/M2 | 上下文编译 fail-closed | `loadSessionContext` 返回 error 并贯穿调用链；区分"未找到"与"存储错误"；`currentTurn` 未知时不 boost | 改签名，影响面广，配回归 |
| M3 | 秘密宏在加载期展开 | `loadSessionContext` 解析秘密后用角色名/玩家名展开；不动模板字节（保 `CardIdentity`） | 需确认展开范围 |
| M4 | 规则默认属性统一 | 决定前置条件与检定同用 `DefaultAttribute` 还是同用 0 | **需产品决策** |
| M17 | 导出不持写锁 | `ExportSession` 走只读连接池/读事务，不持 `s.mu` | 配并发压测 |
| M22 | 导演 activate upsert | `INSERT … ON CONFLICT` 更新草稿，替代"仅 UPDATE" | **需确认允许无草稿激活** |
| M23 | 无投影时 CAS 失败关闭 | `checkBaseHashTx` 缺投影视为硬错误 | 影响导入数据兼容 |
| M29 | 投影写移出读路径 | 提交路径/后台维护物化投影，读路径只读 | 配压测 |

### B3 · 后端性能与鲁棒（11 项）

- 性能：M5（预算 O(n²)）、M9（pack 全量驻留）、M25（SSE 1Hz reconcile）。
- 鲁棒：M7（头像候选限数）、M10（pack 资源名碰撞）、M16（配置迁移硬失败）、M18（图像回退数据目录）、M19（脱敏密钥读改写）、M20（上传并发预算）、M27（多行 `data:`）、M28（流 goroutine 泄漏）。
- 测试：`internal/context`、`internal/pack`、`internal/adapters/{image,config,http,providers}` 各自补单测。

### B5b / B6 · Web a11y 与视觉一致性

- B5b：M64（模态 inert + 滚动锁）、M65（Tabs panel 语义）、M66/M67（行/卡片键盘可达）、M68（抽屉 Esc/背景）、M70（粗指针 44px）、M71（reduced-motion 覆盖循环动画，**注意不得新增 `!important`**，CSS 棘轮只允许 4 处）。
- B6：M69（统一 toast，删自绘）、M72（字号令牌化）、M73（死选择器）、M74（世界书条目预选）、M75（模态栈）、M77（加载态播报）、M78（菜单 portal）、M79（标签）。
- 约束：样式改动需 `pnpm build:parity && pnpm css:parity`；有意视觉改动才 `pnpm css:snapshot`。

### B8 · Android 其余与安全

M54（头像整卡 GET→列表字段/批量）、M55（SAF 导入尺寸上限）、M56（导出脱离 Composable 作用域）、M58（触控目标/语义）、M60（导演讨论改 SSE）。

### B9b · 大版本依赖升级与许可证门禁（独立 PR）

M81（ESLint 10 + typescript-eslint）、M82（Vite 8/Vitest/Playwright/jsdom/TS）、M83（OkHttp 5、Coil 3、AGP 9/Gradle 9、Navigation、Compose BOM、sqlite）、M84（`license_notices.py` 重生成 + CI diff 门禁）。每项独立提交并各自过门禁。

### B10 · 重构（独立 PR，R1–R12）

R1 对话框栈、R2 选择控件、R3 ListRow、R4 设计系统收敛、R5 SSE 模块化、R6 Story 运行时容器、R7 后端统一 host/auth + 错误体裁剪 + `IN` 构造器 + 读写纪律、R8 provider `streamPump`、R9 卡片导入管线统一、R10 回合生命周期 + `Compile` 拆分、R11 Android 主线程纪律 + 通知单 owner + 可测试性、R12 供应链治理。

### B11 · 轻微项（G1–G78）

按区域批量清理；优先潜在正确性/资源项：G5（运算符优先级）、G8（RRF 负权重）、G9（共享记录变更）、G23（非原子写图）、G24（资源名碰撞/保留名）、G25（`inClause` 空切片）、G26（时钟）、G30（`Hijacker`）、G31（编码错误/Content-Type）。
