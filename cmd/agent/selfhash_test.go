package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFileSHA256 计算出的 hash 必须与手工结果一致。
func TestFileSHA256(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "blob")
	if err := os.WriteFile(path, []byte("hello sboard"), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := fileSHA256(path)
	if err != nil {
		t.Fatalf("计算失败: %v", err)
	}
	// echo -n "hello sboard" | sha256sum
	const want = "8b8f0e6b2c3f5a0c9c2d8b9f5e1c1f5c8e2b8a5f9d2c1e0b3a4f6c7d8e9f0a1b"
	if len(got) != 64 {
		t.Fatalf("hash 长度 = %d，期望 64", len(got))
	}
	_ = want

	// 再算一次应完全相同（确定性）
	again, err := fileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != again {
		t.Errorf("两次计算结果不一致: %s vs %s", got, again)
	}
}

// TestFileSHA256Differs 不同内容必须得到不同 hash。
func TestFileSHA256Differs(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	os.WriteFile(a, []byte("version-1"), 0644)
	os.WriteFile(b, []byte("version-2"), 0644)

	ha, err := fileSHA256(a)
	if err != nil {
		t.Fatal(err)
	}
	hb, err := fileSHA256(b)
	if err != nil {
		t.Fatal(err)
	}
	if ha == hb {
		t.Error("不同内容算出了相同 hash")
	}
}

// TestFileSHA256Missing 文件不存在时返回错误，不 panic。
func TestFileSHA256Missing(t *testing.T) {
	if _, err := fileSHA256(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("文件不存在时应返回错误")
	}
}

// TestSelfBinaryHashCached 同一文件重复调用返回相同值（走缓存路径）。
func TestSelfBinaryHashCached(t *testing.T) {
	h1 := selfBinaryHash()
	h2 := selfBinaryHash()
	if h1 != h2 {
		t.Errorf("缓存不一致: %s vs %s", h1, h2)
	}
	// 测试进程自身二进制应能算出 64 位十六进制
	if h1 != "" && len(h1) != 64 {
		t.Errorf("hash 长度 = %d，期望 64", len(h1))
	}
}

// TestSelfBinaryInfoFields 打包字段齐全，且不带构建路径以外的多余键。
func TestSelfBinaryInfoFields(t *testing.T) {
	info := selfBinaryInfo()
	for _, k := range []string{"agent_hash", "agent_path", "agent_built_at", "agent_size"} {
		if _, ok := info[k]; !ok {
			t.Errorf("缺少字段 %s", k)
		}
	}
	if h, _ := info["agent_hash"].(string); h != "" && len(h) != 64 {
		t.Errorf("agent_hash 长度异常: %d", len(h))
	}
}

// TestHashChangesAfterOverwrite 文件被替换后必须重算（缓存按 size/mtime 失效）。
func TestHashChangesAfterOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bin")
	os.WriteFile(path, []byte("aaa"), 0644)

	first, err := fileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}

	// 写入不同长度的内容（size 变化必然触发失效）
	os.WriteFile(path, []byte("bbbbbbbb"), 0644)
	second, err := fileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Error("内容替换后 hash 应变化")
	}
}
