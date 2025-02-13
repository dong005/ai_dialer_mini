package handlers

import (
	"log"
	"net/http"
	"sync"

	"ai_dialer_mini/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// ASRHandler WebSocket ASR 处理器
type ASRHandler struct {
	wsService  models.WSService
	clients    map[*websocket.Conn]string // WebSocket连接到会话ID的映射
	clientsMux sync.Mutex
}

// NewASRHandler 创建新的 ASR 处理器实例
func NewASRHandler(wsService models.WSService) *ASRHandler {
	return &ASRHandler{
		wsService: wsService,
		clients:   make(map[*websocket.Conn]string),
	}
}

// HandleWebSocket 处理 WebSocket 连接
func (h *ASRHandler) HandleWebSocket(c *gin.Context) {
	// 升级 HTTP 连接为 WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("升级 WebSocket 连接失败: %v", err)
		return
	}

	// 生成会话ID
	sessionID := c.Query("session_id")
	if sessionID == "" {
		sessionID = "default" // 如果没有提供会话ID，使用默认值
	}

	// 注册客户端
	h.clientsMux.Lock()
	h.clients[conn] = sessionID
	h.clientsMux.Unlock()

	// 处理连接关闭
	defer func() {
		h.clientsMux.Lock()
		delete(h.clients, conn)
		h.clientsMux.Unlock()
		conn.Close()
	}()

	// 处理 WebSocket 消息
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("读取 WebSocket 消息错误: %v", err)
			}
			break
		}

		// 处理消息
		if err := h.handleMessage(conn, messageType, message); err != nil {
			log.Printf("处理消息失败: %v", err)
		}
	}
}

// handleMessage 处理 WebSocket 消息
func (h *ASRHandler) handleMessage(conn *websocket.Conn, messageType int, message []byte) error {
	h.clientsMux.Lock()
	sessionID := h.clients[conn]
	h.clientsMux.Unlock()

	switch messageType {
	case websocket.BinaryMessage:
		// 处理音频数据
		result, err := h.wsService.ProcessAudio(sessionID, message)
		if err != nil {
			return err
		}

		// 发送识别结果
		response := map[string]interface{}{
			"type": "result",
			"text": result,
		}
		return conn.WriteJSON(response)

	case websocket.TextMessage:
		// 处理文本命令，如清除历史记录等
		response := map[string]interface{}{
			"type":   "error",
			"error": "暂不支持文本命令",
		}
		return conn.WriteJSON(response)
	}

	return nil
}

// RegisterRoutes 注册路由
func (h *ASRHandler) RegisterRoutes(r *gin.Engine) {
	r.GET("/ws/asr", h.HandleWebSocket)
}
