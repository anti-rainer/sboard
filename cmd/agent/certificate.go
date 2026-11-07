package main

// 节点 TLS 证书的安装与有效期检查。
//
// 面板是证书的唯一来源：agent 自身不内嵌证书，证书随部署指令下发，
// 或由独立的 cert_update 指令刷新。
//
// sing-box 的 TLS certificate_path 是热加载的：
//   https://sing-box.sagernet.org/configuration/shared/tls/
//   certificate_path: Will be automatically reloaded if file modified.
// 实测替换文件后 1 秒内新连接即用上新证书、进程 PID 不变，
// 因此刷新证书不需要重启 sing-box 服务。

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

const (
	// certFileName / keyFileName 与配置里 tls.certificate_path / key_path 一致。
	certFileName = "server.crt"
	keyFileName  = "server.key"
	// defaultRenewBeforeDays 未收到面板阈值时使用的续期阈值。
	defaultRenewBeforeDays = 30
)

// certificateExpiry 解析 PEM 证书的失效时间。
func certificateExpiry(certPEM []byte) (time.Time, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return time.Time{}, fmt.Errorf("证书 PEM 解析失败")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, fmt.Errorf("解析证书失败: %w", err)
	}
	return cert.NotAfter, nil
}

// certDaysLeft 返回本地证书剩余有效天数。
//
// 证书不存在或无法解析时返回 ok=false，调用方据此视为「需要写入」。
func certDaysLeft(certPath string, now time.Time) (days int, ok bool) {
	raw, err := os.ReadFile(certPath)
	if err != nil {
		return 0, false
	}
	notAfter, err := certificateExpiry(raw)
	if err != nil {
		return 0, false
	}
	return int(notAfter.Sub(now).Hours() / 24), true
}

// installCertificate 把证书与私钥写入配置目录。
//
// 两个文件都用「同目录临时文件 + rename」原子替换，不产生 .bak。
// 先写私钥再写证书：若中途失败，磁盘上不会出现「新证书 + 旧私钥」的错配，
// 而旧证书 + 新私钥这种错配也只有极短窗口，且下一次刷新会覆盖。
//
// 返回 changed 表示证书内容确实变化了。
func installCertificate(dir, certPEM, keyPEM string) (certPath string, changed bool, err error) {
	if certPEM == "" || keyPEM == "" {
		return "", false, fmt.Errorf("证书或私钥内容为空")
	}
	// 写入前先确认能解析，避免把坏证书推上生产
	if _, err := certificateExpiry([]byte(certPEM)); err != nil {
		return "", false, err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", false, fmt.Errorf("创建证书目录失败: %w", err)
	}

	certPath = filepath.Join(dir, certFileName)
	keyPath := filepath.Join(dir, keyFileName)

	certChanged, err := certFileDiffers(certPath, []byte(certPEM))
	if err != nil {
		return "", false, err
	}
	keyChanged, err := certFileDiffers(keyPath, []byte(keyPEM))
	if err != nil {
		return "", false, err
	}
	if !certChanged && !keyChanged {
		return certPath, false, nil
	}

	if keyChanged {
		if err := atomicWriteFile(keyPath, []byte(keyPEM), 0600); err != nil {
			return "", false, fmt.Errorf("写入私钥失败: %w", err)
		}
	}
	if certChanged {
		if err := atomicWriteFile(certPath, []byte(certPEM), 0644); err != nil {
			return "", false, fmt.Errorf("写入证书失败: %w", err)
		}
	}
	return certPath, true, nil
}

// certFileDiffers 报告磁盘内容与期望内容是否不同（文件不存在视为不同）。
func certFileDiffers(path string, want []byte) (bool, error) {
	got, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, fmt.Errorf("读取 %s 失败: %w", path, err)
	}
	if len(got) != len(want) {
		return true, nil
	}
	for i := range got {
		if got[i] != want[i] {
			return true, nil
		}
	}
	return false, nil
}

// atomicWriteFile 用「同目录临时文件 + rename」原子写入，不产生 .bak 备份。
//
// 与 service_linux.go 里的 writeFileAtomic 行为一致，但跨平台可用
// （那个函数带 linux build tag）。
func atomicWriteFile(path string, content []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

// maybeRefreshCertificate 按阈值决定是否写入下发的证书。
//
// 剩余有效期 > renewBeforeDays 时不动任何文件（面板每天都会推一次，
// 大多数时候这里都是无动作）。
//
// 返回 updated 表示本次是否写入，daysLeft 是写入前/后的剩余天数。
func maybeRefreshCertificate(dir, certPEM, keyPEM string, renewBeforeDays int, now time.Time) (updated bool, daysLeft int, certPath string, err error) {
	if renewBeforeDays <= 0 {
		renewBeforeDays = defaultRenewBeforeDays
	}
	certPath = filepath.Join(dir, certFileName)

	days, ok := certDaysLeft(certPath, now)
	if ok && days > renewBeforeDays {
		return false, days, certPath, nil
	}

	certPath, changed, err := installCertificate(dir, certPEM, keyPEM)
	if err != nil {
		return false, days, certPath, err
	}
	if !changed {
		return false, days, certPath, nil
	}
	newDays, _ := certDaysLeft(certPath, now)
	log.Printf("证书已更新: %s（剩余 %d 天，阈值 %d 天）", certPath, newDays, renewBeforeDays)
	return true, newDays, certPath, nil
}
