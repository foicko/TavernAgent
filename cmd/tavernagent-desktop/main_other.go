//go:build !windows

// 桌面壳目前只支持 Windows（Wails v2 在 macOS/Linux 侧需要 cgo 与系统 WebKit，
// 会破坏本项目 CGO_ENABLED=0 的跨平台构建矩阵）。
//
// 这个文件保证 `go build ./...`、`go vet ./...` 在非 Windows 平台仍能通过，
// 而不是报"build constraints exclude all Go files"。
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "tavernagent-desktop 目前仅支持 Windows；其他平台请使用无头服务 cmd/tavernagent。")
	os.Exit(1)
}
