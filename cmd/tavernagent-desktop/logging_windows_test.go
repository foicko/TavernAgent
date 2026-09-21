//go:build windows

package main

import (
	"bytes"
	"errors"
	"testing"
)

// deadWriter 模拟一个写不进去的目标：GUI 子系统程序双击启动时 os.Stderr 就是这样
// （进程没有控制台，句柄无效）。
type deadWriter struct{}

func (deadWriter) Write([]byte) (int, error) { return 0, errors.New("无效句柄") }

// TestLogSinkKeepsFileCopyWhenStderrFails 守的是这个回归：
//
// 用 io.MultiWriter 时，它在第一个 writer 写失败处直接 return，后面的目标全部跳过。
// 于是"写 stderr 失败"会连带把日志文件那份副本吞掉——双击启动的桌面端日志文件恒为
// 空，而那时文件是唯一的排障入口（没有控制台可看）。日志目标之间必须互不牵连。
func TestLogSinkKeepsFileCopyWhenStderrFails(t *testing.T) {
	file := &bytes.Buffer{}
	sink := logSink{deadWriter{}, file}

	const line = "桌面端服务就绪\n"
	n, err := sink.Write([]byte(line))
	if err != nil {
		t.Fatalf("单个目标写失败不应被当作 logSink 的失败: %v", err)
	}
	if n != len(line) {
		t.Fatalf("短写会让 log 包判定为写失败并丢内容: n=%d 期望 %d", n, len(line))
	}
	if file.String() != line {
		t.Fatalf("stderr 写失败时文件副本丢失，得到 %q", file.String())
	}
}

// TestLogSinkToleratesNilWriter 确认空目标只是被跳过，不会中断后续目标。
func TestLogSinkToleratesNilWriter(t *testing.T) {
	file := &bytes.Buffer{}
	sink := logSink{nil, file}

	if _, err := sink.Write([]byte("x")); err != nil {
		t.Fatalf("空目标不应产生错误: %v", err)
	}
	if file.String() != "x" {
		t.Fatalf("空目标之后的 writer 仍应收到内容，得到 %q", file.String())
	}
}
