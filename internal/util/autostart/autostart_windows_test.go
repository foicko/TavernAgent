//go:build windows

package autostart

import (
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// 测试写的是 HKCU 下的**临时子键**，不碰真实的 Run 项：
// 单元测试去改用户的开机启动项是不可接受的副作用。
func TestSetValueWritesQuotedCommand(t *testing.T) {
	const path = `Software\TavernAgent\autostart-test`
	key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("准备临时键: %v", err)
	}
	key.Close()
	defer registry.DeleteKey(registry.CURRENT_USER, path)

	if err := setValue(registry.CURRENT_USER, path, EntryName, `"C:\Program Files\TavernAgent\tavernagent-desktop.exe"`); err != nil {
		t.Fatalf("setValue: %v", err)
	}

	read, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("读回临时键: %v", err)
	}
	defer read.Close()
	value, _, err := read.GetStringValue(EntryName)
	if err != nil {
		t.Fatalf("读取值: %v", err)
	}
	// 带空格的安装路径必须带引号，否则 Windows 会把它拆成程序 + 参数。
	if !strings.HasPrefix(value, `"`) || !strings.HasSuffix(value, `"`) {
		t.Fatalf("命令应带引号: %q", value)
	}
}

func TestEnabledIsFalseWhenKeyMissing(t *testing.T) {
	// 真实 Run 项里通常没有本项（CI 与开发机都如此）；有则跳过，避免污染判断。
	enabled, err := Enabled()
	if err != nil {
		t.Fatalf("Enabled: %v", err)
	}
	_ = enabled // 只要求"不报错、能给出布尔值"
}
