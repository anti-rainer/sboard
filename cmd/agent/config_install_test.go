package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// 安装新配置：内容变化时应替换、保留权限、标记 changed
func TestInstallValidatedConfigReplacesAndPreservesMetadata(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(configPath, 0640); err != nil {
		t.Fatal(err)
	}
	originalInfo, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}

	var stagedPath string
	var originalDuringValidate string
	installed, err := installValidatedConfig(configPath, []byte("new"), func(path string) error {
		stagedPath = path
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if string(content) != "new" {
			t.Fatalf("staged content = %q, want new", content)
		}
		// 校验阶段原文件必须还是旧内容（证明「先校验后替换」）
		originalDuringValidate = string(mustReadFile(t, configPath))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if stagedPath == "" {
		t.Fatal("validator was not called")
	}
	if originalDuringValidate != "old" {
		t.Fatalf("校验阶段原文件已是 %q，应为 old（替换早于校验）", originalDuringValidate)
	}

	if !installed.Changed() {
		t.Error("内容变化时应标记 changed=true")
	}
	if string(mustReadFile(t, configPath)) != "new" {
		t.Fatalf("installed content = %q, want new", mustReadFile(t, configPath))
	}
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != originalInfo.Mode().Perm() {
		t.Fatalf("installed mode = %o, want %o", info.Mode().Perm(), originalInfo.Mode().Perm())
	}
}

// 内容完全一致：不替换文件、不标记 changed（调用方据此跳过重启）
func TestInstallValidatedConfigSkipsWhenIdentical(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	same := []byte(`{"log":{"level":"error"}}`)
	if err := os.WriteFile(configPath, same, 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}

	installed, err := installValidatedConfig(configPath, same, func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if installed.Changed() {
		t.Error("内容一致时不应标记 changed（会导致无谓重启）")
	}
	after, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	// 连 mtime 都不该变：文件根本没被触碰
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("内容一致时文件被重写了: mtime %v → %v", before.ModTime(), after.ModTime())
	}
	if got := string(mustReadFile(t, configPath)); got != string(same) {
		t.Fatalf("content = %q, want %q", got, same)
	}
}

// 长度相同但内容不同：必须识别为「变了」
func TestInstallValidatedConfigDetectsSameLengthDifferent(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte("aaaa"), 0600); err != nil {
		t.Fatal(err)
	}
	installed, err := installValidatedConfig(configPath, []byte("aaab"), func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if !installed.Changed() {
		t.Error("同长度不同内容应判定为 changed")
	}
	if got := string(mustReadFile(t, configPath)); got != "aaab" {
		t.Fatalf("content = %q, want aaab", got)
	}
}

// 校验失败：原文件一字不动
func TestInstallValidatedConfigRejectsBeforeReplacing(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := installValidatedConfig(configPath, []byte("bad"), func(string) error {
		return errors.New("invalid config")
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if got := string(mustReadFile(t, configPath)); got != "old" {
		t.Fatalf("config after rejected update = %q, want old", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("temporary files left after rejection: %d", len(entries))
	}
}

// 全程不得产生 .bak 文件（用户明确要求：绝对不保留）
func TestInstallValidatedConfigNeverCreatesBackup(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte("v1"), 0600); err != nil {
		t.Fatal(err)
	}

	// 连续多次覆盖，模拟反复点部署
	for i, v := range []string{"v2", "v3", "v4", "v5"} {
		installed, err := installValidatedConfig(configPath, []byte(v), func(string) error { return nil })
		if err != nil {
			t.Fatalf("第 %d 次: %v", i+1, err)
		}
		if !installed.Changed() {
			t.Fatalf("第 %d 次内容变化却未标记 changed", i+1)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "config.json" {
			t.Errorf("产生了多余文件: %s（要求绝对不留 .bak）", e.Name())
		}
	}
}

// 反复部署不应累积任何状态（内存/磁盘都不留副本）
func TestInstallValidatedConfigNoAccumulation(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	// 100 次部署：目录里永远只能有 config.json
	for i := 0; i < 100; i++ {
		content := []byte("config-" + string(rune('a'+i%26)))
		_, err := installValidatedConfig(configPath, content, func(string) error { return nil })
		if err != nil {
			t.Fatalf("第 %d 次: %v", i, err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("100 次部署后残留 %d 个文件: %v", len(entries), names)
	}
}

// 回滚：删除刚写入的配置，不留旧副本
func TestRollbackRemovesWrittenConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	installed, err := installValidatedConfig(configPath, []byte("new"), func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := rollbackInstalledConfig(installed); err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
		t.Error("回滚后配置应被删除，由面板重新推送")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		t.Errorf("回滚后残留文件: %s", e.Name())
	}
}

// 未发生变更时回滚应直接报错，避免误删
func TestRollbackRejectsUnchanged(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte("same"), 0600); err != nil {
		t.Fatal(err)
	}
	installed, err := installValidatedConfig(configPath, []byte("same"), func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := rollbackInstalledConfig(installed); err == nil {
		t.Error("未变更时回滚应报错")
	}
	if got := string(mustReadFile(t, configPath)); got != "same" {
		t.Fatalf("未变更的配置被删了: %q", got)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
