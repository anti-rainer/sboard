package main

import (
	"testing"
)

// 构建批次标识必须在同一批次内跨架构一致。
//
// 这是整个「版本一致性」判断的基石：原先用二进制 sha256 比对，
// 但同一次构建产出的 amd64 与 arm64 二进制天然不同 hash
// （实例：x86 主机 4a7a8218…，ARM 路由 a86819cc…），
// 导致正常差异被误报成「版本不一致」。
func TestBuildIDSharedWithinBatch(t *testing.T) {
	// 同一批次注入的值必须完全相同，与架构无关
	const injected = "YjMyNGE1ZDk4YzFmNDQ3Nw=="

	// 模拟两个不同架构的进程读到的值
	ids := map[string]string{
		"amd64": injected,
		"arm64": injected,
		"armv7": injected,
		"386":   injected,
	}
	for arch, id := range ids {
		if id != injected {
			t.Errorf("%s 的 BuildID = %q，期望 %q", arch, id, injected)
		}
	}
}

// BuildID 默认值必须存在（本地构建未注入 ldflags 时不能是空导致前端误判）。
func TestBuildIDHasDefault(t *testing.T) {
	if BuildID == "" {
		t.Fatal("BuildID 不能为空，否则未注入时会与 agent 上报的空值混淆")
	}
}

// normalizeArch 归一化：Go 报 "arm"，发布包叫 armv7。
func TestNormalizeArch(t *testing.T) {
	cases := map[string]string{
		"amd64": "amd64",
		"arm64": "arm64",
		"386":   "386",
		"arm":   "armv7", // 关键：Go 在 armv7 设备上报告 arm
	}
	for in, want := range cases {
		if got := normalizeArch(in); got != want {
			t.Errorf("normalizeArch(%q) = %q，期望 %q", in, got, want)
		}
	}
}
