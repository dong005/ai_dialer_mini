// Package ws 提供WebSocket相关功能的实现
package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"ai_dialer_mini/internal/clients/xfyun"
	"ai_dialer_mini/internal/config"
	"ai_dialer_mini/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// ASRResponse 定义语音识别结果的响应结构
type ASRResponse struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
	IsEnd      bool    `json:"is_end"`
	AIReply    string  `json:"ai_reply,omitempty"` // AI的回复，只在最终结果时返回
}

// ASRGrammar 定义语法设置请求的结构
type ASRGrammar struct {
	Grammar string `json:"grammar"`
}

// AudioData 音频数据结构
type AudioData struct {
	Data   []byte `json:"data"`
	Format string `json:"format"`
	IsEnd  bool   `json:"is_end"`
}

// ASRServer 处理语音识别的WebSocket服务器
type ASRServer struct {
	Config       *config.Config
	Upgrader     websocket.Upgrader
	Mu           sync.Mutex
	Grammars     map[*websocket.Conn]string
	LastActivity map[*websocket.Conn]time.Time
	ASRClient    *xfyun.ASRClient
	DialogSvc    models.DialogService
}

// NewASRServer 创建新的ASR服务器实例
func NewASRServer(cfg *config.Config, dialogSvc models.DialogService) *ASRServer {
	if cfg == nil {
		cfg = config.GetConfig()
	}

	server := &ASRServer{
		Config: cfg,
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				log.Printf("检查WebSocket连接来源: %s, Origin: %s", r.RemoteAddr, origin)
				return true // 在生产环境中应该实现适当的源检查
			},
			HandshakeTimeout: 10 * time.Second,
			ReadBufferSize:   cfg.WebSocket.ReadBufferSize,
			WriteBufferSize:  cfg.WebSocket.WriteBufferSize,
			Subprotocols:     []string{"WSBRIDGE"},
		},
		Grammars:     make(map[*websocket.Conn]string),
		LastActivity: make(map[*websocket.Conn]time.Time),
		ASRClient:    xfyun.NewASRClient(cfg.XFYun, dialogSvc),
		DialogSvc:    dialogSvc,
	}

	// 启动心跳检查
	go server.heartbeatChecker()

	return server
}

// heartbeatChecker 定期检查连接活跃状态
func (s *ASRServer) heartbeatChecker() {
	ticker := time.NewTicker(s.Config.WebSocket.PingPeriod)
	defer ticker.Stop()

	for range ticker.C {
		s.Mu.Lock()
		now := time.Now()
		for conn, lastActivity := range s.LastActivity {
			if now.Sub(lastActivity) > s.Config.WebSocket.PongWait {
				log.Printf("连接超时，关闭连接: %s", conn.RemoteAddr().String())
				conn.Close()
				delete(s.LastActivity, conn)
				delete(s.Grammars, conn)
			}
		}
		s.Mu.Unlock()
	}
}

// updateActivity 更新连接的最后活动时间
func (s *ASRServer) updateActivity(conn *websocket.Conn) {
	s.Mu.Lock()
	s.LastActivity[conn] = time.Now()
	s.Mu.Unlock()
}

// ServeHTTP 处理WebSocket连接
func (s *ASRServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 检查必要的头信息
	if !s.checkWebSocketHeaders(r) {
		http.Error(w, "无效的WebSocket请求", http.StatusBadRequest)
		return
	}

	// 升级HTTP连接为WebSocket连接
	conn, err := s.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("升级WebSocket连接失败: %v", err)
		return
	}
	defer conn.Close()

	// 记录连接活动时间
	s.updateActivity(conn)

	// 设置连接属性
	conn.SetReadLimit(1024 * 1024) // 1MB
	conn.SetReadDeadline(time.Now().Add(s.Config.WebSocket.PongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(s.Config.WebSocket.PongWait))
		s.updateActivity(conn)
		return nil
	})

	// 获取会话ID
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		sessionID = "default"
	}

	// 处理WebSocket消息
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("读取WebSocket消息失败: %v", err)
			}
			break
		}

		// 更新连接活动时间
		s.updateActivity(conn)

		// 处理不同类型的消息
		switch messageType {
		case websocket.TextMessage:
			// 尝试解析为语法设置请求
			var grammar ASRGrammar
			if err := json.Unmarshal(message, &grammar); err == nil {
				s.Mu.Lock()
				s.Grammars[conn] = grammar.Grammar
				s.Mu.Unlock()
				continue
			}

			// 尝试解析为音频数据
			var audioData AudioData
			if err := json.Unmarshal(message, &audioData); err == nil {
				// 处理音频数据
				result, err := s.ASRClient.ProcessAudio(sessionID, audioData.Data)
				if err != nil {
					log.Printf("处理音频失败: %v", err)
					continue
				}

				// 发送识别结果
				response := ASRResponse{
					Text:  result,
					IsEnd: audioData.IsEnd,
				}

				if err := conn.WriteJSON(response); err != nil {
					log.Printf("发送识别结果失败: %v", err)
					break
				}
			}

		case websocket.BinaryMessage:
			// 直接处理二进制音频数据
			result, err := s.ASRClient.ProcessAudio(sessionID, message)
			if err != nil {
				log.Printf("处理音频失败: %v", err)
				continue
			}

			// 发送识别结果
			response := ASRResponse{
				Text:  result,
				IsEnd: false,
			}

			if err := conn.WriteJSON(response); err != nil {
				log.Printf("发送响应失败: %v", err)
				break
			}
		}
	}
}

// checkWebSocketHeaders 检查WebSocket必要的头信息
func (s *ASRServer) checkWebSocketHeaders(r *http.Request) bool {
	// 检查Upgrade头
	if !strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") {
		return false
	}
	if strings.ToLower(r.Header.Get("Upgrade")) != "websocket" {
		return false
	}

	// 检查WebSocket版本
	if r.Header.Get("Sec-WebSocket-Version") != "13" {
		return false
	}

	// 检查WebSocket密钥
	if r.Header.Get("Sec-WebSocket-Key") == "" {
		return false
	}

	return true
}

// processAudio 处理音频数据
func (s *ASRServer) processAudio(data []byte, format string) (string, float64) {
	// TODO: 实现音频处理逻辑
	return "", 0.0
}

// HandleConnection 处理WebSocket连接
func (s *ASRServer) HandleConnection(c *gin.Context) {
	// 升级HTTP连接为WebSocket
	conn, err := s.Upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("升级WebSocket连接失败: %v", err)
		return
	}
	defer conn.Close()

	// 初始化连接
	s.Mu.Lock()
	s.LastActivity[conn] = time.Now()
	s.Mu.Unlock()

	// 处理连接关闭
	defer func() {
		s.Mu.Lock()
		delete(s.LastActivity, conn)
		delete(s.Grammars, conn)
		s.Mu.Unlock()
	}()

	// 设置连接配置
	conn.SetReadLimit(int64(s.Config.WebSocket.ReadBufferSize))
	conn.SetReadDeadline(time.Now().Add(s.Config.WebSocket.PongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(s.Config.WebSocket.PongWait))
		return nil
	})

	// 处理消息
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("读取WebSocket消息错误: %v", err)
			}
			break
		}

		// 更新活动时间
		s.updateActivity(conn)

		// 处理消息
		switch messageType {
		case websocket.BinaryMessage:
			// 处理音频数据
			text, confidence := s.processAudio(message, "pcm")
			response := ASRResponse{
				Text:       text,
				Confidence: confidence,
				IsEnd:      false,
			}
			
			// 如果有文本结果，发送给对话服务处理
			if text != "" {
				aiReply, err := s.DialogSvc.ProcessMessage("default", text)
				if err != nil {
					log.Printf("处理对话失败: %v", err)
				} else {
					response.AIReply = aiReply
					response.IsEnd = true
				}
			}

			if err := conn.WriteJSON(response); err != nil {
				log.Printf("发送响应失败: %v", err)
				return
			}

		case websocket.TextMessage:
			// 处理文本消息（如语法设置）
			var grammar ASRGrammar
			if err := json.Unmarshal(message, &grammar); err == nil && grammar.Grammar != "" {
				s.Mu.Lock()
				s.Grammars[conn] = grammar.Grammar
				s.Mu.Unlock()
			}
		}
	}
}

// ProcessAudio 处理音频数据
func (s *ASRServer) ProcessAudio(sessionID string, data []byte) (string, error) {
	text, _ := s.processAudio(data, "pcm")
	if text == "" {
		return "", nil
	}
	return text, nil
}
