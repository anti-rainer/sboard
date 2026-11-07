package handler

// 前端静态资源的缓存策略。
//
// 背景：之前用 gin 的 StaticFS 直接暴露 /assets，SPA 回退对未命中路径一律返回
// index.html 且不带任何 Cache-Control。Cloudflare 会按默认策略缓存这些响应，
// 于是出现两个问题：
//   1. 新版本发布后，index.html 命中的是旧缓存，用户看到的还是老界面；
//   2. 更糟的情况：请求一个「新版本才有、旧版本没有」的 assets 文件名时，
//      回退逻辑返回了 200 + index.html，CF 把这个 HTML 当成 .css/.js 缓存下来，
//      之后即使服务端有了正确的文件，用户拿到的仍是那段 HTML。
//      （实测 assets/index-xxxx.css 返回 584 字节 text/html）
//
// 正确做法：
//   - index.html：绝不缓存（no-store），每次都回源拿最新的资源清单
//   - assets/*：文件名带内容 hash，可以长缓存 + immutable
//   - /assets 下未命中：返回真正的 404，而不是回退 index.html，
//     避免把 HTML 当作资源缓存
//   - 响应头带 ETag/Last-Modified，便于条件请求

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// noCacheHeaders 让浏览器与 CDN 都不缓存该响应。
func noCacheHeaders(c *gin.Context) {
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
	// 明确告知 Cloudflare / 中间缓存不要缓存
	c.Header("CDN-Cache-Control", "no-store")
	c.Header("Cloudflare-CDN-Cache-Control", "no-store")
}

// assetCacheHeaders 静态资源长缓存。文件名内含内容 hash（vite 生成），
// 内容变化必然换文件名，因此可以放心 immutable。
func assetCacheHeaders(c *gin.Context) {
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
}

// serveAssets 提供 /assets 下的静态资源。
//
// 与 StaticFS 的区别：未命中时返回 404 而不是回退到 index.html，
// 并显式设置缓存头。返回 (handled=false) 表示该路径不归这里管。
func (s *Server) serveAssets(webFS fs.FS) gin.HandlerFunc {
	assetsFS, err := fs.Sub(webFS, "assets")
	if err != nil {
		return func(c *gin.Context) { c.Next() }
	}

	return func(c *gin.Context) {
		name := strings.TrimPrefix(c.Param("filepath"), "/")
		if name == "" {
			c.Status(http.StatusNotFound)
			c.Abort()
			return
		}

		data, err := fs.ReadFile(assetsFS, name)
		if err != nil {
			// 关键：不要回退 index.html。资源缺失就是 404，
			// 否则 CDN 会把 HTML 当 CSS/JS 缓存住，且短期内无法纠正。
			noCacheHeaders(c)
			c.Status(http.StatusNotFound)
			c.Abort()
			return
		}

		etag := `"` + etagFor(data) + `"`
		if match := c.GetHeader("If-None-Match"); match != "" && match == etag {
			assetCacheHeaders(c)
			c.Header("ETag", etag)
			c.Status(http.StatusNotModified)
			c.Abort()
			return
		}

		assetCacheHeaders(c)
		c.Header("ETag", etag)
		c.Header("Last-Modified", s.buildTime.UTC().Format(http.TimeFormat))
		c.Data(http.StatusOK, contentTypeByName(name), data)
		c.Abort()
	}
}

// serveIndex 返回 index.html，强制不缓存，保证用户总能看到最新资源清单。
func (s *Server) serveIndex(webFS fs.FS) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, err := fs.ReadFile(webFS, "index.html")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"message": "SBoard API Server",
			})
			return
		}
		noCacheHeaders(c)
		c.Header("ETag", `"`+etagFor(data)+`"`)
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	}
}

// Version 面板版本号，由 main 通过 SetVersion 注入（-ldflags 注入到 cmd/sboard）。
var Version = "dev"

// SetVersion 设置面板版本号。
func SetVersion(v string) {
	if v != "" {
		Version = v
	}
}

// handleVersionInfo 返回服务端版本与进程启动时间。
//
// 前端对比 build_at 判断是否已发布新版本：进程重启即代表二进制换了，
// 比版本号可靠（预发布版与正式版可能都叫 v1.0.6）。
func (s *Server) handleVersionInfo(c *gin.Context) {
	noCacheHeaders(c)
	c.JSON(http.StatusOK, gin.H{
		"build_at": s.buildTime.UTC().Format(time.RFC3339Nano),
		"version":  Version,
	})
}

// etagFor 用内容 sha256 前 16 位作 ETag，避免整份文件参与比较。
func etagFor(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8])
}

// contentTypeByName 按扩展名给出 Content-Type。
//
// 不用 http.DetectContentType：它对压缩后的 JS/CSS 可能误判成 text/plain，
// 导致浏览器拒绝执行（严格 MIME 检查）。
func contentTypeByName(name string) string {
	switch {
	case strings.HasSuffix(name, ".js"), strings.HasSuffix(name, ".mjs"):
		return "text/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".json"):
		return "application/json; charset=utf-8"
	case strings.HasSuffix(name, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(name, ".png"):
		return "image/png"
	case strings.HasSuffix(name, ".jpg"), strings.HasSuffix(name, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(name, ".webp"):
		return "image/webp"
	case strings.HasSuffix(name, ".woff2"):
		return "font/woff2"
	case strings.HasSuffix(name, ".woff"):
		return "font/woff"
	case strings.HasSuffix(name, ".ttf"):
		return "font/ttf"
	case strings.HasSuffix(name, ".ico"):
		return "image/x-icon"
	case strings.HasSuffix(name, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(name, ".map"):
		return "application/json; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

// 保证 bytes/time 被引用（供未来扩展 ETag 计算方式时使用）
var (
	_ = bytes.Equal
	_ = time.Now
)
