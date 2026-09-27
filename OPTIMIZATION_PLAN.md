# TavernAgent 全面代码审查与 UI/UX 评估 · OPTIMIZATION_PLAN

> **状态**：仅审查与记录，**未修改任何现有代码或文件**（本文件为本次唯一新增产物）。
> **日期**：2026-09-28
> **范围**：Go 后端（`internal/domain`、`application`、`context`、`protocol`、`pack`、`search`、`adapters/*`、`util`）、Web 前端（`web/src/app|stores|lib|components|ui|styles`、`index.html`）、Android 客户端（`android/`）、依赖与 CI/工具链。
> **方法**：6 路并行只读审查 + 关键结论逐条抽样复核（下列行号均来自审查时工作区，并已对全部 🔴 项与关键 🟡 项复核；个别文件仍可能随后续改动偏移）。
> **图例**：🔴 严重（安全 / 数据丢失 / 崩溃 / 阻塞） · 🟡 中等（明显缺陷 / 性能 / 可维护性） · 🟢 轻微（一致性 / 整洁 / 潜在风险）

---

## 0. 摘要

### 0.1 统计

| 级别 | 数量 | 主要分布 |
| --- | --- | --- |
| 🔴 严重 | 17 | 后端 7 · 前端 2 · Android 3 · UI/UX 5 |
| 🟡 中等 | 约 50 | 后端 17 · 前端 16 · Android 18 · UI/UX 16 · 依赖 6 |
| 🟢 轻微 | 约 40 | 后端 13 · 前端 12 · Android 13 · UI/UX 16 · 依赖 7 |

### 0.2 最需要优先处理的风险（Top 8）

1. **远端内容可触发进程崩溃（DoS）**：导入的 `.tavernpack` 携带 `memoryChanges:[null]` 节点，首次压缩在后台 goroutine 解引用 nil → 未恢复 panic → 整个服务退出。（S1）
2. **局域网默认明文 + 可伪造 Host**：`-tls` 默认 `off`，配对码与 Bearer token 在共享局域网可被被动截获；所谓"仅局域网"只校验可伪造的 Host 头。（S3）
3. **DNS rebinding 可读取本机卡面/图片**：未鉴权路由（卡资源、TTS 音色、生成图）绕过了 Host 白名单，且返回 `Cache-Control: public`。（S4）
4. **Web SSE 事件静默丢失**：`lastEventId` 在调用 `onEvent` 之前推进，handler 抛错被当作"断线"吞掉 → 已提交事件永久丢失并进入重连风暴；`block.delta` 的 `seq<=0` 正是触发点。（S8）
5. **Android 主线程做 TLS 握手**：HTTPS/TOFU 引导流程在主线程做阻塞 socket，`NetworkOnMainThreadException` 或最长 8s 卡顿/ANR。（S10）
6. **历史只读态失效（Android）**：进入历史节点后 `viewingNodeId` 在下一次加载即被清空，`canSend` 变真，用户以为在最新节点而实际在浏览历史并可提交。（S12）
7. **提示词注入边界被绕过**：边界标签中和是**大小写敏感**的，且导演讨论模式的摘要文本完全未经该处理，卡/世界书/记忆内容可"关闭"数据区并伪装成系统指令。（S2）
8. **依赖漏洞 + 合规**：`golang.org/x/image v0.41.0` 命中 WebP 解码内存耗尽漏洞（修复于 v0.45.0+，需 `govulncheck` 复核）；`reference-components/` 整包再分发第三方源码却无任何上游许可证。（S6、S7）

### 0.3 做得好的地方（避免误伤）

- **构建与门禁体系扎实**：前端 `pnpm lint` 的六项 CSS 棘轮（`!important`、颜色字面值、跨文件重复选择器、死类名、玻璃残留、令牌契约）全绿；`gofmt`/`go vet`/`go test -race`/`govulncheck`/五平台冒烟均在 CI；本次复核 `go test -count=1 ./...` 与前端 439 项测试全部通过。
- **锁文件与密钥卫生**：`pnpm install --frozen-lockfile` 在 CI/构建/发布三处一致；`release_audit.py` 未发现凭据；测试子进程会剥离 `TAVERNAGENT_KEY_*` 环境变量。
- **后端若干高风险面已处理正确**：ZIP 路径穿越与声明大小校验（Go 1.26 已强制 `UncompressedSize64`）、`AggregateDeltas` 的 big.Int 防溢出、`coalescingQueue` 的停机/入队顺序、`validateJSON` 的重复键/深度逻辑。
- **前端测试覆盖了最难的路径**：SSE 重连、in-flight 草稿合并、导演恢复等有较充分的用例（本次发现的缺口恰是这些用例未覆盖的分支）。

---

## 1. 🔴 严重问题

### S1 · 远端内容触发 nil 解引用，导致服务进程崩溃（DoS）
- **描述**：`BuildCompactionChatRequest` 解析 `memory_change` 节点的 `memoryChanges` 为 `[]*domain.MemoryRecord`，随后直接 `probe := *m`，未判空。`{"memoryChanges":[null]}` 会得到 `m == nil`，解引用 panic。
- **位置**：`internal/context/summarizer.go:77`（`probe := *m`）；调用链 `internal/application/compactor.go:284-298`；导入校验未检查该节点负载 `internal/application/archive_validation.go`。
- **影响**：`.tavernpack` 可由用户导入且不校验 `NodeKind` 的 `ContentJSON` 结构。导入后第一次压缩在后台 goroutine（`coalescingQueue.run`）中 panic，若未被 recover 会**终止整个服务进程**。属于"远端内容触发"的可用性攻击面，且失败点远离导入处，极难定位。
- **建议**：`for _, m := range change.Memories { if m == nil { continue } … }`；对所有从 JSON 反序列化的 `[]*T` 统一加空元素守卫；在导入阶段按 `NodeKind` 校验 `ContentJSON` 能解析且符合预期信封（同时关闭 S1 与 `memory_change`/summary 载荷未校验的同类问题）；补一条"导入含 `memoryChanges:[null]` 的 pack 并跑 `RunOnce`"的回归测试。

### S2 · 提示词注入：边界标签中和大小写敏感，且摘要文本绕过该机制
- **描述**：`neutralizeBoundaryTags` 用精确大小写的 `strings.ReplaceAll` 处理 `</external_content>`、`<system-reminder>` 等；大写/混合大小写形式不被中和。此外 `buildPlanningMessages`（导演讨论）直接把摘要文本写入消息，未走 `externalContentBlock`/中和。
- **位置**：`internal/context/external_content.go:96-102`；`internal/context/prompts.go`（`buildPlanningMessages` 的摘要渲染，报告中约 731-736）。
- **影响**：卡、世界书、记忆、摘要均为不可信内容（第三方卡 + 模型派生记忆）。构造 `<EXTERNAL_CONTENT>` 变体，或在导演讨论里输出字面 `</external_content>`/`<system-reminder>`，即可"关闭"数据区，使其后文本被模型当作框架指令读取，边界机制失效。属于越权指令注入。
- **建议**：改为大小写不敏感的中和（`(?i)` 正则或先 `strings.ToLower` 比较再替换）；导演讨论的摘要同样经 `externalContentBlock`/`neutralizeBoundaryTags` 渲染；补针对大小写变体与 planning 路径的注入用例。

### S3 · 局域网默认明文传输，配对码与令牌可被截获；"仅局域网"校验可伪造
- **描述**：`-tls` 默认 `off`（明文 HTTP）；当绑定非回环地址时自动生成并打印 6 位配对码，配对返回 Bearer token。`isLANHost` 仅解析**客户端可伪造的 `Host` 头**。
- **位置**：`cmd/tavernagent/main.go:74`（`flag.String("tls", "off", …)`）；`internal/adapters/http/security.go:17-24,166-173`；`internal/adapters/http/lifecycle.go:118-131`；`internal/util/tlscert/resolve.go:53-85`。
- **影响**：在咖啡馆/办公/会议等共享局域网，旁路攻击者被动抓包即可拿到配对码与 Bearer token，从而获得整个故事库的访问权。自动 PIN 制造了"已上锁"的错觉，实际密钥在明文信道上传输。Host 检查只是防 DNS rebinding 的辅助，不是网络边界；`AuthPIN == ""` 的部署（嵌入式/桌面）表面受限实则无保护。
- **建议**：绑定非回环地址时**默认启用 `auto` TLS**（或拒绝非回环绑定，除非显式 `-tls off` 并确认）；非 TLS + 非回环时拒绝 `POST /api/v1/auth/pair`；在 `http.New`/`app.newApp` 中，只要绑定非回环就断言 `AuthPIN != ""`；把 Host 校验的注释明确为"仅防 DNS rebinding，非授权"。

### S4 · 未鉴权资源路由绕过 Host 校验，且返回 public 缓存（DNS rebinding 可读）
- **描述**：卡资源、TTS 音色、生成图等路由未包 `s.security(...)`，因此同时跳过了 `isLANHost`（DNS rebinding 防线）；`serveCardAsset`/`serveImageFile` 强制 `Cache-Control: public, max-age=31536000, immutable`。
- **位置**：`internal/adapters/http/cards.go:33`（`GET /api/v1/cards/{cardId}/assets/{name}`）、`internal/adapters/http/cards.go:351,363`；`internal/adapters/http/server.go:152,158`；`internal/adapters/http/image_handler.go:67`。
- **影响**：恶意网页可将其源 DNS rebinding 到 `127.0.0.1:8890`；由于这些路由不校验 Host，页面即"同源"，可读取卡资源（含 `.json`/`.risum` charx 载荷、图片）与生成图。`public` 缓存还允许共享缓存长期保留私人角色美术。利用需知道 UUIDv7 形式的 cardID/资源名，但信任边界确实不一致。
- **建议**：即使有意省略 token 鉴权，也给这些路由套 `hostOnly`/`isLANHost` 守卫；缓存改为 `Cache-Control: private`（或 `no-store`）。

### S5 · 服务端按请求拉取任意 URL（无出网策略）—— SSRF
- **描述**：`BaseURL`/`APIKey`/`Model` 直接来自请求体（生图、TTS、embedding probe），校验仅要求 `http(s)://` 且无 userinfo；生图还会去拉取上游返回的 `item.URL`。
- **位置**：`internal/adapters/image/image.go:121-130,203-228,319-323`；`internal/adapters/tts/tts.go:232-240,310-318,354-365`；`internal/adapters/embedding/openai.go:55-71,204-216`；入口 `image_handler.go:10-33`、`tts_handler.go:29-59`、`embedding_handler.go:17-89`。
- **影响**：任意已配对客户端（或经 S4 的 DNS rebinding 页面）可让服务端访问内网/元数据地址（`http://169.254.169.254/…`、路由器管理页、localhost 管理 API），并通过错误文本读回响应（见 M15）。
- **建议**：将 `BaseURL` 限制到私网/回环白名单或"用户已确认的端点清单"；显式封禁 link-local/云元数据网段；错误信息绝不回显抓取到的响应体。

### S6 · 依赖漏洞：`golang.org/x/image v0.41.0` 的 WebP 解码内存耗尽
- **描述**：审查指出该版本命中 GO-2026-6222 / CVE-2026-46603（VP8L 解码在大量未使用 Huffman 树组时可过度分配），影响范围 `< v0.45.0`。
- **位置**：`go.mod:9`；可达路径 `internal/application/avatar.go:12`（`_ "golang.org/x/image/webp"`）与 `:43,49`（`image.DecodeConfig`/`image.Decode`）。
- **影响**：卡/charx 内嵌的构造 WebP 可在导入时耗尽服务内存（DoS）。现有像素尺寸预检（`avatarMaxSourcePixels`/`avatarMaxSourceEdge`）**不能**缓解该问题（问题在 Huffman 树数量而非像素数）。注意 `govulncheck` 可能因"空白导入注册格式"而判为 imported 而非 called，CI 通过并不等于安全。
- **建议**：升级到 `golang.org/x/image v0.45.0+`，随后跑 `govulncheck ./...` 复核（以工具结论为准，本文 CVE 号请以 `govulncheck`/官方公告核对）。

### S7 · 再分发第三方源码但无任何许可证文本
- **描述**：`reference-components/` 收录了来自 beautifului.dev / beui.dev / rareui.com / transitions.dev / ui.shadcn.com 的**完整源码**（383 个文件），目录内无 `LICENSE`/`NOTICE`；发布脚本会把所有跟踪文件打进 `source-clean.zip`。
- **位置**：`reference-components/`（`README.md:1-24` 自述来源）；`scripts/release.py:34-36,135-137`；`scripts/release_audit.py` 未覆盖组件许可证。
- **影响**：在"原创或具有明确授权"的仓库约束下，**合规/法律风险**；每次源码发布都在再分发无授权文本的第三方代码。
- **建议**：二选一——把 `reference-components/` 移出跟踪树（改为本地 gitignore 参考目录），或补齐每个上游的 LICENSE/归属并登记到 `docs/ASSET_LICENSES.md`；随后扩展 `release_audit.py`，要求每个 vendored 组件目录下存在许可证文件。

### S8 · Web SSE：handler 抛错在游标推进之后被吞，事件永久丢失并重连风暴
- **描述**：SSE 循环先把 `lastEventId = evt.id`（`:460`）推进，再调用 `handlers.onEvent(evt)`（`:463`）；`onEvent` 抛出的异常被同一个 `catch {}`（`:467`）当成传输失败吞掉，随后按已推进的 `Last-Event-ID` 重连，该事件永不重放。`block.delta` 在 `seq<=0` 时 `blocks[seq-1]` 为 `undefined`，`target.text` 抛 `TypeError`（`:188-193`），正是触发点；后端解析器在未见 `"seq":` 或字段乱序时确会产出 `seq==0`。
- **位置**：`web/src/app/api.ts:460,463,467-469,490`；`web/src/stores/slices/turnSlice.ts:188-193`（对照已正确守卫的 `:151`）。
- **影响**：一条畸形/异常的持久事件即可**静默丢弃已提交的回合或正文更新**，界面停留在旧状态并无限重连，极难诊断。
- **建议**：把 `handlers.onEvent` 放进独立的 `try/catch`：handler 出错时不推进该事件的 `lastEventId`（以便重放）或经 `onFinalError` 终止，绝不当作"连接断开"；同时按 `mergeInFlightDraft` 的方式归一化 `seq`（`payload.seq && payload.seq > 0 ? payload.seq : blocks.length + 1`）并防御 `target` 为空。补"handler 抛错"与"seq=0 delta"用例。

### S9 · Web `loadOlder` 加载的历史会被下一次 `reloadView` 丢弃
- **描述**：`loadOlder` 把更早的消息**前置**进 `messages`，但 `reloadView` 无条件用服务端默认窗口重建 `messages`（不含 `before`）。`reloadView` 由 `watchSession` 的 `session.updated`、`onFocus`/`onReconnect` 复活、提交处理等频繁触发。
- **位置**：`web/src/stores/slices/branchSlice.ts:171-173`；`web/src/stores/slices/turnSlice.ts:477`。
- **影响**：用户向上滚动加载历史后，任意后台刷新或标签页重新聚焦都会让已加载历史消失、滚动位置跳回——分页功能实际不可用。
- **建议**：把已加载的旧消息存为独立状态（`olderMessages` + `oldestTurnId`），`reloadView` 将新窗口与已加载历史按节点 id 去重合并，而非整体替换 `messages`。

### S10 · Android 在主线程做阻塞 TLS 握手（TOFU 引导会卡顿/失败）
- **描述**：`fetchPeerCertificate` 执行阻塞 `socket.connect` + `startHandshake`（8s 超时）；它由 `ConnectViewModel`（`viewModelScope` = `Dispatchers.Main.immediate`）经 `ConnectionRepository.check/trustCertificateAndCheck/trustCheck` 直接调用，**没有** `withContext(Dispatchers.IO)`（HTTP 探针在 `ApiClient.execute` 内部有卸载，证书路径没有）。
- **位置**：`android/.../data/remote/HttpEngine.kt:43-54`；`android/.../data/repo/ConnectionRepository.kt:104,120,131`（`check`/`trustCertificateAndCheck`/`trustCheck`）。
- **影响**：Android 上会抛 `NetworkOnMainThreadException`，或最长 8s 的 UI 冻结/ANR；表现为 HTTPS/TOFU 引导失败并报"读取服务器证书失败"，而 README 恰好把该流程作为卖点。
- **建议**：`suspend fun fetchPeerCertificate(...) = withContext(Dispatchers.IO) { … }`，或在 `trustCheck`/`trustCertificateAndCheck`/`fingerprintOf` 外层统一 `withContext(Dispatchers.IO)`。

### S11 · Android "加载更早的消息" 丢弃当前窗口（分页方向错误）
- **描述**：`loadMoreHistory()` 用 `before=oldestTurnId` 重新 `reload`，`applyView` 随后**替换** `state.messages` 为这个更旧的窗口，而不是前置。
- **位置**：`android/.../ui/screens/story/StoryViewModel.kt:559-563`（`loadMoreHistory`）、`:177-208`（`reload`）、`:210-255`（`applyView`）；接口 `CardAndSessionApis.kt:76`；契约 `android/docs/API_CONTRACT.md`（`before` 仅返回更早回合）。
- **影响**：点一次"加载更早的消息"，最近 100 回合从屏幕消失，用户观感等同"对话丢了"，且除整页重载外无路可回。
- **建议**：`before != null` 时保留增量：`messages = 新窗口 + 旧 messages`，并合并 `nodes`/`oldestTurnId`/`hasMore`；或改用更大 `limit`/绝对窗口重取。

### S12 · Android 历史只读标记加载后立即被清空（可误提交）
- **描述**：`viewAt()` 设置 `viewingNodeId` 后 `reload(full=true)`，`applyView(keepViewingNode = before != null)` 因 `before==null` 为 false，于是 `viewingNodeId` 被置空（`:243`）。经分支树进入历史节点同理（`open()` 设置后被 `reload` 清空）。
- **位置**：`android/.../ui/screens/story/StoryViewModel.kt:549-552,161,189-190,243`；消费方 `:78-80,330-333`；`StoryScreen.kt:384,419-434`；`StoryComposer.kt:121-124`。
- **影响**：`viewingHistory` 仅由 `viewingNodeId` 推导，清空后只读横幅/`ReadOnlyHistoryBar` 消失、`canSend` 变真，且最新节点会把选项当作 head 渲染。用户在浏览历史时以为在最新处并可提交——与"只读历史"契约冲突的正确性回归。
- **建议**：由请求而非"重置"推导：`viewingNodeId = if (before != null || snapshot.viewingNodeId != null) it.viewingNodeId else null`，或在 `applyView` 用 `view.viewNodeId.takeIf { it != view.branch.headNodeId }`。

### S13 · 视口禁用了缩放（低视力用户无法放大正文）
- **描述**：`<meta name="viewport" … maximum-scale=1.0, user-scalable=no …>`。
- **位置**：`web/index.html:5`。
- **影响**：手机上无法双指缩放/浏览器缩放叙事正文，违反 WCAG 2.1 SC 1.4.4；`responsive.css:261` 的 `16px` 输入字号说明团队已知 iOS 缩放问题，但全局禁用缩放惩罚了需要放大的用户。
- **建议**：移除 `maximum-scale`/`user-scalable=no`，保留 `initial-scale=1`；iOS 表单缩放继续用现有的输入 `font-size:16px` 局部控制；可加 `<meta name="color-scheme" content="light dark">`。

### S14 · 回合核心操作仅悬停可见，触屏不可达
- **描述**：`.turn-quick-toolbar` 默认 `opacity:0; pointer-events:none`，仅 `:hover`/`:focus-within` 显示；容器 `.dialogue-turn-wrap` 不可聚焦，触屏不触发 `:hover`。
- **位置**：`web/src/styles/stage.css:104,124-125`；使用方 `web/src/components/HistoricalStory.tsx:83,141`、`web/src/components/NarrativeStream.tsx:259`。
- **影响**：手机/平板上每条历史回合的朗读、引用、复制、生图等核心功能**静默消失**（开场白与实时草稿共用该工具栏）。
- **建议**：加 `@media (hover: none)/(pointer: coarse)` 分支常显工具栏（或改为常驻底栏行）；不要依赖悬停作为唯一入口。

### S15 · 三级文字令牌 `--text-dim` 在三种主题下均不满足 AA 对比度
- **描述**：浅色 `#9ca3af`（白底约 2.5:1）、深色 `#71727d`（约 3.2:1）、幻夜幽紫 `#6d657c`（约 3.1:1），却用于 10–12px 的提示、时间戳、空状态、菜单说明等。
- **位置**：`web/src/styles/tokens.css:90,233,297`；消费方如 `Field.css:19`、`EmptyState.css:35`、`Menu.css:138`、`stage.css:216`、`rail-left.css:122`、`rail-left-sessions.css:212`。
- **影响**：三种主题下次级信息对低视力用户几乎不可读，WCAG AA（正文 4.5:1）失败。
- **建议**：把 `--text-dim` 调整到 ≥4.5:1（如浅 `#6b7280`、深 `#8b8c96`、紫 `#8b8298`），或将其限定为纯装饰用途并新增合规的 `--text-faint`；在 CSS 棘轮中加对比度断言。

### S16 · 主故事输入框没有可访问名称
- **描述**：`<textarea id="story-input" placeholder={char.inputPlaceholder} …>` 无 `<label>`、无 `aria-label`、无 `aria-labelledby`。
- **位置**：`web/src/components/ComposerShelf.tsx:291`（同文件 `:264-276` 的注记输入框同样只有未关联的 `<span class="composer-note-label">`）。
- **影响**：屏幕阅读器无法可靠识别全应用最关键的控件；placeholder 不是标签且输入后消失，违反 WCAG 3.3.2/4.1.2。
- **建议**：加 `aria-label="故事输入"`（或视觉隐藏的 `<label htmlFor="story-input">`）；注记输入框用 `htmlFor`/`id` 关联标签。

### S17 · 视口/朝向变化后移动端抽屉可能永久保持打开
- **描述**：`isLeftRailCollapsed/isRightRailCollapsed` 在 store 创建时按 `window.innerWidth` 计算一次，全仓库无 `resize`/`orientationchange` 监听。宽屏加载（`collapsed=false`）后再变窄/旋转，≤768px 的 CSS 把侧栏变成离屏抽屉，`body:not(.rail-left-collapsed) .rail-left { transform: translateX(0) }` 会把它整片盖在正文上且背景遮罩生效。
- **位置**：`web/src/stores/uiStore.ts:164-165`；`web/src/styles/responsive.css:347-382`；`web/src/styles/mobile.css:53-60`。
- **影响**：手机/平板（或桌面缩放、分屏）出现全屏抽屉盖住故事，除找顶栏切换外无明显恢复路径。
- **建议**：订阅 `matchMedia('(max-width: 768px)')`/`resize`，跨越抽屉断点时自动折叠两侧栏（跨回时恢复），或让抽屉可见性由媒体查询驱动而非持久化 store 标志。

---

## 2. 🟡 中等问题

> 格式：**编号 标题** — 位置：`文件:行`。影响：…。修复：…

### 2.1 Go 后端

- **M1 上下文编译时存储错误静默降级为默认值** — 位置：`internal/context/compile.go:352-354,374-377,450-473,517-524`。影响：一次瞬时 DB 故障会静默改变生成语义（无摘要/无世界书/无秘密），最严重的是真实 ruleset 读取失败却回退 `domain.DefaultRuleset()` 而不失败本轮；调用方与用户均无感。修复：区分"未找到/为空"与"存储错误"，后者返回包装错误（与 `compileLedger` 的 fail-closed 一致）。
- **M2 `currentTurn` 回退为 0 跳过有效期过滤并钉死新近度** — 位置：`internal/context/compile.go:351-354`；`internal/context/memory_select.go:391-393,446-450`。影响：同 M1 的 `GetNode` 错误下，所有已过期（`ValidUntilTurn>0`）记忆仍被注入，且所有记忆得到最大新近度评分，陈旧/被推翻的事实可进入提示词。修复：传播错误；至少"未知轮次不 boost 新近度"。
- **M3 秘密宏展开进被丢弃的对象，持久化模板仍留占位符** — 位置：`internal/application/session_setup.go:93,109`；`internal/context/compile.go:491-511`。影响：解锁后进入模型的秘密文本仍含字面 `{{char}}`/`{{user}}`，看似执行的展开对秘密实为死代码。修复：持久化展开后的卡 JSON，或在加载时展开 `sc.Secrets`。
- **M4 规则前置条件读属性 0，而检定读属性 10** — 位置：`internal/application/check.go:97`（`ruleContextOf`）与 `:171-177`（返回 0），对照 `:105,132-145`（`attributeOf` 返回 `DefaultAttribute=10`）。影响：`attribute:npc_x/strength gte 10` 这类规则对未记录属性的角色失败，而同名检定却按 10 处理——前置与检定对同一事实不一致。修复：统一语义（全用 `attributeOf` 式默认，或明确要求作者记录属性）。
- **M5 O(n²) 预算拟合与压缩输入组装** — 位置：`internal/context/budget.go:82-87,118-125,136-176`；`internal/application/compactor.go:284-298`；`internal/context/compile.go:394-397` 与 `budget.go:87`（mandatory 计算两次）。影响：裁剪 200 轮历史或折叠长区间时反复全量重渲染/重估 token，位于回合准备热路径与后台路径。修复：增量累加消息体积或二分查找；mandatory 估算只算一次并传入 `fitBundleBudget`。
- **M6 导入 pack 的资源落地失败无诊断** — 位置：`internal/application/archive.go:124-131`（`_ = s.assets.SaveAsset(...)`）。影响：整包导入报告 `Assets: len(...)` 却可能全部落地失败（磁盘满/权限），与卡导入路径（`cardlibrary.go:86-95`）不同，无任何警告。修复：统计成功数并写入 `Warnings`。
- **M7 头像提取多候选各读至 64MB** — 位置：`internal/application/charx.go:236-254`；`internal/application/tacard.go:367-388`。影响：声明大量 `icon` 类型资源且均无法解码时，导入期反复 64MB 分配/解码，CPU/内存放大（charx/tacard 未像 `pack.MaxEntries` 那样限条目数）。修复：限制候选数（如前 8），或先读小前缀判图片魔数。
- **M8 导演命令重复校验（重放整条事件）** — 位置：`internal/application/director.go:234-240`。影响：正确性无碍，但每次 pause/resume/rewind/activate 都多一次导演状态计算，且 `ApplyDirectorEvent` 经 marshal/unmarshal 深拷贝（`domain/director.go:229-235`），非零成本。修复：可接受但记录；或让 store 暴露校验结果以免冗余克隆。
- **M9 `pack.Read` 全量驻留内存（含所有资源），上限 1GiB** — 位置：`internal/pack/pack.go:44-49,281-316,338-359`。影响：单次导入可占用约 1GiB Go 堆（外加请求体），本地单二进制上可致 OOM/DoS；资源在 `Import` 后仍与 `persistImportedAssets` 重复驻留。修复：资源流式写入资源存储/临时文件，降低或可配置总量上限。
- **M10 pack 资源名净化后静默碰撞** — 位置：`internal/pack/pack.go:230-238,246-265`。影响：两个不同 map key 净化后同名，`archive/zip` 允许重名，导出静默丢资源，manifest 不列资源故读取侧无完整性校验。修复：用后缀计数去重（如 `charx.uniqueAssetFile`）或直接报错。
- **M11 `ttsEnrichDialogue` 的 `[轻叹]` 插入在无前缀时是空操作** — 位置：`internal/application/ttsplan.go:599-603`（`ttsLeadingPrefixRe` 要求 `^([（(]…[）)])`）。影响：疑似与其声称移植的 TS 行为漂移——仅当已存在风格前缀时才标记叹息。修复：无前缀时补 `[轻叹]`，或收紧注释说明只在已有前缀后追加（参考 TS 文件不在本仓库，置信度中等）。
- **M12 摘要折叠错误把 nil 原因打印成 `%v` of nil** — 位置：`internal/context/summary_folding.go:26-29`。影响：`node==nil && err==nil` 时输出 `...: <nil>`，掩盖"源节点缺失"。修复：区分"节点缺失"与"存储失败"。
- **M13 `memory_change`/summary 节点内容导入时不校验** — 位置：`internal/application/archive_validation.go`（未检查 `NodeKindMemoryChange` 的 `ContentJSON`）。影响：与 S1 同根，任何节点负载形状都可经 pack 注入，之后才 panic 或静默跳过。修复：导入时按 `NodeKind` 校验 `ContentJSON` 可解析且匹配信封。
- **M14 `isLANHost` 依赖可伪造的 Host 头** — 位置：`internal/adapters/http/security.go:17-24,166-173`。影响：Host 检查不是网络边界，仅防 DNS rebinding；`AuthPIN==""` 的部署表面受限实则无保护。修复：明确其为 rebinding-only，并在非回环绑定时断言 PIN 非空（并入 S3）。
- **M15 图像供应商错误回显至 32MB 上游响应体** — 位置：`internal/adapters/image/image.go:169-177,284-291,469-472`；`internal/adapters/http/image_handler.go:33-36`。影响：错误端点/恶意端点的响应体（可能含私有数据、内部 HTML、被反射的 token）原样进入客户端可见错误与日志；TTS/embedding 已用 `bodySnippet`/`errorSnippet` 限幅，图像路径没有。修复：统一 `bodySnippet`（≤512 字符），绝不含原始响应体与端点 URL。
- **M16 配置读取路径可生成不确定的模型 ID，迁移失败被吞** — 位置：`internal/adapters/config/file.go:506-522,330-421`（`:411` 用 `"m_" + id.New()`）；`internal/app/app.go:124-128`（仅记日志继续）。影响：`MigrateLegacy` 无法持久化（磁盘满/权限/并发写）时，模型列表 ID 在 `Reload()` 间不稳定，槽位引用/UI 选择可能静默失效。修复：迁移持久化作为硬启动要求；读取路径不得生成随机 ID。
- **M17 `ExportSession` 在只读导出期间持有全局写锁** — 位置：`internal/adapters/sqlite/pack.go:20-27`。影响：已鉴权客户端反复导出即可阻塞所有回合提交/写入（配合无 `WriteTimeout` 与慢客户端是廉价的写饥饿 DoS）；读取本不需要写锁（已有 `s.rdb()`）。修复：在只读连接池上以读事务/一致快照导出，不持 `s.mu`。
- **M18 图像服务回退硬编码 `data/images`，逃逸出配置的数据目录** — 位置：`internal/adapters/http/server.go:118-121`；`image_handler.go:23-31,50-58`；`app.go:182-185`。影响：图像服务初始化失败后，服务会静默把生成图写到进程 CWD 下的 `./data/images`（不被备份、不随故事删除、可能不可写），且回退错误被 `_ =` 吞。修复：把 `DataDir` 传入 `http.Deps`，回退基于它构造；失败返回 503 而非静默改址。
- **M19 环境变量来源的脱敏密钥被当成"新密钥"拒绝** — 位置：`internal/adapters/config/file.go:591-602`（对照 catalog 脱敏 `125-186`、HTTP 回显 `models.go:93-112`）。影响：`TAVERNAGENT_KEY_*` 解析出的密钥以 `sk-1…abcd` 脱敏返回；客户端原样 PUT 回来（常见读-改-写）会因 `strings.Contains(key,"…")` 报"脱敏凭据不能作为新密钥"，导致改名称/BaseURL/上限都失败。修复：把等于 `MaskAPIKey(resolvedKey)` 的值一律视为"未变更"，与来源无关。
- **M20 单请求 2GiB 上传上限，无并发/速率限制** — 位置：`internal/adapters/http/request_body.go:16-28`；`cards.go:139-198`。影响：回环（免鉴权）或已配对客户端可并发灌满系统临时盘；无按 IP 并发门或总量预算。修复：降低上限或加全局并发/字节预算与 `Retry-After` 背压；清理 panic 路径残留临时文件。
- **M21 `CommitMemoryBatch` 忽略分支 CAS 的 `RowsAffected` 并无条件 +1 版本** — 位置：`internal/adapters/sqlite/memorybatch.go:183-186`（对照 `store_turn.go:446-453`）。影响：当前 `s.mu` + 单写连接使 CAS 不会失败，属潜在正确性缺口——WHERE 一旦失配，调用方仍收到 `Committed:true` 与错误 head/version。修复：检查 `res.RowsAffected()==1`，否则返回 `HEAD_CONFLICT`。
- **M22 导演 activate 更新草稿但不校验存在性** — 位置：`internal/adapters/sqlite/director.go:293-301`（无 `RowsAffected` 检查）。影响：分支无草稿行时，变更作为事件提交而草稿从未更新，事件中的 `c.Change.Plan` 与 `director_drafts` 漂移，后续版本校验基于陈旧草稿。修复：校验草稿存在（或 `INSERT … ON CONFLICT`），无目标草稿则命令失败。
- **M23 `checkBaseHashTx` 在无投影时静默跳过 CAS 校验** — 位置：`internal/adapters/sqlite/store_turn.go:492-508`（`sql.ErrNoRows` 直接 `return nil`）。影响：任一节点缺可达 checkpoint（稀疏/导入数据、写入中断）时，基于陈旧 head 状态的提交被接受而无 `STATE_MISMATCH` 防护。修复：提交期把"无投影"视为硬错误或重建基线后 fail closed。
- **M24 `ensureMemoryIndex` 可阻断 `Open`，违背"仅降级"契约** — 位置：`internal/adapters/sqlite/store.go:74-78`；`memoryindex.go:47-64,78-100`。影响：可选加速器的瞬时 DB 错误会让整个故事库打不开，与"失败必须只降级不阻断开库"矛盾。修复：backfill 失败记日志并 `fts=false`，区分"FTS 不可用"与"数据库损坏"。
- **M25 SSE 轮询失败时静默关闭；每连接 1Hz reconcile 查询** — 位置：`internal/adapters/http/sse.go:63-70,89-114`。影响：`drain()` 在 `bus.Poll` 出错时返回 false，`eventStream` 直接 `return` 无终态事件，客户端无法区分瞬时错误与正常结束，可能挂死/无限重连；每连接每秒一次 `PollOutbox`。修复：poll 失败先发终态 `event: error`/`retry:` 再关闭；以"通知驱动 + 较慢（如 30s）安全轮询"替代固定 1s。
- **M26 `probeModel` 在 `ContentLength<=0` 时跳过请求体** — 位置：`internal/adapters/http/models.go:130-137`。影响：chunked 请求（`ContentLength==-1`）的 body 被静默忽略，`req.Format` 默认 false，探测行为与请求不符且无错误。修复：始终 `decodeJSON`（容忍 `io.EOF`）或明确拒绝 chunked。
- **M27 OpenAI SSE 解析器未实现多行 `data:` 拼接** — 位置：`internal/adapters/providers/openai/openai.go:334-349`（`responses.go:202-212`、`anthropic.go:242-252` 同）。影响：合规但少见的"网关把 JSON 跨多行 `data:`"会被解析成无法解析的碎片并静默跳过，可能以 `ErrTruncated`/空流收尾。修复：累积 `data:` 行至空行再派发。
- **M28 流读取 goroutine 在禁用总超时时可能泄漏** — 位置：`internal/adapters/providers/openai/openai.go:141-146,175-201`；`responses.go:141-146`；`anthropic.go:178-183`。影响：`TimeoutTotal<=0`（或绕过 `New` 直接构造）时 `cancel=func(){}`；sink 出错提前返回后，阻塞在满 channel 发送的 goroutine 因无 ctx 取消而**终生泄漏**。修复：无条件 `defer cancel()`（即便总超时关闭也派生 ctx），或让 goroutine 始终 `select ctx.Done()`。
- **M29 记忆投影在热召回读路径上写库** — 位置：`internal/adapters/sqlite/memory_projection.go:68-80`（`RecallMemories` 路径 `memoryindex.go:346`）。影响：读操作在单写连接上不加 `s.mu` 地写，与写事务串行化并给最热路径加写放大。修复：在写/提交路径或后台维护物化投影，保持读路径只读。
- **M30 `DeleteSession` 把 `awaiting_continuation` 当非忙碌** — 位置：`internal/adapters/sqlite/session_delete.go:49-58`。影响：`awaiting_continuation` 的回合仍持分支锁且可续写（`store_turn.go:101-110`），此时可删会话，续写随后对缺失行失败，产生困惑错误与可能的孤儿 outbox。修复：视为忙碌，或删除前取消挂起回合。

### 2.2 Web 前端

- **M31 前端 30s 硬取消与后端 120s 不一致** — 位置：`web/src/app/api.ts:77,107,599-600`；后端 `internal/adapters/image/image.go:62-64`。影响：生图/长 TTS 常超 30s，前端报"请求超时"并中止，而后端仍在工作并持久化结果；重试会（无幂等键）产生重复图片/语音。修复：为 `http`/`binaryRequest` 增加按调用可配的 `timeoutMs`，与后端最坏情况对齐。
- **M32 供应商密钥与鉴权 token 明文存 localStorage** — 位置：`ttsSettingsStore.ts:10,49,64-66`；`imageSettingsStore.ts:11,54,108-110`；`embeddingSettingsStore.ts:10,50,100-102`；`app/api.ts:31-51`。影响：三处 `persist(...)` 无 `partialize`，`customApiKey`/`apiKey` 原样落盘；配合 token，一旦将来出现 XSS/恶意扩展/共享机器即可一次性窃取全部凭据。修复：密钥仅存内存/sessionStorage，或迁到服务端（后端已有 `data/config/secrets.json`）；至少 `partialize` 掉密钥字段并记录风险。
- **M33 `cardAssetStore` 缓存无界增长，且一次瞬时失败被永久缓存** — 位置：`web/src/stores/cardAssetStore.ts:11-65`。影响：`byCard`/`typedByCard`/`themeByCard`/`sdDataByCard`/`attempted` 从不清理；`ensure` 在请求前即置 `attempted=true`，网络错误写入空表后本会话不再重试——卡的头像/情绪/主题永久空白且无重试入口。修复：LRU 限容；失败时移除 `attempted[cardId]`（或存错误态）并提供 `retry(cardId)`。
- **M34 `http()` 仅在 JSON `AUTH_REQUIRED` 时触发重新配对** — 位置：`web/src/app/api.ts:117-128`（对照 binary `:78-81`、SSE `:428-432` 对任意 401 都通知）。影响：服务端返回非 JSON 的 401（代理/网关）时 REST 路径不弹重新配对，用户只看到反复失败。修复：`http` 对所有 401 调用 `notifyAuthRequired()`。
- **M35 `associateSessionWithAvatar` 近乎每次渲染都重写整份头像映射** — 位置：`web/src/lib/characterCardStore.ts:323-359`（守卫 `:337-340` 只对 map 首项短路）；调用方 `web/src/lib/characterPresentation.ts:74-75`（8 个组件渲染期调用）。影响：当前会话不是首个 key 时，每次调用都把最多 40 项（含 base64 data URL）序列化并写 localStorage，流式渲染期造成卡顿与潜在 `QuotaExceededError`；注释声称的优化只对首项生效。修复：无论位置都先判 `map[sessionId]===avatar`，仅在值变化时写入；更应从会话加载处关联而非渲染期。
- **M36 `loadSessions` 无错误态，可能未处理拒绝** — 位置：`web/src/stores/slices/sessionSlice.ts:28-32`。影响：启动失败时故事列表为空且无错误/重试，与"你没有故事"不可区分；直接调用方可能产生未处理拒绝。修复：加 `sessionsError` + loading 标志并在列表面板呈现重试。
- **M37 `discussDirector`/`cancelDirector` 绕过忙碌守卫** — 位置：`web/src/stores/slices/directorSlice.ts:154-188`（对照 `commandDirector` `:133`）。影响：生成中仍可派发导演讨论（对 `expectedHeadId` 的模型写入），与 head/version 竞争产生 409 或不一致的导演草稿视图。修复：套用相同的 `isTurnBusy`/`directorCommanding` 守卫并禁用按钮。
- **M38 `navigateView` 回滚丢失草稿态与 `lastTurnId`** — 位置：`web/src/stores/slices/branchSlice.ts:23-41`。影响：分支切换/查看节点失败时恢复了 `phase` 但 `draftBlocks`/`draftOptions`/`lastTurnId` 未恢复，生成 UI 空白直到流对账。修复：把 `draftBlocks`/`draftOptions`/`lastTurnId`/`hasMoreHistory`/`oldestTurnId` 一并纳入快照回滚。
- **M39 `branchBusy` 在 `finally` 无条件清除，与更新操作竞争** — 位置：`web/src/stores/slices/branchSlice.ts:131-134,148-150`。影响：单全局标志下，A 的迟到 `finally` 可能提前释放 B 的忙锁，快速交互下出现重复 fork/edit 提交。修复：用单调 `branchOpSeq`（如 `reloadSeq`）确保只有最新操作可清除。
- **M40 `imageSettingsStore` 持久化无界的插画映射；各持久化 store 缺 `version`/`migrate`** — 位置：`imageSettingsStore.ts:17-18,46-72,108-110`。影响：`turnIllustrations` 随每次生图增长并入库，最终挤占 `characterCardStore` 的配额；形状变更无法迁移。修复：`partialize` 掉 `turnIllustrations`（可由服务端资源 URL 重建）或限容；为所有持久化 store 加 `version`/`migrate`。
- **M41 `getTTSVoices` 吞掉一切失败** — 位置：`web/src/app/api.ts:542-549`。影响：网络/鉴权失败时音色选择器静默为空，与"没有音色"不可区分，用户无法选音色也无解释。修复：让错误上抛或返回 `{voices,error}`，由设置页展示重试态。
- **M42 模块级 `pendingSubmission` 在导航时不清除** — 位置：`web/src/stores/slices/turnSlice.ts:249-278`。影响：5xx/超时后残留；导航（`beginNavigation`/`emptyStory`）不清理，`owner` 陈旧时 `retry` 可能读到陈旧状态（虽被 `owns` 挡住），状态难推理、测试需间接重置。修复：`beginNavigation` 中清除，或移入 per-scope 对象。
- **M43 `uiStore` 即发即弃的定时器与永不移除的 `matchMedia` 监听** — 位置：`web/src/stores/uiStore.ts:296-305,323-330,354-362`。影响：定时器在 store 拆除后仍可能触发；Vite HMR 下 `matchMedia` 监听累积，拆除后 `setState` 告警。修复：记录并清除定时器；监听注册进 store action 并提供拆除，或用 `import.meta.hot.dispose` 防重复。
- **M44 `thinkingStartedAt`/thinking 映射对失败/取消回合泄漏** — 位置：`web/src/stores/slices/turnSlice.ts:31,171,232-245,467`。影响：仅在 `reloadView` 的 completed 分支删除；`turn.cancelled/failed/conflicted` 分支从不删除，`turnThinking`/`turnThinkingSeconds` 按 turnId 终生增长。修复：所有终态分支删除 `thinkingStartedAt[turnId]`；thinking 映射限容（保留最近 N）或会话关闭时清空。
- **M45 超大 SSE 块卡死流且无法前进** — 位置：`web/src/app/api.ts:446`（`if (buf.length > 1024*1024) throw new Error("SSE event too large")`）。影响：>1MB 的单事件（重思考/持久帧）会丢弃缓冲并从同一 `lastEventId` 重连，可无限循环。修复：溢出时若已知事件 id 则推进游标越过该事件（或 `onFinalError`），并记录事件类型。

### 2.3 Android 客户端

- **M46 每次 reload 在主线程解码所有回合 `contentJson` + 世界状态** — 位置：`StoryViewModel.kt:210-212`；`StoryModels.kt:55-66,89-111`；`CardAndStoryDtos.kt:213`。影响：`view.nodes` 可达 100 个，每个 `contentJson` 与状态 JSON 都在 UI 线程急切解析，长会话/大载荷下每次 reload、提交、切分支、跳历史都卡顿甚至 ANR。修复：在 `withContext(Dispatchers.Default)` 内解析，仅把派生结果回主线程。
- **M47 失败回合的"重试"是取消+重订阅而非重新生成** — 位置：`StoryViewModel.kt:419-466`（`:457-462`）；`StoryComposer.kt:396-410`。影响：provider 失败后回合已终态，`cancelTurn` 是空操作，`startStream` 重放同一终态回合立即再次 `Failed`；错误卡上的主恢复按钮空转再失败。修复：终态失败时重发新回合/重生成（新幂等键），保留 `pendingInput` 正是为此。
- **M48 `reloadAfterCommit` 可能覆盖更新的在途回合** — 位置：`StoryViewModel.kt:602-626`。影响：该协程在提交后先做网络 `view()` 再重置状态，未校验 `turnId` 仍是活动回合；若用户取消并已发新消息，重置后 `currentTurnId` 变 null，STOP 按钮 `cancel()` 提前返回，前台保活服务停止。修复：重置前判 `if (_state.value.currentTurnId == turnId)`。
- **M49 `inlineMarkdown` 每次重组重建 3 个 Regex 并全量扫描** — 位置：`StoryContent.kt:673-707`；调用点 `:262,551`（无 `remember`，流式草稿每个 delta 重组）。影响：每段每帧分配+扫描，长回复卡顿与 GC 压力。修复：三个 Regex 提升为顶层常量，调用点 `remember(text) { inlineMarkdown(text) }`。
- **M50 每个 `ThinkingBlock` 都跑无限动画（含历史/折叠）** — 位置：`StoryContent.kt:558-568`（用于 `:124-130,316-320,530`）。影响：`rememberInfiniteTransition` 无视 `streaming` 恒运行，非流式时 alpha 被丢弃；长历史下持续调度帧与重组，耗电与卡顿。修复：仅 `streaming` 时创建/收集动画，否则静态 alpha。
- **M51 `stopSelfSoon()` 在新回合开始时未取消** — 位置：`GenerationService.kt:78-112,64-75`。影响：终态后计划 1.5s 停止；若用户 1.5s 内续写/重生成（如截断卡上的"续写"），前台服务在生成中途停止，失去后台保活且通知状态不一致。修复：保存 `stopJob` 并在 `onStartCommand`/`observeTurn` 顶部取消。
- **M52 完成通知重复且通知渠道冲突** — 位置：`GenerationService.kt:95-100,145-154,197-226`；`TavernApp.kt:23-27`；`AppContainer.kt:37-43`。影响：同一 `COMMITTED` 由 Service（id `0x7A11`）与 `NotificationPresenter`（`0x7A12`）各发一条；同一渠道 id 两处以不同 importance（LOW vs DEFAULT）创建，谁先谁赢，可能导致进行中渠道发声/横幅；前台时也发通知。修复：完成通知单一 owner，渠道在 `Application.onCreate` 创建一次，活动可见时抑制。
- **M53 `TurnStreamRegistry` 的 `states`/`jobs` 对致命流结束不清理；thinking 映射无界** — 位置：`TurnStreamRegistry.kt:68-69,111-116,227-242`；`StoryViewModel.kt:143-144`。影响：`forget` 仅在 `COMMITTED` 路径调用，致命 4xx 路径置 `Disconnected/FAILED` 却不移除条目；长进程多次失败回合使注册表与思考文本缓存缓慢无界增长。修复：注册表在终态事件时驱逐；thinking 映射 `LruCache`/限容。
- **M54 卡头像每行触发一次整卡 GET** — 位置：`CardsRepository.kt:36-51,90-109`；`CardsViewModel.kt:94-106`；`ChatsViewModel.kt:107-138`。影响：打开角色/聊天页产生 O(卡数+会话数) 请求，每个卡请求下载完整规范化卡 JSON（可能 MB 级）+ base64 头像，只为画 56dp 缩略图；`ChatsViewModel.fillPreviews` 还会每会话发一次 `view(limit=1)`。修复：优先用列表接口已返回的 `CharacterCardEntryDto.avatar`，或新增轻量 assets 端点；批量解析；预览分页（`PREVIEW_LIMIT=12` 会让更旧会话永久显示"还没有消息"，误导）。
- **M55 SAF 导入无尺寸上限，整文件读入 ByteArray** — 位置：`FilePickers.kt:12-17`（`openInputStream(uri)?.use { it.readBytes() }`）；入口 `CardsScreen.kt:105`、`ArchiveScreen.kt:109` 接受 `*/*`。影响：误选大文件即 `OutOfMemoryError` 崩溃；`ApiClient.upload` 再建一份请求体副本，峰值双份。修复：先查 `OpenableColumns.SIZE`，超客户端上限（如 8–64MiB）拒绝；或自定义 `RequestBody` 流式上传。
- **M56 归档导出绑定 `rememberCoroutineScope`，会留下半截文件** — 位置：`ArchiveScreen.kt:57,63-80`。影响：导航离开/配置变化取消协程，写入中途中断，用户以为成功（或误报失败）。修复：把导出+写入放到生命周期长于 Composable 的作用域（前台服务/WorkManager），写完再上报成功。
- **M57 配对 token 与 TOFU 信任材料明文存储，且允许备份 + 全局明文流量** — 位置：`ServerStore.kt:13-53`；`AndroidManifest.xml:12,17`（`allowBackup="true"`、`usesCleartextTraffic="true"`）。影响：Bearer token（局域网完整授权）明文入库并随云/`adb backup` 外带；全局明文允许 token 被发往**任意**主机，地址填错/被控即泄漏。修复：用 `EncryptedSharedPreferences`/Keystore 包装；`allowBackup=false` 或 `dataExtractionRules` 排除该 store；用 `networkSecurityConfig` 仅对已配置主机/内网放行明文。
- **M58 触控目标与语义低于可访问性下限** — 位置：`StoryContent.kt:352-359,361-372,811-822`；`StoryAvatar.kt:43-54`；`StoryScreen.kt:175-179`。影响：每条消息的朗读/引用/重演/分叉/回溯等操作仅约 20–28dp，低于 48dp 最小目标且无 role 公告，运动障碍用户易误触。修复：`sizeIn(minWidth=48.dp,minHeight=48.dp)` + `role=Role.Button` 与合并语义。
- **M59 记忆修订幂等键每次调用重新生成（不重试复用）** — 位置：`MemoryRepository.kt:37-47`；`MemoryViewModel.kt:157-185`。影响：超时/"结果未知"的记忆修订会用新键再建一条 CoW 修订，而非安全重放；`load` 合并游标页时也未防搜索/筛选变更与在途页竞争。修复：在意图时生成键并跨重试复用（照回合路径）；页请求带 generation 计数丢弃过期响应。
- **M60 导演讨论静默放弃且从不使用 SSE 端点** — 位置：`DirectorViewModel.kt:202-218`；`DirectorRepository.kt:90-92`（`events` 从不调用）。影响：约 4 分钟轮询后 UI 静默退出"导演正在思考…"，无结果也无错误；花 120 次请求而非一条 SSE。修复：订阅导演 SSE（复用退避模式），耗尽时给出显式超时消息。
- **M61 回合重试未复用原始乐观并发基线** — 位置：`StoryViewModel.kt:371-382,419-446`。影响：重试从**实时**状态构建 accept 体；契约的幂等载荷哈希覆盖 `input+mode+expectedHeadId`，head/version 变动时同键不同值 → `409 IDEMPOTENCY_CONFLICT`，重试可能假失败并清键。修复：在意图时把 `(key,input,expectedHeadId,expectedVersion)` 一起捕获，重试原样重发。
- **M62 `runCatching` 吞掉 `CancellationException`** — 位置：`StoryViewModel.kt:181,195,373,426`；`ConnectionRepository.kt:64-75`；`TurnStreamRegistry.kt:192`。影响：取消的加载仍执行 `onFailure`/状态更新且不传播，破坏结构化并发，允许陈旧写入 ViewModel 状态。修复：`catch (e: Throwable) { if (e is CancellationException) throw e; … }`。
- **M63 非致命流错误无限重试且失败细节丢失** — 位置：`TurnStreamRegistry.kt:229-242`；`TurnEvents.kt:71-80`。影响：非 4xx 错误无限退避重试，无逃生口/健康信号；轮询兜底把 `failureCode`/`failureMessage` 丢成通用"生成失败"，丢失可操作的服务端码。修复：设重试上限并给出"连接不稳定，点击重试"；把 `failureCode`/`failureMessage` 传入 `TurnEvent.Failed`。

### 2.4 UI / UX 与可访问性

- **M64 模态不锁背景滚动、不置背景为 inert** — 位置：`web/src/ui/Modal.tsx:64-108`；`web/src/lib/useDialogFocus.ts:1-39`。影响：`useDialogFocus` 设了 `role=dialog`/`aria-modal` 并陷阱 Tab，但未对兄弟内容设 `inert`/`aria-hidden`，全仓库无 `body` 滚动锁；辅助技术仍可达背景控件，iOS 上背景可滚动。修复：打开时对 `.app-root` 兄弟设 `inert`+`aria-hidden` 并加滚动锁；让 `DirectorWorkspace` 复用同一 helper。
- **M65 Tabs 用了 `role="tab"` 但无 panel/`aria-controls`** — 位置：`web/src/ui/Tabs.tsx:113-136`；消费方 `SettingsModal.tsx:50`、`DossierModal.tsx:88`、`LeftRail.tsx:208`、`TopHeader.tsx:74`。影响：读屏播报"tab"却无法关联内容，方向键模型暗示的 panel 关系不存在。修复：给 `Tabs` 增加 `panelId`/`renderPanel` 以输出 `role=tabpanel aria-labelledby`，或改称"分段控件"用 `role=radiogroup`/`aria-pressed`。
- **M66 可交互行在 `role="button"` 内嵌套按钮** — 位置：`LeftRailSessionsPanel.tsx:134-152,197`；`LeftRailTreePanel.tsx:85-99,139-228`；`CharacterCardRow.tsx:45-65,87`。影响：ARIA 禁止 `button` role 内有交互后代，许多 AT 会把子树扁平为展示文本，删除/分支控件对读屏不可见或不可达。修复：去掉包裹层的 `role="button"`，主操作用真实 `<button class="row-main">` 与尾部操作并列（`ui/ListRow.tsx` 已是此模式）——迁移到 `ListRow`。
- **M67 可点击卡片/槽位无法键盘到达** — 位置：`web/src/components/RightRail.tsx:359-373`（物品槽 `div` + onClick）；`LeftRail.tsx:299-319`（世界书条目 `div` + onClick）。影响：键盘/开关设备无法打开物品操作或世界书条目，违反 WCAG 2.1.1。修复：改为 `<button type="button">`（或 `role=button tabIndex=0` + Enter/Space）。
- **M68 移动抽屉无键盘关闭、背景不可访问** — 位置：`App.tsx:189-195`；`responsive.css:318-320`；`mobile.css:39-51`。影响：`.mobile-drawer-backdrop` 是无 role/标签的裸 `div onClick`；≤768px 时抽屉自身关闭手柄 `display:none`，App 的 Escape 链不关抽屉——键盘用户只能靠再点顶栏按钮关闭，读屏拿到无标签可点区域。修复：Escape 关闭抽屉；背景 `role=presentation`（或标注"关闭导航"的按钮）；移动端保留可见关闭控件。
- **M69 两套 toast，设计系统版是死代码** — 位置：`App.tsx:219-223`（自绘 `.quiet-toast-notification`）对照 `web/src/ui/Toast.tsx`（含 tone/关闭/堆叠/`aria-live`，仅测试使用）。影响：线上 toast 无成功/警告/错误色调、不可关闭、新消息立即替换旧消息（快速操作丢反馈），且 `.quiet-toast-notification` 的 `z-index:9999`（`toast.css:19`）高于 `--z-modal:1000`，会盖住模态。修复：把 `notifyQuiet` 接到小型 toast store 并渲染 `ToastStack`，失败消息映射 `danger/warning`；删除自绘 CSS 或降为兼容别名。
- **M70 触控目标在设计系统层面低于 44px** — 位置：`ui/Button.css:28-38,149-156`（md 28px、sm 24px、icon 32/25px）；`ui/Tabs.css:95-99`（sm tab 22px）；`ui/Menu.css:75-93`（item ≈30px）；`styles/composer.css:175-181,334-337`。影响：`mobile.css:62-95` 只补了顶栏/发送/子标签，对话框、侧栏、菜单、回合操作、设置表单仍是 22–32px，手机易误触（WCAG 2.5.5/2.5.8）。修复：在 `@media (pointer: coarse)` 下把原语最小高度提到 44px，而非逐组件覆盖。
- **M71 `prefers-reduced-motion` 未覆盖循环动画与硬编码过渡** — 位置：`rail-left.css:200`、`composer.css:143`、`stage.css:355,448,488,493`、`rail-left-sessions.css:198`（无限动画）；`shell.css:17,76-79,109`、`TopHeader.css:29`（硬编码时长）。影响：仅有少数文件定义了 reduced-motion 规则，脉冲状态点、呼吸思考图标、闪烁光标与侧栏滑动仍运行，前庭敏感用户受影响（WCAG 2.3.3）。修复：把时长令牌化，或加全局 `@media (prefers-reduced-motion: reduce) { *,*::before,*::after { animation-duration/iteration-count … } }`，至少让装饰性无限循环 `animation:none`。
- **M72 字号体系被大量字面 px 绕过** — 位置（示例）：`shell.css:29,44,211,235,257,304`；`rail-right.css:104,263,301,308,370`；`rail-right-dossier.css:22-24,42,87,300`；`stage.css:68,199,208,289`；`rail-left.css:253,263,302`；`rail-left-sessions.css:160,170,226`。影响：令牌只有 5 档，代码却用了 9–22px 十余种值，同一概念标签在不同文件 9.5–12.5px 不等，视觉层级漂移且改令牌不传播。修复：在 `tokens.css` 增加中间档（或 4 档模块化音阶）并迁移；扩展 `scripts/check.mjs` 检测 token 外的裸 `font-size`。
- **M73 回合操作条存在永不匹配的死选择器** — 位置：`web/src/components/TurnActions.css:151-159`（`.btn-quiet[title="新建候选分支，重新掷骰并演绎新结果"]`）对照 `TurnActions.tsx:178`（实际 title 已改文案）。影响："重新掷骰"失去预期的强调样式，与重新生成/改正文视觉无差别。修复：用稳定 class（如 `.btn-recheck`）而非按用户可见文案匹配。
- **M74 示例世界书所有卡片打开同一个无关模态** — 位置：`web/src/components/LeftRail.tsx:299-304`（每个 `.lore-entry-card` 都 `setWorldbookModalOpen(true)`）。影响：点具体示例条目（如"灰烬商队"）却打开全局"世界书示例"模态，用户不知点击是否生效、也看不到想要的那条。修复：传入 `initialEntryId` 预选/滚动到该条，或内联渲染详情。
- **M75 全局快捷键可造成模态叠模态，两个 `aria-modal` 并存** — 位置：`App.tsx:122-124`（`Ctrl/Cmd+,` 无条件开设置）配合 `HistoricalStory.tsx:269-278`（IllustrationModal）与右栏模态。影响：在已有模态上再挂一个 `aria-modal` 对话框，两个焦点陷阱同时生效、下层仍在可访问树，堆叠/遮罩/读屏上下文混乱。修复：全局快捷键在"无对话框"时才生效，或引入模态栈管理器只让最上层为模态。
- **M76 幻夜幽紫主按钮文字对比度 4.2:1（低于 AA）** — 位置：`tokens.css:299-300`（`--accent-solid:#8b5cf6` + `--accent-contrast:#ffffff`）；消费方 `Button.css:59-63`、`rail-left-sessions.css:267-277`、`composer.css:395-406`。影响：仅紫主题下主操作（发送/保存/连接）的 11.5–13px 常规字重标签略低于 4.5:1。修复：文字承载面用更深的紫（如 `#7c4ddb`）或更深的对比色；加三主题令牌级对比度测试。
- **M77 叙事/剧情图加载态未播报** — 位置：`NarrativeStream.tsx:232-254`（骨架无 `role=status`/`aria-busy`）；`StoryMapModal.tsx:118`（`已加载 N 个节点`）。影响：切换会话时读屏只感到"卡住"；同仓库 `LorebookPanel.tsx:46`、`MemoryBankModal.tsx:228` 已正确使用 `role=status`，行为不一致。修复：加载区加 `role="status" aria-live="polite"` + 视觉隐藏"正在载入故事…"，容器加 `aria-busy`。
- **M78 菜单绝对定位且可被裁切** — 位置：`ui/Menu.tsx:95-107`；`Menu.css:3-42`；`ModelQuickSwitch.css:2-6`；`ThinkingEffortControl.css:2-6`；父级 `stage.css:8-15`（`overflow:hidden`）与 `shell.css:102-111`。影响：`.ui-menu` 未 portal，会被任意 `overflow:hidden` 祖先裁切；矮屏（横屏手机）向上弹出的模型/强度菜单顶部被切且无法滚动到。修复：portal 到 `document.body` 并按视口定位（`Popover` 原语已这么做），或按触发器上方实测空间限制高度。
- **M79 表单/主输入用 placeholder 代替标签** — 位置：`ComposerShelf.tsx:294,268`；`LeftRail.tsx:283`（回退世界书搜索无 `aria-label`）。影响：同一"搜索世界书"控件在会话模式（`LorebookPanel.tsx:32` 有标签）被播报、在回退视图不被播报，注记输入无标签。修复：补 `aria-label`，用 `htmlFor`/`id` 关联注记标签。

### 2.5 依赖与 CI / 工具链

- **M80 Android 完全没有 CI 覆盖** — 位置：`.github/workflows/`（仅 `quality.yml`、`model-eval.yml`，无 Android）。影响：整个 Kotlin/Compose 客户端与 REST+SSE 契约可在任何提交上静默回归；接口漂移无门禁。修复：加 `android` job（ubuntu + JDK 17 + `setup-java` + `setup-gradle` 缓存）跑 `:app:testDebugUnitTest` 与 `:app:assembleDebug`，按 `android/**` 变更触发。
- **M81 ESLint 9.39.1 已 EOL** — 位置：`web/package.json:30`。影响：linter 无安全/缺陷修复（9.x 于 2026-08-06 EOL），且低于最终 9.x 补丁；无该版本自身 CVE。修复：迁 ESLint 10.x，并把 `typescript-eslint`（当前 8.50.0 只声明支持 `^8.57||^9`）升到支持 10 的版本。
- **M82 Vite/Vitest/Playwright/jsdom/TypeScript 版本漂移** — 位置：`web/package.json:24,32,33,35,36`（vite `^7.0.0`→lock 7.3.6、vitest 5.0.0、@playwright/test 1.56.1、jsdom 30.0.1、typescript 5.8.2）。影响：落后于修复与性能改进；Playwright 1.56.1 捆绑的 Chromium 已多版陈旧（仅测试用，但缺浏览器安全更新）；`showcase/package.json` 同样 `vite ^7.0.0` 造成漂移重复。修复：规划 Vite 8（Rolldown）迁移；升 vitest 5.0.2、Playwright 最新 1.6x、jsdom 30.1.1；TS 升级需核对 `typescript-eslint` peer 范围（版本请以 npm 为准）。
- **M83 Go/Android 依赖漂移（无已知 CVE）** — 位置：`go.mod:11`（`modernc.org/sqlite v1.58.0`，最新 1.59.0）；`android/gradle/libs.versions.toml:2,3,4,7,8,10,13`（AGP 8.13.2、Kotlin 2.3.21、Compose BOM 2026.06.00、lifecycle 2.9.4、navigation 2.9.0、**OkHttp 4.12.0**、**Coil 2.7.0**）；`gradle-wrapper.properties:3`（Gradle 8.14.4）。影响：OkHttp 4 / Coil 2 已是旧大版本（5.x / 3.x 为当前），`material-icons-extended 1.7.8` 上游趋于冻结；AGP 9 / Gradle 9 是大版本迁移。修复：sqlite→≥1.59.0；OkHttp→5.x（核对拦截器兼容）；Coil→3.x（包名变更 + `coil-network-okhttp`）；排期 AGP 9 + Gradle 9 + Compose BOM 刷新；navigation→≥2.10.2。
- **M84 `THIRD_PARTY_NOTICES.md`/`LICENSES/` 陈旧且无门禁** — 位置：`THIRD_PARTY_NOTICES.md`、`LICENSES/`、`scripts/license_notices.py`（未被任何 workflow/脚本引用）。影响：通知列出了已不在依赖图里的组件（over-inclusive），且无门禁防止新依赖缺许可证。修复：重跑 `license_notices.py` 并提交结果 + 裁剪 `LICENSES/`；CI 增加"重新生成后 diff 为空"步骤。
- **M85 CI 无依赖更新自动化 / SAST** — 位置：`.github/`（无 `dependabot.yml`/`renovate.json`/CodeQL）；`.golangci.yml:3-10` 未启用 `gosec`。影响：S6/M81 这类 EOL/漏洞 pin 可长期潜伏；PR 期无依赖 diff 审查。修复：加 `.github/dependabot.yml`（gomod、npm `/web`、gradle `/android`、github-actions）；`.golangci.yml` 启用 `gosec`；PR 上考虑 `dependency-review-action`。
- **M86 Vite 开发服务器绑定 `0.0.0.0` 无主机限制** — 位置：`web/vite.config.ts:6-7`（`host:"0.0.0.0"`）。影响：dev server 对局域网暴露源码与代理 API 面（后端仍需配对，但 dev server 本身非面向不可信网络）。修复：默认 `127.0.0.1`，`0.0.0.0` 通过环境开关显式开启。

---

## 3. 🟢 轻微问题

### 3.1 Go 后端

- **G1** `protocol/parser.go:191-202`：`seq:0` 时非数字字符不终止扫描，可能把后续字段里的数字并入，误标在途分块。修复：消费到数字后遇任意非数字即 `break`（含 0）。
- **G2** `protocol/parser.go:279-284`：`extractInFlightBlock` 的 UTF-8 修复循环最坏 O(n²)（每次重扫整串）。修复：`strings.ToValidUTF8` 或只回退尾部不完整序列。
- **G3** `domain/state.go:227-231`：`WorldState.Clone` 未深拷贝 `CharacterInfo.Tags`（切片共享底层数组），当前只读故潜伏。修复：`v.Tags = slices.Clone(v.Tags)`。
- **G4** `domain/state.go:272-338`：`worldState.String()` 遗漏 `Characters`/`Variables`，虽未用于生产，但注释引导"旧快照指纹校验"会误判相等。修复：删除或标注"仅诊断、不得用于相等比较"。
- **G5** `application/card.go:208`：`ScanSecretLeakWarnings` 运算符优先级导致 `len(pub)>=8` 只约束第二个条件（`A || (B && C)`）。修复：`l>=8 && (A || B)`。
- **G6** `context/prompts.go`（`buildPlanningMessages` 摘要渲染，约 733-735）：未判 nil，与 `renderSummarySection` 的守卫不一致。修复：跳过 nil。
- **G7** `context/summary_schema.go:58-59`：`EndElement` 弹栈无空栈守卫（`encoding/xml` 当前保证不会触发）。修复：加 `len(stack)>0` 校验并返回校验错误。
- **G8** `search/hybrid.go:62-86`：`ReciprocalRankFusion` 接受负权重并产出负分（仅"两者皆 ≤0"时归一化）。修复：逐权重 clamp 到 ≥0 或拒绝。
- **G9** `application/memory.go:248-265`：主题键编辑在提交前原地修改 store 返回的共享 Effective 记录。修复：先拷贝再改（如 `next` 那样）。
- **G10** `application/turn_accept.go:283-284`、`memory.go:88`：对 store 返回值缺 nil 守卫（仅当实现返回 `(nil,nil)` 才可达）。修复：显式判空。
- **G11** `application/service.go:73,76`：`SetRuleset`/`SetRandom` 是无同步的公开可写字段，注释却称其为运行时"规则升级入口"。修复：加锁/原子，或标注为仅启动期使用。
- **G12** `pack/pack.go:446-467`：`safePath` 未拒绝控制字符/超长名（路径穿越已正确防护）。修复：拒绝控制字符并限长。
- **G13** `application/pngcard.go:65`：`ImportCardPNG` 的 64MiB 上限只由调用方（`request_body.go:22-23`）保证，第二个调用方（CLI/Android 桥）易遗漏。修复：把上限放到解析器内部。

### 3.2 Go 适配器

- **G14** `adapters/http/catalog.go:18-23`：ETag 用 `strings.Contains(match, version)` 弱比较，忽略 `W/` 语义。修复：解析实体标签集合后精确比较。
- **G15** `adapters/http/server.go:216-240`：SPA 回退对任意方法与未知非 API 路径都返回 200 + HTML。修复：非 GET/HEAD 返回 404，仅导航路径重写为 `/`。
- **G16** `adapters/http/sessions.go:110-139`：`getGraph` 静默忽略非法 `up`/`down`，与 `getLorebook`/`getSession` 的 400 不一致。修复：非法数值返回 400。
- **G17** `adapters/tts/tts.go:267,364`：每次合成都新建 `http.Client`，丢失连接池、每次重握手。修复：Service 持有一个 client。
- **G18** `adapters/tts/tts.go:173-176`：TTS 缓存键不含凭据，不同账号同 BaseURL 共享缓存音频。修复：把 key/account 纳入键。
- **G19** `adapters/tts/tts.go:447-486`：Edge-TTS 音频缓冲无总量上限（仅每次读取 20s deadline）。修复：加 `maxAudioBytes` 上限。
- **G20** `adapters/providers/providerutil/firsttoken.go:46-49`：首 token 预算被 clamp 到 1ns，头恰好到达时 `NewTimer` 立即触发 → 误报 `ErrTimeoutFirstToken`。修复：给一个下限（如 `TimeoutIdle`）或把"头已到达"视为满足首 token 阶段。
- **G21** `adapters/image/image.go:89-100`：引擎靠端口/字符串子串自动识别并默认 ComfyUI（硬编码模型文件名），其他 OpenAI 兼容图像端点会得到困惑的失败。修复：要求显式引擎或能力探测。
- **G22** `adapters/image/image.go:562-563`：ComfyUI `view` URL 参数未转义（服务端返回的文件名直接拼 query）。修复：用 `url.Values`。
- **G23** `adapters/image/image.go:599-616`：`saveImage` 忽略 `rand.Read` 错误且非原子写（无 temp+rename/fsync），崩溃可留截断图片仍被 200 提供。修复：用 `atomicfile` 并检查随机错误。
- **G24** `adapters/cardassets/store.go:162-181,40-71`：`safeSegment` 会把不同 ref 映射到同名（静默覆盖），允许 Windows 保留设备名（`CON`/`NUL`/`COM1`），且 temp+rename 无 `Sync`。修复：加后缀去重、拒绝保留名、写入后 fsync。
- **G25** `adapters/sqlite/pack.go:185-191`（`inClause` 空切片 panic）、`director.go:159,337`（`(&Store{})` 仅为调用忽略 receiver 的 `parseTime`）。修复：守卫空切片；把 `parseTime` 改为自由函数。
- **G26** `adapters/sqlite/store_turn.go:438-443`：快照版本用 `time.Now()` 而非注入时钟 `s.clock`，破坏确定性测试与记录顺序假设。修复：统一用 `s.clock`。
- **G27** `adapters/mcp/server.go:381-385`：`compile_prompt` 恒用第一个分支，且 `initialize` 无协议版本协商。修复：使用活动分支并加版本协商。
- **G28** `adapters/http/security.go:76-100`：配对失败表无容量上限，每次尝试线性扫描全部条目。修复：加容量与淘汰策略。
- **G29** `util/tlscert/cert.go:160-182,195-206`：证书检查只验 `NotAfter` 不验 `NotBefore`；生成的叶子 `IsCA:true`（符合"装为本地根"流程，但复用检查应同时校验密钥强度/SAN 相等/未生效）。修复：补校验。
- **G30** `adapters/http/observe.go:94-123`：`statusRecorder` 只转发 `Flush`/`Unwrap`，未实现 `Hijacker`/`ReaderFrom`，WebSocket 升级或 `io.Copy` 快路径经日志包装会失败。修复：转发 `Hijacker`。
- **G31** `adapters/http/respond.go:15-19`（`writeJSON` 丢弃编码错误，NaN 等会在 200 后留截断体）、`request_body.go:30-51`（`decodeJSON` 不校验 Content-Type）。修复：处理编码错误；校验 Content-Type。

### 3.3 Web 前端

- **G32** `stores/settingsStore.ts:51-137`：`optionMode`/`optionsFrequency`/`autoContinue` 未持久化（同类 store 已持久化），刷新即重置。修复：`partialize` 持久化这三项或注明会话级。
- **G33** `stores/storyHelpers.ts:69-113`：`resolveCharKeyFromSession` 在 `characterId` 存在时提前返回，`state.characters`/标题匹配分支（78-109）成为死代码；`presetIdentity` 每键做 JSON 往返。修复：删除不可达分支或重构为 id 未命中时再回退。
- **G34** `stores/slices/branchSlice.ts:171-173`：`loadOlder` 去重为嵌套线性扫描 O(n·m)。修复：先建 id `Set`。
- **G35** `app/api.ts:295-297,307-309`：`binaryRequest` 已对 `!res.ok` 抛错，调用方再检查 `!res.ok` 是不可达死代码（且会二次消费已读 body）。修复：删除冗余检查。
- **G36** `stores/slices/sessionSlice.ts:36-55,91`：`openSession` 命中缓存分支从不 `loadMemories`，`memories` 可能持续为空（除非打开记忆库或 reload）。修复：缓存分支也触发 `loadMemories()`。
- **G37** `stores/slices/turnSlice.ts:291-326`：`send` 的"先判后设"间隙在 `!view` 时先 `await createSessionFromPreset`，第二次 `send` 可穿过守卫。修复：在 await 前置提交标志/`phase:"generating"`，或用 token 守卫。
- **G38** `stores/slices/sessionSlice.ts:166-186` + `stores/helpers.ts:177`：`createSession` 幂等键被导航重置（epoch 变化），丢失响应后导航再重试会用新键 → 服务端可能重复建故事。修复：把待建键在重试窗口内独立于导航 epoch 持久化。
- **G39** `stores/slices/turnSlice.ts:357`：`attachTurnStream(get().view!.sessionId, …)` 在 await 后用非空断言，若不变式被破会运行期抛错而非降级。修复：取局部变量并守卫。
- **G40** `stores/slices/branchSlice.ts:33,38`：`as never` 掩盖 `StoryState` 形状不匹配，未来字段改名不会被 `tsc` 捕获。修复：把回滚快照与 `apply` 返回值类型化为 `Partial<StoryState>`。
- **G41** `stores/storyTypes.ts:96-127`、`storyHelpers.ts:182-226`、`turnSlice.ts:472-485`、`sessionSlice.ts:41-88`：`hud`/`activeCharacterId`/`activeBranchId`/`messages` 冗余于 `view`，`emptyStory` 与 `createInitialStoryState` 各列 20+ 字段，易漂移。修复：用记忆化 selector 从 `view` 派生 `hud`/`messages`；合并两个工厂。
- **G42** `stores/storyHelpers.ts:131-158`、`turnSlice.ts:26-27,249`、`directorSlice.ts:10`：`sendSeq`/`navigationEpoch`/`reloadSeq`/`pendingSubmission`/`directorWatch` 等模块级单例使测试需间接重置、HMR 携带状态。修复：封装为随 store 创建的运行时对象并提供 `reset()`。
- **G43** `lib/characterMacros.ts:31` + `lib/characterPresentation.ts:103`：`{{getvar::key}}` 恒展开为空（`characterPresentation` 未传 `variables`）。修复：把 `view.state?.variables` 透传到 `replaceMacros`。

### 3.4 Android 客户端

- **G44** `ModelsScreen.kt:324-331`：API key 输入框仅设 `KeyboardType.Password`，无 `visualTransformation`，密钥明文显示且可被截屏。修复：加 `PasswordVisualTransformation()` 与显隐切换。
- **G45** `ModelsScreen.kt:190,200-204`：`enabled = !busy || true` 恒真（疑似 `!busy` 笔误）；槽位关闭时选模型只改本地态，刷新后丢失。修复：`enabled = !busy`；关闭时也持久化 `modelId`。
- **G46** `GenerationService.kt:135,219`：通知小图标用系统 drawable（`android.R.drawable.ic_menu_edit`），部分 OEM 状态栏显示为实心方块。修复：用单色应用矢量。
- **G47** `TtsEngine.kt:30-33`：`stop()` 无调用点，离开故事或切到别处朗读不停止。修复：在离开故事/新朗读时停止。
- **G48** `MemoryScreen.kt:147-192`：同一 11 字段 `MemoryRow` 构造重复三份。修复：抽 `MemoryViewDto.toRow()`。
- **G49** `SseLineParser.kt:31-48,52-67`：空 `event:` 未按规范回落为 `message`，且忽略 `retry:`。当前服务端无害，但服务端若发空事件名会静默丢事件。修复：规范化空事件名并处理 `retry`。
- **G50** `RelativeTime.kt:22-25`：`minutes<1 -> "刚刚"` 也匹配时钟偏差产生的负值。修复：`minutes<=0` 守卫。
- **G51** `CharactersScreen.kt:89`：仍用已废弃的 `TabRow`（另一处已用 `PrimaryTabRow`）。修复：统一到 `PrimaryTabRow`。
- **G52** 死代码：`ApiClient.kt:226-246`（`withRetry`）、`StoryRepository.kt:184`（`detachStream`）、`ConnectionRepository.kt:117-123`（`fingerprintOf`）、`ApiException.kt:57-68`（`ApiErrorParser.fromEventData`）、`DirectorRepository.kt:90-92`（`events`）均无调用点，不被 51 项测试覆盖（`fingerprintOf` 重新启用会重引入 S10）。修复：删除或补测。
- **G53** `ApiClient.kt:90-91,129-134,159-166`：`DownloadBytes`/`executeRaw` 把整段 body 读入内存，大导出缓冲两份。修复：流式写入 SAF `OutputStream`。
- **G54** `MainScaffold.kt:59-66,150-164`；`nav/Routes.kt:17-25`：深链导航仅对深链路径 `launchSingleTop`，无 `popUpTo`，每次分支树跳转都压入新的 `story/...`（新 ViewModel）。修复：加 `popUpTo`/单实例策略。
- **G55** `app/build.gradle.kts:28-32,77`：release 关闭 minify/shrink 且无签名配置；`okhttp.logging` 声明但未用（若启用默认拦截器会记录 `Authorization` 头与请求体）。修复：开启 shrink/配置签名；删除未用依赖或确保日志不落敏感头。

### 3.5 UI / 视觉一致性

- **G56** `web/index.html:8-9` 的 `theme-color`（`#f9f8f5`/`#111215`）与 `tokens.css:75,220,284` 不一致，且无紫主题值。修复：对齐令牌并补紫主题（或按主题动态设置）。
- **G57** `var()` 回退值与令牌矛盾：`ui/Modal.css:22`（`--radius-xl` 回退 18px，实际 14px）、`panels.css:44,101,123`、`components/NarrativeStream.css:304`。修复：删除回退或与令牌一致。
- **G58** `ui/Switch.css:87-90`：`.ui-switch__hint` 硬编码 `padding-left:40px`（需手工与 32px track+8px 同步）。修复：用令牌 `calc()` 或改结构。
- **G59** `components/NarrativeStream.css:296`：注释乱码（编码损坏），提示编码风险。修复：重写为正常 UTF-8。
- **G60** `styles/toast.css:19`：`z-index:9999` 绕过 `--z-modal`/`--z-toast` 体系。修复：随 M69 统一后改用令牌。
- **G61** 空状态实现五套：`ui/EmptyState.tsx` vs `.session-empty-state`（`rail-left-sessions.css:230-255`）、`.tree-empty-card`（`LeftRail.css:38-63`）、`.memory-empty-card`（`MemoryBankModal.tsx:291-297`）、`.hud-empty-card`（`RightRail.tsx:354-357`）、`.director-empty`（`DirectorWorkspace.tsx:233`）。修复：统一到 `ui/EmptyState`（带尺寸/变体）。
- **G62** 分段控件四处重复：`ui/Tabs.tsx` vs `.session-segmented-control`（`rail-left-sessions.css:34-66`）、`.composer-syntax-group`（`composer.css:163-229`）、`.tachie-expr-switcher`（`rail-right.css:252-284`）、`.memory-filter-row`（`modals.css:94-123`），后三者缺 roving tabindex 与 ARIA 选中语义。修复：迁到 `Tabs`/新 `SegmentedControl`。
- **G63** 图标无尺寸体系：`ui/Icon.tsx:506-516` 默认 16，调用点散用 11–28px，且 `strokeWidth` 在 ≤14 自动切换（13→14 改动会同时改变描边）。修复：加 `--icon-*` 令牌。
- **G64** `ComposerShelf.tsx:346-377`：发送按钮同时有 `aria-label="推进剧情"` 与 `.sr-only` 动态文案，`aria-label` 胜出导致"演义中"等生成态不被播报。修复：去掉 `aria-label`，保留可见动态标签 + 视觉隐藏文本，并加 `aria-busy`。
- **G65** 同一"世界书条目"外观不一致：`panels.css:152`（`.lore-entry-card{cursor:default}`）vs `LeftRail.tsx:300-304`（可点但无 cursor/hover/focus）。修复：用不同类/元素并让交互提示与实际一致。
- **G66** 对话框焦点/关闭逻辑碎片化：`App.tsx:131-145`（Escape 链）与散落各处的模态、`DirectorWorkspace.tsx:190-199`（自行实现 Tab 陷阱）。修复：统一模态栈上下文。
- **G67** `mobile.css:39-51`：抽屉渲染了"抓手"条但无拖拽实现，提示误导。修复：实现下滑关闭或移除抓手。
- **G68** `FullTachieModal.tsx:68,84`：`dangerouslySetInnerHTML` 注入卡派生内容（`modalDesc`/`breadcrumb`），是全 UI 唯一裸 HTML 注入，与其余经 `DossierMarkdown` 的路径不一致。修复：改用 `DossierMarkdown`/净化。
- **G69** `ComposerShelf.tsx:298-304`：Enter 发送 / Shift+Enter 换行无任何提示（命令面板有 `<Kbd>` 页脚）。修复：发送按钮旁加提示（触屏隐藏）。
- **G70** `RightRail.tsx:126-129`：折叠按钮无 `aria-label`，可访问名为"收起 ›"（左栏已用 `aria-label`）。修复：补 `aria-label` 并把箭头 `aria-hidden`。
- **G71** `TurnActions.tsx:36-41,104-109`：内联编辑器无 Escape 取消（注记输入框 `ComposerShelf.tsx:270-275` 有）。修复：三个编辑器加 `onKeyDown` Escape → 取消。

### 3.6 依赖 / 仓库卫生

- **G72** `golang.org/x/sys v0.47.0` 已含 CVE-2026-39824 修复（0.44.0 起），`gorilla/websocket v1.5.3` 无库自身漏洞（相关 CVE 在消费者 `CheckOrigin: return true`），Go 模块图整体健康——仅需在升 `modernc.org/sqlite` 时核对内置 SQLite 版本。
- **G73** `android/gradle/wrapper/gradle-wrapper.properties` 无 `distributionSha256Sum`，无 `verification-metadata.xml`。修复：固定校验和并启用 Gradle 依赖验证。
- **G74** `web/package.json` 无 `packageManager`/`engines`，`@types/node` 解析为 22.x 而 CI 用 Node 24。修复：加 `"packageManager":"pnpm@11.9.0"`、`engines.node>=24`，对齐 `@types/node` 到 24。
- **G75** `.gitignore` 无 `.env`/`.env.*`（`release_audit.py:19` 只在已跟踪后才发现）。修复：加 `.env`/`.env.*`（保留 `.env.example`）。
- **G76** `scripts/inspect_card.py`、`inspect_card_texts.py` 仍被跟踪（与已忽略的私人脚本同类）；`scripts/quality_check.py:8-9` 硬编码作者机器路径 `data/cardtest/...`。修复：移入忽略列表或参数化。
- **G77** `scripts/architecture_diagram.py`（434 行）与 `function_architecture_diagram.py`（393 行）共享大量重复脚手架。修复：抽公共 SVG/render 模块。
- **G78** `.github/workflows/model-eval.yml:27,63-86`：`cancel-in-progress:false` 使并发运行都消耗付费额度；eval secrets 无 `environment` 保护。修复：加互斥/环境审批说明（成本控制，非代码执行风险）。
- **G79** `quality.yml:16-68` 无 `timeout-minutes`；`web/playwright.config.ts:5-16` `retries:0`、单浏览器。修复：加 30 分钟超时、CI 下 `retries:1`、按需加 Firefox/WebKit。

---

## 4. 重构建议

> 以下为"单点修复不足以覆盖"的结构性改造。每项给出理由、影响范围、风险与回归点、实施步骤。

### R1 · 统一对话框/模态栈管理器（Web）
- **理由**：`ui/Modal` + `useDialogFocus` + `DirectorWorkspace` 内联陷阱 + `StoryMapModal`/`IllustrationModal` 尺寸 hack + 手写 `role=dialog` 的命令面板，共 4+ 套"portal + scrim + focus trap + Esc + 恢复"实现，直接导致 S17/M64/M68/M75/G66。
- **影响范围**：`web/src/ui/Modal.tsx`、`web/src/lib/useDialogFocus.ts`、`App.tsx`、所有模态消费方（设置、档案、承诺、记忆、剧情图、插画、导演、导入、命令面板）。
- **风险与回归点**：焦点恢复顺序、Escape 关闭顺序、嵌套模态（如从记忆库打开修订确认）、`inert` 对当前焦点的处理；移动端抽屉与模态的叠层；Playwright 用例对 `role=dialog` 的选择器。
- **实施步骤**：① 建 `ModalStackProvider`（栈、`closeTop`、`registerEscape`、背景 `inert`/滚动锁）；② 迁移 `Modal`/`useDialogFocus` 到它；③ 让 `DirectorWorkspace`/`CommandPalette`/抽屉注册；④ 全局快捷键改查"栈是否为空"；⑤ 补焦点陷阱/背景 inert/滚动锁用例。

### R2 · 统一选择/分段控件原语（Web）
- **理由**：`.session-segmented-control`、`.composer-syntax-group`、`.tachie-expr-switcher`、`.memory-filter-row` 各自重造药丸轨道，缺少 `Tabs` 已有的 roving tabindex 与 ARIA 状态（M65/G62）。
- **影响范围**：`ui/Tabs.tsx`、`styles/rail-left-sessions.css`、`composer.css`、`rail-right.css`、`modals.css` 及对应组件。
- **风险与回归点**：视觉像素级一致（CSS 声明集合门禁 `pnpm build:parity`/`css:parity`）、键盘方向键行为、选中态在三种主题下的对比度。
- **实施步骤**：① 给 `Tabs` 增 `variant="segmented"`/`size`/`fill` 与 `role=radiogroup` 选项；② 逐个迁移并删除旧 CSS；③ 用 `css:parity` 证明声明集合未变（纯结构迁移），有意视觉改动才 `css:snapshot`。

### R3 · 统一列表行原语 `ListRow`（Web）
- **理由**：`ui/ListRow` 已正确实现"主按钮 + 尾部操作"，而 `session-entry-card`、`char-card-entry`、`node-item`、物品槽仍在 `role="button"` 内嵌套按钮（M66/M67），使删除/分支控件对 AT 不可见。
- **影响范围**：`ui/ListRow.tsx`、`LeftRailSessionsPanel.tsx`、`LeftRailTreePanel.tsx`、`CharacterCardRow.tsx`、`RightRail.tsx`、`LeftRail.tsx`。
- **风险与回归点**：行内删除的 Enter 语义（已有测试 `leftRail.test.tsx`）、长按/右键菜单、键盘 Tab 顺序、`overflow` 下的布局。
- **实施步骤**：① 扩展 `ListRow`（可选的尾部操作槽、确认态）；② 逐行迁移，保留既有 `data-testid`；③ 更新/新增键盘与读屏语义用例。

### R4 · 设计系统收敛（空状态/徽章/图标尺寸/动效令牌）（Web）
- **理由**：五套空状态、十余种徽章、无图标尺寸体系、字号/圆角被字面 px 绕过、动效时长硬编码（G61/G63/M72/M71/G57/G58/G60）。这类漂移会让每次视觉迭代成本递增。
- **影响范围**：`tokens.css`、`ui/EmptyState|Pill|Icon|Switch`、`styles/*`、`scripts/check.mjs`。
- **风险与回归点**：三主题对比度（尤其紫主题 M76/S15）、CSS 棘轮基线（收紧会打印放宽项）、`css:snapshot` 后的视觉回归。
- **实施步骤**：① 在 `tokens.css` 补 `--fs-*`/`--radius-*`/`--icon-*`/`--duration-*` 并加 `--text-faint`；② 把五套空状态迁到 `EmptyState` 变体；③ 徽章收敛到 `Pill` 变体；④ 扩展 `check.mjs` 禁 token 外裸 `font-size`/`border-radius`/hex；⑤ 用 `css:snapshot` 落新基线并在 PR 说明。

### R5 · SSE 传输模块化 + 事件处理错误边界（Web）
- **理由**：`app/api.ts`（639 行）混合 REST、二进制上传/下载、SSE 解析、退避、停滞检测；S8/M45/G35 都源于此，且难以单测重连策略。
- **影响范围**：`web/src/app/api.ts` → 新增 `web/src/app/stream.ts`；消费方 `stores/slices/turnSlice.ts`、`sessionSlice.ts`。
- **风险与回归点**：`Last-Event-ID`/聚合 id 语义、终态事件后主动断开、退避计数归零、`turnRecoveryFlow`/`stream` 既有测试。
- **实施步骤**：① 抽出 `subscribeEvents`/`parseSSEBlock`/`readWithStall`/`sleepCancellable`；② 给 `onEvent` 独立 try/catch 并定义"handler 出错"策略（不推进游标或终止）；③ 归一化 `seq`；④ 为"handler 抛错""seq=0""超大事件""401 非 JSON"补用例。

### R6 · Story 运行时状态容器（替换模块级单例）（Web）
- **理由**：`sendSeq`/`navigationEpoch`/`reloadSeq`/`pendingSessionSetup`/`pendingSubmission`/`directorWatch`/`thinkingStartedAt` 等模块级可变单例使 HMR/测试携带状态，并直接导致 M42/M43/M44/G42、间接 G38。
- **影响范围**：`stores/storyHelpers.ts`、`stores/slices/*`、测试的 `beforeEach/afterEach` 重置逻辑。
- **风险与回归点**：状态隔离（多会话切换）、幂等键跨重试复用、`owns(owner)` 判据、`storyIsolation.test.ts` 的 41 项隔离用例。
- **实施步骤**：① 定义 `createStoryRuntime()`（含 seq/epoch/pending/订阅句柄）；② store 创建时实例化，`beginNavigation`/`emptyStory` 调用 `runtime.reset()`；③ 迁移各 slice；④ 删除测试里的间接重置。

### R7 · 后端路由统一 host/auth 中间件 + 错误体裁剪 + SQL `IN` 构造器 + 读写连接纪律
- **理由**：S3/S4/M14（鉴权/Host 分裂）、M15/G14/G31（错误体与响应卫生）、G25（五套 `IN`/占位符构造）、M17/M29（读路径误用写锁/写连接）都是同一"缺少统一层"的不同表现。
- **影响范围**：`adapters/http/{server,security,respond,request_body,observe}.go`、`adapters/sqlite/{pack,memory_projection,store_*,memorybatch,graph}.go`、`adapters/{image,tts,embedding,cardassets}` 的错误构造。
- **风险与回归点**：SSE/WebSocket 升级路径、`Last-Event-ID`、缓存头变化对前端缓存的影响、既有 HTTP 契约测试与 `release_audit`。
- **实施步骤**：① 建 mux 级中间件：**始终**校验 `isLANHost`，非回环时强制 token，路由可显式 opt-out（如 `<img>` 资源仍保留 Host 校验）；② 统一 `bodySnippet`（≤512）并替换 image/cardassets/import 的原始回显；③ 抽 `sqlIN(column, ids)` 并替换全部手写构造；④ 立"读 API 不碰 `s.mu`/`s.db`"规则并改 `ExportSession`/`ProjectedMemories`。

### R8 · Provider 流式骨架统一（`streamPump`）
- **理由**：`openai`/`responses`/`anthropic` 的 `Stream` 共享约 150 行首 token/idle/total 定时器 + 读取 goroutine 逻辑，且三份都有 M27（多行 data）与 M28（goroutine 泄漏）同类缺陷。
- **影响范围**：`adapters/providers/{openai,responses,anthropic,providerutil}`。
- **风险与回归点**：思考链增量、截断归类、超时/取消区分、SSE 分帧（已有 `openai` 相关测试）；三协议的差异点（事件名、结束标记）必须参数化而非吞并。
- **实施步骤**：① 抽 `streamPump`（读行 → 组帧 → 派发、上下文取消、定时器策略）；② 各协议只提供"行→事件"映射；③ 修多行 `data:` 与 `defer cancel()`；④ 以现有 provider 测试 + 新增多行/取消用例回归。

### R9 · 卡片导入管线统一（charx / tacard / png）+ 有界归档读取
- **理由**：三个导入器各自实现 ZIP/资源/告警/上限，导致 S1/M6/M7/M13 这类"某条路径漏校验/漏告警"的问题反复出现。
- **影响范围**：`application/{charx,tacard,pngcard,cardlibrary,archive,archive_validation}.go`、`application/avatar.go`、`internal/cardmarkers`。
- **风险与回归点**：`.tacard` 零脚本策略（拒绝 `.risum/.lua/.js/.exe`）、charx 多种图像标记兼容、PNG 内嵌 chunk、压缩炸弹上限、既有 `card_test.go`/`tacard_test.go`/`cards_tacard_test.go`。
- **实施步骤**：① 定义有界归档读取器（统一 entry 数/单entry/总量上限 + 魔数预检）；② 统一资源落地与 `Warnings` 计数；③ 按 `NodeKind` 校验导入节点负载（关闭 S1/M13）；④ 头像候选限数；⑤ 三路径共用同一报告结构。

### R10 · 回合生命周期收敛 + `context.Compile` 拆分（Go）
- **理由**：回合生命周期散在 6 个文件、6 个互不统属的互斥锁与 `WaitGroup` 记账；`Compile` 是约 130 行编排器并多次构建消息列表。这是未来死锁/泄漏最可能藏身处，也是 M1/M2/M5/M11 的根源。
- **影响范围**：`application/{turn_*,service,lifecycle,compactor}.go`、`context/{compile,budget,summarizer}.go`。
- **风险与回归点**：事务化提交与终态一致性、迟到取消/旧 attempt 不覆盖、`PROCESS_INTERRUPTED` 恢复幂等、预算/截断行为、`service_test.go`（1165 行）与 `compile_test.go`（930 行）。
- **实施步骤**：① 抽出 `turnRunner`（单一锁序 + 生命周期记账）并让现有测试全绿；② 把 `Compile` 改为 `compileSession`（每依赖加载一次并暴露已测量尺寸）；③ 增量预算拟合与 mandatory 单次估算；④ 存储错误 fail-closed；⑤ 全程以 `go test -race` 守护。

### R11 · Android：主线程纪律 + 通知单一 owner + 资产/归档服务化 + 可测试性
- **理由**：S10/M46/M55/M56 同属"I/O 与解析误在 Main"；M52 双通知 owner；M54 资产解析三处重复；ViewModel 工厂依赖 `TavernApp.instance.container` 导致仅纯域逻辑可测。
- **影响范围**：`android/.../data/remote/*`、`data/repo/*`、`service/*`、`ui/screens/story/*`、`ui/screens/{cards,chats}/*`、`AppContainer.kt`。
- **风险与回归点**：SSE 流与前台服务生命周期、TOFU 指纹流程、卡资源解析（Unicode NFC/后缀变体）、51 项 JVM 测试。
- **实施步骤**：① 引入 `io {}`/注入 `Dispatcher`，把 TLS/JSON/文件 I/O 一律移出 Main（一次修 S10/M46/M55）；② 通知收敛为 `Application.onCreate` 创建的单一 `TurnNotifier`；③ 资产解析收敛到单一带缓存的仓库路径；④ 导出/导入迁入前台服务；⑤ 构造函数注入仓库，补 ViewModel 相位机/重试/分页测试；⑥ 拆分 700+ 行的 `StoryViewModel`/`StoryContent`/`StoryComposer`。

### R12 · 依赖与供应链治理
- **理由**：S6/M80-M86/G73-G79 共同指向"缺自动化与门禁"，靠人工排查不可持续。
- **影响范围**：`.github/`、`go.mod`、`web/package.json`、`android/gradle/*`、`THIRD_PARTY_NOTICES.md`/`LICENSES/`、`scripts/release_audit.py`。
- **风险与回归点**：大版本升级（Vite 8、ESLint 10、OkHttp 5、Coil 3、AGP 9）可能牵动构建/测试；Android CI 首次接入需缓存与 JDK 配置。
- **实施步骤**：① 先修 S6（`x/image` 升级 + `govulncheck`）；② 加 `dependabot.yml` 与 `gosec`；③ 加 Android CI job；④ 让 `license_notices.py` 进 CI（diff 门禁）；⑤ 排期大版本升级（各自独立 PR，带回归门禁）。

---

## 5. 可执行优先级清单

> 建议按阶段推进；每项完成即以现有门禁（`go test -race`、`pnpm lint/test/build`、`release_audit --strict`）回归。

### Phase 0 · 立即（安全 / 崩溃 / 数据丢失，建议本轮内处理）
- [ ] **S1** 修复 `memoryChanges:[null]` nil 解引用（`summarizer.go:77`）+ 导入期按 `NodeKind` 校验负载（M13）。
- [ ] **S6** 升级 `golang.org/x/image` 到 v0.45.0+ 并跑 `govulncheck`。
- [ ] **S3** 非回环绑定默认启用 TLS / 拒绝明文配对；断言非回环时 PIN 非空。
- [ ] **S4** 未鉴权资源路由补 Host 校验，缓存改 `private`。
- [ ] **S8** Web SSE：`onEvent` 独立 try/catch（不推进游标或终止）+ 归一化 `seq`。
- [ ] **S9** Web `loadOlder` 与 `reloadView` 合并而非覆盖。
- [ ] **S10** Android `fetchPeerCertificate` 移出主线程。
- [ ] **S12** Android 历史只读标记由请求推导，不再被清空。
- [ ] **S13** 移除 viewport 缩放禁用；**S15** 修正 `--text-dim` 对比度；**S16** 主输入框补可访问名称；**S14** 触屏常显回合工具栏。
- [ ] **S2** 边界标签大小写不敏感中和 + 导演讨论摘要纳入保护。
- [ ] **S7** 处理 `reference-components/` 许可证（移出跟踪树或补授权）。

### Phase 1 · 高优先（明确缺陷与高风险面）
- [ ] M14/M15/M17/M20（Host 语义、错误体裁剪、导出不持写锁、上传并发预算）
- [ ] M31/M32/M33（前端超时对齐、密钥落盘、资产缓存失败不缓存）
- [ ] M46/M47/M48/M51/M52/M57（Android 主线程解码、重试语义、提交后重置、前台服务与通知、token 存储）
- [ ] M64/M65/M66/M67/M68/M70/M71（Web 模态 inert、Tabs 语义、行/卡片键盘、抽屉、触控目标、reduced-motion）
- [ ] M80/M84/M85（Android CI、许可证通知门禁、dependabot + gosec）
- [ ] M1/M2（后端存储错误 fail-closed，避免语义静默降级）

### Phase 2 · 中优先（性能 / 可维护性 / 一致性）
- [ ] M3–M13、M16、M18、M19、M21–M30（后端其余中等项）
- [ ] M34–M45（前端其余中等项）
- [ ] M49/M50/M53/M54/M55/M56/M58–M63（Android 其余中等项）
- [ ] M69/M72–M79（Web 其余中等项：toast 统一、字号令牌化、死选择器、模态叠层、紫主题对比度、加载播报、菜单 portal、标签）
- [ ] M81–M83/M86（ESLint/Vite/Go+Android 依赖升级、dev server 绑定）
- [ ] 启动 **R5**（SSE 模块化）与 **R6**（运行时容器），为后续修复降本。

### Phase 3 · 低优先（整洁 / 潜在风险 / 重构）
- [ ] G1–G79（按区域批量处理；优先 G5/G8/G9/G23/G24/G25/G26/G30/G31 等潜在正确性/资源项）
- [ ] **R1/R2/R3/R4**（Web 设计系统收敛）——与 M64–M79 同步推进。
- [ ] **R7/R8/R9/R10**（后端统一层与管线重构）——独立 PR，`-race` + 契约测试守护。
- [ ] **R11**（Android 架构/可测试性）与 **R12**（供应链治理长期项）。

### 建议的验证与回归基线
- Go：`gofmt -l internal cmd` 为空、`go vet ./...`、`go test -race -count=1 ./...`、`govulncheck ./...`。
- Web：`pnpm typecheck`、`pnpm lint`（六项 CSS 棘轮）、`pnpm test`、`pnpm build`；涉及样式结构改动加 `pnpm build:parity && pnpm css:parity`，有意视觉改动才 `pnpm css:snapshot`。
- Android：`./gradlew :app:testDebugUnitTest :app:assembleDebug`（建议纳入 CI）。
- 合规：`python scripts/release_audit.py --strict`、`python -m unittest discover -s scripts/tests -v`。
- 新增用例优先覆盖本次发现的分支：pack 含 `memoryChanges:[null]`、边界标签大小写变体、SSE handler 抛错、`seq=0` delta、`loadOlder` 后 `reloadView`、Android 分页/历史只读/主线程 IO。

---

*本文档仅为审查记录与建议，未对任何现有代码/文件做出修改。待确认后再决定进入修复/重构阶段。*
