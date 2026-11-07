package scheduler

import (
	"log"
	"time"

	"github.com/sboard-go/sboard/internal/certmgr"
	"github.com/sboard-go/sboard/internal/database"
)

// ConfigSyncCallback 配置同步回调函数类型
type ConfigSyncCallback func()

// CertRefreshCallback 证书刷新广播回调函数类型。
//
// 面板每天检查证书有效期；需要续期时重新签发，然后把新证书推给所有 agent。
// 推送是单向的，agent 不回应。
type CertRefreshCallback func(certPEM, keyPEM string)

// Scheduler 定时任务调度器
type Scheduler struct {
	stopChan        chan struct{}
	onConfigChange  ConfigSyncCallback
	onCertRefresh   CertRefreshCallback
	lastCertDay     string // 上次执行证书检查的日期，保证每天只查一次
}

// New 创建新的调度器
func New() *Scheduler {
	return &Scheduler{
		stopChan: make(chan struct{}),
	}
}

// SetConfigSyncCallback 设置配置同步回调
func (s *Scheduler) SetConfigSyncCallback(callback ConfigSyncCallback) {
	s.onConfigChange = callback
}

// SetCertRefreshCallback 设置证书刷新广播回调
func (s *Scheduler) SetCertRefreshCallback(callback CertRefreshCallback) {
	s.onCertRefresh = callback
}

// Start 启动调度器
func (s *Scheduler) Start() {
	log.Println("定时任务调度器已启动")

	// 每10分钟检查一次到期用户，首次立即触发
	go s.runPeriodically(10*time.Minute, s.checkExpiredUsers, true)
	// 每10分钟检查一次是否需要重置月度流量
	go s.runPeriodically(10*time.Minute, s.checkMonthlyTrafficReset, false)
	// 每10分钟看一眼证书，每天最多真正执行一次
	go s.runPeriodically(10*time.Minute, s.checkCertificate, false)
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	close(s.stopChan)
	log.Println("定时任务调度器已停止")
}

// runPeriodically 定期执行任务，如果 runImmediately 为 true，首次立即触发
func (s *Scheduler) runPeriodically(interval time.Duration, task func(), runImmediately bool) {
	if runImmediately {
		task()
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			task()
		case <-s.stopChan:
			return
		}
	}
}

// checkExpiredUsers 检查并禁用到期用户
func (s *Scheduler) checkExpiredUsers() {
	today := time.Now().Format("2006-01-02")

	// 批量禁用所有已启用但已到期的用户
	result := database.GetDB().Model(&database.ProxyUser{}).
		Where("enabled = ? AND expiry_date < ?", 1, today).
		Update("enabled", 0)

	if result.Error != nil {
		log.Printf("检查到期用户失败: %v", result.Error)
		return
	}

	if result.RowsAffected > 0 {
		log.Printf("共禁用 %d 个到期用户，触发配置同步", result.RowsAffected)
		if s.onConfigChange != nil {
			go s.onConfigChange()
		}
	}
}

// checkCertificate 每天检查一次节点证书有效期。
//
// 剩余有效期 ≤ 阈值时重新签发，并把新证书推给所有在线 agent：
//   - 部署/同步时本来就会带证书，但那依赖有配置变更；证书过期与配置无关，
//     必须由定时任务主动推。
//   - agent 收到后仍会按同一阈值判断，> 阈值则无动作，所以重复推送无害。
//
// 推送是单向的，agent 不回应。
func (s *Scheduler) checkCertificate() {
	now := time.Now()
	today := now.Format("2006-01-02")
	if s.lastCertDay == today {
		return // 今天已经查过
	}

	cert, renewed, err := certmgr.Ensure(now)
	if err != nil {
		log.Printf("检查节点证书失败: %v", err)
		return
	}
	s.lastCertDay = today

	daysLeft := cert.DaysLeft(now)
	if renewed {
		log.Printf("节点证书已续期，剩余 %d 天，正在推送给所有 agent", daysLeft)
	} else {
		log.Printf("节点证书剩余 %d 天（阈值 %d 天），无需续期，仅推送确认",
			daysLeft, certmgr.RenewBeforeDays)
	}

	// 无论是否续期都推一次：新上线的 agent 或曾经离线的 agent 靠这次补齐，
	// agent 侧阈值判断保证无变化时不写文件。
	if s.onCertRefresh != nil {
		s.onCertRefresh(cert.CertPEM, cert.KeyPEM)
	}
}

// checkMonthlyTrafficReset 检查并重置月度流量（只在每月1日重置）
func (s *Scheduler) checkMonthlyTrafficReset() {
	now := time.Now()

	// 只在每月1日执行重置
	if now.Day() != 1 {
		return
	}

	// 获取本月1日0点作为阈值
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	// 只重置那些 traffic_reset 早于本月1日的服务器（即本月还未重置的）
	result := database.GetDB().Model(&database.Server{}).
		Where("traffic_reset IS NULL OR traffic_reset < ?", monthStart).
		Updates(map[string]interface{}{
			"monthly_in":    0,
			"monthly_out":   0,
			"traffic_reset": now,
		})

	if result.Error != nil {
		log.Printf("重置月度流量失败: %v", result.Error)
		return
	}

	if result.RowsAffected > 0 {
		log.Printf("已重置 %d 台服务器的月度流量", result.RowsAffected)
	}
}

// CheckExpiredUsersNow 立即检查到期用户（供 API 调用）
func CheckExpiredUsersNow() (int, error) {
	today := time.Now().Format("2006-01-02")

	// 查找并禁用所有已到期的用户
	result := database.GetDB().Model(&database.ProxyUser{}).
		Where("enabled = ? AND expiry_date < ?", 1, today).
		Update("enabled", 0)

	if result.Error != nil {
		return 0, result.Error
	}

	return int(result.RowsAffected), nil
}

// GetExpiredUsers 获取即将到期的用户列表
func GetExpiredUsers(days int) ([]database.ProxyUser, error) {
	futureDate := time.Now().AddDate(0, 0, days).Format("2006-01-02")
	today := time.Now().Format("2006-01-02")

	var users []database.ProxyUser
	err := database.GetDB().Where("enabled = ? AND expiry_date >= ? AND expiry_date <= ?",
		1, today, futureDate).Find(&users).Error

	return users, err
}
