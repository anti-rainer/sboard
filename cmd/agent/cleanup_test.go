package main

import (
	"os"
	"path/filepath"
	"testing"
)

// 清理更新遗留文件：只删自己那几种后缀，绝不能误删别的。
//
// 历史遗留会占实质空间（旧版每次更新留一个近百 MB 的 .bak，
// 多台机器累积起来很可观），但也正因为是删除操作，
// 必须确认匹配范围严格限定在 <binary>.* 的已知后缀上。
func TestCleanupUpdateLeftovers(t *testing.T) {
	dir := t.TempDir()
	base := "sboard-agent"

	// 该删的：更新过程中的临时与历史备份
	shouldRemove := []string{
		base + ".bak",
		base + ".bak-20261028-162104",
		base + ".zip",
		base + ".checksums.txt",
		base + ".new",
	}
	// 不该删的：可执行本体、配置、以及与更新无关的其他文件
	keep := []string{
		base,
		"agent.json",
		"net_static.json",
		"other.bak",           // 不是本二进制的备份
		base + "2",            // 前缀相同但不是 <binary>.
		"server.crt",
	}

	for _, name := range append(append([]string{}, shouldRemove...), keep...) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	removeUpdateLeftoversIn(dir, base)

	for _, name := range shouldRemove {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			t.Errorf("应删除但仍在: %s", name)
		}
	}
	for _, name := range keep {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("误删了不该动的文件: %s", name)
		}
	}
}

// 目录不存在或不可读时必须安全返回，不能 panic。
func TestCleanupUpdateLeftoversMissingDir(t *testing.T) {
	removeUpdateLeftoversIn(filepath.Join(t.TempDir(), "not-exist"), "sboard-agent")
}
