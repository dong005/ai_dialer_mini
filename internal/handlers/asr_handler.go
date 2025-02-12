package handlers

import (
	"log"
	"net/http"
	"sync"

	"ai_dialer_mini/internal/clients/asr"

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
	clients    map[*websocket.Conn]*asr.XunfeiClient
	clientsMux sync.Mutex
}

// NewASRHandler 创建新的 ASR 处理器实例
func NewASRHandler() *ASRHandler {
	return &ASRHandler{
		clients: make(map[*websocket.Conn]*asr.XunfeiClient),
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

	// 初始化讯飞 ASR 客户端
	xfClient := h.createXunfeiClient(conn)
	if xfClient == nil {
		conn.Close()
		return
	}

	// 注册客户端
	h.clientsMux.Lock()
	h.clients[conn] = xfClient
	h.clientsMux.Unlock()

	// 处理连接关闭
	defer func() {
		h.clientsMux.Lock()
		if client, ok := h.clients[conn]; ok {
			client.Close()
			delete(h.clients, conn)
		}
		h.clientsMux.Unlock()
		conn.Close()
	}()

	// 处理 WebSocket 消息
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("读取 WebSocket 消息错误: %v", err)
			}
			break
		}

		// 处理消息
		if err := h.handleMessage(conn, message); err != nil {
			log.Printf("处理消息失败: %v", err)
		}
	}
}

// createXunfeiClient 创建讯飞 ASR 客户端
func (h *ASRHandler) createXunfeiClient(conn *websocket.Conn) *asr.XunfeiClient {
	config := asr.Config{
		AppID:     "c0de4f24",
		APIKey:    "51012a35448538a8396dc564cf050f68",
		APISecret: "NWRhZDBkNzA5ZDQxNGMzYmQ1NWMwMWNh",
		HostURL:   "wss://iat-api.xfyun.cn/v2/iat",
	}

	client := asr.NewXunfeiClient(config)

	// 设置识别结果回调
	client.SetResultCallback(func(text string, isLast bool) error {
		response := map[string]interface{}{
			"type": "result",
			"text": text,
		}
		return conn.WriteJSON(response)
	})

	// 连接讯飞 ASR 服务
	if err := client.Connect(); err != nil {
		log.Printf("连接讯飞 ASR 服务失败: %v", err)
		return nil
	}

	return client
}

// handleMessage 处理 WebSocket 消息
func (h *ASRHandler) handleMessage(conn *websocket.Conn, message []byte) error {
	// 获取客户端
	h.clientsMux.Lock()
	client, ok := h.clients[conn]
	h.clientsMux.Unlock()
	if !ok {
		return nil
	}

	// 发送音频帧给 ASR 客户端
	return client.SendAudioFrame(message)
}

// RegisterRoutes 注册路由
func (h *ASRHandler) RegisterRoutes(r *gin.Engine) {
	r.GET("/asr", h.HandleWebSocket)
}
