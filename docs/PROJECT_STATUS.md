# TavernAgent / SillyDog 项目现状与待办

本文件是项目的**唯一权威文档**：记录当前架构、已交付能力、工程门禁、已知技术债与待做事项。设计取舍与历史决策已并入本文；面向终端用户的安装运维与授权信息见 [运行维护说明](OPERATIONS.md) 与 [素材授权](ASSET_LICENSES.md)。

> 最近更新：2026-09-15

## 1. 项目概述

本地优先的剧情与角色扮演应用：Go 单二进制服务 + SQLite 故事库 + React 界面。回合、人物状态、物品与分支保存在本机；记忆修订以新节点追加，支持历史回看与剧情包导入导出。

- **技术栈**：Go 1.26.6、SQLite（`modernc.org/sqlite`，无 CGO）、React 19 + TypeScript + Vite + Zustand、Node 24 / pnpm 11.9.0。
- **交付形态**：前端经 `//go:embed` 编译期嵌入，产物为单个可执行文件；发布目标为 Windows/macOS/Linux 五平台。
- **默认边界**：默认回环绑定、不调用任何未配置的外部模型；局域网访问需配对码。

## 2. 架构

分层依赖按以下约定组织（禁止反向依赖，例如 domain 不得 import application）：

```
internal/
├── domain/       纯领域：世界状态投影、事件重放、规则、导演/认知模型（不依赖任何外层）
├── ports/        端口接口：存储、模型、时钟、用量台账
├── context/      上下文编译：预算、召回、摘要折叠、提示词、导演上下文、账本
├── application/  应用编排：回合生命周期、提交计划、记忆、导演、存档、分支、计划
├── protocol/     逐行 JSON 帧协议的解析与增量还原
├── pack/         剧情包（.tavernpack）读写
├── search/       检索
├── adapters/
│   ├── sqlite/   存储实现、迁移、投影
│   ├── http/     REST + SSE 契约（含鉴权、来源校验）
│   ├── config/   模型配置（实例 + 三槽位，原子保存、凭据脱敏）
│   └── providers/ 三条协议适配器（chat / responses / anthropic）+ providerutil 共享
└── util/         原子文件、数据目录锁、ID
```

## 3. 已交付能力

| 领域 | 能力 |
| --- | --- |
| 回合一致性 | 事务化状态转换；终态、结果节点、分支锁、收据、outbox 一致；迟到取消/失败/旧 attempt 不覆盖已提交结果 |
| 中断恢复 | 数据目录 OS 锁先于恢复；完整结果重校验提交，部分正文待续写，其余以 `PROCESS_INTERRUPTED` 失败；重复恢复不重复结算 |
| 状态与规则 | 关系/情绪/目标/承诺/场景/里程碑；物品整实例转移与数量消耗；骰子复用与重新检定；秘密过滤 |
| 记忆 | 来源引文五层核验、实体别名消歧、置信度、检索、修订（copy-on-write）、置顶/隐藏/恢复、整理流水线与配额 |
| 上下文 | 承载摘要 + 最近对话；预算与截断；8K/32K/128K；摘要失效区间与受阻退避 |
| 导演模式 | 三阶段大纲、AI 讨论协商、事件驱动进度推进、暂停/恢复/跳过/回退，随分支与剧情包保存 |
| 分支与历史 | 分叉、重生成（候选版本）、编辑输入/正文、只读历史、图谱与剧情/记忆图 |
| 剧情包 | 导出/导入往返、引用重映射、损坏与路径穿越拒绝、导入后续写 |
| 模型配置 | 模型实例 CRUD + primary/assist/reflection 三槽位；服务商预设；密钥脱敏；探测；用量台账 |
| 协议适配 | Chat Completions、Responses、Anthropic Messages；思考链增量；截断归类；超时/取消区分 |
| 局域网 | 可绑定 `0.0.0.0`、随机配对码、恒时鉴权、来源白名单、请求限额、SSE 截止时间 |
| 前端 | 双主题设计系统 + 原语；阅读/工作台双视口流式渲染；IME、焦点、滚动、网络重试 |

## 4. 运行与构建

```powershell
# 开发（离线 Mock）
Set-Location web; pnpm install --frozen-lockfile; pnpm build; Set-Location ..
go run ./cmd/tavernagent -provider mock
# 真实模型
go run ./cmd/tavernagent -data ./data -addr 127.0.0.1:8890
```

- 前端 `web/dist` 不入库、被编译期嵌入，**必须先 `pnpm build` 再执行任何 Go 构建**。
- 热更新：`powershell -ExecutionPolicy Bypass -File scripts/dev.ps1`（前端 <http://localhost:5173>）。
- 完整构建：`powershell -ExecutionPolicy Bypass -File scripts/build.ps1 -Version dev`，产物 `build/tavernagent.exe` 与 `build/BUILD-INFO.json`。

## 5. 质量与门禁

```powershell
go test -count=1 ./...          # 全部 Go 测试
go vet ./...                    # 静态检查
golangci-lint run               # lint（版本锁 .golangci-version）
python -m unittest discover -s scripts/tests   # 脚本单测
python scripts/release_audit.py --strict       # 许可证/合规审计
# 前端
Set-Location web; pnpm typecheck; pnpm lint; pnpm test; pnpm build
```

CI（`.github/workflows/quality.yml`）在 Ubuntu 跑 `go test -race`、`govulncheck`、浏览器测试，并在五平台构建 + 解压冒烟。真实模型验收由 `scripts/release_eval.py` 与 `scripts/gemini_meter.py` 负责，普通 CI 不调用付费模型。

评估报告同时给出 `passAtK`（能力上限）与 `passPowerK`（业务可靠性）两个口径，失败按类别聚簇
并逐条记录首错归因，见 `scripts/eval_metrics.py` 与 [ADS 合规记录](ADS_GAP_ANALYSIS.md)。
造裸模型基线用消融开关（列出即关闭，`-ablate=all` 表示全关）：

```powershell
go run ./cmd/tavernagent -provider mock -ablate=all   # /api/status 的 ablation 字段自证
python scripts/eval_protocol.py --base-url http://127.0.0.1:8890 --kind mock --model-label scripted-fixture -n 20 --seeds 1,2,3 --out output/protocol.json
```

## 6. 已知技术债

- **真实模型质量未闭环**：当前主要是确定性回归与 Mock 链路；真实模型的成功率有了口径与归因
  （`scripts/eval_metrics.py` 的 Pass@k / Pass^k、失败聚簇、多种子波动），但**结论本身仍需**
  用 `scripts/release_eval.py` 跑真实网关产出 100 例证据。
- **大函数/大文件**：`internal/adapters/http/server.go`、`internal/application/commitplan.go:buildPlan`、`internal/domain/state.go:ApplyEvent`、`internal/adapters/sqlite/import.go:ImportSession`、`internal/context` 若干函数等；只在需要改动时再拆。
- **前端样式体积**：`web/src/styles.css` 单文件 5340 行，改动时优先拆分或收敛。
- **发布门禁**：五平台实机运行与真实模型验收尚未全部完成。

## 7. 待做功能

1. **发布闭环**：完成真实模型协议/叙事验收与五平台解压产物实机验证，产出脱敏、与 `BUILD-INFO.json` 指纹一致的报告。
2. **逐步偿还技术债**：按“改到哪、拆到哪”拆分大函数/大文件。
3. **M5 规划**：桌面原生壳层（Tauri 2 / Wails）、群像聚光灯、Edge-TTS 流式发音、立绘联动、外部受控 MCP、多 NPC 视角隔离。
4. **LAN 增强**：确有手机访问需求时推进第二设备网络/防火墙验证。

## 8. 相关文件

- [ADS 合规评审与整改记录](ADS_GAP_ANALYSIS.md)：对照 `Agent开发规范.md`（ADS v1.0）的
  整改项、显式取舍（无工具的工作流式设计、压缩用轮数触发、未上 OTel 等）与未处理缺口。
- [运行维护说明](OPERATIONS.md)：安装、三槽位配置、局域网、备份与升级回退。
- [素材授权](ASSET_LICENSES.md)、[第三方声明](../THIRD_PARTY_NOTICES.md)、[MIT 许可证](../LICENSE)。
- [贡献指南](../CONTRIBUTING.md)、[安全报告](../SECURITY.md)。
- 架构图（脚本生成）：`docs/artifacts/architecture/architecture.svg`、`functions.svg`。
