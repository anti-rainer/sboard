//go:build !windows

package main

import "syscall"

// detachSysProcAttr 让子进程脱离父进程的会话与进程组。
//
// 自我更新后要重启自己：如果重启命令留在同一进程组、且是当前进程的
// 子进程，procd 会把它当成当前实例的一部分，kill 时父子互相牵制，
// 结果什么都没重启。Setsid 让脚本成为新会话首进程，与父进程解耦。
func detachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
