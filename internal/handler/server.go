package handler

import (
	"context"
	"embed"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sboard-go/sboard/internal/config"
	"github.com/sboard-go/sboard/internal/middleware"
)

// Server HTTP 服务器
type Server struct {
	config     *config.Config
	router     *gin.Engine
	frontendFS embed.FS
	httpServer *http.Server
	// buildTime 进程启动时间，用于静态资源的 Last-Modified
	buildTime time.Time
}

// NewServer 创建新的服务器实例
func NewServer(cfg *config.Config, frontendFS embed.FS) *Server {
	// 设置 Gin 模式
	if cfg.Server.Debug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// 初始化 JWT
	middleware.InitJWT(cfg.Security.JWTSecret)

	// 初始化 sublink 的 SS2022 混淆盐值
	sublinkSalt = cfg.Security.JWTSecret

	router := gin.New()
	router.Use(gin.Recovery())
	// 不输出请求日志
	router.Use(middleware.CORSMiddleware())

	s := &Server{
		config:     cfg,
		router:     router,
		frontendFS: frontendFS,
		buildTime:  time.Now(),
	}

	s.setupRoutes()

	// 启动 Agent Hub 消息泵（原 init() 副作用）
	InitAgentHub()

	return s
}

// setupRoutes 设置路由
func (s *Server) setupRoutes() {
	// API 路由组
	api := s.router.Group("/api")
	{
		// 认证路由（无需登录）
		auth := api.Group("/auth")
		{
			auth.POST("/login", middleware.LoginRateLimit(), s.handleLogin)
			auth.GET("/oauth/providers", s.handleGetOAuthProviders)
			auth.GET("/github/login", s.handleGitHubLogin)
			auth.GET("/github/callback", s.handleGitHubCallback)
		}

		// 需要认证的路由
		protected := api.Group("")
		protected.Use(middleware.AuthMiddleware())
		{
			// 认证相关
			protected.POST("/auth/logout", s.handleLogout)
			protected.POST("/auth/change-password", s.handleChangePassword)
			protected.POST("/auth/change-username", s.handleChangeUsername)
			protected.GET("/auth/me", s.handleGetCurrentUser)

			// 用户管理
			users := protected.Group("/users")
			{
				users.GET("", s.handleListUsers)
				users.POST("", s.handleCreateUser)
				users.GET("/:id", s.handleGetUser)
				users.PUT("/:id", s.handleUpdateUser)
				users.DELETE("/:id", s.handleDeleteUser)
			}

			// 节点管理
			nodes := protected.Group("/nodes")
			{
				nodes.GET("", s.handleListNodes)
				nodes.POST("", s.handleCreateNode)
				nodes.GET("/:id", s.handleGetNode)
				nodes.PUT("/:id", s.handleUpdateNode)
				nodes.DELETE("/:id", s.handleDeleteNode)
				nodes.POST("/generate-reality-keys", s.handleGenerateRealityKeys)
				nodes.POST("/batch-delete", s.handleBatchDeleteNodes)
			}

			// 服务器管理
			servers := protected.Group("/servers")
			{
				servers.GET("", s.handleListServers)
				servers.GET("/status", s.handleGetAllServersStatus) // 批量获取状态
				servers.POST("", s.handleCreateServer)
				servers.POST("/reorder", s.handleReorderServers)     // 重新排序
				servers.POST("/deploy-all", s.handleDeployAll)       // 全部部署
				servers.POST("/update-agents", s.handleUpdateAgents) // Agent 更新
				servers.GET("/:id", s.handleGetServer)
				servers.PUT("/:id", s.handleUpdateServer)
				servers.DELETE("/:id", s.handleDeleteServer)
				servers.POST("/:id/nodes", s.handleSetServerNodes)
				servers.POST("/:id/deploy", s.handleDeployServer)
				// 节点配置管理
				servers.GET("/:id/node-configs", s.handleGetServerNodeConfigs)
				servers.POST("/:id/node-configs/:nodeId", s.handleSaveServerNodeConfig)
				servers.DELETE("/:id/node-configs/:nodeId", s.handleDeleteServerNodeConfig)
				// 落地出站管理
				servers.GET("/:id/outbounds", s.handleListServerOutbounds)
				servers.POST("/:id/outbounds", s.handleCreateServerOutbound)
				servers.GET("/:id/outbounds/:slot", s.handleGetServerOutbound)
				servers.PUT("/:id/outbounds/:slot", s.handleUpdateServerOutbound)
				servers.DELETE("/:id/outbounds/:slot", s.handleDeleteServerOutbound)
				servers.POST("/:id/outbounds/:slot/toggle", s.handleToggleServerOutbound)
				// 服务器配置文件
				servers.GET("/:id/config", s.handleGetServerConfig)
				// Agent 相关
				servers.GET("/:id/agent/status", s.handleGetAgentStatus)
				servers.POST("/:id/agent/command", s.handleSendAgentCommand)

				servers.POST("/:id/agent/deploy", s.handleDeployCoreToAgent)
				servers.POST("/:id/agent/regenerate-token", s.handleRegenerateAgentToken)
			}

			protected.GET("/nodes/config", s.handleGetNodeConfig) // 系统设置
			protected.GET("/settings", s.handleGetSettings)
			protected.POST("/settings", s.handleUpdateSettings)

			// OAuth 管理
			oauth := protected.Group("/oauth")
			{
				oauth.GET("/providers", s.handleGetOAuthProvidersAdmin)   // 获取所有提供商（管理）
				oauth.GET("/providers/:name", s.handleGetOAuthProvider)   // 获取指定提供商
				oauth.POST("/providers/:name", s.handleSaveOAuthProvider) // 保存提供商配置
				oauth.POST("/settings", s.handleSaveOAuthSettings)        // 保存全局 OAuth 设置
			}

			// 订阅配置
			protected.GET("/subscription/configs", s.handleGetSubscriptionConfigs)
			protected.POST("/subscription/configs", s.handleSaveSubscriptionConfigs)

			// 配置预览
			protected.GET("/config/preview", s.handlePreviewConfig)

			// 外部节点管理
			externalNodes := protected.Group("/external-nodes")
			{
				externalNodes.GET("", s.handleListExternalNodes)
				externalNodes.GET("/:id", s.handleGetExternalNode)
				externalNodes.POST("", s.handleCreateExternalNode)
				externalNodes.PUT("/:id", s.handleUpdateExternalNode)
				externalNodes.DELETE("/:id", s.handleDeleteExternalNode)
			}

			// 用户到期检查
			protected.POST("/users/check-expiry", s.handleCheckExpiredUsers)
			protected.GET("/users/expiring", s.handleGetExpiringUsers)
		}
	}

	// 订阅链接（无需认证，使用 UUID）
	s.router.GET("/sublink", s.handleSublink)
	s.router.GET("/sublink.php", s.handleSublinkPHP)    // 兼容旧 PHP 路径
	s.router.GET("/sub/:uuid", s.handleSubByUUID)       // 短订阅链接
	s.router.GET("/subscribe/:uuid", s.handleSubByUUID) // 兼容格式

	// 路径式客户端配置与 provider 地址，形如
	// 路径式客户端配置与 provider 地址，形如
	//   /subscribe/<uuid>/<node>/mihomo/sboard.yaml
	//   /subscribe/<uuid>/<node>/sing-box/sboard.json
	//   /subscribe/<uuid>/<node>/shadowrocket/sboard.conf
	//   /subscribe/<uuid>/<node>/loon/sboard.conf
	// 末段文件名只为客户端识别后缀，服务端按 <client> 段决定输出。
	s.router.GET("/subscribe/:uuid/:node/:client/:file", s.handleSubByPath)

	// 版本信息（无需认证）：前端据此判断服务端是否已更新，
	// 用于对冲 CDN 缓存旧前端造成的「更新后仍是老界面」问题。
	s.router.GET("/api/version", s.handleVersionInfo)

	// Agent WebSocket 端点（使用 token 认证）
	s.router.GET("/api/agent/ws", s.handleAgentWebSocket)

	// Agent 二进制下载（无需认证）
	// 安装脚本已移至 GitHub: https://github.com/anti-rainer/sboard/blob/main/scripts/install-agent.sh
	s.router.GET("/download/agent-linux-:arch", s.handleDownloadAgent)

	// 静态文件服务（前端）
	s.serveFrontend()
}

// serveFrontend 提供前端静态文件服务
func (s *Server) serveFrontend() {
	// 尝试从嵌入的文件系统提供静态文件
	webFS, err := fs.Sub(s.frontendFS, "web")
	if err != nil {
		// 如果没有嵌入的前端文件，使用简单的响应
		s.router.NoRoute(func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"message": "SBoard API Server",
				"version": "3.0.0",
				"docs":    "/api",
			})
		})
		return
	}

	// /assets 静态资源：文件名带内容 hash，长缓存；未命中返回 404
	// （绝不回退 index.html —— 否则 CDN 会把 HTML 当成 CSS/JS 缓存住）
	//
	// GET 与 HEAD 用 Match 一次注册：CDN 常用 HEAD 探测资源，
	// 只注册 GET 会让 HEAD 落到 404，反而让 CDN 判定「资源不存在」。
	assetsHandler := s.serveAssets(webFS)
	s.router.Match([]string{http.MethodGet, http.MethodHead}, "/assets/*filepath", assetsHandler)

	// SPA 回退：index.html 强制不缓存，保证拿到的总是最新资源清单
	indexHandler := s.serveIndex(webFS)
	s.router.Match([]string{http.MethodGet, http.MethodHead}, "/", indexHandler)
	s.router.NoRoute(func(c *gin.Context) {
		// API 与订阅路径不走 SPA 回退，避免掩盖真实的 404
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/subscribe/") ||
			strings.HasPrefix(p, "/assets/") {
			c.Status(http.StatusNotFound)
			return
		}
		indexHandler(c)
	})
}

// Run 启动服务器
func (s *Server) Run() error {
	s.httpServer = &http.Server{
		Addr:    s.config.Server.Listen,
		Handler: s.router,
	}
	return s.httpServer.ListenAndServe()
}

// Shutdown 优雅关闭服务器
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}
