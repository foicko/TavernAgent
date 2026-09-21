//go:build windows

package autostart

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// runKeyPath 是当前用户的"登录时运行"项。用 HKCU 而不是 HKLM：
// 后者需要管理员权限，而自启是用户的个人偏好，不该要求提权。
const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

// Enable 写入自启项。exePath 为空时使用当前进程的可执行文件。
//
// 命令带引号：安装路径常含空格（Program Files、中文目录），
// 不带引号会被 Windows 拆成"程序 + 参数"，表现为开机时什么都没发生。
func Enable(exePath string) error {
	command := strings.TrimSpace(exePath)
	if command == "" {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("无法确定可执行文件路径: %w", err)
		}
		command = exe
	}
	return setValue(registry.CURRENT_USER, runKeyPath, EntryName, `"`+command+`"`)
}

// Disable 删除自启项；不存在时按幂等处理（"已经没有了"与"删成功"是同一个结果）。
func Disable() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return nil
	}
	defer key.Close()
	if err := key.DeleteValue(EntryName); err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}

// Enabled 报告当前是否已开启自启。
func Enabled() (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		// 键不存在 = 从未设置过任何自启项，等同于"未开启"。
		return false, nil
	}
	defer key.Close()
	_, _, err = key.GetStringValue(EntryName)
	if err == registry.ErrNotExist {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// setValue 是 Enable 的可注入版本（测试用临时子键，避免动真实启动项）。
func setValue(root registry.Key, path, name, value string) error {
	key, _, err := registry.CreateKey(root, path, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("打开注册表项失败: %w", err)
	}
	defer key.Close()
	return key.SetStringValue(name, value)
}
