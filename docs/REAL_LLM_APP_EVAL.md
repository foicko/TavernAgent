# 真实 LLM 应用编排验收（长程对话 + 图片加载）设计

> 目标读者：实现者与发布评审。**只描述设计与判定口径，不写凭据。**
> 关联：`docs/PROJECT_STATUS.md` §6「真实模型质量未闭环」、`docs/OPERATIONS.md` §「真实模型验收」、`.github/workflows/model-eval.yml`。
>
> 实现状态：**已落地**为 `scripts/functional_eval.py` 的 `longplay` 场景（`--scenarios longplay`）。
> 运行方式见 `docs/OPERATIONS.md` §「长程应用验收」；结论尚未在真实网关上登记（见 `PROJECT_STATUS.md` §6）。

## 1. 目标与边界

用**真实模型**（非 mock）驱动**被测二进制的 HTTP 接口**，模拟一个玩家真实游玩：

1. **随机**挑一张角色卡导入并开局；
2. 连续进行 **20 轮**对话（提交 → 流式生成 → 提交成功）；
3. 对每轮正文做**可自动化的"输出是否有问题"检查**（不是主观打分）；
4. 校验**图片能正常加载**（资源清单、资源端点、正文里的图片标记）。
5. 全过程可复现（记录随机种子与卡片指纹）、可归因（失败分类）、有预算闸（额度不可逆）。

**不做**：不评估文风"好不好看"（主观），不替代既有 `functional_eval.py` 的协议/记忆/导演/压缩场景，不接入常规 CI（不调用付费模型）。

## 2. 挂载点：复用既有框架，不新建

现有资产已经覆盖了 90% 的脚手架，直接扩展：

| 能力 | 复用 |
| --- | --- |
| 起隔离实例、密钥走环境变量、独立 `-data` | `scripts/functional_eval.py` 的 `Server` |
| 供应商解析 / 预检 / 余额闸 | `resolve_provider`、`preflight`、`scripts/release_eval.py` |
| HTTP 客户端、回合等待、结果解析 | `scripts/eval_protocol.py` 的 `Client`、`wait_turn`、`result_content` |
| 人读日志 + 机读 JSONL + report.json | `Recorder`、`Scenario`、`main()` |
| 失败聚类与能力/可靠性双口径 | `scripts/eval_metrics.py`（`passAtK` / `passPowerK`） |
| 手动触发 + artifact 证据 | `.github/workflows/model-eval.yml` |

**落地形态**：在 `scripts/functional_eval.py` 新增场景 `longplay`（`SCENARIOS` 增加一项），并做两处必要的通用化改造（见 §3、§6.2）。这样 `--scenarios longplay` 即可跑，报告/指标/工作流全部自动继承。

## 3. 随机选卡（可复现）

新增参数：

- `--card <path>`：显式指定单卡（优先级最高）。
- `--card-dir <dir>`：卡池目录，默认按顺序探测 `charx/` → `testdata/` → `scripts/fixtures/`。
- `--seed <int>`：随机种子；不传则随机生成并**写入报告**。

选卡规则：

1. 递归收集 `*.charx` / `*.tacard` / `*.json`（SillyTavern V2/V3 PNG 视需要）；过滤明显坏文件（大小 0）。
2. `random.Random(seed).choice(pool)` 选一张；记录 `path`、`size`、`sha256`、卡内 `cardId/name` 到报告，保证换机可复现。
3. 卡池为空 → 明确报错并给出获取方式；**绝不**回退到 mock。

仓库合规约束（`release_audit.py --strict` 禁止入库位图）：**不要**为了本测试提交 `.png/.charx` 到仓库。需要"自带一张卡"时，改为在脚本内用 base64 常量在临时目录**现场合成**一张 `.tacard`（含 1×1 PNG、`assets` 清单、`ui_contract`），既满足矢量约束又可复现。真实大卡仍从本地 `charx/`（已 gitignore）取。

## 4. 20 轮驱动策略

单纯把同一句话发 20 次会退化成复读，测不出长程问题。采用**玩家"意图脚本" + 随机措辞**：

- 预置 20 条**意图**（推进剧情、提问、表态、检定、切换场景、回看旧事、提及物品、沉默观察……），每条绑定可选的 `{{char}}/{{user}}` 与动作标记。
- 每轮把意图渲染成自然语言，并**注入 1~2 句随机措辞**（从意图短语池里按 `seed` 采样），保证输入不重复。
- 第 N 轮的输入可**回指前面轮次**（如"你刚才说的那件事"），逼迫上下文/记忆链路真正生效。
- 每轮之间读一次 `GET /api/v1/sessions/{id}`，记录状态、分支头、`turnNumber`。
- 失败重试沿用 `submit()`（首答失败留痕并重试一次），但**重试次数计入报告**。

产物：`conversation.log`（人读，20 轮全文）与 `conversation.jsonl`（每轮结构化记录），沿用 `Recorder`。

## 5. 输出问题检查（客观、可机读）

分**硬失败**（报告判 FAIL）与**软告警**（留痕不影响总判定）。阈值写进报告，便于回归比较。

### 5.1 硬失败（正确性/契约）

| 检查 | 口径 |
| --- | --- |
| 回合状态 | 每轮最终 `status == committed`（自动续写 ≤3 次后仍非 committed 即失败） |
| 正文非空 | `prose.strip()` 长度 ≥ 20 字（模型有一次重试机会） |
| 协议不泄漏 | 正文不出现 `{"v":1`、`"proposalId"`、`type":"final"` 等原始帧/JSON |
| 宏已展开 | 不残留 `{{char}}` / `{{user}}` / `{{getvar` |
| 无脚本/HTML 注入 | 不出现 `<script`、`onerror=`、`javascript:`、`<iframe` |
| 无乱码 | U+FFFD 替换符占比 < 0.5%，且合法字符占比达标 |
| 图片键合法 | 正文里每个 `<img cmd="X">` / `[IMG:X]` 的 `X` 必须 ∈ 卡资源清单（见 §6） |
| 服务端无致命告警 | `server.log` 无 `ERROR`/`panic`；WARN 计数进入报告 |
| 上下文预算 | `/api/status` 的 `budgetUnfit/budgetExceeded` 不增长（预算饥饿是缺陷不是风格） |

### 5.2 软告警（质量观察）

| 检查 | 口径 |
| --- | --- |
| 复读 | 相邻轮/前 3 轮的 8-gram 重合率超阈值时告警（长程最常见退化） |
| 语言漂移 | 正文主体应为中文；出现大段非中日韩字符块时告警 |
| 人设漂移 | 角色名/性别/身份在其后轮次被自相矛盾地改写时告警 |
| 选项质量 | 有选项时数量 2~4、文本非空且不与正文重复 |
| 长度异常 | 单轮 prose 长度落在合理区间（如 40~4000 字）之外时告警 |
| 延迟/用量 | 单轮耗时、token 用量进入报告；超阈值告警（不判 FAIL） |

> 原则：**能客观断言的才判 FAIL**；"写得好不好"只做软告警，避免把评测变成主观噪声源。

## 6. 图片加载检查

分三层，覆盖"模型引用"到"浏览器真能取到字节"。

### 6.1 资源清单（导入后）

- 导入卡后 `GET /api/v1/cards/{cardId}`，读 `assets: [{name, url}]`（见 `internal/adapters/http/cards.go` 的 `cardAssetsBody`）。
- 断言：清单非空（对有内嵌资源的卡）；`name` 无重复；`url` 均指向 `/api/v1/cards/{id}/assets/{name}`。
- 与卡内 `assets` 定义交叉核对，确认归档内路径**没有**被当成本地 ref 回传（回归 `97074d3`：卡资源清单不回传归档内路径）。

### 6.2 资源端点（HTTP 真取）

对清单里每个 `url`：

- `GET` 必须 **200**，且该路由设计上**不需要鉴权**（`<img>` 无法带 token）——顺带验证它没被新加鉴权误伤。
- `Content-Type` 匹配扩展名（`png/webp/jpeg/gif`）。
- 字节数 > 0；用最小魔数校验真实格式（PNG `\x89PNG`、WebP `RIFF....WEBP`、JPEG `\xFF\xD8\xFF`），不依赖 Pillow。
- 同一 URL 取两次，SHA-256 一致（资源不可变）。
- 负例：取一个不存在的 ref，必须 404 而非 500（错误路径不吞成 200）。

### 6.3 正文图片标记与提示词契约

- 从 `conversation.jsonl` 收集所有轮次的 `<img ...>` / `[IMG:...]` / `<img_obj>` / `<imgBG>` 标记。
- 每个键必须 ∈ 清单 `name`（或 `CardUI.ImageKeys`）；出现自造键即**硬失败**——这正是 `0b75160`「契约列出可用图片资源键，模型不再自造键名」要守的行为。
- 对卡里没有资源的情况：断言模型**不产出**图片标记（不得凭空造图）。
- HUD/SCENE/VAR 标记（`[HUD:a=b]` 等）做语法校验：能解析成结构化字段，不当普通正文；进度值 `n/m`、`n%` 可解析。
- 若配置了生图槽位：可选跑 `imagePlanRefs`（`internal/application/imageplan.go`）产出的计划，校验引用的 ref 也在清单内；默认关闭以免额外计费。

### 6.4 前端渲染等价性（可选、低成本）

图片最终由 Web 渲染。可复用 `web/e2e`：把 20 轮记录导成静态夹具，断言 `CardHUD`/`SceneBar` 能把标记渲染成真实 `<img src>`（而非空框）。此项作为 P2，先不做也不影响主验收。

## 7. 成本与预算闸

额度不可逆，必须前置：

1. 运行前 `preflight`（0 token 验凭据）。
2. **预算闸**：按 `20 轮 × (输入窗口 + 输出预算)` 估算上限，读 `release_eval.py ledger` 余额；不足则拒绝开始（沿用 `model-eval.yml` 的既定策略）。
3. 场景内累计 `turn_usage` token；一旦超过 `--max-tokens` 或 `--max-minutes`，**停止并标 FAIL(超预算)**，而不是继续烧。
4. 失败重试次数有上限（默认单轮 1 次），计入成本。
5. 不自动重跑整个场景；一次运行一份报告。

## 8. 报告与指标

`report.json` 在既有结构上增补：

- `card`：`path/sha256/cardId/name/assetCount`。
- `seed`、`turnCount`。
- `checks`：按 §5/§6 汇总的通过/失败/告警计数与**阈值快照**。
- `images`：清单、逐 URL 状态与哈希、正文引用键与是否命中。
- `usage`：来自被测实例 `turn_usage` 台账的用量指标——输入/输出/命中缓存 token、**缓存命中率**、**吞吐与首字后 token 速率**、平均延迟与首字延迟、估算漂移，并按 `slot`/`task` 分组。
- `turns[]`：每轮 `status/elapsed/tokens/repetition/length`。
- `failureAttributions`：沿用 `attribute_failed_steps`，让首错可归因。

接入 `scripts/eval_metrics.py` 的 `passAtK`（能力上限）与 `passPowerK`（业务可靠性）双口径；多 seed 时按类聚簇失败。

## 9. 运行方式

```powershell
# 0) 先构建被测二进制（前端必须先 build：dist 被 //go:embed 嵌入）
Set-Location web; pnpm install --frozen-lockfile; pnpm build; Set-Location ..
go build -o build/release-eval/tavernagent.exe ./cmd/tavernagent

# 1) 凭据只走环境变量（不落仓库）
$env:TAVERNAGENT_EVAL_GATEWAY_KEY = "<网关密钥>"
$env:TAVERNAGENT_EVAL_TOKEN = "<被测实例令牌>"
$env:TAVERNAGENT_EVAL_MODEL = "gemini-3.8-flash-high"

# 2) 起隔离网关 + 实例，查余额、预检
python scripts/release_eval.py start
python scripts/release_eval.py ledger
python scripts/release_eval.py preflight

# 3) 小样本探路（1 张卡 / 3 轮），确认协议与图片链路
python scripts/functional_eval.py --scenarios longplay --card-dir charx --seed 7 --turns 3

# 4) 正式：随机卡 + 20 轮；多 seed 提高可靠性口径
python scripts/functional_eval.py --scenarios longplay --card-dir charx --turns 20
python scripts/functional_eval.py --scenarios longplay --card-dir charx --turns 20 --seed 42
```

`model-eval.yml` 增加一个可选 `longplay` 步骤（同一 shell 内复用已启动的网关），artifact 收 `output/functional/**/report.json` 与 `conversation.log`。

## 10. 落地步骤（建议顺序）

1. 参数化卡片：新增 `--card/--card-dir/--seed/--turns`，修正 `_submit_once` 里硬编码的 `npc_chixia` 角色名读取（改为从 `view.state.characters` 中取参与者名），否则任意卡会误标"角色"。
2. 实现 `scenario_longplay`：选卡 → 导入 → 开局 → 20 轮驱动 → 每轮硬检查。
3. 实现图片三层检查（清单 / 端点 / 标记键），负例 404 一并覆盖。
4. 报告增补 `card/seed/checks/images/turns`，接 `eval_metrics.py`。
5. 更新 `model-eval.yml`、`docs/OPERATIONS.md`、`docs/PROJECT_STATUS.md` §6（登记脱敏结论）。
6. 合成卡 fixture 用 base64 现场生成，禁止把位图/大卡提交进仓库。

## 11. 风险与限制

- **成本**：20 轮 × 多 seed 是付费路径；只在手动工作流跑，默认 1 个 seed、探路 3 轮。
- **随机性**：不同卡难度差异大；报告必须带 seed 与卡指纹，结论按多 seed 聚合，单次失败不代表系统缺陷。
- **上游波动**：429/超时会计入软告警并留痕；`preflight` 与预算闸是为了不死在半路。
- **主观项**：文风、创意不判 FAIL，避免评测不可复现。
- **卡片合规**：本地卡池不入库；仓库只放矢量，fixture 现场合成。
