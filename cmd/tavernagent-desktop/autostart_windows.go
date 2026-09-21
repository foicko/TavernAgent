//go:build windows

package main

import (
	"fmt"
	"log"
	"strings"

	"tavernagent/internal/util/autostart"
)

// applyAutostart 处理 `-autostart on|off`：写/删注册表项后立即返回，不打开窗口。
//
// 为什么先用命令行开关而不是托盘菜单：自启是"设一次就不再动"的偏好，而托盘菜单
// 需要在设备上人工验证。先用一个可脚本化、可断言的入口把它落地，托盘后续接同一个
// 函数即可——两处都调 autostart 包，不存在两套语义。
func applyAutostart(spec string) error {
	switch strings.ToLower(strings.TrimSpace(spec)) {
	case "on":
		if err := autostart.Enable(""); err != nil {
			return fmt.Errorf("开启开机自启失败: %w", err)
		}
		log.Printf("开机自启已开启（注册表 HKCU\\...\\Run）")
		return nil
	case "off":
		if err := autostart.Disable(); err != nil {
			return fmt.Errorf("关闭开机自启失败: %w", err)
		}
		log.Printf("开机自启已关闭")
		return nil
	default:
		return fmt.Errorf("不支持的 -autostart 取值 %q；可用 on / off", spec)
	}
}
