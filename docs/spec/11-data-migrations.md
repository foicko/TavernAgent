# 11 数据目录与迁移契约

## 1. 数据目录布局

数据目录由 `-data` 指定（默认 `data/`，见 `cmd/tavernagent`）。**运行时数据只在 `data/`，且 gitignore；绝不入库。**

| 路径 | 内容 | 创建者 |
|---|---|---|
| `data/storage.db`（+ `-wal`/`-shm`） | SQLite 权威存储（WAL 模式） | `adapters/sqlite/store.go` |
| `data/backups/storage-v<V>-before-v<N>-<ts>.db` | 迁移前 `VACUUM INTO` 备份（仅 `user_version>0` 时） | `adapters/sqlite/store.go` |
| `data/.tavernagent.lock` | 单实例 OS 文件锁（常驻，崩溃时 OS 释放） | `util/datalock` |
| `data/config/settings.json` | 非密模型配置（v2：models + slots） | `adapters/config/file.go` |
| `data/config/secrets.json` | 明文 API 密钥（`0o600`） | `adapters/config/file.go` |
| `data/config/settings.json.v1.bak` | v1→v2 一次性迁移备份 | `adapters/config/file.go` |
| `data/cards/<cardID>/` | charx/PNG 内嵌资源 | `adapters/cardassets` + `internal/app` |
| `data/images/` | 生成的图像 | `adapters/image` |
| `data/tls/`（`tls.crt`/`tls.key`） | 自签证书 | `util/tlscert` |

- 空数据目录被拒绝（`internal/app`）。
- **单实例锁在 `app.Bootstrap` 内获取，且在所有初始化之前、最后释放**（LIFO closer）。

## 2. 单实例锁（`util/datalock`）

- 目录 `0o700`；锁文件 `.tavernagent.lock`，`O_CREATE|O_RDWR, 0o600`。
- Windows：`LockFileEx`（独占 + 立即失败）；Unix：`flock(LOCK_EX|LOCK_NB)`。
- 崩溃时 OS 释放锁，无需清理陈旧 PID；锁文件常驻以避免 inode 竞争。

## 3. SQLite 迁移

- **当前 16 个迁移（V1–V16）**，用 `PRAGMA user_version` 跟踪；定义分散在 `adapters/sqlite/*.go`，按 `schema.go` 中的有序切片收集。
- 应用流程（`Store.migrate`）：
  1. 读 `user_version`。
  2. 若 `v > len(migrations)` → **拒绝**（不重写）。
  3. 若已是最新 → 空操作。
  4. 若 `v > 0` → 先 `VACUUM INTO` 备份到 `data/backups/`。
  5. **所有待应用迁移在同一事务内**逐个执行，每个迁移后在同一事务内 `PRAGMA user_version = i+1`；任一失败即 `Rollback`。
- 迁移后：`backfillSessionIdentities()`、FTS 索引重建。
- 守卫测试：`migration_safety_test.go`（强制中途失败 → 版本与表一起回滚、备份可用、故事数据存活；`user_version=999` → 拒绝且不重写）。

## 4. 规则（强制）

- **每个迁移一个版本号，只追加不修改已发布迁移。** 修改已发布迁移会导致新老库分叉。
- 迁移必须是**幂等可重放**的 SQL 或能在事务内安全执行；不得依赖外部状态。
- 迁移**不得删除用户故事数据**；需要清理时用显式、可审计的步骤。
- 新增迁移时：在对应功能文件加 SQL 常量 → 加入有序切片 → 加守卫测试（安全/不可逆路径）。
- **绝不**把 `data/`、`build/`、`output/`、`chars/` 或任何密钥/图片入库（`release_audit.py --strict` 会拦截）。

## 5. 差距

- 迁移定义分散在多个功能文件（`director.go`、`usage.go`、`turnlifecycle.go`、`memory_projection.go`、`migrations_m4.go`）而非独立目录。规范不强制拆分，但要求**有序切片是唯一注册点**，且每个迁移有注释说明目的与版本。
