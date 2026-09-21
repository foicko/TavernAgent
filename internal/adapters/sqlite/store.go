// Package sqlite 实现权威持久层：事务、查询、索引、迁移。
// SQLite 是唯一权威存储（技术契约 §6）；JSONL 只在导出时生成。
package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// Store 是 SQLite 实现。
type Store struct {
	projectionBuildMu sync.Mutex
	projectionMu      sync.Mutex
	projectionCache   map[string][]*domain.MemoryRecord
	projectionOrder   []string
	databasePath      string
	db                *sql.DB // 独占写入连接（MaxOpenConns = 1），用于短写事务与数据迁移
	reader            *sql.DB // 并发只读连接池（MaxOpenConns = max(4, NumCPU)），用于并发只读查询
	clock             ports.Clock
	mu                sync.Mutex // 写事务串行化（短事务，满足单进程模型）
	// fts 表示记忆词法索引（FTS5）是否可用。不可用时检索退回 Go 侧双字重合。
	fts     bool
	trigram bool
	loreFTS bool
}

var _ ports.Store = (*Store)(nil)

// rdb 返回只读查询连接池；若未初始化则回退至主连接。
func (s *Store) rdb() *sql.DB {
	if s.reader != nil {
		return s.reader
	}
	return s.db
}

// Open 打开并迁移数据目录下的 storage.db。
func Open(dataDir string, clock ports.Clock) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, "storage.db")
	dsn := "file:" + path +
		"?_pragma=foreign_keys(1)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=synchronous(FULL)"
	db, err := sql.Open(DriverName, dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // 单写协程化，保持 SQLite 一致性
	if clock == nil {
		clock = ports.RealClock{}
	}
	s := &Store{db: db, clock: clock, databasePath: path}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.backfillSessionIdentities(); err != nil {
		db.Close()
		return nil, err
	}
	// 词法索引在迁移之后建立（它只是加速结构，失败必须只降级不阻断开库）。
	if err := s.ensureMemoryIndex(); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.ensureLorebookIndex(); err != nil {
		db.Close()
		return nil, err
	}

	// 建立并发只读连接池（PERF-01）
	numReaders := runtime.NumCPU()
	if numReaders < 4 {
		numReaders = 4
	}
	readerDSN := "file:" + path +
		"?_pragma=query_only(1)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)"
	reader, err := sql.Open(DriverName, readerDSN)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("open sqlite reader pool: %w", err)
	}
	reader.SetMaxOpenConns(numReaders)
	reader.SetMaxIdleConns(numReaders)
	s.reader = reader

	return s, nil
}

func (s *Store) Close() error {
	var errs []error
	if s.reader != nil {
		if err := s.reader.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.db != nil {
		if err := s.db.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *Store) migrate() error {
	var v int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return err
	}
	if v > len(migrations) {
		return fmt.Errorf("数据库版本 %d 高于本程序支持的 %d；请使用较新程序或恢复升级前备份", v, len(migrations))
	}
	if v == len(migrations) {
		return nil
	}
	if v > 0 && s.databasePath != "" {
		dir := filepath.Join(filepath.Dir(s.databasePath), "backups")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		backup := filepath.Join(dir, fmt.Sprintf("storage-v%d-before-v%d-%d.db", v, len(migrations), time.Now().UnixNano()))
		if _, err := s.db.Exec("VACUUM INTO ?", backup); err != nil {
			return fmt.Errorf("升级前备份失败: %w", err)
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i := v; i < len(migrations); i++ {
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// SystemInfo 探测 SQLite 驱动版本与 FTS5 可用性。
// Checkpoint 把 WAL 合并回主库并截断 WAL 文件。
func (s *Store) Checkpoint() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	return err
}

func (s *Store) SystemInfo() (ports.SystemInfo, error) {
	info := ports.SystemInfo{}
	var sqliteVer string
	row := s.db.QueryRow("SELECT sqlite_version()")
	_ = row.Scan(&sqliteVer)
	info.DriverVersion = fmt.Sprintf("%s (SQLite %s)", DriverKind, sqliteVer)
	_, err := s.db.Exec("CREATE VIRTUAL TABLE IF NOT EXISTS temp.fts5_probe USING fts5(x)")
	info.FTS5Available = err == nil
	if err != nil {
		info.AttributeError = err.Error()
	} else {
		_, _ = s.db.Exec("DROP TABLE IF EXISTS temp.fts5_probe")
	}
	return info, nil
}

// ---- 时间与编码 ----

func (s *Store) now() string { return s.clock.Now().UTC().Format(time.RFC3339Nano) }

func (s *Store) parseTime(v string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return time.Time{}
	}
	return t
}

// ---- nodes & events ----

type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// queryer 是 *sql.DB 与 *sql.Tx 的查询子集。
// StateAt 的重放路径要在事务内（提交前 CAS 校验）与事务外（读取）通用。
type queryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// checkpointInterval 是稀疏检查点间隔（技术契约 §7：每 20 个节点建检查点）。
// 每节点都存完整状态投影会让长链的存储与恢复成本线性膨胀，
// 因此只在该间隔落快照；其余节点的状态由「最近快照 + 事件重放」得到。
const checkpointInterval = 20

// shouldCheckpoint 判定某深度的节点是否要落一份完整快照。
// 深度 1 是首个回合（根节点快照由 Setup 写入，深度 0），
// 之后每 checkpointInterval 层一份，保证任意节点的重放步数 < checkpointInterval。
func shouldCheckpoint(depth int) bool {
	if depth <= 1 {
		return true
	}
	return depth%checkpointInterval == 0
}

func mapErr(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrItemNotFound // 复用：统一 "not found" 语义由调用方解释
	}
	return err
}

// HashState 计算世界状态的规范哈希（委托 domain，保持兼容）。
func HashState(s *domain.WorldState) string { return s.HashID() }

// MarshalState 序列化世界状态（委托 domain，保持兼容）。
func MarshalState(s *domain.WorldState) string {
	b, _ := s.Marshal()
	return b
}

// UnmarshalState 反序列化世界状态。
func UnmarshalState(data string) (*domain.WorldState, error) {
	return domain.UnmarshalWorld(data)
}

// SaveSummary 写入摘要产物（实现见 summaries.go，此处满足 ports.Store 组合）。
// SaveSummary 由 summaries.go 提供。

// IntegrityCheck runs SQLite PRAGMA integrity_check and foreign_key_check.
func (s *Store) IntegrityCheck() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var issues []string
	rows, err := s.db.Query("PRAGMA integrity_check")
	if err != nil {
		return nil, fmt.Errorf("integrity_check: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var res string
		if err := rows.Scan(&res); err != nil {
			return nil, err
		}
		if res != "ok" {
			issues = append(issues, "integrity_check: "+res)
		}
	}

	fkRows, err := s.db.Query("PRAGMA foreign_key_check")
	if err != nil {
		return nil, fmt.Errorf("foreign_key_check: %w", err)
	}
	defer fkRows.Close()

	for fkRows.Next() {
		var table, parent string
		var rowid int64
		var fkid int
		if err := fkRows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			return nil, err
		}
		issues = append(issues, fmt.Sprintf("foreign_key_violation in table %s, rowid %d -> parent %s", table, rowid, parent))
	}

	return issues, nil
}
