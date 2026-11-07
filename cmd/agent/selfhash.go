package main

// 自身二进制指纹：用于面板确认各 VPS 上的 agent 是否真的更新到位。
//
// 只看版本号不够——版本号是编译时 -ldflags 注入的字符串，预发布版和正式版
// 可能报同一个 "v1.0.6"，而实际二进制完全不同。sha256 才能证明「跑的是哪一个」。
//
// 计算时机：启动时算一次并缓存。agent 的自我更新会替换二进制文件后重启进程，
// 所以缓存值天然对应「当前正在运行的这份文件」；反过来，如果文件在进程运行期间
// 被外部替换（比如手工 scp 覆盖），缓存值会变成旧文件的 hash，因此额外记录文件
// 的 mtime/size，两者任一变化就重新计算。

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

var (
	selfHashMu      sync.Mutex
	selfHashValue   string
	selfHashPath    string
	selfHashMtime   time.Time
	selfHashSize    int64
	selfHashBuiltAt string
)

// selfExecutablePath 返回「当前进程正在运行的那份二进制」的路径。
//
// 关键：必须走 /proc/self/exe，不能用 os.Executable() 拼出来的安装路径。
//
// 自我更新是「下载到同目录临时文件 → rename 覆盖目标路径」。rename 只换
// 目录项，正在运行的进程仍持有旧 inode。此时若按安装路径去算 hash，读到
// 的是磁盘上的新文件，而上报的心跳却来自旧进程 —— 面板会据此认定
// 「已更新到最新」，实际新版本根本没生效。
// 现场表现：进程 PID 不变、/proc/PID/exe 带 " (deleted)"，但面板显示
// 的 hash 已经是新产物，看不出任何异常。
//
// /proc/self/exe 指向进程真实在执行的 inode，替换前后都准确。
func selfExecutablePath() string {
	if p, err := filepath.EvalSymlinks("/proc/self/exe"); err == nil {
		return p
	}
	// 非 Linux 或 /proc 不可用时退回原路径
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		return resolved
	}
	return exe
}

// fileSHA256 计算文件 sha256，小写十六进制。
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// selfBinaryHash 返回当前 agent 二进制的 sha256。
//
// 文件 mtime 或大小变化会触发重算（外部替换文件的场景）；
// 正常情况下只在首次调用时算一次，避免每个心跳都读 100MB 二进制。
func selfBinaryHash() string {
	path := selfExecutablePath()
	if path == "" {
		return ""
	}

	info, err := os.Stat(path)
	if err != nil {
		return ""
	}

	selfHashMu.Lock()
	defer selfHashMu.Unlock()

	// 命中缓存：同一路径、同样的大小与 mtime
	if selfHashValue != "" && selfHashPath == path &&
		selfHashSize == info.Size() && selfHashMtime.Equal(info.ModTime()) {
		return selfHashValue
	}

	sum, err := fileSHA256(path)
	if err != nil {
		// 算不出来时返回空，面板会显示「未知」而不是卡住心跳
		return selfHashValue
	}

	selfHashValue = sum
	selfHashPath = path
	selfHashSize = info.Size()
	selfHashMtime = info.ModTime()
	selfHashBuiltAt = info.ModTime().UTC().Format(time.RFC3339)
	return selfHashValue
}

// selfBinaryInfo 一次性返回 hash / 路径 / 构建时间 / 大小，打包进心跳。
func selfBinaryInfo() map[string]interface{} {
	hash := selfBinaryHash()

	selfHashMu.Lock()
	info := map[string]interface{}{
		"agent_hash":     hash,
		"agent_path":     selfHashPath,
		"agent_built_at": selfHashBuiltAt,
		"agent_size":     selfHashSize,
		// 架构按发布包命名归一化：Go 在 armv7 设备上报告 "arm"，
		// 而发布包叫 armv7。前端用它分组比较 hash。
		"agent_arch": normalizeArch(runtime.GOARCH),
		// build_id 是本次构建的批次标识，跨架构一致，用于版本一致性判断
		"build_id": BuildID,
	}
	selfHashMu.Unlock()

	// 最近一次自我更新的结果，让面板能看到失败原因而不是只看到「发过指令」
	if text, at := updateStatusInfo(); text != "" {
		info["update_status"] = text
		info["update_status_at"] = at
	}
	return info
}

// normalizeArch 把 runtime.GOARCH 归一化成发布包使用的架构名。
func normalizeArch(goarch string) string {
	if goarch == "arm" {
		return "armv7"
	}
	return goarch
}
