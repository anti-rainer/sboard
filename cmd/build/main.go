package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// GitHub 加速代理列表（按顺序回退）
var ghProxies = []string{
	"",                    // 直接下载
	"https://ghfast.top/", // 加速代理
}

// 嵌入目录
const embedDir = "cmd/agent/embed/configs"

// sing-box 官方仓库（使用官方预编译正式版，不再自行编译 fork）
const (
	singboxLatestAPI   = "https://api.github.com/repos/SagerNet/sing-box/releases/latest"
	singboxDownloadURL = "https://github.com/SagerNet/sing-box/releases/download"
)

// 缓存的版本信息
var singboxVersion string

// GitHubRelease GitHub Release API 响应结构
type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
}

func main() {
	var (
		targetOS     string
		targetArch   string
		skipFrontend bool
		skipDownload bool
		onlyDownload bool
		verbose      bool
	)

	flag.StringVar(&targetOS, "os", runtime.GOOS, "目标操作系统 (linux/windows/darwin)")
	flag.StringVar(&targetArch, "arch", runtime.GOARCH, "目标架构 (amd64/386/arm64/armv7/armv6)")
	flag.BoolVar(&skipFrontend, "skip-frontend", false, "跳过前端构建")
	flag.BoolVar(&skipDownload, "skip-download", false, "跳过核心下载")
	flag.BoolVar(&onlyDownload, "only-download", false, "只下载核心，不编译")
	flag.BoolVar(&verbose, "v", false, "详细输出")
	flag.Parse()

	fmt.Println("==========================================")
	fmt.Println("  Sboard 构建工具")
	fmt.Printf("  目标平台: %s/%s\n", targetOS, targetArch)
	fmt.Println("==========================================")

	// 1. 构建前端
	if !skipFrontend && !onlyDownload {
		if err := buildFrontend(); err != nil {
			fmt.Printf("前端构建失败: %v\n", err)
			os.Exit(1)
		}
	}

	// 2. 下载核心
	if !skipDownload {
		if err := downloadCores(targetOS, targetArch, verbose); err != nil {
			fmt.Printf("核心下载失败: %v\n", err)
			os.Exit(1)
		}
	}

	if onlyDownload {
		fmt.Println("\n核心下载完成!")
		return
	}

	// 3. 编译 Sboard
	if err := buildSboard(); err != nil {
		fmt.Printf("Sboard 编译失败: %v\n", err)
		os.Exit(1)
	}

	// 4. 编译 Agent
	if err := buildAgent(targetOS, targetArch); err != nil {
		fmt.Printf("Agent 编译失败: %v\n", err)
		os.Exit(1)
	}

	showResult(targetOS, targetArch)
}

func buildFrontend() error {
	fmt.Println("\n=== 构建前端 ===")

	if _, err := os.Stat("web"); os.IsNotExist(err) {
		fmt.Println("web 目录不存在，跳过前端构建")
		return nil
	}

	cmd := exec.Command("npm", "run", "build")
	cmd.Dir = "web"
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func downloadCores(targetOS, targetArch string, verbose bool) error {
	fmt.Println("\n=== 下载核心文件 ===")

	fmt.Println("获取 sing-box 官方最新正式版...")
	if err := fetchLatestOfficialVersion(verbose); err != nil {
		return fmt.Errorf("获取版本信息失败: %v", err)
	}
	fmt.Printf("  sing-box: v%s (官方正式版)\n", singboxVersion)

	cleanOldCores()

	if err := downloadSingbox(targetOS, targetArch, verbose); err != nil {
		return fmt.Errorf("下载 sing-box 失败: %v", err)
	}

	return nil
}

// fetchLatestOfficialVersion 从 sing-box 官方仓库获取最新正式版版本号。
func fetchLatestOfficialVersion(verbose bool) error {
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest("GET", singboxLatestAPI, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "sboard-build-tool")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API 返回 HTTP %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return err
	}
	if release.TagName == "" {
		return fmt.Errorf("GitHub API 未返回版本号")
	}

	singboxVersion = strings.TrimPrefix(release.TagName, "v")
	if verbose {
		fmt.Printf("  最新正式版标签: %s\n", release.TagName)
	}
	return nil
}

// singboxAssetName 根据目标平台返回官方 release 的资产文件名。
// Linux 统一使用 -musl 变体：官方该变体为完全静态（无 INTERP），
// 可同时运行在 glibc（Debian/Ubuntu）与 musl（Alpine/OpenWrt）系统上。
func singboxAssetName(version, targetOS, targetArch string) (string, error) {
	switch targetOS {
	case "linux":
		switch targetArch {
		case "amd64", "arm64", "386", "armv7":
			return fmt.Sprintf("sing-box-%s-linux-%s-musl.tar.gz", version, targetArch), nil
		case "armv6":
			return fmt.Sprintf("sing-box-%s-linux-armv6.tar.gz", version), nil
		case "armv5":
			return fmt.Sprintf("sing-box-%s-linux-armv5.tar.gz", version), nil
		default:
			return fmt.Sprintf("sing-box-%s-linux-%s.tar.gz", version, targetArch), nil
		}
	case "windows":
		switch targetArch {
		case "amd64", "arm64", "386":
			return fmt.Sprintf("sing-box-%s-windows-%s.zip", version, targetArch), nil
		default:
			return "", fmt.Errorf("Windows 不支持的架构: %s", targetArch)
		}
	case "darwin":
		switch targetArch {
		case "amd64", "arm64":
			return fmt.Sprintf("sing-box-%s-darwin-%s.tar.gz", version, targetArch), nil
		default:
			return "", fmt.Errorf("macOS 不支持的架构: %s", targetArch)
		}
	default:
		return "", fmt.Errorf("不支持的操作系统: %s", targetOS)
	}
}

func downloadSingbox(targetOS, targetArch string, verbose bool) error {
	assetName, err := singboxAssetName(singboxVersion, targetOS, targetArch)
	if err != nil {
		return err
	}

	binaryName := "sing-box"
	if targetOS == "windows" {
		binaryName = "sing-box.exe"
	}
	targetPath := filepath.Join(embedDir, "sing-box", binaryName)

	url := fmt.Sprintf("%s/v%s/%s", singboxDownloadURL, singboxVersion, assetName)
	fmt.Printf("\n下载 sing-box v%s (%s/%s)...\n", singboxVersion, targetOS, targetArch)
	fmt.Printf("  资产: %s\n", assetName)

	archivePath, err := downloadWithProxy(url, verbose)
	if err != nil {
		return fmt.Errorf("下载 %s 失败: %v", assetName, err)
	}
	defer os.Remove(archivePath)

	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("创建目标目录失败: %v", err)
	}

	if strings.HasSuffix(assetName, ".zip") {
		err = extractZipSingbox(archivePath, targetPath, binaryName)
	} else {
		err = extractTarGz(archivePath, targetPath, binaryName)
	}
	if err != nil {
		return fmt.Errorf("解压核心失败: %v", err)
	}

	if err := os.Chmod(targetPath, 0755); err != nil {
		return fmt.Errorf("设置可执行权限失败: %v", err)
	}

	// 同平台时直接执行一次版本校验，确认二进制可用且版本正确。
	if targetOS == runtime.GOOS && targetArch == runtime.GOARCH {
		out, verr := exec.Command(targetPath, "version").Output()
		if verr != nil {
			return fmt.Errorf("核心无法执行: %v", verr)
		}
		firstLine := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
		if !strings.Contains(firstLine, singboxVersion) {
			return fmt.Errorf("核心版本不匹配: 期望 %s，实际 %q", singboxVersion, firstLine)
		}
		fmt.Printf("  版本校验通过: %s\n", firstLine)
	}

	fmt.Printf("sing-box 就绪: %s\n", targetPath)
	return nil
}

func cleanOldCores() {
	fmt.Println("清理旧的核心文件...")
	files := []string{
		filepath.Join(embedDir, "sing-box", "sing-box"),
		filepath.Join(embedDir, "sing-box", "sing-box.exe"),
	}
	for _, f := range files {
		os.Remove(f)
	}
}

func downloadWithProxy(baseURL string, verbose bool) (string, error) {
	client := &http.Client{
		Timeout: 10 * time.Minute,
	}

	var lastErr error
	for _, proxy := range ghProxies {
		url := proxy + baseURL
		if verbose {
			if proxy == "" {
				fmt.Println("  尝试直接下载")
			} else {
				fmt.Printf("  尝试代理下载: %s\n", proxy)
			}
		}

		tmpFile, err := downloadFile(client, url)
		if err != nil {
			lastErr = err
			if verbose {
				fmt.Printf("  下载失败: %v\n", err)
			}
			continue
		}

		return tmpFile, nil
	}

	return "", fmt.Errorf("所有下载源都失败: %v", lastErr)
}

func downloadFile(client *http.Client, url string) (string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "sboard-download-*")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}

	return tmpFile.Name(), nil
}

func extractTarGz(srcFile, destFile, binaryName string) error {
	f, err := os.Open(srcFile)
	if err != nil {
		return err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if header.Typeflag == tar.TypeReg &&
			(strings.HasSuffix(header.Name, "/"+binaryName) || header.Name == binaryName) {
			os.MkdirAll(filepath.Dir(destFile), 0755)
			out, err := os.Create(destFile)
			if err != nil {
				return err
			}
			defer out.Close()
			_, err = io.Copy(out, tr)
			return err
		}
	}

	return fmt.Errorf("压缩包中未找到 %s", binaryName)
}

func extractZipSingbox(srcFile, destFile, binaryName string) error {
	r, err := zip.OpenReader(srcFile)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if strings.HasSuffix(f.Name, "/"+binaryName) || filepath.Base(f.Name) == binaryName {
			return extractZipFile(f, destFile)
		}
	}

	return fmt.Errorf("压缩包中未找到 %s", binaryName)
}

func extractZipFile(f *zip.File, destFile string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	os.MkdirAll(filepath.Dir(destFile), 0755)
	out, err := os.Create(destFile)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, rc)
	return err
}

func buildSboard() error {
	fmt.Println("\n=== 编译 Sboard ===")

	cmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", "sboard", "./cmd/sboard")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return err
	}

	fmt.Println("编译完成: sboard")
	return nil
}

func buildAgent(targetOS, targetArch string) error {
	fmt.Printf("\n=== 编译 Agent (%s/%s) ===\n", targetOS, targetArch)

	outputName := "sboard-agent"
	if targetOS == "windows" {
		outputName = "sboard-agent.exe"
	}

	cmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", outputName, "./cmd/agent")
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"GOOS="+targetOS,
		"GOARCH="+targetArch,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return err
	}

	fmt.Printf("编译完成: %s\n", outputName)
	return nil
}

func showResult(targetOS, targetArch string) {
	fmt.Println("\n==========================================")
	fmt.Println("  构建完成!")
	fmt.Println("==========================================")
	fmt.Printf("目标平台: %s/%s\n", targetOS, targetArch)
	fmt.Println()
	fmt.Println("嵌入的核心版本:")
	fmt.Printf("  - sing-box: v%s (官方正式版)\n", singboxVersion)
	fmt.Println()
	fmt.Println("生成文件:")

	files := []string{"sboard", "sboard-agent"}
	if targetOS == "windows" {
		files = []string{"sboard.exe", "sboard-agent.exe"}
	}

	for _, f := range files {
		if info, err := os.Stat(f); err == nil {
			fmt.Printf("  %s (%d KB)\n", f, info.Size()/1024)
		}
	}
}
