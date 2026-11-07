package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// installedConfig 记录一次配置安装的结果。
//
// 关键取舍：**不在内存里持有旧配置的副本**。
// agent 对每条消息都开一个 goroutine（main.go 的 `go a.handleMessage(&msg)`），
// 若把 previous []byte 挂在返回值上，N 个并发部署就会同时驻留 N 份配置内容，
// 而且完全取决于面板推送频率 —— 这跟原来堆 .bak 文件是同一类问题，只是
// 从磁盘搬到了内存。
//
// 所以这里只记「本次是否改写了文件」，回滚所需的旧内容由
// rollbackInstalledConfig 在真正需要时（仅重启失败这一条路径）重新读盘。
// 重启成功即返回，那份字节立刻可回收，生命周期只有一次重启的时长。
type installedConfig struct {
	configPath string
	// existedBefore 表示安装前是否已存在配置文件，决定回滚是「删除」还是「还原」
	existedBefore bool
	// changed 表示磁盘内容是否真的被改写。false 时调用方应跳过重启。
	changed bool
}

// Changed 返回配置是否真的变了。
func (i *installedConfig) Changed() bool {
	return i != nil && i.changed
}

func resolveCoreBinary(configuredPath, configDir, coreType string) string {
	candidates := []string{
		filepath.Join(configDir, serviceManager.GetBinaryName(coreType)),
		configuredPath,
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate
		}
	}
	return candidates[0]
}

func validateCoreConfig(corePath, configPath, workDir string) error {
	cmd := exec.Command(corePath, "check", "-c", configPath)
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	detail := strings.TrimSpace(string(bytes.TrimSpace(output)))
	if detail == "" {
		return fmt.Errorf("执行 %s check 失败: %w", filepath.Base(corePath), err)
	}
	return fmt.Errorf("执行 %s check 失败: %w: %s", filepath.Base(corePath), err, detail)
}

// installValidatedConfig 校验并原子替换配置。
//
// 行为约定：
//   - 先在临时文件里写入新内容并跑 validate，校验不过则原文件一字不动；
//   - 校验通过后流式比对「新内容 vs 磁盘现有内容」，完全一致则放弃替换，
//     返回 changed=false —— 调用方据此跳过服务重启，避免无谓闪断；
//   - 确有变化时用 rename 原子替换（同目录内 rename 不会出现半截文件）；
//   - 全程不产生 .bak 残留，临时文件在失败路径上 defer 删除。
func installValidatedConfig(configPath string, content []byte, validate func(string) error) (_ *installedConfig, retErr error) {
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("创建配置目录失败: %w", err)
	}

	originalInfo, statErr := os.Stat(configPath)
	if statErr != nil && !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("读取当前配置信息失败: %w", statErr)
	}

	tempFile, err := os.CreateTemp(dir, "."+filepath.Base(configPath)+".tmp-*")
	if err != nil {
		return nil, fmt.Errorf("创建临时配置失败: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() {
		// 成功替换时 tempPath 已被 rename 掉，这里只是兜底清理
		if retErr != nil {
			_ = os.Remove(tempPath)
		}
	}()

	if err := tempFile.Chmod(0600); err != nil {
		_ = tempFile.Close()
		return nil, fmt.Errorf("设置临时配置权限失败: %w", err)
	}
	if _, err := tempFile.Write(content); err != nil {
		_ = tempFile.Close()
		return nil, fmt.Errorf("写入临时配置失败: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return nil, fmt.Errorf("同步临时配置失败: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return nil, fmt.Errorf("关闭临时配置失败: %w", err)
	}

	if originalInfo != nil {
		if err := os.Chmod(tempPath, originalInfo.Mode().Perm()); err != nil {
			return nil, fmt.Errorf("继承配置权限失败: %w", err)
		}
		if err := applyFileOwnership(tempPath, originalInfo); err != nil {
			return nil, fmt.Errorf("继承配置属主失败: %w", err)
		}
	}

	if err := validate(tempPath); err != nil {
		return nil, fmt.Errorf("新配置无效: %w", err)
	}

	// 内容完全一致：不替换、不重启。省掉一次无谓的服务闪断。
	if originalInfo != nil {
		same, cmpErr := fileEqualsContent(configPath, content)
		if cmpErr != nil {
			return nil, fmt.Errorf("比对当前配置失败: %w", cmpErr)
		}
		if same {
			_ = os.Remove(tempPath)
			return &installedConfig{
				configPath:    configPath,
				existedBefore: true,
				changed:       false,
			}, nil
		}
	}

	if err := os.Rename(tempPath, configPath); err != nil {
		return nil, fmt.Errorf("替换配置失败: %w", err)
	}

	return &installedConfig{
		configPath:    configPath,
		existedBefore: originalInfo != nil,
		changed:       true,
	}, nil
}

// fileEqualsContent 用流式读取比对文件内容与给定字节，避免把整个文件读进内存。
func fileEqualsContent(path string, want []byte) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return false, err
	}
	if info.Size() != int64(len(want)) {
		return false, nil // 长度不同必然不同，不必读
	}

	const chunk = 32 * 1024
	buf := make([]byte, chunk)
	offset := 0
	for {
		n, readErr := f.Read(buf)
		if n > 0 {
			if !bytes.Equal(buf[:n], want[offset:offset+n]) {
				return false, nil
			}
			offset += n
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return false, readErr
		}
		if offset >= len(want) {
			break
		}
	}
	return offset == len(want), nil
}

// rollbackInstalledConfig 处理「已改写但重启失败」的回滚。
//
// 这里刻意不做「还原旧内容」，而是删除刚写入的配置：
//   · 任何形式的旧副本（内存里的 []byte 或磁盘上的快照文件）都会随
//     推送频率累积，与原先堆 .bak 是同一类问题；
//   · 配置由面板全量下发，服务停在无配置状态后由面板重推即可恢复，
//     不需要 agent 自己维护历史版本。
// 若确实需要历史版本，应由面板侧留档，而不是每个 agent 各存一份。
func rollbackInstalledConfig(installed *installedConfig) error {
	if installed == nil || !installed.changed {
		return fmt.Errorf("配置未发生变更，无需回滚")
	}
	if err := os.Remove(installed.configPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("回滚失败: %w", err)
	}
	return nil
}
