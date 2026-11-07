package handler

// 面板侧的证书下发：所有把配置/核心推到 agent 的地方都要带上证书。
//
// 证书由面板签发并保存在 system_configs 表里（见 internal/certmgr），
// agent 不再内嵌证书文件。

import (
	"log"
	"time"

	"github.com/sboard-go/sboard/internal/certmgr"
)

// nodeCertificate 返回当前应下发的节点证书。
//
// 证书缺失或接近过期时这里会顺带签发新证书。部署本身就应该拿到一张可用证书，
// 所以走 Ensure 而不是只读：首次部署时表里还没有证书，直接读会拿到空值，
// 导致 agent 上缺证书、sing-box 起不来。
//
// 失败时返回空字符串，调用方据此跳过证书下发（不阻塞部署本身）。
func nodeCertificate() (certPEM, keyPEM string) {
	cert, renewed, err := certmgr.Ensure(time.Now())
	if err != nil {
		log.Printf("获取节点证书失败，本次不下发证书: %v", err)
		return "", ""
	}
	if renewed {
		log.Printf("已签发新的节点证书，有效期至 %s（剩余 %d 天）",
			cert.NotAfter.Format("2006-01-02"), cert.DaysLeft(time.Now()))
	}
	return cert.CertPEM, cert.KeyPEM
}
