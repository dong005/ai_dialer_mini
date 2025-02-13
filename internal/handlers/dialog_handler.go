package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"ai_dialer_mini/internal/clients/ollama"
	"ai_dialer_mini/internal/clients/xfyun"
	"ai_dialer_mini/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// DialogHandler 对话处理器
type DialogHandler struct {
	asrClient    *xfyun.ASRClient
	ollamaClient *ollama.Client
	upgrader     websocket.Upgrader
	sessions     map[string]*DialogSession
	mu           sync.RWMutex
}

// DialogSession 对话会话
type DialogSession struct {
	ID           string
	WSConn       *websocket.Conn
	ASRClient    *xfyun.ASRClient
	OllamaClient *ollama.Client
	mu           sync.Mutex
}

// NewDialogHandler 创建对话处理器
func NewDialogHandler(asrConfig xfyun.Config, ollamaConfig ollama.Config) *DialogHandler {
	return &DialogHandler{
		asrClient:    xfyun.NewASRClient(asrConfig, nil),
		ollamaClient: ollama.NewClient(ollamaConfig),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		sessions: make(map[string]*DialogSession),
	}
}

// HandleWebSocket 处理WebSocket连接
func (h *DialogHandler) HandleWebSocket(c *gin.Context) {
	// 升级HTTP连接为WebSocket
	ws, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("升级WebSocket连接失败: %v", err)
		return
	}

	// 创建新的会话
	sessionID := c.Query("session_id")
	if sessionID == "" {
		sessionID = "default"
	}

	session := &DialogSession{
		ID:           sessionID,
		WSConn:       ws,
		ASRClient:    h.asrClient,
		OllamaClient: h.ollamaClient,
	}

	// 保存会话
	h.mu.Lock()
	h.sessions[sessionID] = session
	h.mu.Unlock()

	// 处理WebSocket消息
	go h.handleSession(session)
}

// handleSession 处理会话消息
func (h *DialogHandler) handleSession(session *DialogSession) {
	defer func() {
		session.WSConn.Close()
		h.mu.Lock()
		delete(h.sessions, session.ID)
		h.mu.Unlock()
	}()

	// 处理音频数据
	for {
		messageType, data, err := session.WSConn.ReadMessage()
		if err != nil {
			log.Printf("读取WebSocket消息失败: %v", err)
			return
		}

		// 处理二进制音频数据
		if messageType == websocket.BinaryMessage {
			// 发送音频数据到ASR服务
			result, err := session.ASRClient.ProcessAudio(session.ID, data)
			if err != nil {
				log.Printf("处理音频失败: %v", err)
				continue
			}

			// 发送ASR结果给Ollama
			ollamaResp, err := session.OllamaClient.Generate(result, ollama.Options{
				Temperature: 0.7,
				TopP:       0.9,
				TopK:       40,
				MaxTokens:  2000,
			})
			if err != nil {
				log.Printf("生成回复失败: %v", err)
				continue
			}

			// 构建响应
			response := models.DialogResponse{
				Type:     "text",
				Content:  ollamaResp.Response,
				SessionID: session.ID,
			}

			// 发送响应给客户端
			responseJSON, err := json.Marshal(response)
			if err != nil {
				log.Printf("序列化响应失败: %v", err)
				continue
			}

			session.mu.Lock()
			err = session.WSConn.WriteMessage(websocket.TextMessage, responseJSON)
			session.mu.Unlock()
			if err != nil {
				log.Printf("发送响应失败: %v", err)
				return
			}
		}
	}
}
