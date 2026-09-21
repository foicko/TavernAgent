//go:build windows

// 单实例守卫：必须在抢数据目录锁**之前**执行。
//
// 顺序是硬约束，不是风格问题。`app.Bootstrap` 会取得数据目录的独占锁，而
// `options.SingleInstanceLock` 只在 `wails.Run` 内部才生效——它排在 Bootstrap 之后。
// 于是第二个实例的真实路径是：
//
//	listen 失败 → 降级到随机端口（打印"本地偏好不会保留"）
//	→ Bootstrap 抢锁失败 → 弹原生错误框 → 退出
//
// `SingleInstanceLock` 永远轮不到执行，用户看到的是"启动报错"而不是"唤起已有窗口"。
// 所以这里把判定提到最前面：先问"是不是已经有一份在跑"，是就唤起它并安静退出。
package main

import (
	"errors"
	"fmt"
	"log"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// singleInstanceMutex 是单实例互斥体名。不带 "Global\" 前缀，作用域是当前登录会话
// （与 Wails 自身一致）：同一台机器的另一个用户会话可以各开一份，各自的
// %APPDATA%\TavernAgent 本来就是分开的。
const singleInstanceMutex = "tavernagent-desktop-single-instance-mutex"

// desktopWindowTitle 必须与 wails.Run 里 options.App.Title 保持一致：
// 唤起已有窗口靠 FindWindowW 按标题查找，标题改了这里也要改。
const desktopWindowTitle = "SillyDog · TavernAgent"

// errorAlreadyExists 是 Win32 的 ERROR_ALREADY_EXISTS（183）。x/sys/windows 内部
// 用它但没有导出，所以这里自带一份。
const errorAlreadyExists = syscall.Errno(183)

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procFindWindowW              = user32.NewProc("FindWindowW")
	procShowWindow               = user32.NewProc("ShowWindow")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procAllowSetForegroundWindow = user32.NewProc("AllowSetForegroundWindow")
)

// singleInstanceHandle 持有互斥体句柄，生命周期与进程一致（退出时由内核回收）。
// 必须留着：句柄一关，互斥体就释放了，单实例判定随之失效。
var singleInstanceHandle windows.Handle

// acquireSingleInstance 尝试成为唯一实例（用产品互斥体名）。
//
// 返回 false 表示已有实例在运行——这是**正常情况**，不是错误：调用方应当唤起
// 已有窗口并安静退出。
func acquireSingleInstance() (bool, error) {
	unique, handle, err := acquireNamedMutex(singleInstanceMutex)
	if err != nil {
		return false, err
	}
	if unique {
		singleInstanceHandle = handle
	}
	return unique, nil
}

// acquireNamedMutex 按名字取得互斥体，并报告是否抢到了唯一所有权。
//
// 名字作为参数而不是直接用常量，是为了让测试能换一个独立名字：否则测试会和开发
// 机上正在运行的桌面端抢同一个内核对象，测试结果取决于"你有没有开着这个应用"。
func acquireNamedMutex(name string) (bool, windows.Handle, error) {
	ptr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return false, 0, fmt.Errorf("构造单实例互斥体名失败: %w", err)
	}
	// CreateMutex 在互斥体已存在时把 ERROR_ALREADY_EXISTS 作为错误返回，这是唯一
	// 无竞态的判据（OpenMutex 再 CreateMutex 中间有窗口期，两个实例可能同时通过）。
	handle, err := windows.CreateMutex(nil, false, ptr)
	if err != nil {
		if errors.Is(err, errorAlreadyExists) {
			return false, handle, nil
		}
		return false, 0, fmt.Errorf("创建单实例互斥体失败: %w", err)
	}
	return true, handle, nil
}

// wakeExistingWindow 把已有实例的窗口从最小化/后台状态拉到前台。
//
// 找不到窗口不算失败：对方可能还在启动中。此时"第二个实例安静退出"依然是正确的
// ——总比弹一个"数据目录被其他进程占用"的错误框好，用户从任务栏点一下即可。
func wakeExistingWindow() {
	title, err := windows.UTF16PtrFromString(desktopWindowTitle)
	if err != nil {
		return
	}
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(title)))
	if hwnd == 0 {
		log.Printf("提示：已有实例在运行，但没找到它的窗口（标题 %q），请从任务栏打开。", desktopWindowTitle)
		return
	}
	const (
		swRestore = 9
		// -1 = ASFW_ANY：允许本进程把窗口带到前台。Windows 默认只让前台进程调用
		// SetForegroundWindow，而这里是用户主动双击触发的，理应放行。
		asfwAny = ^uintptr(0)
	)
	_, _, _ = procAllowSetForegroundWindow.Call(asfwAny)
	_, _, _ = procShowWindow.Call(hwnd, swRestore)
	_, _, _ = procSetForegroundWindow.Call(hwnd)
	log.Printf("已有实例在运行，已唤起它的窗口（本次不再启动）。")
}
