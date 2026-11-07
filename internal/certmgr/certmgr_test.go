package certmgr

import (
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"
)

// TestGenerateValidity 证书有效期必须是 90 天。
//
// 这不是随手定的数字：naive 出站走 Chromium 的 cronet 证书栈，有效期过长会被
// 直接拒绝（ERROR: cert validity too long）。实测 90 / 180 天可用，
// 270 / 365 / 398 天全部失败。改这个值会导致 naive 节点连不通。
func TestGenerateValidity(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	cert, err := Generate(now)
	if err != nil {
		t.Fatalf("签发证书失败: %v", err)
	}

	block, _ := pem.Decode([]byte(cert.CertPEM))
	if block == nil {
		t.Fatal("证书 PEM 解码失败")
	}
	parsed, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("解析证书失败: %v", err)
	}

	days := parsed.NotAfter.Sub(parsed.NotBefore).Hours() / 24
	if days < 89 || days > 91 {
		t.Errorf("有效期 = %.1f 天，期望约 90 天（cronet 限制）", days)
	}
	if parsed.NotAfter.Before(now) {
		t.Error("证书已过期")
	}
}

// TestGenerateHasSAN naive 的 cronet 要求证书带 SAN，只写 CN 不够。
func TestGenerateHasSAN(t *testing.T) {
	cert, err := Generate(time.Now())
	if err != nil {
		t.Fatalf("签发证书失败: %v", err)
	}
	block, _ := pem.Decode([]byte(cert.CertPEM))
	parsed, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("解析证书失败: %v", err)
	}
	if len(parsed.DNSNames) == 0 {
		t.Fatal("证书缺少 SAN，naive 客户端会拒绝")
	}
	if parsed.DNSNames[0] != commonName {
		t.Errorf("SAN = %v，期望 %s", parsed.DNSNames, commonName)
	}
	if parsed.Subject.CommonName != commonName {
		t.Errorf("CN = %s，期望 %s", parsed.Subject.CommonName, commonName)
	}
}

// TestNeedsRenewal 续期阈值：剩余 ≤ 30 天时重新签发。
func TestNeedsRenewal(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	cert, err := Generate(now)
	if err != nil {
		t.Fatalf("签发证书失败: %v", err)
	}

	cases := []struct {
		name     string
		now      time.Time
		want     bool
	}{
		{"刚签发", now, false},
		{"剩 59 天", now.AddDate(0, 0, 30), false},
		{"剩 31 天", now.AddDate(0, 0, 58), false},
		{"剩 30 天（达阈值）", now.AddDate(0, 0, 59), true},
		{"剩 10 天", now.AddDate(0, 0, 79), true},
		{"已过期", now.AddDate(0, 0, 100), true},
	}
	for _, c := range cases {
		if got := cert.NeedsRenewal(c.now); got != c.want {
			t.Errorf("%s: NeedsRenewal = %v，期望 %v（剩余 %d 天）",
				c.name, got, c.want, cert.DaysLeft(c.now))
		}
	}
}

// TestGenerateUnique 每次签发必须是新的证书与密钥对。
func TestGenerateUnique(t *testing.T) {
	a, err := Generate(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	b, err := Generate(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if a.CertPEM == b.CertPEM {
		t.Error("两次签发的证书相同")
	}
	if a.KeyPEM == b.KeyPEM {
		t.Error("两次签发的私钥相同")
	}

	// 私钥必须与证书匹配
	block, _ := pem.Decode([]byte(a.CertPEM))
	parsed, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	keyBlock, _ := pem.Decode([]byte(a.KeyPEM))
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		t.Fatalf("私钥解析失败: %v", err)
	}
	if !key.PublicKey.Equal(parsed.PublicKey) {
		t.Error("私钥与证书不匹配")
	}
}
