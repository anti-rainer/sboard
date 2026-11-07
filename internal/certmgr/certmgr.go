// Package certmgr 管理节点 TLS 伪装证书的生命周期。
//
// 设计要点：
//
//  1. 面板是证书的唯一来源。证书在面板端生成并保存在 system_configs 表里，
//     部署时随配置一起下发到 agent，agent 自身不再内嵌任何证书文件。
//
//  2. 有效期 90 天。naive 出站走 Chromium 的 cronet 证书栈，有效期过长会被拒：
//     ERROR: cert validity too long
//     实测 90 / 180 天可用，270 / 365 / 398 天全部失败。
//
//  3. 续期阈值 30 天。剩余有效期 ≤ 30 天时重新签发，留出足够余量：某台 agent
//     若在阈值附近离线，下次上线（下次部署或下次刷新指令）仍能拿到有效期充足
//     的新证书，不会因为错过一次推送就过期。
//
//  4. 证书只用于节点入站 TLS。面板自身的 web 服务是纯 HTTP，不使用这张证书。
package certmgr

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"time"

	"gorm.io/gorm"

	"github.com/sboard-go/sboard/internal/database"
)

const (
	// ValidityDays 证书有效期（天）。上限受 naive 出站的 cronet 校验约束。
	ValidityDays = 90
	// RenewBeforeDays 剩余有效期不足该天数时重新签发。
	RenewBeforeDays = 30
	// commonName 伪装域名，与内置证书保持一致。
	commonName = "*.dingtalk.com"
	// systemConfigKey 证书在 system_configs 表中的键。
	systemConfigKey = "node_certificate"
)

// Certificate 一张节点证书及其私钥（均为 PEM 文本）。
type Certificate struct {
	CertPEM   string    `json:"cert_pem"`
	KeyPEM    string    `json:"key_pem"`
	NotBefore time.Time `json:"not_before"`
	NotAfter  time.Time `json:"not_after"`
}

// DaysLeft 返回证书剩余有效天数（向下取整，可能为负）。
func (c *Certificate) DaysLeft(now time.Time) int {
	return int(c.NotAfter.Sub(now).Hours() / 24)
}

// NeedsRenewal 报告证书是否需要在 now 时刻重新签发。
func (c *Certificate) NeedsRenewal(now time.Time) bool {
	return c.DaysLeft(now) <= RenewBeforeDays
}

// Generate 签发一张新的自签证书。
func Generate(now time.Time) (*Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("生成私钥失败: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("生成序列号失败: %w", err)
	}

	notBefore := now.Add(-time.Hour) // 留 1 小时时钟误差
	notAfter := notBefore.AddDate(0, 0, ValidityDays)
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		// naive 的 cronet 要求证书带 SAN，只写 CN 不够。
		DNSNames: []string{commonName},
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("签发证书失败: %w", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("编码私钥失败: %w", err)
	}

	return &Certificate{
		CertPEM:   string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		KeyPEM:    string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})),
		NotBefore: notBefore,
		NotAfter:  notAfter,
	}, nil
}

// Load 从数据库读取当前证书，尚未签发过时返回 nil。
func Load() (*Certificate, error) {
	var row database.SystemConfig
	err := database.GetDB().Where("key = ?", systemConfigKey).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取证书失败: %w", err)
	}
	var cert Certificate
	if err := json.Unmarshal([]byte(row.Value), &cert); err != nil {
		return nil, fmt.Errorf("解析证书失败: %w", err)
	}
	if cert.CertPEM == "" || cert.KeyPEM == "" {
		return nil, fmt.Errorf("证书内容不完整")
	}
	return &cert, nil
}

// Save 写入（覆盖）证书。
func Save(cert *Certificate) error {
	raw, err := json.Marshal(cert)
	if err != nil {
		return fmt.Errorf("序列化证书失败: %w", err)
	}
	value := string(raw)

	var row database.SystemConfig
	err = database.GetDB().Where("key = ?", systemConfigKey).First(&row).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("查询证书失败: %w", err)
		}
		row = database.SystemConfig{
			Key:         systemConfigKey,
			Value:       value,
			Description: "节点 TLS 伪装证书（面板签发，部署时下发给 agent）",
		}
		return database.GetDB().Create(&row).Error
	}
	return database.GetDB().Model(&row).Update("value", value).Error
}

// Ensure 返回可用证书：不存在或剩余有效期不足时重新签发并保存。
//
// renewed 表示本次是否发生了重新签发。
func Ensure(now time.Time) (cert *Certificate, renewed bool, err error) {
	cert, err = Load()
	if err != nil {
		return nil, false, err
	}
	if cert != nil && !cert.NeedsRenewal(now) {
		return cert, false, nil
	}
	fresh, err := Generate(now)
	if err != nil {
		return nil, false, err
	}
	if err := Save(fresh); err != nil {
		return nil, false, err
	}
	return fresh, true, nil
}
