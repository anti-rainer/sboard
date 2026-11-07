package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// makeCertPEM 生成一张指定有效期的自签证书与私钥（PEM）。
func makeCertPEM(t *testing.T, notBefore, notAfter time.Time) (certPEM, keyPEM string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("生成私钥失败: %v", err)
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "*.dingtalk.com"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		DNSNames:     []string{"*.dingtalk.com"},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("签发失败: %v", err)
	}
	keyDER, _ := x509.MarshalECPrivateKey(key)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
}

// TestInstallCertificateWritesBoth 证书与私钥都要落盘，且权限合理。
func TestInstallCertificateWritesBoth(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	certPEM, keyPEM := makeCertPEM(t, now, now.AddDate(0, 0, 90))

	certPath, changed, err := installCertificate(dir, certPEM, keyPEM)
	if err != nil {
		t.Fatalf("安装证书失败: %v", err)
	}
	if !changed {
		t.Error("首次安装应报告 changed=true")
	}
	if certPath != filepath.Join(dir, certFileName) {
		t.Errorf("证书路径 = %s", certPath)
	}
	if got, _ := os.ReadFile(certPath); string(got) != certPEM {
		t.Error("证书内容不一致")
	}
	if got, _ := os.ReadFile(filepath.Join(dir, keyFileName)); string(got) != keyPEM {
		t.Error("私钥内容不一致")
	}
	// 私钥必须是 0600
	info, err := os.Stat(filepath.Join(dir, keyFileName))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("私钥权限 = %o，期望 600", perm)
	}
	// 不应留下临时文件
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Errorf("目录里应只有 2 个文件，实际 %d 个", len(entries))
	}
}

// TestInstallCertificateIdempotent 内容一致时报告 changed=false，不重写文件。
func TestInstallCertificateIdempotent(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	certPEM, keyPEM := makeCertPEM(t, now, now.AddDate(0, 0, 90))

	if _, _, err := installCertificate(dir, certPEM, keyPEM); err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(dir, certFileName)
	before, _ := os.Stat(certPath)

	_, changed, err := installCertificate(dir, certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("内容相同应报告 changed=false")
	}
	after, _ := os.Stat(certPath)
	if !before.ModTime().Equal(after.ModTime()) {
		t.Error("内容相同时不应重写文件")
	}
}

// TestInstallCertificateRejectsBroken 坏证书必须在写盘前被拒绝。
func TestInstallCertificateRejectsBroken(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := installCertificate(dir, "not a pem", "also not a pem"); err == nil {
		t.Error("坏证书应被拒绝")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("拒绝时不应写入任何文件，实际 %d 个", len(entries))
	}
}

// TestMaybeRefreshCertificateThreshold 阈值判断：
// 剩余超过阈值不动，等于或低于阈值才写入。
func TestMaybeRefreshCertificateThreshold(t *testing.T) {
	now := time.Now()

	// 目录里先放一张剩余 60 天的证书
	dir := t.TempDir()
	oldCert, oldKey := makeCertPEM(t, now.AddDate(0, 0, -30), now.AddDate(0, 0, 60))
	if _, _, err := installCertificate(dir, oldCert, oldKey); err != nil {
		t.Fatal(err)
	}

	// 新证书有效期 90 天
	newCert, newKey := makeCertPEM(t, now, now.AddDate(0, 0, 90))

	// 阈值 30 天：剩余 60 天 > 30 天 → 无动作
	updated, daysLeft, _, err := maybeRefreshCertificate(dir, newCert, newKey, 30, now)
	if err != nil {
		t.Fatal(err)
	}
	if updated {
		t.Error("剩余 60 天超过阈值 30 天，不应更新")
	}
	if daysLeft < 59 || daysLeft > 61 {
		t.Errorf("剩余天数 = %d，期望约 60", daysLeft)
	}

	// 剩余 60 天，阈值提到 90 天 → 需要更新
	updated, daysLeft, certPath, err := maybeRefreshCertificate(dir, newCert, newKey, 90, now)
	if err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Error("阈值 90 天时剩余 60 天应触发更新")
	}
	if daysLeft < 89 || daysLeft > 91 {
		t.Errorf("更新后剩余天数 = %d，期望约 90", daysLeft)
	}
	got, _ := os.ReadFile(certPath)
	if string(got) != newCert {
		t.Error("磁盘上的证书应已更新为新证书")
	}
}

// TestMaybeRefreshCertificateWhenMissing 证书缺失时必须写入。
func TestMaybeRefreshCertificateWhenMissing(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	certPEM, keyPEM := makeCertPEM(t, now, now.AddDate(0, 0, 90))

	updated, daysLeft, _, err := maybeRefreshCertificate(dir, certPEM, keyPEM, 30, now)
	if err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Error("证书缺失时应写入")
	}
	if daysLeft < 89 || daysLeft > 91 {
		t.Errorf("剩余天数 = %d，期望约 90", daysLeft)
	}
}

// TestMaybeRefreshCertificateDefaultThreshold 阈值未下发时使用默认 30 天。
func TestMaybeRefreshCertificateDefaultThreshold(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	// 剩余 40 天：默认阈值 30 天时不该动
	certPEM, keyPEM := makeCertPEM(t, now.AddDate(0, 0, -50), now.AddDate(0, 0, 40))
	if _, _, err := installCertificate(dir, certPEM, keyPEM); err != nil {
		t.Fatal(err)
	}
	newCert, newKey := makeCertPEM(t, now, now.AddDate(0, 0, 90))

	updated, _, _, err := maybeRefreshCertificate(dir, newCert, newKey, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if updated {
		t.Error("剩余 40 天超过默认阈值 30 天，不应更新")
	}
}
