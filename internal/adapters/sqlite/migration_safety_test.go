package sqlite

import (
	"database/sql"
	"path/filepath"
	"testing"

	"tavernagent/internal/ports"
)

func TestMigrationBackupIsConsistentAndFailureRollsBack(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	session, _, _ := seedSession(t, s)
	// The v14 migration will succeed, then the deliberately preexisting v15
	// columns will fail. Both the schema version and v14 table must roll back.
	if _, err = s.db.Exec(`DROP TABLE memory_projection_snapshots; PRAGMA user_version=13`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if unexpected, err := Open(dir, ports.RealClock{}); err == nil {
		unexpected.Close()
		t.Fatal("broken migration succeeded")
	}
	backups, err := filepath.Glob(filepath.Join(dir, "backups", "*.db"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups=%v err=%v", backups, err)
	}
	for _, path := range []string{filepath.Join(dir, "storage.db"), backups[0]} {
		db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
		if err != nil {
			t.Fatal(err)
		}
		var version, count int
		var integrity, title string
		if err = db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
			t.Fatalf("integrity %s: %s %v", path, integrity, err)
		}
		if err = db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 13 {
			t.Fatalf("version=%d err=%v", version, err)
		}
		if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='memory_projection_snapshots'`).Scan(&count); err != nil || count != 0 {
			t.Fatal("partial migration escaped transaction")
		}
		if err = db.QueryRow(`SELECT title FROM sessions WHERE session_id=?`, session.SessionID).Scan(&title); err != nil || title != session.Title {
			t.Fatal("story lost")
		}
		db.Close()
	}
}

func TestNewerSchemaIsRejectedWithoutChanges(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, ports.RealClock{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`PRAGMA user_version=999`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if unexpected, err := Open(dir, ports.RealClock{}); err == nil {
		unexpected.Close()
		t.Fatal("newer database was opened")
	}
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "storage.db")+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	if err = db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 999 {
		t.Fatal("newer schema was rewritten")
	}
}
