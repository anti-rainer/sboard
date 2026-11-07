package handler

import (
	"encoding/base64"
	"testing"
)

// SS2022 主密钥长度必须与 method 匹配，否则客户端直接崩：
//   cipher: 2022-blake3-aes-128-gcm initialize error:
//   bad key length, required 16, got 32
func TestSS2022KeyMatchesMethod(t *testing.T) {
	key := func(n int) string {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(i + 1)
		}
		return base64.StdEncoding.EncodeToString(b)
	}

	cases := []struct {
		name     string
		password string
		method   string
		want     bool
	}{
		{"aes-128 用 16 字节", key(16), "2022-blake3-aes-128-gcm", true},
		{"aes-128 用 32 字节", key(32), "2022-blake3-aes-128-gcm", false},
		{"aes-256 用 32 字节", key(32), "2022-blake3-aes-256-gcm", true},
		{"aes-256 用 16 字节", key(16), "2022-blake3-aes-256-gcm", false},
		{"chacha20 用 32 字节", key(32), "2022-blake3-chacha20-poly1305", true},
		{"chacha20 用 16 字节", key(16), "2022-blake3-chacha20-poly1305", false},
		// 传统加密方式不校验长度
		{"aes-256-gcm 不校验长度", key(32), "aes-256-gcm", true},
		{"aes-128-gcm 不校验长度", "plain-password", "aes-128-gcm", true},
		// 非法 base64 在 SS2022 下视为不匹配
		{"aes-128 非法 base64", "not-base64!!!", "2022-blake3-aes-128-gcm", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ss2022KeyMatchesMethod(c.password, c.method); got != c.want {
				t.Errorf("ss2022KeyMatchesMethod(%q, %q) = %v, want %v",
					c.password, c.method, got, c.want)
			}
		})
	}
}

// 改 method 后密钥长度不匹配时必须重新生成，且新密钥要符合新 method
func TestRegenerateSS2022PasswordIfMismatch(t *testing.T) {
	key32 := base64.StdEncoding.EncodeToString(make([]byte, 32))

	// 32 字节密钥 + aes-128 → 必须换成 16 字节
	pw := key32
	if !regenerateSS2022PasswordIfMismatch(&pw, "2022-blake3-aes-128-gcm") {
		t.Fatal("32 字节密钥配 aes-128 应当触发重新生成")
	}
	if pw == key32 {
		t.Error("密码没有被换掉")
	}
	if !ss2022KeyMatchesMethod(pw, "2022-blake3-aes-128-gcm") {
		t.Errorf("重新生成的密钥仍不匹配 aes-128: %q", pw)
	}

	// 16 字节密钥 + aes-128 → 不动
	key16 := base64.StdEncoding.EncodeToString(make([]byte, 16))
	pw = key16
	if regenerateSS2022PasswordIfMismatch(&pw, "2022-blake3-aes-128-gcm") {
		t.Error("长度匹配时不应重新生成")
	}
	if pw != key16 {
		t.Error("长度匹配时密码被改动了")
	}

	// 传统加密方式 → 永不重新生成
	pw = "whatever"
	if regenerateSS2022PasswordIfMismatch(&pw, "aes-256-gcm") {
		t.Error("传统加密方式不应重新生成密钥")
	}
	if pw != "whatever" {
		t.Error("传统加密方式的密码被改动了")
	}
}

// 生成的密钥必须匹配对应的 method
func TestGenerateSS2022PasswordLengths(t *testing.T) {
	for method, wantLen := range map[string]int{
		"2022-blake3-aes-128-gcm":        16,
		"2022-blake3-aes-256-gcm":        32,
		"2022-blake3-chacha20-poly1305":  32,
	} {
		pw := generateSS2022Password(method)
		raw, err := base64.StdEncoding.DecodeString(pw)
		if err != nil {
			t.Errorf("%s 生成的密码不是合法 base64: %q", method, pw)
			continue
		}
		if len(raw) != wantLen {
			t.Errorf("%s 生成的密钥 %d 字节，期望 %d", method, len(raw), wantLen)
		}
	}
}

// 入站节点管理只接受 SS2022 三种 method。
// 传统加密是「单密码全员共享」，无法按用户下发独立密钥，已移除。
func TestSupportedSSMethods(t *testing.T) {
	allowed := []string{
		"2022-blake3-aes-128-gcm",
		"2022-blake3-aes-256-gcm",
		"2022-blake3-chacha20-poly1305",
	}
	for _, m := range allowed {
		if !isSupportedSSMethod(m) {
			t.Errorf("%s 应当被允许", m)
		}
	}

	rejected := []string{
		"aes-128-gcm",
		"aes-256-gcm",
		"chacha20-ietf-poly1305",
		"xchacha20-ietf-poly1305",
		"",
		"none",
		"2022-blake3-aes-192-gcm", // 不存在的变体
	}
	for _, m := range rejected {
		if isSupportedSSMethod(m) {
			t.Errorf("%s 应当被拒绝", m)
		}
	}

	if len(supportedSSMethods) != len(allowed) {
		t.Errorf("白名单条目数 %d，期望 %d", len(supportedSSMethods), len(allowed))
	}
}
