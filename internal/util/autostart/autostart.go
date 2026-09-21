// Package autostart 管理"开机自启"。
//
// 只在 Windows 上真正生效（写 HKCU\...\Run）；其它平台是显式的 no-op——
// 桌面壳目前也只有 Windows 一种，但包要能在任何平台编译与测试。
//
// 为什么不用任务计划程序：Run 项不需要管理员权限、不需要额外文件，
// 且用户能在"任务管理器 → 启动"里看到并关掉它——把控制权留在用户手上。
package autostart

// EntryName 是注册表里显示的名字（任务管理器的启动项列表用的就是它）。
const EntryName = "TavernAgent"
