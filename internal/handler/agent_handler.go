package handler

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/sboard-go/sboard/internal/agent"
	"github.com/sboard-go/sboard/internal/database"
	"gorm.io/gorm"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// 允许无 Origin 头的请求（agent 直连）
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		// 仅允许 panel 自身域名的 WebSocket 连接
		host := r.Host
		if host == "" {
			return false
		}
		return origin == "http://"+host || origin == "https://"+host
	},
}

// AgentConnection Agent 连接
// agentAliveWindow 判定 Agent 在线的时间窗。
// 心跳 2 秒一次，取 15 秒可容错 7 次丢失。判据用「距最后一次心跳的时长」，
// 而不是「12 秒内攒够 4 次心跳」——后者会让刚连上的 agent 在前 8 秒被判离线，
// 把启动毛刺和真正掉线混为一谈。
const agentAliveWindow = 15 * time.Second

type AgentConnection struct {
	ID            string
	Conn          *websocket.Conn
	ServerID      uint
	RegisterTime  time.Time
	LastHeartbeat time.Time
	Status        *agent.StatusData
	mu            sync.RWMutex

	// writeMu 串行化 WebSocket 写操作。
	// gorilla/websocket 只允许一个并发写者，并发调用 WriteJSON 会直接 panic
	// （concurrent write to websocket connection）并拖垮整个面板进程。
	// 面板侧有多条路径会写同一连接：配置推送、命令下发、自更新通知。
	writeMu sync.Mutex
}

// Send 串行化地发送消息到 Agent。
// 所有写操作必须走这个方法，禁止直接调用 Conn.WriteJSON。
func (c *AgentConnection) Send(v interface{}) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.Conn == nil {
		return fmt.Errorf("连接已关闭")
	}
	// 写超时：对端不读（半开连接、网络黑洞）时不能让写操作永久阻塞，
	// 否则会把这个 goroutine 连同写锁一起挂死。
	_ = c.Conn.SetWriteDeadline(time.Now().Add(agentWriteWait))
	return c.Conn.WriteJSON(v)
}

// IsAlive 判断 Agent 是否存活：距最后一次心跳不超过 agentAliveWindow。
func (c *AgentConnection) IsAlive() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.Conn == nil {
		return false
	}
	if c.LastHeartbeat.IsZero() {
		return false
	}
	return time.Since(c.LastHeartbeat) <= agentAliveWindow
}

// AgentHub Agent 连接管理器
type AgentHub struct {
	agents         map[string]*AgentConnection // agentID -> connection
	serverMap      map[uint]*AgentConnection   // serverID -> connection
	mu             sync.RWMutex
	msgChan        chan *AgentMessage
	pendingReq     map[string]chan *agent.Message // msgID -> response channel
	pendingMu      sync.RWMutex
	syncDebounce   *time.Timer // 配置同步防抖定时器
	syncDebounceMu sync.Mutex

	// pendingMetrics 暂存每台服务器最新一次心跳指标，由后台定时器批量落库。
	// 心跳是 2 秒一次，若每次心跳都写 SQLite（一次 SELECT 加两到三次 UPDATE）
	// 会形成持续的写放大与锁竞争；实时展示走内存，落库只需保持足够新即可。
	pendingMetrics map[uint]*agent.HeartbeatData
	metricsMu      sync.Mutex
}

// AgentMessage 带连接的消息
type AgentMessage struct {
	Conn    *AgentConnection
	Message *agent.Message
}

var agentHub = &AgentHub{
	agents:     make(map[string]*AgentConnection),
	serverMap:  make(map[uint]*AgentConnection),
	msgChan:    make(chan *AgentMessage, 100),
	pendingReq: make(map[string]chan *agent.Message),

	pendingMetrics: make(map[uint]*agent.HeartbeatData),
}

// metricsFlushInterval 心跳指标落库间隔。
const metricsFlushInterval = 30 * time.Second

// InitAgentHub starts the agent hub message pump. Called from NewServer.
func InitAgentHub() {
	go agentHub.run()
	go agentHub.metricsFlushLoop()
}

func (h *AgentHub) run() {
	for msg := range h.msgChan {
		h.handleMessage(msg)
	}
}

func (h *AgentHub) handleMessage(am *AgentMessage) {
	msg := am.Message
	conn := am.Conn

	switch msg.Type {
	case agent.MsgTypeRegister:
		h.handleRegister(conn, msg)
	case agent.MsgTypeHeartbeat:
		h.handleHeartbeat(conn, msg)
	case agent.MsgTypeStatus:
		h.handleStatus(conn, msg)
	case agent.MsgTypeCommandResp:
		h.handleCommandResp(msg)
	}
}

func (h *AgentHub) handleRegister(conn *AgentConnection, msg *agent.Message) {
	var data agent.RegisterData
	if err := msg.ParseData(&data); err != nil {
		return
	}

	// 验证 Token 并查找服务器
	var server database.Server
	if err := database.GetDB().Where("agent_token = ?", data.Token).First(&server).Error; err != nil {
		conn.Send(&agent.Message{
			Type:  agent.MsgTypeRegisterResp,
			Error: "认证失败",
		})
		conn.Conn.Close()
		return
	}

	// 更新连接信息
	conn.ID = data.AgentID
	conn.ServerID = server.ID
	conn.RegisterTime = time.Now()
	conn.LastHeartbeat = time.Now()

	// 注册到 hub
	h.mu.Lock()
	h.agents[data.AgentID] = conn
	h.serverMap[server.ID] = conn
	h.mu.Unlock()

	// 清除该服务器的已推送配置哈希：新连接上的 Agent 可能已经丢了配置
	// （重启/换机/首次注册），若沿用旧哈希会被判定「无变化」而永不推送。
	pushedConfigHashMu.Lock()
	delete(pushedConfigHash, server.ID)
	pushedConfigHashMu.Unlock()

	// 更新服务器状态
	updates := map[string]interface{}{
		"agent_online":   true,
		"agent_id":       data.AgentID,
		"agent_version":  data.Version,
		"last_heartbeat": time.Now(),
	}
	// 使用 Agent 上报的本机 IP (从网卡获取) 更新服务器 host
	if data.LocalIP != "" {
		updates["host"] = data.LocalIP
	}
	if data.LocalIPv6 != "" {
		updates["host_ipv6"] = data.LocalIPv6
	}
	database.GetDB().Model(&server).Updates(updates)

	// 发送注册成功响应
	conn.Send(&agent.Message{
		Type:      agent.MsgTypeRegisterResp,
		Timestamp: time.Now().Unix(),
	})
}

func (h *AgentHub) handleHeartbeat(conn *AgentConnection, msg *agent.Message) {
	var data agent.HeartbeatData
	if err := msg.ParseData(&data); err != nil {
		return
	}

	now := time.Now()
	conn.mu.Lock()
	conn.LastHeartbeat = now
	if conn.Status == nil {
		conn.Status = &agent.StatusData{}
	}
	conn.Status.HeartbeatData = data
	conn.mu.Unlock()

	// 只暂存到内存，由 metricsFlushLoop 定期批量落库。
	// 实时指标（CPU/内存/磁盘/网速）由 /servers/status 直接读内存，
	// 不依赖这里写库的频率。
	if conn.ServerID > 0 {
		cp := data
		h.metricsMu.Lock()
		h.pendingMetrics[conn.ServerID] = &cp
		h.metricsMu.Unlock()
	}
}

// metricsFlushLoop 定期把内存里的心跳指标写回数据库。
func (h *AgentHub) metricsFlushLoop() {
	ticker := time.NewTicker(metricsFlushInterval)
	defer ticker.Stop()
	for range ticker.C {
		h.flushAllMetrics()
	}
}

// flushAllMetrics 取走并落库全部待写入指标。
func (h *AgentHub) flushAllMetrics() {
	h.metricsMu.Lock()
	pending := h.pendingMetrics
	h.pendingMetrics = make(map[uint]*agent.HeartbeatData, len(pending))
	h.metricsMu.Unlock()

	for serverID, data := range pending {
		h.flushServerMetrics(serverID, data)
	}
}

// flushServerMetrics 落库单台服务器的心跳指标并累加月度流量。
func (h *AgentHub) flushServerMetrics(serverID uint, data *agent.HeartbeatData) {
	if serverID == 0 || data == nil {
		return
	}

	// 先取上次落库的 transfer 值，用于计算增量
	var server database.Server
	if err := database.GetDB().Select("last_net_in_transfer", "last_net_out_transfer").
		Where("id = ?", serverID).First(&server).Error; err != nil {
		return
	}

	// 计算差值（仅当新值大于旧值时累加，避免服务器重启导致负值）
	var deltaIn, deltaOut uint64
	if data.NetInTransfer > server.LastNetInTransfer && server.LastNetInTransfer > 0 {
		deltaIn = data.NetInTransfer - server.LastNetInTransfer
	}
	if data.NetOutTransfer > server.LastNetOutTransfer && server.LastNetOutTransfer > 0 {
		deltaOut = data.NetOutTransfer - server.LastNetOutTransfer
	}

	updates := map[string]interface{}{
		"last_heartbeat":        time.Now(),
		"cpu_usage":             data.CPUPercent,
		"mem_usage":             data.MemPercent,
		"disk_usage":            data.DiskPercent,
		"net_in":                data.NetIn,
		"net_out":               data.NetOut,
		"last_net_in_transfer":  data.NetInTransfer,
		"last_net_out_transfer": data.NetOutTransfer,
	}
	if data.LocalIP != "" {
		updates["host"] = data.LocalIP
	}
	if data.LocalIPv6 != "" {
		updates["host_ipv6"] = data.LocalIPv6
	}
	database.GetDB().Model(&database.Server{}).Where("id = ?", serverID).Updates(updates)

	// 累加月度流量（使用精确差值）
	if deltaIn > 0 || deltaOut > 0 {
		database.GetDB().Model(&database.Server{}).Where("id = ?", serverID).
			UpdateColumn("monthly_in", gorm.Expr("monthly_in + ?", deltaIn)).
			UpdateColumn("monthly_out", gorm.Expr("monthly_out + ?", deltaOut))
	}
}

// flushServerMetricsNow 立即落库某台服务器的待写指标（断连时收尾用）
func (h *AgentHub) flushServerMetricsNow(serverID uint) {
	h.metricsMu.Lock()
	data := h.pendingMetrics[serverID]
	delete(h.pendingMetrics, serverID)
	h.metricsMu.Unlock()
	h.flushServerMetrics(serverID, data)
}

func (h *AgentHub) handleStatus(conn *AgentConnection, msg *agent.Message) {
	var data agent.StatusData
	if err := msg.ParseData(&data); err != nil {
		return
	}

	conn.mu.Lock()
	conn.Status = &data
	conn.mu.Unlock()
}

func (h *AgentHub) handleCommandResp(msg *agent.Message) {
	if msg.ID == "" {
		return
	}

	h.pendingMu.RLock()
	ch, ok := h.pendingReq[msg.ID]
	h.pendingMu.RUnlock()

	if ok {
		select {
		case ch <- msg:
		default:
		}
	}
}

// SendCommand 发送命令到 Agent（等待响应）
func (h *AgentHub) SendCommand(serverID uint, msg *agent.Message, timeout time.Duration) (*agent.Message, error) {
	h.mu.RLock()
	conn, ok := h.serverMap[serverID]
	h.mu.RUnlock()

	if !ok || conn == nil {
		return nil, &AgentOfflineError{ServerID: serverID}
	}

	// 生成消息 ID
	msg.ID = generateMsgID()
	msg.Timestamp = time.Now().Unix()

	// 创建响应通道
	respChan := make(chan *agent.Message, 1)
	h.pendingMu.Lock()
	h.pendingReq[msg.ID] = respChan
	h.pendingMu.Unlock()

	defer func() {
		h.pendingMu.Lock()
		delete(h.pendingReq, msg.ID)
		h.pendingMu.Unlock()
	}()

	// 发送消息
	if err := conn.Send(msg); err != nil {
		return nil, err
	}

	// 等待响应
	select {
	case resp := <-respChan:
		return resp, nil
	case <-time.After(timeout):
		return nil, &TimeoutError{MsgID: msg.ID}
	}
}

// SendNotify 发送单向通知到 Agent（不等待响应）
func (h *AgentHub) SendNotify(serverID uint, msg *agent.Message) error {
	h.mu.RLock()
	conn, ok := h.serverMap[serverID]
	h.mu.RUnlock()

	if !ok || conn == nil {
		return &AgentOfflineError{ServerID: serverID}
	}

	msg.ID = generateMsgID()
	msg.Timestamp = time.Now().Unix()

	return conn.Send(msg)
}

// IsAgentOnline 检查 Agent 是否在线（12秒内心跳达到4次）
func (h *AgentHub) IsAgentOnline(serverID uint) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	conn, ok := h.serverMap[serverID]
	return ok && conn != nil && conn.IsAlive()
}

// GetAgentStatus 获取 Agent 状态
func (h *AgentHub) GetAgentStatus(serverID uint) *agent.StatusData {
	h.mu.RLock()
	conn, ok := h.serverMap[serverID]
	h.mu.RUnlock()

	if !ok || conn == nil {
		return nil
	}

	// 返回副本：调用方在锁外读取字段，直接暴露共享指针会与心跳写入构成数据竞争
	conn.mu.RLock()
	defer conn.mu.RUnlock()
	if conn.Status == nil {
		return nil
	}
	cp := *conn.Status
	return &cp
}

// RemoveAgent 移除 Agent
func (h *AgentHub) RemoveAgent(agentID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if conn, ok := h.agents[agentID]; ok {
		if conn.ServerID > 0 {
			delete(h.serverMap, conn.ServerID)
			// 断连前把最后一批指标落库，避免丢掉最后一个刷新周期内的流量统计
			h.flushServerMetricsNow(conn.ServerID)
			// 更新服务器离线状态
			database.GetDB().Model(&database.Server{}).Where("id = ?", conn.ServerID).Update("agent_online", false)
		}
		delete(h.agents, agentID)
		conn.Conn.Close()
	}
}

// GetOnlineServers 获取所有在线的服务器 ID
func (h *AgentHub) GetOnlineServers() []uint {
	h.mu.RLock()
	defer h.mu.RUnlock()

	serverIDs := make([]uint, 0, len(h.serverMap))
	for serverID := range h.serverMap {
		serverIDs = append(serverIDs, serverID)
	}
	return serverIDs
}

// BroadcastConfigUpdate 向所有存活的 Agent 并发广播配置更新（单向通知，不等待响应）
// 使用 3 秒防抖：连续多次调用只触发最后一次，避免批量修改时重复推送配置
// 存活判断：12秒内心跳达到4次
// 完全异步执行，不阻塞调用者
func (h *AgentHub) BroadcastConfigUpdate() {
	h.syncDebounceMu.Lock()
	if h.syncDebounce != nil {
		h.syncDebounce.Reset(3 * time.Second)
		h.syncDebounceMu.Unlock()
		return
	}
	h.syncDebounce = time.AfterFunc(3*time.Second, func() {
		h.syncDebounceMu.Lock()
		h.syncDebounce = nil
		h.syncDebounceMu.Unlock()

		h.doSyncConfig()
	})
	h.syncDebounceMu.Unlock()
}

// doSyncConfig 实际执行配置同步（由防抖定时器触发）
func (h *AgentHub) doSyncConfig() {
	// 第二道保险：通用配置指纹对比（除非强制推送，如服务器落地出站变化）
	configPushForceMu.Lock()
	force := configPushForce
	configPushForce = false
	configPushForceMu.Unlock()

	if !force && !configFingerprintChanged() {
		return // 通用配置未变且非强制推送，跳过
	}

	// 快速收集存活的 Agent 连接信息，尽快释放锁
	h.mu.RLock()
	aliveAgents := make([]struct {
		ServerID uint
		Conn     *AgentConnection
	}, 0, len(h.serverMap))

	for serverID, conn := range h.serverMap {
		if conn != nil && conn.IsAlive() {
			aliveAgents = append(aliveAgents, struct {
				ServerID uint
				Conn     *AgentConnection
			}{serverID, conn})
		}
	}
	h.mu.RUnlock()

	if len(aliveAgents) == 0 {
		return
	}

	// 每个 Agent 独立的 goroutine 处理，完全异步
	for _, agentInfo := range aliveAgents {
		go func(serverID uint, conn *AgentConnection) {
			// 数据库查询在 goroutine 中执行，不阻塞主流程
			var server database.Server
			if err := database.GetDB().First(&server, serverID).Error; err != nil {
				return
			}

			// 生成配置
			config, err := GenerateServerConfig(&server, "sing-box")
			if err != nil {
				return
			}

			// 按服务器比对配置指纹：内容没变就不推送，避免无谓地重启该服务器的
			// sing-box（重启会掐断该节点上所有用户的连接）。全局指纹只能说明
			// 「某个地方变了」，不能说明这台服务器变了。
			sum := fmt.Sprintf("%x", sha256.Sum256([]byte(config)))
			pushedConfigHashMu.Lock()
			unchanged := pushedConfigHash[serverID] == sum
			pushedConfigHashMu.Unlock()
			if unchanged && !force {
				return
			}

			// 构造消息
			data := &agent.SyncConfigData{
				ConfigType: "sing-box",
				Content:    config,
				Restart:    true,
				TargetPath: "/opt/sboard/sing-box",
			}
			rawData, _ := json.Marshal(data)

			msg := &agent.Message{
				ID:        generateMsgID(),
				Type:      agent.MsgTypeSyncConfig,
				Data:      rawData,
				Timestamp: time.Now().Unix(),
			}

			// 单向发送，不等待响应（Send 内部有写锁）
			if err := conn.Send(msg); err != nil {
				return
			}
			pushedConfigHashMu.Lock()
			pushedConfigHash[serverID] = sum
			pushedConfigHashMu.Unlock()
		}(agentInfo.ServerID, agentInfo.Conn)
	}
}

// BroadcastConfigUpdate 包级别导出函数，供外部调用
func BroadcastConfigUpdate() {
	agentHub.BroadcastConfigUpdate()
}

var (
	// 每台服务器最近一次成功推送的配置哈希，用于跳过无变化的推送
	pushedConfigHash   = make(map[uint]string)
	pushedConfigHashMu sync.Mutex

	configPushForce   bool
	configPushForceMu sync.Mutex
)

// BroadcastConfigUpdateForce 强制推送配置（用于服务器落地出站等不改变通用配置的变化）
func BroadcastConfigUpdateForce() {
	configPushForceMu.Lock()
	configPushForce = true
	configPushForceMu.Unlock()
	agentHub.BroadcastConfigUpdate()
}

// AgentOfflineError Agent 离线错误
type AgentOfflineError struct {
	ServerID uint
}

func (e *AgentOfflineError) Error() string {
	return "Agent 离线"
}

// TimeoutError 超时错误
type TimeoutError struct {
	MsgID string
}

func (e *TimeoutError) Error() string {
	return "请求超时"
}

// Agent 连接保活参数。
// 此前两侧都没有 ping/pong 与读超时：面板被 kill 或 NAT 静默断链时，
// agent 会永久阻塞在 ReadJSON 上不会重连，面板侧也会残留永不回收的
// 僵尸连接（fd + goroutine 泄漏）。
const (
	agentPongWait  = 30 * time.Second // 允许的最久无入站数据
	agentPingEvery = 10 * time.Second // 必须显著小于 agentPongWait
	agentWriteWait = 10 * time.Second
	agentReadLimit = 4 << 20 // 单帧上限，防超大消息
)

// handleAgentWebSocket 处理 Agent WebSocket 连接
func (s *Server) handleAgentWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	agentConn := &AgentConnection{
		Conn: conn,
	}

	conn.SetReadLimit(agentReadLimit)
	_ = conn.SetReadDeadline(time.Now().Add(agentPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(agentPongWait))
	})

	// 面板主动 ping；写控制帧同样要走写锁，避免与业务消息并发写
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(agentPingEvery)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				agentConn.writeMu.Lock()
				_ = conn.SetWriteDeadline(time.Now().Add(agentWriteWait))
				err := conn.WriteMessage(websocket.PingMessage, nil)
				agentConn.writeMu.Unlock()
				if err != nil {
					return
				}
			}
		}
	}()

	// 读取消息循环
	for {
		var msg agent.Message
		if err := conn.ReadJSON(&msg); err != nil {
			break
		}
		// 收到任何入站数据都视为活跃（agent 每 2 秒发心跳）
		_ = conn.SetReadDeadline(time.Now().Add(agentPongWait))

		agentHub.msgChan <- &AgentMessage{
			Conn:    agentConn,
			Message: &msg,
		}
	}

	// 清理连接
	if agentConn.ID != "" {
		agentHub.RemoveAgent(agentConn.ID)
	}
}

// handleGetAgentStatus 获取 Agent 状态
func (s *Server) handleGetAgentStatus(c *gin.Context) {
	serverID := parseUint(c.Param("id"))
	if serverID == 0 {
		errorJSON(c, http.StatusBadRequest, "无效的服务器ID")
		return
	}

	online := agentHub.IsAgentOnline(uint(serverID))
	status := agentHub.GetAgentStatus(uint(serverID))

	successJSON(c, gin.H{
		"online": online,
		"status": status,
	})
}

// handleSendAgentCommand 发送命令到 Agent
func (s *Server) handleSendAgentCommand(c *gin.Context) {
	serverID := parseUint(c.Param("id"))
	if serverID == 0 {
		errorJSON(c, http.StatusBadRequest, "无效的服务器ID")
		return
	}

	var req struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
		Timeout int      `json:"timeout"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errorJSON(c, http.StatusBadRequest, "请求参数错误")
		return
	}

	data := &agent.CommandData{
		Command: req.Command,
		Args:    req.Args,
		Timeout: req.Timeout,
	}
	rawData, _ := json.Marshal(data)

	msg := &agent.Message{
		Type: agent.MsgTypeCommand,
		Data: rawData,
	}

	timeout := time.Duration(req.Timeout) * time.Second
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	resp, err := agentHub.SendCommand(uint(serverID), msg, timeout)
	if err != nil {
		if _, ok := err.(*AgentOfflineError); ok {
			errorJSON(c, http.StatusServiceUnavailable, "Agent 离线")
			return
		}
		errorJSON(c, http.StatusGatewayTimeout, err.Error())
		return
	}

	if resp.Error != "" {
		errorJSON(c, http.StatusInternalServerError, resp.Error)
		return
	}

	var respData agent.CommandRespData
	resp.ParseData(&respData)

	successJSON(c, respData)
}

// handleSyncConfigToAgent 同步配置到 Agent
func (s *Server) handleSyncConfigToAgent(c *gin.Context) {
	serverID := parseUint(c.Param("id"))
	if serverID == 0 {
		errorJSON(c, http.StatusBadRequest, "无效的服务器ID")
		return
	}

	var req struct {
		ConfigType string `json:"config_type"`
		Restart    bool   `json:"restart"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errorJSON(c, http.StatusBadRequest, "请求参数错误")
		return
	}

	// 获取服务器
	var server database.Server
	if err := database.GetDB().First(&server, serverID).Error; err != nil {
		errorJSON(c, http.StatusNotFound, "服务器不存在")
		return
	}

	// 生成配置
	configType := req.ConfigType
	if configType == "" {
		configType = "sing-box"
	}

	config, err := GenerateServerConfig(&server, configType)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "生成配置失败: "+err.Error())
		return
	}

	// 发送同步命令
	data := &agent.SyncConfigData{
		ConfigType: configType,
		Content:    config,
		Restart:    req.Restart,
		TargetPath: "/opt/sboard/sing-box",
	}
	rawData, _ := json.Marshal(data)

	msg := &agent.Message{
		Type: agent.MsgTypeSyncConfig,
		Data: rawData,
	}

	resp, err := agentHub.SendCommand(uint(serverID), msg, 60*time.Second)
	if err != nil {
		if _, ok := err.(*AgentOfflineError); ok {
			errorJSON(c, http.StatusServiceUnavailable, "Agent 离线")
			return
		}
		errorJSON(c, http.StatusGatewayTimeout, err.Error())
		return
	}

	if resp.Error != "" {
		errorJSON(c, http.StatusInternalServerError, resp.Error)
		return
	}

	successMsgJSON(c, "配置同步成功")
}

// handleDeployCoreToAgent 部署核心到 Agent
func (s *Server) handleDeployCoreToAgent(c *gin.Context) {
	serverID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errorJSON(c, http.StatusBadRequest, "无效的服务器ID")
		return
	}

	var req struct {
		TargetPath string `json:"target_path"` // 目标安装路径
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errorJSON(c, http.StatusBadRequest, "请求参数错误")
		return
	}

	// 获取服务器
	var server database.Server
	if err := database.GetDB().First(&server, serverID).Error; err != nil {
		errorJSON(c, http.StatusNotFound, "服务器不存在")
		return
	}

	// 目标路径，默认 /opt/sboard/sing-box
	targetPath := req.TargetPath
	if targetPath == "" {
		targetPath = "/opt/sboard/sing-box"
	}

	// 生成配置
	config, err := GenerateServerConfig(&server, "sing-box")
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "生成配置失败: "+err.Error())
		return
	}

	// 发送部署命令
	data := &agent.DeployCoreData{
		CoreType:   "sing-box",
		TargetPath: targetPath,
		Config:     config,
	}
	rawData, _ := json.Marshal(data)

	msg := &agent.Message{
		Type: agent.MsgTypeDeployCore,
		Data: rawData,
	}

	resp, err := agentHub.SendCommand(uint(serverID), msg, 5*time.Second)
	if err != nil {
		if _, ok := err.(*AgentOfflineError); ok {
			errorJSON(c, http.StatusServiceUnavailable, "Agent 离线")
			return
		}
		errorJSON(c, http.StatusGatewayTimeout, err.Error())
		return
	}

	if resp.Error != "" {
		errorJSON(c, http.StatusInternalServerError, resp.Error)
		return
	}

	// 解析响应
	var respData agent.DeployCoreRespData
	if err := resp.ParseData(&respData); err != nil {
		errorJSON(c, http.StatusInternalServerError, "解析响应失败")
		return
	}

	successJSON(c, respData)
}

// 辅助函数
func parseUint(s string) uint64 {
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func generateMsgID() string {
	now := time.Now()
	return fmt.Sprintf("%s-%06d", now.Format("20060102150405"), now.Nanosecond()/1000)
}

// handleDeployAll 全部部署（向所有存活 Agent 发送部署核心指令）
// 异步执行，立即返回，不阻塞面板
func (s *Server) handleDeployAll(c *gin.Context) {
	// 获取所有启用的服务器
	var servers []database.Server
	if err := database.GetDB().Where("enabled = ?", true).Find(&servers).Error; err != nil {
		errorJSON(c, http.StatusInternalServerError, "获取服务器列表失败")
		return
	}

	if len(servers) == 0 {
		errorJSON(c, http.StatusBadRequest, "没有启用的服务器")
		return
	}

	// 收集存活的 Agent
	type aliveServer struct {
		Server database.Server
		Conn   *websocket.Conn
	}
	aliveServers := []aliveServer{}

	agentHub.mu.RLock()
	for _, server := range servers {
		if conn, ok := agentHub.serverMap[server.ID]; ok && conn != nil && conn.IsAlive() {
			aliveServers = append(aliveServers, aliveServer{
				Server: server,
				Conn:   conn.Conn,
			})
		}
	}
	agentHub.mu.RUnlock()

	if len(aliveServers) == 0 {
		errorJSON(c, http.StatusBadRequest, "没有存活的 Agent")
		return
	}

	// 立即返回，后台异步执行部署（最多 8 并发）
	go func(aliveServers []aliveServer) {
		sem := make(chan struct{}, 8) // 并发限制
		var wg sync.WaitGroup

		for _, srv := range aliveServers {
			wg.Add(1)
			sem <- struct{}{}
			go func(srv aliveServer) {
				defer wg.Done()
				defer func() { <-sem }()

				// 生成配置
				config, err := GenerateServerConfig(&srv.Server, "sing-box")
				if err != nil {
					return
				}

				// 发送部署核心指令
				deployCoreData := &agent.DeployCoreData{
					CoreType:   "sing-box",
					TargetPath: "/opt/sboard/sing-box",
					Config:     config,
				}
				rawData, _ := json.Marshal(deployCoreData)
				deployCoreMsg := &agent.Message{
					ID:        generateMsgID(),
					Type:      agent.MsgTypeDeployCore,
					Data:      rawData,
					Timestamp: time.Now().Unix(),
				}

				agentHub.SendCommand(srv.Server.ID, deployCoreMsg, 5*time.Second)
			}(srv)
		}
		wg.Wait()
	}(aliveServers)

	successJSON(c, gin.H{
		"message": "部署任务已启动",
		"total":   len(aliveServers),
	})
}

// handleUpdateAgents 更新所有 Agent（向所有存活 Agent 发送自我更新指令）
func (s *Server) handleUpdateAgents(c *gin.Context) {
	// 获取所有启用的服务器
	var servers []database.Server
	if err := database.GetDB().Where("enabled = ?", true).Find(&servers).Error; err != nil {
		errorJSON(c, http.StatusInternalServerError, "获取服务器列表失败")
		return
	}

	if len(servers) == 0 {
		errorJSON(c, http.StatusBadRequest, "没有启用的服务器")
		return
	}

	// 收集存活的 Agent
	// 先在读锁内收集连接，释放锁后再发送：
	// 持锁做网络写会阻塞整个 hub 的注册/心跳处理。
	var targets []*AgentConnection
	agentHub.mu.RLock()
	for _, server := range servers {
		if conn, ok := agentHub.serverMap[server.ID]; ok && conn != nil && conn.IsAlive() {
			targets = append(targets, conn)
		}
	}
	agentHub.mu.RUnlock()

	aliveCount := 0
	for _, conn := range targets {
		selfUpdateMsg := &agent.Message{
			ID:        generateMsgID(),
			Type:      agent.MsgTypeSelfUpdate,
			Timestamp: time.Now().Unix(),
		}
		if err := conn.Send(selfUpdateMsg); err == nil {
			aliveCount++
		}
	}

	if aliveCount == 0 {
		errorJSON(c, http.StatusBadRequest, "没有存活的 Agent")
		return
	}

	successJSON(c, gin.H{
		"message": "Agent 更新指令已发送",
		"total":   aliveCount,
	})
}
