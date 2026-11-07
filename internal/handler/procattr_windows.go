//go:build windows

package handler

import "syscall"

// detachProcAttr Windows 下的占位实现。
// 面板自更新在 Windows 上直接拒绝（见 runPanelSelfUpdate），不会走到这里。
func detachProcAttr() *syscall.SysProcAttr {
	return nil
}
