//go:build !windows

// 非 Windows 平台的显式 no-op。
//
// 之所以"报错"而不是"假装成功"：自启是用户在界面上主动打开的一个开关，
// 如果打开后什么都没发生又不说明，用户只会以为程序坏了。这里返回明确的错误，
// 让调用方能在界面上如实说"当前平台不支持"。
package autostart

import "errors"

// ErrUnsupported 表示当前平台没有实现开机自启。
var ErrUnsupported = errors.New("当前平台不支持开机自启（仅 Windows 桌面壳提供）")

// Enable 在非 Windows 上始终返回 ErrUnsupported。
func Enable(string) error { return ErrUnsupported }

// Disable 在非 Windows 上无害地成功（本来就没有自启项可删）。
func Disable() error { return nil }

// Enabled 在非 Windows 上恒为未开启。
func Enabled() (bool, error) { return false, nil }
