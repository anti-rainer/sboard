//go:build !windows

package handler

import "syscall"

// detachProcAttr 让子进程脱离父进程的会话与进程组。
//
// 面板自更新时要重启自己：如果重启命令是当前进程的子进程、且留在同一进程组，
// 重启时把自己杀掉会连带中断动作。Setsid 让重启命令成为新会话首进程，
// 与父进程彻底解耦。
func detachProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
