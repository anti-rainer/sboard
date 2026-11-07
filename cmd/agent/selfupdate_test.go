package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 自我更新的关键陷阱：rename 覆盖二进制后，进程仍持有旧 inode。
//
// 如果按安装路径读 hash，读到的是磁盘上的新文件，而上报心跳的却是旧进程，
// 面板会据此认为「已更新到最新」，实际新版本根本没生效。
// 现场表现就是 openwrt 那次：面板显示 hash 已是新产物，PID 却纹丝不动。
//
// selfExecutablePath 必须走 /proc/self/exe，指向进程真实执行的 inode。
func TestSelfExecutablePathFollowsRunningInode(t *testing.T) {
	// 当前测试进程的真实可执行文件
	real, err := filepath.EvalSymlinks("/proc/self/exe")
	if err != nil {
		t.Skip("当前环境没有 /proc/self/exe")
	}

	got := selfExecutablePath()
	if got == "" {
		t.Fatal("selfExecutablePath 返回空")
	}

	// 解析到同一 inode 才算指向「正在运行的那份」
	gotInfo, err := os.Stat(got)
	if err != nil {
		t.Fatalf("stat 结果路径失败: %v", err)
	}
	realInfo, err := os.Stat(real)
	if err != nil {
		t.Fatalf("stat /proc/self/exe 失败: %v", err)
	}

	if !os.SameFile(gotInfo, realInfo) {
		t.Errorf("selfExecutablePath 指向的不是当前运行的 inode\n"+
			"  got:  %s\n  want: %s", got, real)
	}
}

// rename 覆盖后，安装路径与运行中 inode 必须能被区分开。
// 这是 selfExecutablePath 存在的理由：两者在更新后是不同的文件。
func TestRenameMakesPathDivergeFromRunningInode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "binary")

	if err := os.WriteFile(path, []byte("old-content"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	// 模拟自我更新：写临时文件再 rename 覆盖（与 handleSelfUpdate 一致）
	tmp := path + ".new"
	if err := os.WriteFile(tmp, []byte("new-content-different-length"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}

	newInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	// 路径指向的已是新 inode；任何仍持有旧 inode 的进程都跑着旧代码
	if os.SameFile(oldInfo, newInfo) {
		t.Error("rename 后 inode 未变化，与预期不符")
	}

	// 内容也应更新，证明替换真实发生
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new-content-different-length" {
		t.Errorf("内容未更新: %q", data)
	}
}

// 更新状态必须能记录失败原因，否则面板只能看到「发过指令」。
func TestUpdateStatusRecordsFailure(t *testing.T) {
	updateStatusMu.Lock()
	updateStatusText = ""
	updateStatusAt = time.Time{}
	updateStatusMu.Unlock()

	// 未记录任何状态时不上报
	if text, _ := updateStatusInfo(); text != "" {
		t.Errorf("初始状态应为空，得到 %q", text)
	}

	// 记录一次失败
	setUpdUpdateStatus("下载更新包", errString("HTTP 404"))

	text, at := updateStatusInfo()
	if text == "" {
		t.Fatal("失败状态未被记录")
	}
	if at == "" {
		t.Error("缺少状态时间戳")
	}
	// 必须包含阶段与原因，面板才能定位
	for _, want := range []string{"下载更新包", "404"} {
		if !containsStr(text, want) {
			t.Errorf("状态 %q 缺少 %q", text, want)
		}
	}
}

// 成功与失败要能区分开。
func TestUpdateStatusRecordsSuccess(t *testing.T) {
	updateStatusMu.Lock()
	updateStatusText = ""
	updateStatusAt = time.Time{}
	updateStatusMu.Unlock()

	setUpdUpdateStatus("替换二进制", nil)

	text, _ := updateStatusInfo()
	if !containsStr(text, "成功") {
		t.Errorf("成功状态应含「成功」，得到 %q", text)
	}
	if containsStr(text, "失败") {
		t.Errorf("成功状态不应含「失败」，得到 %q", text)
	}
}

// --- 测试小工具，避免引入额外 import ---

type errString string

func (e errString) Error() string { return string(e) }

func containsStr(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
