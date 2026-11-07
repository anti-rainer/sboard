package handler

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sboard-go/sboard/internal/database"
	"golang.org/x/crypto/curve25519"
)

// CreateNodeRequest 创建节点请求
type CreateNodeRequest struct {
	Tag      string `json:"tag" binding:"required"`
	Protocol string `json:"protocol" binding:"required"`
	Listen   string `json:"listen"`
	Port     int    `json:"port" binding:"required"`

	// TLS
	TlsEnabled bool   `json:"tls_enabled"`
	ServerName string `json:"server_name"`
	CertPath   string `json:"cert_path"`
	KeyPath    string `json:"key_path"`

	// Reality
	RealityEnabled bool   `json:"reality_enabled"`
	RealityServer  string `json:"reality_server"`
	RealityPubkey  string `json:"reality_pubkey"`
	RealityPrivkey string `json:"reality_privkey"`
	RealityShortId string `json:"reality_short_id"`

	// Transport
	TransportEnabled bool   `json:"transport_enabled"`
	TransportType    string `json:"transport_type"`
	WsPath           string `json:"ws_path"`
	GrpcService      string `json:"grpc_service"`
	TransportHost    string `json:"transport_host"`

	// Flow
	Flow string `json:"flow"`

	// Shadowsocks
	SsMethod   string `json:"ss_method"`
	SsPassword string `json:"ss_password"`

	// Hysteria2

	Hy2UpMbps       int    `json:"hy2_up_mbps"`
	Hy2DownMbps     int    `json:"hy2_down_mbps"`
	Hy2Obfs         string `json:"hy2_obfs"`
	Hy2ObfsPassword string `json:"hy2_obfs_password"`

	// TUIC（QUIC 系，强制 TLS）

	TuicCongestion string `json:"tuic_congestion"`
	TuicZeroRtt    bool   `json:"tuic_zero_rtt"`

	// Status
	Enabled bool   `json:"enabled"`
	Notes   string `json:"notes"`
}

// UpdateNodeRequest 更新节点请求
type UpdateNodeRequest struct {
	Tag      string `json:"tag"`
	Protocol string `json:"protocol"`
	Listen   string `json:"listen"`
	Port     int    `json:"port"`

	// TLS
	TlsEnabled bool   `json:"tls_enabled"`
	ServerName string `json:"server_name"`
	CertPath   string `json:"cert_path"`
	KeyPath    string `json:"key_path"`

	// Reality
	RealityEnabled bool   `json:"reality_enabled"`
	RealityServer  string `json:"reality_server"`
	RealityPubkey  string `json:"reality_pubkey"`
	RealityPrivkey string `json:"reality_privkey"`
	RealityShortId string `json:"reality_short_id"`

	// Transport
	TransportEnabled bool   `json:"transport_enabled"`
	TransportType    string `json:"transport_type"`
	WsPath           string `json:"ws_path"`
	GrpcService      string `json:"grpc_service"`
	TransportHost    string `json:"transport_host"`

	// Flow
	Flow string `json:"flow"`

	// Shadowsocks
	SsMethod   string `json:"ss_method"`
	SsPassword string `json:"ss_password"`

	// Hysteria2

	Hy2UpMbps       int    `json:"hy2_up_mbps"`
	Hy2DownMbps     int    `json:"hy2_down_mbps"`
	Hy2Obfs         string `json:"hy2_obfs"`
	Hy2ObfsPassword string `json:"hy2_obfs_password"`

	// TUIC（QUIC 系，强制 TLS）

	TuicCongestion string `json:"tuic_congestion"`
	TuicZeroRtt    bool   `json:"tuic_zero_rtt"`

	// Status
	Enabled bool   `json:"enabled"`
	Notes   string `json:"notes"`
}

// handleListNodes 获取节点列表
func (s *Server) handleListNodes(c *gin.Context) {
	var nodes []database.InboundNode

	query := database.GetDB().Order("id ASC")

	// 支持协议过滤
	if protocol := c.Query("protocol"); protocol != "" {
		query = query.Where("protocol = ?", protocol)
	}

	// 支持启用状态过滤
	if enableStr := c.Query("enabled"); enableStr != "" {
		enabled := enableStr == "true" || enableStr == "1"
		query = query.Where("enabled = ?", enabled)
	}

	if err := query.Find(&nodes).Error; err != nil {
		errorJSON(c, http.StatusInternalServerError, "查询失败")
		return
	}

	// 获取统计信息
	var total int64
	database.GetDB().Model(&database.InboundNode{}).Count(&total)

	var enabledCount int64
	database.GetDB().Model(&database.InboundNode{}).Where("enabled = ?", true).Count(&enabledCount)

	// 按协议统计
	type ProtocolStat struct {
		Protocol string `json:"protocol"`
		Count    int64  `json:"count"`
	}
	var protocolStats []ProtocolStat
	database.GetDB().Model(&database.InboundNode{}).Select("protocol, count(*) as count").Group("protocol").Scan(&protocolStats)

	successJSON(c, gin.H{
		"nodes": nodes,
		"stats": gin.H{
			"total":     total,
			"enabled":   enabledCount,
			"protocols": protocolStats,
		},
	})
}

// handleGetNode 获取单个节点
func (s *Server) handleGetNode(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errorJSON(c, http.StatusBadRequest, "无效的节点ID")
		return
	}

	var node database.InboundNode
	if err := database.GetDB().First(&node, id).Error; err != nil {
		errorJSON(c, http.StatusNotFound, "节点不存在")
		return
	}

	successJSON(c, node)
}

// handleCreateNode 创建节点
func (s *Server) handleCreateNode(c *gin.Context) {
	var req CreateNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorJSON(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 验证协议
	validProtocols := []string{"trojan", "vless", "vmess", "anytls", "shadowsocks", "hysteria2", "tuic", "socks", "naive"}
	isValidProtocol := false
	for _, p := range validProtocols {
		if req.Protocol == p {
			isValidProtocol = true
			break
		}
	}
	if !isValidProtocol {
		errorJSON(c, http.StatusBadRequest, "无效的协议类型")
		return
	}

	// 检查 tag 是否已存在
	var count int64
	database.GetDB().Model(&database.InboundNode{}).Where("tag = ?", req.Tag).Count(&count)
	if count > 0 {
		errorJSON(c, http.StatusBadRequest, "节点 Tag 已存在")
		return
	}

	// 检查端口是否已存在
	database.GetDB().Model(&database.InboundNode{}).Where("port = ?", req.Port).Count(&count)
	if count > 0 {
		errorJSON(c, http.StatusBadRequest, "端口已被使用")
		return
	}

	// 设置默认值
	if req.Listen == "" {
		req.Listen = "::"
	}
	if req.ServerName == "" {
		req.ServerName = "down.dingtalk.com"
	}
	if req.CertPath == "" {
		req.CertPath = "server.crt"
	}
	if req.KeyPath == "" {
		req.KeyPath = "server.key"
	}
	if req.RealityEnabled {
		req.TlsEnabled = true
		req.RealityServer = strings.TrimSpace(req.RealityServer)
		req.RealityPubkey = strings.TrimSpace(req.RealityPubkey)
		req.RealityPrivkey = strings.TrimSpace(req.RealityPrivkey)
		req.RealityShortId = strings.TrimSpace(req.RealityShortId)
		if req.RealityServer == "" || req.RealityPubkey == "" || req.RealityPrivkey == "" {
			errorJSON(c, http.StatusBadRequest, "启用 Reality 时必须填写握手服务器、公钥和私钥")
			return
		}
		if !isValidRealityKey(req.RealityPubkey) || !isValidRealityKey(req.RealityPrivkey) {
			errorJSON(c, http.StatusBadRequest, "Reality 公钥和私钥必须是 32 字节 Base64 密钥")
			return
		}
		if req.RealityShortId != "" && !isValidRealityShortID(req.RealityShortId) {
			errorJSON(c, http.StatusBadRequest, "Reality short_id 必须是最多 16 位十六进制字符串")
			return
		}
	}
	if req.WsPath == "" {
		req.WsPath = "/"
	}
	if req.TuicCongestion == "" {
		req.TuicCongestion = "bbr"
	}

	// 如果启用 Reality 且 short_id 为空，自动生成随机 short_id
	if req.RealityEnabled && req.RealityShortId == "" {
		req.RealityShortId = generateRandomShortId()
	}

	// Shadowsocks 协议：密码为空时按加密方法生成；填了但与 method 要求的
	// 密钥长度不符时，同样重新生成——否则客户端会因 bad key length 直接崩，
	// 而面板上看不出问题。
	if req.Protocol == "shadowsocks" {
		if req.SsMethod == "" {
			req.SsMethod = "2022-blake3-aes-256-gcm" // 默认：SS2022，32 字节密钥
		}
		if !isSupportedSSMethod(req.SsMethod) {
			errorJSON(c, http.StatusBadRequest,
				"Shadowsocks 仅支持 SS2022 的 AES 系：2022-blake3-aes-128-gcm / "+
					"2022-blake3-aes-256-gcm（chacha20 不支持多用户，已移除）")
			return
		}
		if req.SsPassword == "" {
			req.SsPassword = generateSS2022Password(req.SsMethod)
		} else if !ss2022KeyMatchesMethod(req.SsPassword, req.SsMethod) {
			req.SsPassword = generateSS2022Password(req.SsMethod)
		}
	}

	node := database.InboundNode{
		Tag:              req.Tag,
		Protocol:         req.Protocol,
		Listen:           req.Listen,
		Port:             req.Port,
		TlsEnabled:       req.TlsEnabled,
		ServerName:       req.ServerName,
		CertPath:         req.CertPath,
		KeyPath:          req.KeyPath,
		RealityEnabled:   req.RealityEnabled,
		RealityServer:    req.RealityServer,
		RealityPubkey:    req.RealityPubkey,
		RealityPrivkey:   req.RealityPrivkey,
		RealityShortId:   req.RealityShortId,
		TransportEnabled: req.TransportEnabled,
		TransportType:    req.TransportType,
		WsPath:           req.WsPath,
		GrpcService:      req.GrpcService,
		TransportHost:    req.TransportHost,
		Flow:             req.Flow,
		SsMethod:         req.SsMethod,
		SsPassword:       req.SsPassword,
		Hy2UpMbps:        req.Hy2UpMbps,
		Hy2DownMbps:      req.Hy2DownMbps,
		Hy2Obfs:          req.Hy2Obfs,
		Hy2ObfsPassword:  req.Hy2ObfsPassword,
		TuicCongestion:   req.TuicCongestion,
		TuicZeroRtt:      req.TuicZeroRtt,
		Enabled:          req.Enabled,
		Notes:            req.Notes,
	}

	if err := database.GetDB().Create(&node).Error; err != nil {
		errorJSON(c, http.StatusInternalServerError, "创建失败: "+err.Error())
		return
	}

	// 自动链接所有用户到该节点
	s.linkAllUsersToNode(&node)

	// 广播配置更新到所有在线 Agent
	go agentHub.BroadcastConfigUpdate()

	successDataMsgJSON(c, "节点创建成功", node)
}

// linkAllUsersToNode 将所有用户链接到节点（批量操作，避免 N+1 查询）
func (s *Server) linkAllUsersToNode(node *database.InboundNode) {
	var users []database.ProxyUser
	database.GetDB().Where("enabled = ?", 1).Find(&users)

	if len(users) == 0 {
		return
	}

	// 1. 构建用户 ID 列表
	userIDs := make([]uint, len(users))
	for i, u := range users {
		userIDs[i] = u.ID
	}

	// 2. 单次查询已存在的关联
	var existing []database.NodeUserRelation
	database.GetDB().Where("node_id = ? AND user_id IN ?", node.ID, userIDs).Find(&existing)
	existingSet := make(map[uint]bool, len(existing))
	for _, r := range existing {
		existingSet[r.UserID] = true
	}

	// 3. 批量插入新关联
	var newRelations []database.NodeUserRelation
	for _, user := range users {
		if existingSet[user.ID] {
			continue
		}
		newRelations = append(newRelations, database.NodeUserRelation{
			NodeID: node.ID,
			UserID: user.ID,
			UUID:   user.UUID,
			Flow:   node.Flow,
		})
	}
	if len(newRelations) > 0 {
		database.GetDB().Create(&newRelations)
	}
}

// handleUpdateNode 更新节点
func (s *Server) handleUpdateNode(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errorJSON(c, http.StatusBadRequest, "无效的节点ID")
		return
	}

	var node database.InboundNode
	if err := database.GetDB().First(&node, id).Error; err != nil {
		errorJSON(c, http.StatusNotFound, "节点不存在")
		return
	}

	var req UpdateNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorJSON(c, http.StatusBadRequest, "请求参数错误")
		return
	}
	if req.RealityEnabled {
		req.TlsEnabled = true
		req.RealityServer = strings.TrimSpace(req.RealityServer)
		req.RealityPubkey = strings.TrimSpace(req.RealityPubkey)
		req.RealityPrivkey = strings.TrimSpace(req.RealityPrivkey)
		req.RealityShortId = strings.TrimSpace(req.RealityShortId)
		if req.RealityServer == "" || req.RealityPubkey == "" || req.RealityPrivkey == "" {
			errorJSON(c, http.StatusBadRequest, "启用 Reality 时必须填写握手服务器、公钥和私钥")
			return
		}
		if !isValidRealityKey(req.RealityPubkey) || !isValidRealityKey(req.RealityPrivkey) {
			errorJSON(c, http.StatusBadRequest, "Reality 公钥和私钥必须是 32 字节 Base64 密钥")
			return
		}
		if req.RealityShortId != "" && !isValidRealityShortID(req.RealityShortId) {
			errorJSON(c, http.StatusBadRequest, "Reality short_id 必须是最多 16 位十六进制字符串")
			return
		}
	}

	// 跟踪配置相关字段变化（Notes/CertPath/KeyPath 不参与配置生成，忽略）
	configChanged := false
	oldFlow := node.Flow // 保存旧值用于后续 flow 同步判断

	// 如果有新 Tag，检查是否重复
	if req.Tag != "" && req.Tag != node.Tag {
		var count int64
		database.GetDB().Model(&database.InboundNode{}).Where("tag = ? AND id != ?", req.Tag, id).Count(&count)
		if count > 0 {
			errorJSON(c, http.StatusBadRequest, "节点 Tag 已存在")
			return
		}
		configChanged = true
		node.Tag = req.Tag
	}

	// 如果有新端口，检查是否重复
	if req.Port > 0 && req.Port != node.Port {
		var count int64
		database.GetDB().Model(&database.InboundNode{}).Where("port = ? AND id != ?", req.Port, id).Count(&count)
		if count > 0 {
			errorJSON(c, http.StatusBadRequest, "端口已被使用")
			return
		}
		configChanged = true
		node.Port = req.Port
	}

	// 更新字段 — 仅在配置相关字段变化时记录
	if req.Protocol != "" {
		validProtocols := []string{"trojan", "vless", "vmess", "anytls", "shadowsocks", "hysteria2", "tuic", "socks", "naive"}
		isValid := false
		for _, p := range validProtocols {
			if req.Protocol == p {
				isValid = true
				break
			}
		}
		if !isValid {
			errorJSON(c, http.StatusBadRequest, "无效的协议类型")
			return
		}
		if req.Protocol != node.Protocol {
			configChanged = true
		}
		node.Protocol = req.Protocol
	}
	if req.Listen != "" {
		if req.Listen != node.Listen {
			configChanged = true
		}
		node.Listen = req.Listen
	}

	if req.TlsEnabled != node.TlsEnabled {
		configChanged = true
	}
	node.TlsEnabled = req.TlsEnabled
	if req.ServerName != "" && req.ServerName != node.ServerName {
		configChanged = true
		node.ServerName = req.ServerName
	}
	// CertPath/KeyPath 不参与配置生成，不跟踪变化
	if req.CertPath != "" {
		node.CertPath = req.CertPath
	}
	if req.KeyPath != "" {
		node.KeyPath = req.KeyPath
	}

	if req.RealityEnabled != node.RealityEnabled {
		configChanged = true
	}
	node.RealityEnabled = req.RealityEnabled
	if req.RealityServer != node.RealityServer {
		configChanged = true
	}
	node.RealityServer = req.RealityServer
	if req.RealityPubkey != node.RealityPubkey {
		configChanged = true
	}
	node.RealityPubkey = req.RealityPubkey
	if req.RealityPrivkey != node.RealityPrivkey {
		configChanged = true
	}
	node.RealityPrivkey = req.RealityPrivkey
	newShortId := req.RealityShortId
	if req.RealityEnabled && newShortId == "" {
		newShortId = node.RealityShortId
		if newShortId == "" {
			newShortId = generateRandomShortId()
		}
	}
	if newShortId != node.RealityShortId {
		configChanged = true
	}
	node.RealityShortId = newShortId

	if req.TransportEnabled != node.TransportEnabled {
		configChanged = true
	}
	node.TransportEnabled = req.TransportEnabled
	if req.TransportType != node.TransportType {
		configChanged = true
	}
	node.TransportType = req.TransportType
	if req.WsPath != node.WsPath {
		configChanged = true
	}
	node.WsPath = req.WsPath
	if req.GrpcService != node.GrpcService {
		configChanged = true
	}
	node.GrpcService = req.GrpcService
	if req.TransportHost != node.TransportHost {
		configChanged = true
	}
	node.TransportHost = req.TransportHost

	if req.Flow != oldFlow {
		configChanged = true
	}
	node.Flow = req.Flow
	if req.SsMethod != node.SsMethod {
		configChanged = true
	}
	// 只接受 SS2022：传统加密无法按用户分发独立密钥
	if node.Protocol == "shadowsocks" && req.SsMethod != "" && !isSupportedSSMethod(req.SsMethod) {
		errorJSON(c, http.StatusBadRequest,
			"Shadowsocks 仅支持 SS2022 的 AES 系：2022-blake3-aes-128-gcm / "+
				"2022-blake3-aes-256-gcm（chacha20 不支持多用户，已移除）")
		return
	}
	node.SsMethod = req.SsMethod
	// 改加密方式不会自动换密钥，但 SS2022 的 key 长度随 method 固定
	// （aes-128 要 16 字节、aes-256 要 32 字节）。从 aes-256 改成
	// aes-128 而沿用旧密钥时，客户端会直接崩溃：
	//   cipher: 2022-blake3-aes-128-gcm initialize error:
	//   bad key length, required 16, got 32
	// 所以此处按新 method 重新生成主密钥。
	if node.Protocol == "shadowsocks" && regenerateSS2022PasswordIfMismatch(&req.SsPassword, req.SsMethod) {
		configChanged = true
	}
	if req.SsPassword != node.SsPassword {
		configChanged = true
	}
	node.SsPassword = req.SsPassword
	if req.Hy2UpMbps != node.Hy2UpMbps {
		configChanged = true
	}
	node.Hy2UpMbps = req.Hy2UpMbps
	if req.Hy2DownMbps != node.Hy2DownMbps {
		configChanged = true
	}
	node.Hy2DownMbps = req.Hy2DownMbps
	if req.Hy2Obfs != node.Hy2Obfs {
		configChanged = true
	}
	node.Hy2Obfs = req.Hy2Obfs
	if req.Hy2ObfsPassword != node.Hy2ObfsPassword {
		configChanged = true
	}
	node.Hy2ObfsPassword = req.Hy2ObfsPassword

	if req.TuicCongestion != node.TuicCongestion {
		configChanged = true
	}
	node.TuicCongestion = req.TuicCongestion
	if req.TuicZeroRtt != node.TuicZeroRtt {
		configChanged = true
	}
	node.TuicZeroRtt = req.TuicZeroRtt

	if req.Enabled != node.Enabled {
		configChanged = true
	}
	node.Enabled = req.Enabled
	// Notes 不参与配置生成，不跟踪
	node.Notes = req.Notes

	if err := database.GetDB().Save(&node).Error; err != nil {
		errorJSON(c, http.StatusInternalServerError, "保存失败")
		return
	}

	// 同步更新关联表中的 flow 字段（只在 flow 变化时）
	if req.Flow != "" && req.Flow != oldFlow {
		database.GetDB().Model(&database.NodeUserRelation{}).
			Where("node_id = ?", node.ID).
			Update("flow", node.Flow)
	}

	// 只在配置相关字段变化时广播
	if configChanged {
		go agentHub.BroadcastConfigUpdate()
	}

	successDataMsgJSON(c, "节点更新成功", node)
}

// handleDeleteNode 删除节点
func (s *Server) handleDeleteNode(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errorJSON(c, http.StatusBadRequest, "无效的节点ID")
		return
	}

	// 删除节点的用户关联
	database.GetDB().Unscoped().Where("node_id = ?", id).Delete(&database.NodeUserRelation{})

	// 删除节点的服务器配置
	database.GetDB().Unscoped().Where("node_id = ?", id).Delete(&database.ServerNodeConfig{})

	result := database.GetDB().Unscoped().Delete(&database.InboundNode{}, id)
	if result.Error != nil {
		errorJSON(c, http.StatusInternalServerError, "删除失败")
		return
	}

	if result.RowsAffected == 0 {
		errorJSON(c, http.StatusNotFound, "节点不存在")
		return
	}

	// 广播配置更新到所有在线 Agent
	go agentHub.BroadcastConfigUpdate()

	successMsgJSON(c, "节点删除成功")
}

// handleGenerateRealityKeys 生成 Reality 密钥对
func (s *Server) handleGenerateRealityKeys(c *gin.Context) {
	// 使用进程内 X25519 生成，避免依赖外部 sing-box 命令或其 PATH 配置。
	privkey, pubkey, err := generateX25519KeyPair()
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "生成密钥对失败")
		return
	}

	// 生成 short_id (8字节随机十六进制)
	shortIdBytes := make([]byte, 4)
	if _, err := rand.Read(shortIdBytes); err != nil {
		errorJSON(c, http.StatusInternalServerError, "生成 short_id 失败")
		return
	}
	shortId := hex.EncodeToString(shortIdBytes)

	successJSON(c, gin.H{
		"private_key": privkey,
		"public_key":  pubkey,
		"short_id":    shortId,
	})
}

// generateX25519KeyPair 生成 X25519 密钥对
func generateX25519KeyPair() (privateKeyBase64, publicKeyBase64 string, err error) {
	// 生成私钥 (32 字节随机数)
	privateKey := make([]byte, 32)
	if _, err := rand.Read(privateKey); err != nil {
		return "", "", err
	}

	// 计算公钥
	var publicKey [32]byte
	var privateKeyArray [32]byte
	copy(privateKeyArray[:], privateKey)
	curve25519.ScalarBaseMult(&publicKey, &privateKeyArray)

	// Base64 编码
	privateKeyBase64 = base64.RawURLEncoding.EncodeToString(privateKey)
	publicKeyBase64 = base64.RawURLEncoding.EncodeToString(publicKey[:])

	return privateKeyBase64, publicKeyBase64, nil
}

// handleBatchDeleteNodes 批量删除节点
func (s *Server) handleBatchDeleteNodes(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errorJSON(c, http.StatusBadRequest, "请求参数错误")
		return
	}

	// 删除节点的用户关联
	database.GetDB().Unscoped().Where("node_id IN ?", req.IDs).Delete(&database.NodeUserRelation{})

	// 删除节点的服务器配置
	database.GetDB().Unscoped().Where("node_id IN ?", req.IDs).Delete(&database.ServerNodeConfig{})

	result := database.GetDB().Unscoped().Delete(&database.InboundNode{}, req.IDs)
	if result.Error != nil {
		errorJSON(c, http.StatusInternalServerError, "删除失败")
		return
	}

	// 广播配置更新到所有在线 Agent
	go agentHub.BroadcastConfigUpdate()

	successMsgJSON(c, "批量删除成功，共删除 "+strconv.FormatInt(result.RowsAffected, 10)+" 个节点")
}

// generateRandomShortId 生成随机 Reality short_id (4-8字节16进制字符串)
func generateRandomShortId() string {
	// 随机生成 4-8 字节
	lengthByte := make([]byte, 1)
	rand.Read(lengthByte)
	length := 4 + int(lengthByte[0])%5 // 4, 5, 6, 7, 或 8 字节
	bytes := make([]byte, length)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func isValidRealityKey(value string) bool {
	for _, encoding := range []*base64.Encoding{
		base64.RawURLEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.StdEncoding,
	} {
		decoded, err := encoding.DecodeString(value)
		if err == nil && len(decoded) == 32 {
			return true
		}
	}
	return false
}

func isValidRealityShortID(value string) bool {
	if value == "" || len(value) > 16 || len(value)%2 != 0 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

// generateRandomPassword 生成随机密码（用于 Shadowsocks）
func generateRandomPassword(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	bytes := make([]byte, length)
	rand.Read(bytes)
	for i := range bytes {
		bytes[i] = chars[int(bytes[i])%len(chars)]
	}
	return string(bytes)
}

// supportedSSMethods 是入站节点管理允许的 Shadowsocks 加密方式。
// 只保留 SS2022 三种：它们支持 EIH 多用户各自独立密钥，才能按用户下发订阅。
// 传统加密（aes-128-gcm / aes-256-gcm / chacha20-ietf-poly1305 /
// xchacha20-ietf-poly1305）是单密码全员共享，无法区分用户，已移除。
// supportedSSMethods 允许作为入站的 Shadowsocks 加密方式。
//
// 只保留两种 AES：sing-box 的 SS2022 多用户（EIH）仅支持 AES 系，源码里写死
//     Shadowsocks 2022 EIH support only available in AES ciphers
// chacha20-poly1305 加上 users 会 FATAL invalid argument，只能退化为「单一
// 密码多人共享」——所有用户密码相同、无法区分用户，auth_user 分流也永远
// 只命中同一个名字。这种形同虚设的多用户没有存在意义，直接从入站移除。
//
// 落地出站仍可选 chacha20：那是连别人的服务端，加密方式由对方决定。
var supportedSSMethods = map[string]bool{
	"2022-blake3-aes-128-gcm": true,
	"2022-blake3-aes-256-gcm": true,
}

// isSupportedSSMethod 判断加密方式是否受支持
func isSupportedSSMethod(method string) bool {
	return supportedSSMethods[method]
}

// ss2022KeyLen 返回 SS2022 method 要求的密钥字节数；非 SS2022 返回 0
func ss2022KeyLen(method string) int {
	switch method {
	case "2022-blake3-aes-128-gcm":
		return 16
	case "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305":
		return 32
	}
	return 0
}

// ss2022KeyMatchesMethod 判断密码字节长度是否与 method 匹配。
// 传统加密方式（aes-256-gcm 等）不校验长度，恒为 true。
//
// 不匹配时客户端会直接崩：mihomo 报
//   cipher: 2022-blake3-aes-128-gcm initialize error: bad key length, required 16, got 32
// 而 sing-box 同样拒绝启动，且面板侧从订阅文本上看不出任何异常。
func ss2022KeyMatchesMethod(password, method string) bool {
	want := ss2022KeyLen(method)
	if want == 0 {
		return true
	}
	raw, err := base64.StdEncoding.DecodeString(password)
	if err != nil {
		return false
	}
	return len(raw) == want
}

// regenerateSS2022PasswordIfMismatch 在 method 变更导致密钥长度不匹配时重新生成主密钥。
// 返回是否发生了重新生成。
func regenerateSS2022PasswordIfMismatch(password *string, method string) bool {
	if ss2022KeyMatchesMethod(*password, method) {
		return false
	}
	*password = generateSS2022Password(method)
	return true
}

// generateSS2022Password 生成 Shadowsocks 2022 格式的密码（base64 编码的随机字节）
func generateSS2022Password(method string) string {
	var keyLen int
	switch method {
	case "2022-blake3-aes-128-gcm":
		keyLen = 16
	case "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305":
		keyLen = 32
	default:
		// 非 2022 方法，生成普通随机密码
		return generateRandomPassword(32)
	}

	bytes := make([]byte, keyLen)
	rand.Read(bytes)
	return base64.StdEncoding.EncodeToString(bytes)
}
