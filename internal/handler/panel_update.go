package handler

// 面板（sboard）自身的更新。
//
// 与 agent 自更新的关键差异：面板是被请求的那一方。更新一旦开始就要替换
// 自身的可执行文件并重启服务 —— 进程被杀，HTTP 响应根本发不出去。因此这里
// 采用「先返回、后执行」：
//   1. HTTP handler 只做参数校验并启动后台 goroutine，立刻返回 202；
//   2. goroutine 延迟几秒（让响应先写回浏览器）再下载、校验、原子替换；
//   3. 最后重启 systemd 服务完成切换。
//
// 安全约束：
//   - 发布方 checksums.txt 直连 GitHub 获取，不经过加速代理，
//     避免「代理同时控制制品与校验值」导致校验形同虚设；
//   - 替换前校验 sha256，失败则不动现有二进制；
//   - 用同目录临时文件 + rename 原子替换，不留 .bak；
//     正在运行的旧进程持有旧 inode，直到服务重启才切换，所以不会写坏自己。

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/sboard-go/sboard/internal/agent"
)

// panelUpdateState 面板更新状态（供前端轮询/展示）。
type panelUpdateState struct {
	mu        sync.Mutex
	running   bool
	startedAt time.Time
	message   string
	success   bool
	finished  bool
}

var panelUpdate = &panelUpdateState{}

// finish 记录结束状态。success 必须显式给出，否则前端会把成功当失败。
func (s *panelUpdateState) finish(success bool, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
	s.finished = true
	s.success = success
	s.message = msg
}

// snapshot 返回当前状态。
func (s *panelUpdateState) snapshot() gin.H {
	s.mu.Lock()
	defer s.mu.Unlock()
	return gin.H{
		"running":   s.running,
		"message":   s.message,
		"success":   s.success,
		"finished":  s.finished,
		"started_at": s.startedAt.Format(time.RFC3339),
	}
}

// handlePanelSelfUpdate 触发面板自我更新。
//
// 立即返回，实际更新在后台进行 —— 因为更新会重启本进程。
func (s *Server) handlePanelSelfUpdate(c *gin.Context) {
	panelUpdate.mu.Lock()
	if panelUpdate.running {
		panelUpdate.mu.Unlock()
		errorJSON(c, http.StatusConflict, "面板更新已在进行中")
		return
	}
	panelUpdate.running = true
	panelUpdate.startedAt = time.Now()
	panelUpdate.finished = false
	panelUpdate.success = false
	panelUpdate.message = "更新任务已启动"
	panelUpdate.mu.Unlock()

	params := buildSelfUpdateData()

	go func() {
		// 先让 HTTP 响应回到浏览器，再动自己的二进制
		time.Sleep(3 * time.Second)
		if err := runPanelSelfUpdate(params); err != nil {
			panelUpdate.finish(false, "更新失败: "+err.Error())
			return
		}
		// 走到这里二进制已替换成功。重启会在几秒后发生，
		// 状态先记成成功，让前端在服务恢复后能读到正确结论。
		panelUpdate.finish(true, "更新成功，服务正在重启")
	}()

	successJSON(c, gin.H{
		"message": "面板更新已启动，服务将在几秒后重启，请稍后刷新页面",
	})
}

// handlePanelUpdateStatus 查询面板更新状态。
func (s *Server) handlePanelUpdateStatus(c *gin.Context) {
	noCacheHeaders(c)
	successJSON(c, panelUpdate.snapshot())
}

// runPanelSelfUpdate 执行下载 → 校验 → 解压 → 原子替换 → 重启。
func runPanelSelfUpdate(p *agent.SelfUpdateData) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("Windows 下不支持面板自更新，请手动替换程序")
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取可执行文件路径失败: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(execPath); err == nil {
		execPath = resolved
	}

	arch := runtime.GOARCH
	// Go 在 armv7 设备上报告 "arm"，发布包命名用 armv7
	if arch == "arm" {
		arch = "armv7"
	}
	zipName := fmt.Sprintf("sboard_%s_%s.zip", runtime.GOOS, arch)
	binaryName := "sboard"

	releaseBase := fmt.Sprintf("https://github.com/%s/releases/%s", p.Repo, p.ReleaseType)

	// 校验文件直连 GitHub，不经加速代理
	checksums, err := fetchChecksums(releaseBase + "/checksums.txt")
	if err != nil {
		return fmt.Errorf("下载校验文件失败: %w", err)
	}

	// 制品可按加速前缀回退
	candidates := proxyCandidates(p, releaseBase+"/"+zipName)

	dir := filepath.Dir(execPath)
	zipPath, err := os.CreateTemp(dir, ".sboard-update-*.zip")
	if err != nil {
		// 安装目录不可写时退到系统临时目录
		zipPath, err = os.CreateTemp("", "sboard-update-*.zip")
		if err != nil {
			return fmt.Errorf("创建临时文件失败: %w", err)
		}
	}
	zipFile := zipPath.Name()
	zipPath.Close()
	defer os.Remove(zipFile)

	var lastErr error
	downloaded := false
	for _, u := range candidates {
		if err := downloadToFile(u, zipFile); err != nil {
			lastErr = err
			continue
		}
		if fi, err := os.Stat(zipFile); err != nil || fi.Size() < 512*1024 {
			lastErr = fmt.Errorf("下载文件过小或不可读")
			continue
		}
		downloaded = true
		break
	}
	if !downloaded {
		return fmt.Errorf("所有下载地址均失败: %v", lastErr)
	}

	want, ok := checksums[zipName]
	if !ok {
		return fmt.Errorf("校验文件中没有 %s", zipName)
	}
	got, err := fileSHA256Hex(zipFile)
	if err != nil {
		return fmt.Errorf("计算校验值失败: %w", err)
	}
	if !strings.EqualFold(want, got) {
		return fmt.Errorf("校验失败：期望 %s 实际 %s", want[:16], got[:16])
	}

	// 解压到同目录临时文件，再原子替换
	tmpBin, err := os.CreateTemp(dir, ".sboard-new-*")
	if err != nil {
		return fmt.Errorf("创建临时二进制失败: %w", err)
	}
	tmpPath := tmpBin.Name()
	tmpBin.Close()
	defer os.Remove(tmpPath)

	if err := extractBinaryFromZip(zipFile, binaryName, tmpPath); err != nil {
		return fmt.Errorf("解压失败: %w", err)
	}
	if err := os.Chmod(tmpPath, 0755); err != nil {
		return fmt.Errorf("设置权限失败: %w", err)
	}

	if err := os.Rename(tmpPath, execPath); err != nil {
		return fmt.Errorf("替换二进制失败: %w", err)
	}

	// 替换完成后重启服务。用 systemd 起一个新的分离进程来重启，
	// 避免「重启自己」时把执行重启的进程也杀掉。
	restartDetached()
	return nil
}

// restartDetached 在一个独立会话里重启 sboard 服务。
//
// 为什么不能直接 `systemctl restart sboard`：那会杀掉当前进程，
// 而当前进程正是执行命令的父进程，命令可能被中断在中间状态。
// 这里用 setsid + 后台延迟，让重启动作脱离当前进程组。
//
// 两种部署形态都要覆盖：
//   - 生产：由 systemd 管理，让 systemd 重启（保持 unit 的 Restart 策略与日志）
//   - 手动/容器：没有对应 unit，systemd 重启会失败。此时兜底由自己
//     exec 新二进制重新拉起，否则更新完二进制后服务就没了。
func restartDetached() {
	execPath, _ := os.Executable()
	if resolved, err := filepath.EvalSymlinks(execPath); err == nil {
		execPath = resolved
	}
	args := os.Args[1:]

	// 用一段 shell 完成「判断有无 unit → 有则 systemctl，无则自拉起」
	script := buildRestartScript(execPath, args)
	cmd := exec.Command("sh", "-c", script)
	cmd.SysProcAttr = detachProcAttr()
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		// 连 shell 都起不来：直接退出，交给上层监督（systemd/docker）拉起
		os.Exit(0)
	}
}

// buildRestartScript 生成重启脚本。
//
// 关键点：不能简单 `systemctl restart`——在没有 unit 的环境（手动运行、
// 容器）会失败并让服务彻底停摆。所以先探测 unit 是否存在。
func buildRestartScript(execPath string, args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = shellQuote(a)
	}
	argStr := strings.Join(quoted, " ")

	return fmt.Sprintf(`sleep 2
if command -v systemctl >/dev/null 2>&1 && systemctl list-unit-files 2>/dev/null | grep -q '^sboard\.service'; then
  systemctl restart sboard && exit 0
fi
# 没有 systemd unit：直接把新二进制重新拉起（脱离当前会话）
cd %s || exit 1
nohup %s %s >/dev/null 2>&1 &
`, shellQuote(filepath.Dir(execPath)), shellQuote(execPath), argStr)
}

// shellQuote 给参数加单引号，避免路径含空格或特殊字符时被 shell 拆开。
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// fetchChecksums 下载并解析 checksums.txt（每行 "<sha256>  <filename>"）。
func fetchChecksums(url string) (map[string]string, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	out := make(map[string]string)
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		out[name] = strings.ToLower(fields[0])
	}
	return out, nil
}

// downloadToFile 下载到指定路径。
func downloadToFile(url, dest string) error {
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

// proxyCandidates 组装下载候选：各加速前缀 + 直连兜底。
func proxyCandidates(p *agent.SelfUpdateData, rawURL string) []string {
	var out []string
	add := func(prefix string) {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" {
			return
		}
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		out = append(out, prefix+rawURL)
	}

	add(p.Proxy)
	for _, f := range p.FallbackProxies {
		add(f)
	}
	// 直连兜底
	out = append(out, rawURL)
	return out
}

// fileSHA256Hex 计算文件 sha256。
func fileSHA256Hex(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// extractBinaryFromZip 从 zip 里取出指定文件写到 dest。
func extractBinaryFromZip(zipPath, name, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if filepath.Base(f.Name) != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		out, err := os.OpenFile(dest, os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			return err
		}
		defer out.Close()

		_, err = io.Copy(out, rc)
		return err
	}
	return fmt.Errorf("zip 内未找到 %s", name)
}
