// Package web 提供前端单二进制内嵌文件系统支持。
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// DistFS 返回以 dist 目录为根的嵌入式文件系统。
func DistFS() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
