// Package datalock gives one process exclusive ownership of a data directory.
// The operating system releases the lock on crashes; no stale-PID deletion is
// needed. The lock file stays in place to avoid inode replacement races.
package datalock

import (
	"fmt"
	"os"
	"path/filepath"
)

type Lock struct{ file *os.File }

func Acquire(dir string) (*Lock, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, ".tavernagent.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("数据目录已被其他 TavernAgent 进程使用，或无法取得独占锁: %w", err)
	}
	return &Lock{file: f}, nil
}

func (l *Lock) Close() error { return l.file.Close() }
