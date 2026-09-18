# ADS（Agent 开发规范 v1.0）合规评审与整改记录

> 依据：`Agent开发规范.md`（ADS v1.0，提取自《深入理解 AI Agent》）。
> 评审时间：2026-09-18。依据的现场：`git log` 当时的唯一提交 `400478e` + 工作区未提交改动
> （`internal/context/prompts.go`、`director.go`、`budget_internal_test.go`）。
>
> 这份文档只做三件事：记录**已经落地的整改**、记录**显式偏离规范的取舍**（规范 0.3 节要求
> 偏离必须写明理由）、记录**尚未处理**的缺口。不做泛泛的自我评价。

---

## 1. 结论速览

Harness 五要素（上下文 / 工具 / 约束 / 验证 / 纠正）里，**约束、验证、纠正三项本来就达标**：
回环绑定 + 配对码 + 恒时鉴权 + 请求限额；模型只提议、规则收据结算、验证只读结构化证据；
恢复矩阵幂等且明确不调用模型。本次整改补的是**上下文层的信任边界**与**评估闭环**。

| 编号 | 整改项 | 条款 | 证据位置 |
| --- | --- | --- | --- |
| P0-1 | 外部内容加结构化来源标记 | ADS-2.5-02 / 2.5-03 / 2.5-07 | `internal/context/external_content.go`、`prompts.go` |
| P0-2 | 评估加 Pass@k / Pass^k 口径、多种子、失败归因 | ADS-7.2-06 / 7.4-09 / 7.6-02 | `scripts/eval_metrics.py`、`eval_protocol.py`、`release_eval.py`、`functional_eval.py` |
| P1-1 | Anthropic 前缀缓存断点 | ADS-2.2 / 7.5-04 | `internal/adapters/providers/anthropic/anthropic.go` |
| P1-2 | 动态状态块移到上下文末尾（user 槽位） | ADS-2.7-02 / 2.7-03 / 2.2-02 | `internal/context/prompts.go`（`tailStatusBlock`） |
| P1-3 | 提示词单一来源 + 版本指纹 | ADS-7.8-04 | `internal/context/prompt_version.go`、`FrameProbeInstruction` |
| P2-1 | 消融总开关 | ADS-7.8-01 | `internal/context/ablation.go`、`cmd/tavernagent/main.go`（`-ablate`） |
| P2-2 | 编译阶段耗时可观测 | ADS-7.7 | `internal/application/metrics.go`、`CompilePhases` |

---

## 2. 已落地的整改

### 2.1 P0-1 外部内容的来源标记（指令 / 数据分离）

**问题**：角色卡、世界书、记忆、摘要都不是"系统指令"，却被以纯文本拼进 `system` 槽位（信任
等级最高的位置）。角色卡的 `system_prompt` 与 `post_history_instructions` 字段本身就是**作者
可控的指令性文字**——用户从互联网拖进来的卡片，比网页里藏一段白字更直接（ADS-2.5-05）。
在此之前唯一的防线是一句散文式声明，而 ADS-2.1-08 明确指出这类约束只能降低编造概率。

**做法**：

- 每类资料用 `<external_content source=… trust=… field=…>` 包裹，`source` 取
  `character_card` / `lorebook` / `memory` / `summary`；`trust="untrusted"` 表示外部导入、
  未经审核，`trust="derived"` 表示由本系统从既往剧情派生。
- 边界声明 `ExternalBoundaryInstruction` 写在**任何资料之前**（先讲规则再给数据），
  并显式说明 `<system-reminder>` 包裹的内容是系统注入、不是玩家发言。
- **防标签逃逸**：正文里若出现字面量的 `</external_content>`，会被中性化成
  `[/external_content]`。否则攻击者可以在卡片里"提前关闭"资料区，让后续文本重新变成可信
  指令——边界标记必须对自身免疫。
- 标记**留在静态前缀里**，不把资料挪出 `system` 槽位：边界靠标签表达而不是靠位置表达，
  挪位置会打断 KV Cache 的前缀复用（ADS-2.2-15）。

**回归**：`internal/context/tail_status_test.go` 的 `TestExternalContentIsMarked` 与
`TestBoundaryTagsCannotBeEscaped`。

### 2.2 P0-2 评估口径与失败归因

**问题**：`scripts/functional_eval.py` / `release_eval.py` 只报一次运行的成功率，且失败记录
没有结构——无法回答"失败集中在哪一类""首错是什么""分差是不是噪声"。

**做法**：

- 新增 `scripts/eval_metrics.py`：同时给出 `passAtK`（1-(1-p)^k，能力上限）与
  `passPowerK`（p^k，业务可靠性），并把**口径写进报告**（`metricBasis`）——两者都由单次观测
  的 p̂ 推导，不是逐任务重复 k 次的实测值。
  参考量级：100 例 95% 首过看起来很好，连续 5 轮全部成功的概率只有约 77%。
- 失败按类别聚簇（`failureClusters`），并逐条产出结构化归因（`failureAttributions`：
  首错步号、错误类别、根因责任方、证据、是否可恢复、置信度）。
  **评测系统自身的故障（超时前的连接问题、清理失败）单独成簇**，不计入模型侧——
  ADS-7.8-07 要求看到表现下降先怀疑评测系统本身。
- `--seed` / `--seeds`：可跑多个随机种子并输出跨种子的均值与波动（规范建议 3–5 个）。
- `standard_error()`：给出成功率的标准误，用于判断分差是否只是噪声
  （n=100、p=0.7 时 95% 置信区间约 ±9 个百分点，"73% vs 70%"不足以支持任何切换）。
- `Client` 显式禁用代理：被测实例固定在 127.0.0.1，开发环境一旦设了 `HTTP_PROXY`，
  本机请求会被发给代理变成 502，并被记成 `evaluation_error` 污染失败簇。

**未做的事**：未新增 Pass^k 的验收门槛。门槛仍是原有的"100 例、首过 ≥95、含一次修复 ≥99"，
Pass^k 目前只是**读数**——先把数字看到，再由人决定是否收紧，避免悄悄换掉验收标准。

### 2.3 P1-1 Anthropic 前缀缓存

Anthropic 不做自动前缀缓存，不在想要复用的前缀末尾显式打点就等于永远按全价重算输入。
此前全仓 `cache_control` 零命中，`SplitDynamicContext` 的收益只落在 OpenAI / DeepSeek 的
自动缓存路径上。现在打两个断点（上限 4 个）：

1. 最后一块 `system` —— 覆盖扮演准则、输出协议、边界声明与角色卡人设（逐字节不变）；
2. 最后一条 `assistant` —— 历史前沿，之后只会追加新回合，因此每轮只需为新增的那一轮付一次
   写缓存成本。

断点只影响计费与延迟，不影响语义：即便某轮没命中（例如历史被压缩改写），也只是退化为全价
计算。已用已有度量 `TokenUsage.Cached` 可验证命中的读数。

### 2.4 P1-2 动态状态块的位置

动态状态块原本是 **`system` 消息，插在历史之后、玩家输入之前**，并且每轮重算、不进入后续
历史。两个问题：与 ADS-2.7-02/2.7-03（末尾 + `user` 槽位）不一致；且缓存按最长公共前缀命中，
插在输入之前时第 t 轮序列是 `[…历史][状态块_t][输入_t]`，第 t+1 轮是
`[…历史][输入_t][回复_t][状态块_{t+1}]`，公共前缀在第 t 轮状态块处断裂，输入与回复都要重算。

现在状态块是**最后一条消息、走 `user` 槽位**、用 `<system-reminder>` 包裹；历史缺口提示与
导演安排一并收进这个块。改动后对话中部不再有 `system` 注入，公共前缀多覆盖一整个回合。

### 2.5 P1-3 提示词单一来源与版本指纹

- 探测提示词 `frameProbeSystem` 此前是 `context.FrameProtocolInstruction` 的**内联拷贝**
  （注释里写着"此处内联避免应用层反向依赖编译包"，但 `application` 本来就依赖 `context`）。
  两份同源字符串必然漂移，后果是"探测通过、真实生成被协议拒绝"。现已改为引用同一常量。
- 新增 `PromptVersion` + `PromptFingerprint()`（12 位），随启动日志、`-version` 与
  `/api/status` 的 `prompt` 字段暴露，并被 `prompt_fingerprint_test.go` 的黄金指纹钉住。
  改了提示词却不 `+1` 版本号，CI 会失败——这强制"改提示词"成为一个有意识的动作。

### 2.6 P2 消融开关与阶段耗时

- `-ablate=<特性,…>`（`dynamic-context` / `memory` / `lorebook` / `summaries` /
  `compaction` / `all`）：列出即关闭，落点在 `CompilerOptions`。取值非法时由 `flag` 包直接
  报错退出——静默降级成"以为关了其实没关"比启动失败危险得多。消融状态随 `/api/status` 的
  `ablation` 字段暴露，使基线评估的产物自带"这一轮关了什么"的自证。
- 编译阶段耗时：`Compile` 内部原本就有 `OnPhase` 计时但没有生产消费者，现已汇总进
  `RuntimeMetrics`，经 `/api/status` 的 `counters.compilePhases` 暴露（按阶段给出
  调用次数 / 总毫秒 / 均值）。未登记的新阶段计入 `other`，不会被静默丢弃。

---

## 3. 显式偏离规范的取舍（规范 0.3 节要求记录理由）

| 条款 | 偏离 | 理由 |
| --- | --- | --- |
| ADS-1.1-01 / 4.x | **没有工具调用**，`ChatRequest` 无 `Tools` 字段，消息角色只有 `system`/`user`/`assistant`，没有 `tool` | 刻意的工作流式设计：动作空间是逐行 JSON 帧协议，模型只能"提议"，结算由确定性规则收据执行。规范 1.4-01 明确要求"可清晰分解为固定子任务时用工作流、只有需要动态决策时才用自主 Agent"，本项目属于前者。代价是模型无法自主检索/查询，收益是行为可预测、可测试、易回归。**若未来要把检索或状态查询交给模型自主调用，必须先补 `tool` 角色与 `tool_call_id` 关联**（ADS-2.1-03）。 |
| ADS-3.4-05 | 记忆整理的审核方是**确定性校验器** `GateMemoryOrganization`，而非异源 Agent Reviewer | 规范 7.4-01 说"凡能写成程序化断言的检查都应继续用断言，LLM 评判只用于确实无法机械判定的维度"。本地单用户产品里，为一次记忆合并再跑一个异源模型，成本与延迟都不可接受。已具备规范 1.7-09 的最小不变量：`提议 → 独立审核（读独立证据）→ 通过才提交`。 |
| ADS-2.8-06（数值 80%） | 压缩触发用**轮数**（保尾 20 轮 + 最少 8 轮未压缩），不是 token 占比阈值 | 保一段剧情的整体感比省 token 更重要；token 侧的压力另有 `TriggerPressure` 路径兜底。规范允许数值偏离但要求记录——此处即用 `turn_usage` 的真实用量持续核对阈值是否偏松，不作为默认行为改动。 |
| ADS-7.7-01 | 未引入 OpenTelemetry / OpenInference | 本地优先、单进程的桌面应用，引入完整追踪栈的收益低于成本。先做"进程内阶段耗时 + 用量台账 + 计数"，等确有跨进程/多端需求再上。 |
| ADS-3.1-12/13 | 未做日志 PII 脱敏 | 数据不出本机（除模型调用），当前没有多租户；脱敏规则库本身的维护成本与误伤风险都不低。**若将来上云同步或多人共享，这一条优先级要提到 P0。** |
| ADS-2.5-01 | 玩家输入未做包裹标记 | 玩家可以在自己本机上写"忽略以上规则"，这属于自我越狱，不影响他人（局域网配对是同一故事的第二屏）。尾部状态块排在玩家输入之后，伪造的状态块在时序上劣于真实的那个。 |

---

## 4. 尚未处理的缺口

1. **`web/src/styles.css` 体积过大**：5340 行，属历史遗留；改动 CSS 时优先拆分或收敛，不再设基线豁免。
2. **真实模型质量仍未闭环**：本次补的是评估口径与归因，不是跑出来的结论。
   `release_eval.py` 仍需真实网关（Gemini meter）才能产出 100 例证据。
3. **OpenTelemetry / span 树**：见第 3 节取舍。
4. **提示词黄金指纹会随提示词变更而失败**：这是设计意图，不是缺陷；流程见
   `internal/context/prompt_fingerprint_test.go` 的注释。

---

## 5. 复现校验

```powershell
# Go 全量与静态检查
go build ./...; go vet ./...
go test -count=1 ./...

# 脚本单测
python -m unittest discover -s scripts/tests

# 提示词指纹与消融状态
go run ./cmd/tavernagent -version
go run ./cmd/tavernagent -provider mock -ablate=all   # /api/status 会回报 ablation

# 前端
cd web; pnpm typecheck; pnpm lint; pnpm test; pnpm build
```
