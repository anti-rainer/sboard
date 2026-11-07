package main

// 自我更新结果记录。
//
// 背景：self_update 是单向通知（面板发完即忘），而 handleSelfUpdate 里
// 每条失败路径原本都只是 log + return。结果是「面板以为发过了、agent
// 以为做完了」，实际没更新成功也没人知道 —— openwrt 卡了一整轮就是
// 这种情况：二进制已下载替换，但进程没重启，日志里只有一行「开始自我更新」。
//
// 这里把最近一次更新结果随心跳上报，面板就能看到「尝试过、卡在哪一步」，
// 而不是只能靠人去机器上翻 syslog。

import (
	"sync"
	"time"
)

var (
	updateStatusMu   sync.Mutex
	updateStatusText string    // 最近一次更新的结果描述
	updateStatusAt   time.Time // 结果产生时间
)

// setUpdUpdateStatus 记录一次自我更新的结果。
//
// step 是阶段名（下载/校验/解压/替换/重启），err 为 nil 表示该阶段成功。
func setUpdUpdateStatus(step string, err error) {
	updateStatusMu.Lock()
	defer updateStatusMu.Unlock()
	if err == nil {
		updateStatusText = step + ":成功"
	} else {
		updateStatusText = step + ":失败 - " + err.Error()
	}
	updateStatusAt = time.Now()
}

// updateStatusInfo 返回待上报的更新状态（供心跳打包）。
//
// 只上报最近 30 分钟内的结果：更早的已经没参考价值，
// 持续上报只会让界面一直挂着一条旧失败记录。
func updateStatusInfo() (text string, at string) {
	updateStatusMu.Lock()
	defer updateStatusMu.Unlock()
	if updateStatusText == "" || time.Since(updateStatusAt) > 30*time.Minute {
		return "", ""
	}
	return updateStatusText, updateStatusAt.Format(time.RFC3339)
}
