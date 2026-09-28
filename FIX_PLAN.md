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
| B2b | 后端上下文 fail-closed 与导出读锁 | M1, M2, M17 | ✅ 已完成 |
| B2d | 后端结构性其余 | M3→R10；M23/M29→R7；M4 待决策 | 🔀 并入 R |
| B2c | 导演 activate 草稿 upsert | M22 | ✅ 已完成 |
| B3 | 后端性能与鲁棒（部分） | M10, M25, M28 | ✅ 已完成 |
| B3b | 后端性能与鲁棒（其余） | M7, M18 | ✅ 已完成 |
| B3c | 后端性能与鲁棒（余下） | M16, M19 | ✅ 已完成 |
| B3d | 后端性能/DoS（部分） | M20, M27 | ✅ 已完成 |
| B3e | 后端性能/DoS（余下） | M5→R10；M9 独立 | 🔀 部分并入 R |
| B4 | Web SSE / 状态正确性（核心） | S8, M34, M44, M45 | ✅ 已完成 |
| B4b | Web 状态/持久化其余项 | S9, M31, M33, M37, M38, M39, M40 | ✅ 已完成 |
| B4c | Web 状态清理其余项 | M41, M42, M43 | ✅ 已完成 |
| B4d | Web 会话列表错误态 | M36 | ✅ 已完成 |
| B5 | Web 可访问性与触屏（核心） | S13, S14, S15, S16, S17, M76 | ✅ 已完成 |
| B5b | Web a11y/触控其余项 | M64, M70, M71 | ✅ 已完成 |
| B5c | Web 语义/键盘可达（部分） | M67, M68 | ✅ 已完成 |
| B5d | Web 语义/键盘可达（分段控件语义） | M65 | ✅ 已完成 |
| B5e | Web 语义（会话行嵌套按钮） | M66（会话行） | ✅ 已完成 |
| B5f | Web 语义（角色卡行/节点树嵌套按钮） | M66（余下） | ✅ 已完成 |
| B6 | Web 视觉一致性与 UX（部分） | M73, M77, M79 | ✅ 已完成 |
| B6b | Web 视觉/UX 其余（部分） | M69 | ✅ 已完成 |
| B6c | Web 视觉/UX（菜单 portal） | M78 | ✅ 已完成 |
| B6d | Web 视觉/UX（模态栈） | M75（已回退，见备注） | ↩️ 已回退 |
| B6e | Web 字号令牌化（R4/M72，全量迁移） | M72 | ✅ 已完成 |
| B7 | Android 关键缺陷（核心） | S10, S12, M48, M49, M50 | ✅ 已完成 |
| B7b | Android 关键其余项 | S11, M51, M57（禁用备份面） | ✅ 已完成 |
| B7c | Android 协程取消传播 | M62 | ✅ 已完成 |
| B7d | Android 注册表驱逐与失败码透传 | M53, M63 | ✅ 已完成 |
| B7e | Android 主线程/重试/通知/幂等/Keystore | M46, M47, M52, M59, M61, M57（Keystore 加密） | ✅ 已完成 |
| B7f | Android 明文流量收敛 | M57（networkSecurityConfig） | ⬜ |
| B8 | Android 其余与安全（部分） | M55 | ✅ 已完成 |
| B8b | Android 其余与安全（部分） | M56, M58 | ✅ 已完成 |
| B8c | Android 头像轻量化与导演 SSE | M54, M60 | ✅ 已完成 |
| B9 | 依赖与 CI / 工具链（基础） | M80, M85, M86, G79 | ✅ 已完成 |
| B9b | 大版本依赖升级与许可证门禁 | M81, M82, M83, M84 | ⬜ |
| B10 | 重构（独立 PR） | R1–R12（详见 B10-重构执行计划） | 📋 计划已定 |
| B10-R4 | 设计系统收敛 | M72 ✅；G61 ✅；G63 ✅；G62 ✅（经 R2）；徽章收敛留独立项 | ✅ 完成 |
| B10-R3 | 列表行统一到 `ui/ListRow`（语义已由 M66 修好，结构迁移待做） | R3 | ⬜ |
| B11 | 轻微项清理（按子批处理） | G1–G78 | 🔄 见子批 |
| B11a | 轻微项（Go 正确性/资源优先项） | G3, G5, G8, G23, G25, G26, G30, G31 | ✅ 已完成 |
| B11b | 轻微项（解析/并发/资源/样式） | G1, G2, G6, G7, G9, G10, G12, G20, G22, G51, G57, G59 | ✅ 已完成 |
| B11c | 轻微项（HTTP 卫生） | G14, G15, G16, G28 | ✅ 已完成 |
| B11d | 轻微项（TTS 缓存键与音频上限） | G18, G19 | ✅ 已完成 |
| B11e | 轻微项（其余 G，拆子批） | G4, G11, G13, G17, G21, G24, G27, G29, G32–G50, G52–G56, G58, G61–G78 | ⬜ |
| B11e-A | 轻微项 Go（含 G21 保守处理） | G4, G11, G13, G17, G21, G24, G27, G29 | ✅ 已完成 |
| B11e-B | 轻微项 Web（状态/死代码/样式） | G32, G35, G36, G39, G40, G43, G56, G58（G34 复核已修） | ✅ 已完成 |
| B11e-B2 | Web（并发/幂等/结构） | G33/G37/G38/G41/G42 → 并入 R5/R6 | 🔀 并入 R |
| B11e-C | 轻微项 Android（API Key/槽位/通知图标） | G44, G45, G46 | ✅ 已完成 |
| B11e-C2 | Android（TTS/记忆行/SSE/导航/构建） | G47–G49, G52–G55 → 并入 R11 | 🔀 并入 R |
| B11e-D | 轻微项 工具/仓库 | G74, G75, G78 | ✅ 已完成 |
| B11e-D2 | 工具/仓库（余下） | G72/G73/G76/G77 → 并入 R12 | 🔀 并入 R |

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
- **M22（导演 activate 草稿存在性）**：加 `RowsAffected` 校验会破坏 `TestDirectorRestartRecoveryAndProjectionReplay`（允许无草稿行时 activate）。**已改为 `INSERT … ON CONFLICT` 的 upsert 语义（B2c）**：activate 现在会创建/更新草稿行；测试同步改为读回 activate 建立的草稿。
- **M1/M2（上下文存储错误 fail-closed）**：需要改 `loadSessionContext` 签名并贯穿调用链，属结构性改动，**顺延到 B2b/R10**。
- **M3（秘密宏展开）**：模板 `Content` 参与 `CardIdentity` 校验，直接持久化展开后的卡 JSON 会破坏导入/导出身份校验；正确做法是在加载期展开，需单独设计，**顺延到 B2b**。
- **M4（规则前置条件默认属性）**：改默认值会改变既有规则语义，需产品决策，**顺延**。
- **M17/M23/M29（SQLite 读写连接纪律）**：并发/锁语义改动风险较高，归入 **B2b** 单独处理并配压测。
- **M35（会话头像映射重复写盘）**：复核发现原守卫"命中首位即返回"会在一次重排后立即短路，**并非每次渲染都写**；`characterCardStore.test.ts:146` 已固化该意图。**非缺陷，不改**。
- **M23（无投影时 CAS fail-closed）**：实测把 `checkBaseHashTx` 的 `sql.ErrNoRows` 改为硬错误会破坏首次提交（根节点尚无投影，`TestCommitOnceIdempotent`/`TestReceiptsAtResultNodes` 直接失败）。正确做法需先保证根节点投影在提交前物化（见 M29），**顺延到 B2d**。
- **M29（投影写移出读路径）**：需把记忆投影物化从读路径迁到提交路径，属设计改动，**顺延到 B2d**。
- **M74（示例世界书条目预选）**：复核 `characterPresets.ts:112`，`INITIAL_LORE_ENTRIES` 为**空数组**，侧栏"示例世界书词条"不会渲染任何可点卡片（同时 `LeftRail.tsx:302` 的 `role=button` 缺失也因此不可达）。原描述的场景在当前源码下不成立，**不适用，不改**；若将来填充示例条目，应同时补预选与键盘可达。
- **G60（自绘 toast 的 `z-index:9999`）**：已随 M69（B6b）删除自绘 `.quiet-toast-notification`、改用令牌化 `ToastStack` 而一并消除，**已随 M69 完成**。
- **M75（全局快捷键造成模态叠模态）**：本已实现「无对话框时才响应 Ctrl+K/Ctrl+,」，但 `e2e/palette.spec.ts` 明确验收「允许叠层 + Esc 只关最上层」，两者互斥；**以验收测试为规格，已回退**。真正的解决方案是 R1（统一模态栈），届时再收。附带发现并修复了一个真实回归：M64 对 `#root` 设 `inert` 后，未 portal 的 `CommandPalette` 会一并失效——命令面板已改为 portal 到 `document.body`（与 `Modal` 一致）。

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

> 复核更新：本机实际**已安装 Android SDK**（`android/local.properties` → `platforms 34–37`、JDK 21）。
> 自 B7c 起，Android 改动均在本地执行 `:app:compileDebugKotlin` 与 `:app:testDebugUnitTest` 验证；
> 并借此修复了 B7b 引入、此前从未编译过的 `StoryMessage.id` → `nodeId` 字段错误。

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

### B10-重构 · 大型重构执行计划（R1–R12）

> 目标：先完成全部结构性重构（R1–R12）。**每个 R 吸收对应的零散项**——单独修一遍会与重构重复改动同一批文件。
> 规则：每个 R 拆成「可独立过门禁、可独立回滚」的提交，每步全绿才进入下一步；**不 push**。

#### 进度

| R | 状态 | 备注 |
| --- | --- | --- |
| R2 | ✅ 已完成 | 会话筛选、立绘神情差分 → `Tabs`；另两处经核实为不同模式（见下）|
| R3 | ⬜ | |
| R1 | ⬜ | |
| R6 | ⬜ | |
| R5 | ⬜ | |
| R8 | ⬜ | |
| R9 | ⬜ | |
| R7 | ⬜ | |
| R10 | ⬜ | |
| R11 | ⬜ | |
| R12 | ⬜ | |

#### 0. 吸收映射（重构覆盖的剩余条目）

| 重构 | 吸收条目 | 依据 |
| --- | --- | --- |
| R1 模态栈 | M75、M64、M68、G66 | 4+ 套 portal/scrim/focus trap/Esc 是 S17/M64/M68/M75/G66 的共同根因 |
| R2 分段控件 | **G62**、M65 | `Tabs` 已有 `variant=segmented`/`size`/`fill`/`radiogroup`，只需迁移 4 站点 + 删旧 CSS |
| R3 ListRow | M66（结构迁移）、M67 | `ListRow` 已实现「主按钮 + 尾部操作」，无嵌套按钮 |
| R5 SSE | G35、S8 边界 | `app/api.ts` 639 行混合 REST/二进制/SSE/退避 |
| R6 运行时容器 | **G33**、**G37**、G38、G41、G42、M42、M43、M44 | `sendSeq`/`navigationEpoch`/`pending*` 等模块级单例是共同根因 |
| R7 后端统一层 | M14、M15、G14、G25、M17、**M29**、**M23** | host/auth、错误体、`IN` 构造、读写纪律同源 |
| R8 streamPump | M27、M28 | 三 provider 共享约 150 行骨架 |
| R9 导入管线 | M6、M7、M13、S1（导入期 `NodeKind` 校验） | 三导入器各自实现归档/资源/告警 |
| R10 回合生命周期 | **M1、M2、M3、M5**、M11；M4 见决策点 | `Compile`/生命周期 6 处互斥锁是根源 |
| R11 Android | S10、M46、M52、M54、M55、M56、M57（仅文档）、G47、G48、G52、G53、G54、G55 | 同属「主线程纪律/通知 owner/资产服务化/可测性」 |
| R12 供应链 | M80、M84、M85、M86、G72、G73、G76、G77；大版本升级 → B9b | — |
| R4 设计系统 | M72 ✅、G61 ✅、G63 ✅；G62 → 并入 R2；徽章收敛见决策点 | 已基本完成 |

> **结论**：原 `B2d`、`B3e`、`B11e-B2/C2/D2` 的剩余项**全部被上述重构吸收**，不再单列批次；仅 **M4**（规则默认属性，需产品决策）与 **M57-`networkSecurityConfig`**（不可静态实施，仅文档）为例外。

#### 1. 执行顺序（5 个 Wave，按 风险 × 依赖 排序）

| Wave | 内容 | 风险 | 为什么在这个位置 |
| --- | --- | --- | --- |
| **1** | **R2 → R3**（+R4 收尾）Web 设计系统收官 | 低 | 现有 Playwright e2e（45 通过/3 跳过）可完全覆盖；先做完可缩小后续 Web 重构的 CSS/组件面 |
| **2** | **R1 → R6 → R5** Web 架构 | 中 | R1 收敛 Esc/inert/滚动锁（R6/R5 都依赖 App 全局键盘语义）；R6 先于 R5，避免同一批 slice 二次churn |
| **3** | **R8 → R9 → R7 → R10** Go 后端 | 中→高 | R8/R9 耦合最低先做；R7 建立统一层（含 M29/M23）后 R10 更易；R10 风险最高放最后（此时测试/可观测网最强） |
| **4** | **R11** Android（拆 a/b/c） | 高 | 依赖本机 `gradlew` 编译验证；`StoryViewModel`(772 行) 拆分是主要风险 |
| **5** | **R12** + **B9b** 供应链 | 中 | 大版本升级需联网，且会牵动 Wave 1–4 的构建/测试基线，放最后 |

#### 2. 各 R 任务分解（提交粒度 / 关键步骤 / 验证）

**Wave 1 · Web 设计系统收官**

- **B10-R2**（`refactor(web,r2)`，3 提交）：**已核对 4 处真实语义**——只有 2 处是分段控件：会话筛选、立绘神情差分 → `Tabs`；`composer-syntax-group`（`handleTagClick` 向输入框插入包裹，是动作组）与 `memory-filter-row`（5 项含计数、窄屏必须换行的筛选芯片行）不适用 `Tabs`，保留但补 `role="group"`/`aria-label`。删除对应自绘 CSS；有意视觉改动已 `pnpm css:snapshot`。补方向键/`aria-checked` 用例。
- **B10-R3**（`refactor(web,r3)`，4 提交/行类型）：`session-entry-card`、`char-card-entry`、`node-item`、物品槽 → `ListRow`；保留全部既有 `data-testid`；确认删除/分支控件恢复可 Tab 达。更新 `leftRail.test.tsx`。
- **B10-R4 收尾**：仅剩徽章收敛（78 个自绘 badge/pill/chip/tag 选择器 → `Pill` 变体），范围大且无对应 G 编号 → **见决策点**。

**Wave 2 · Web 架构**

- **B10-R1**（`refactor(web,r1)`，3–4 提交）：建 `ModalStackProvider`（栈/`closeTop`/`registerEscape`/背景 `inert`+滚动锁）→ 迁移 `Modal`/`useDialogFocus` → 让 `DirectorWorkspace`/`CommandPalette`/`Popover`/`TurnIllustration` lightbox/移动抽屉注册 → 全局快捷键改查「栈空」。补嵌套 Esc、焦点恢复、背景 inert 用例；把 `palette.spec` 的叠层验收扩到 3 层。
- **B10-R6**（`refactor(web,r6)`，3 提交）：定义 `createStoryRuntime()` → store 创建时实例化、`beginNavigation`/`emptyStory` 调 `reset()` → 迁各 slice → 删除测试里的间接重置。以 `storyIsolation.test.ts`(41 项) 守护。
- **B10-R5**（`refactor(web,r5)`，3 提交）：抽 `app/stream.ts`（`subscribeEvents`/`parseSSEBlock`/`readWithStall`/`sleepCancellable`）→ `onEvent` 独立 try/catch + 归一化 `seq` → 补「handler 抛错/`seq=0`/超大事件/401 非 JSON」用例。`api.ts` 随之降到 ~400 行。

**Wave 3 · Go 后端**

- **B10-R8**（`refactor(providers,r8)`）：抽 `streamPump`，三协议只提供「行→事件」映射；修多行 `data:`、`defer cancel()`。以现有 provider 测试 + 多行/取消用例回归。
- **B10-R9**（`refactor(app,r9)`，3–4 提交）：有界归档读取器（entry 数/单 entry/总量上限 + 魔数预检）→ 统一资源落地与 `Warnings` → 三路径共用同一报告 + `NodeKind` 校验。以 `card_test.go`/`tacard_test.go`/`cards_tacard_test.go` 守护。
- **B10-R7**（`refactor(http,sqlite,r7)`，4 提交）：mux 中间件（始终 `isLANHost` + 非回环强制 token，路由显式 opt-out）→ 统一 `bodySnippet(≤512)` 替换原始回显 → `sqlIN(column, ids)` 替换全部手写构造 → **M29**（投影物化移出读路径）→ **M23**（无投影 fail-closed）。以 HTTP 契约测试 + `ExportSession` 并发用例守护。
- **B10-R10**（`refactor(app,context,r10)`，4–5 提交）：抽 `turnRunner`（单一锁序 + 生命周期记账）并让现有测试全绿 → `Compile` 改 `compileSession`（每依赖加载一次、暴露已测尺寸）→ 增量预算 + mandatory 单次估算（**M5**）→ **M1/M2** 存储错误 fail-closed → **M3** 秘密宏加载期展开。全程 `go test -race` 守护。

**Wave 4 · Android**

- **B10-R11a**（`refactor(android,r11a)`）：`io {}`/注入 `Dispatcher`，TLS/JSON/文件 I/O 一律移出 Main（一次覆盖 S10/M46/M55/M56/G53）。
- **B10-R11b**（`refactor(android,r11b)`）：通知收敛为 `Application.onCreate` 创建的单一 `TurnNotifier`（M52）；离线 `TtsEngine.stop()` 接线（G47）、`SseLineParser` 空事件名/`retry`（G49）、`MemoryRow.toRow()`（G48）、死代码清理（G52）。
- **B10-R11c**（`refactor(android,r11c)`）：资产解析收敛到单一带缓存仓库；导出/导入迁前台服务；导航 `popUpTo`/单实例（G54）；release `shrink`+签名、清理未用 `okhttp.logging`（G55）；构造函数注入仓库并补 ViewModel 相位机/分页测试；拆 `StoryViewModel`(772 行)/`StoryContent`/`StoryComposer`。验证：`:app:compileDebugKotlin` + `:app:testDebugUnitTest` + `:app:assembleDebug`。

**Wave 5 · 供应链**

- **B10-R12**（`chore(ci,r12)`，3 提交）：`dependabot.yml`（gomod/npm `/web`/gradle `/android`/actions）+ `golangci.yml` 启用 `gosec` → Android CI job（JDK 21 + SDK 36 + 缓存）→ `license_notices.py` 进 CI（重生成后 diff 为空）。
- **B9b**（各自独立提交，需联网）：M81 ESLint 10、M82 Vite 8/Vitest/Playwright/jsdom/TS、M83 OkHttp 5/Coil 3/AGP 9/Gradle 9/Compose BOM/sqlite、M84 `THIRD_PARTY_NOTICES` 重生成。

#### 3. 每步验证基线

- Web：`pnpm build && pnpm lint && pnpm test`；结构改动 `pnpm build:parity && pnpm css:parity`，有意视觉改动 `pnpm css:snapshot`；**每个 Web R 收尾跑一次全量 Playwright e2e**（现 45 通过/3 跳过）。
- Go：`gofmt -l internal cmd`（空）、`go vet ./...`、`go test -race -count=1 ./...`。
- Android：`gradlew.bat :app:testDebugUnitTest :app:assembleDebug`。
- 合规：`python scripts/release_audit.py --strict`、`python -m unittest discover -s scripts/tests`。
- 单文件 ~800 行上限：`api.ts`(639)、`compile.go`(557)、`pack.go`(597)、`StoryViewModel.kt`(772) 均为拆分目标，不得继续加行。

#### 4. 决策点（待确认）

1. **R4 徽章收敛**：78 个自绘 badge/pill/chip/tag 选择器 → `Pill` 变体，范围大、无对应 G 编号。纳入本轮 / 拆为低优先独立项 / 仅收敛 `ui-pill` 命名最接近的一批？
2. **M4（规则默认属性 0 vs 10）**：需产品语义决策（统一用 `DefaultAttribute=10`，还是要求作者显式记录）。并入 R10 还是保持单独处理？
3. **R11 粒度**：按 a/b/c 三个子批（推荐）还是整体一批？
4. **提交策略**：每个 R 完成即 commit（推荐，便于回滚）还是每个 Wave 结束一起 commit？

### B11 · 轻微项（G1–G78）

按区域批量清理；优先潜在正确性/资源项：G5（运算符优先级）、G8（RRF 负权重）、G9（共享记录变更）、G23（非原子写图）、G24（资源名碰撞/保留名）、G25（`inClause` 空切片）、G26（时钟）、G30（`Hijacker`）、G31（编码错误/Content-Type）。
