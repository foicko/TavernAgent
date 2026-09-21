package main_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTavernctlAuditOnFreshDir(t *testing.T) {
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "data")

	cmd := exec.Command("go", "run", "./cmd/tavernctl", "audit", "--data", dataDir)
	cmd.Dir = "../.." // run from root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tavernctl audit failed: %v\nOutput:\n%s", err, string(out))
	}

	output := string(out)
	if !strings.Contains(output, "SQLite PRAGMA integrity_check") {
		t.Errorf("expected audit output to mention SQLite integrity check, got:\n%s", output)
	}
	if !strings.Contains(output, "全部存储与分支数据通过自检，状态完好") {
		t.Errorf("expected clean audit conclusion, got:\n%s", output)
	}
}

func TestTavernctlVersionAndHelp(t *testing.T) {
	cmd := exec.Command("go", "run", "./cmd/tavernctl", "version")
	cmd.Dir = "../.."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("version failed: %v\nOutput: %s", err, string(out))
	}
	if !strings.Contains(string(out), "tavernctl") {
		t.Errorf("expected version output, got: %s", string(out))
	}
}
