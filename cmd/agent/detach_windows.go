//go:build windows

package main

import "syscall"

// detachSysProcAttr Windows 下的占位实现。
//
// Windows 没有会话脱离的概念，agent 自更新后的重启由服务管理器负责，
// 这里返回 nil 表示使用默认属性。
func detachSysProcAttr() *syscall.SysProcAttr {
	return nil
}
