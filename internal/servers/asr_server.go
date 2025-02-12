package servers

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// ASRResponse 定义语音识别结果的响应结构
type ASRResponse struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
}

// ASRGrammar 定义语法设置请求的结构
type ASRGrammar struct {
	Grammar string `json:"grammar"`
}

// ASRServer 处理语音识别的WebSocket服务器
type ASRServer struct {
	upgrader websocket.Upgrader
	// 保护并发访问的互斥锁
	mu sync.Mutex
	// 存储每个连接的语法设置
	grammars map[*websocket.Conn]string
}

// NewASRServer 创建新的ASR服务器实例
func NewASRServer() *ASRServer {
	return &ASRServer{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // 在生产环境中应该实现适当的源检查
			},
		},
		grammars: make(map[*websocket.Conn]string),
	}
}

// ServeHTTP 处理WebSocket连接
func (s *ASRServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("无法升级到WebSocket连接: %v", err)
		http.Error(w, "无法升级到WebSocket连接", http.StatusInternalServerError)
		return
	}
	defer func() {
		s.mu.Lock()
		delete(s.grammars, conn)
		s.mu.Unlock()
		conn.Close()
	}()

	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("读取消息错误: %v", err)
			}
			break
		}

		switch messageType {
		case websocket.TextMessage:
			// 处理文本消息（如语法设置）
			var grammar ASRGrammar
			if err := json.Unmarshal(message, &grammar); err == nil && grammar.Grammar != "" {
				s.mu.Lock()
				s.grammars[conn] = grammar.Grammar
				s.mu.Unlock()
				log.Printf("设置语法: %s", grammar.Grammar)
			}

		case websocket.BinaryMessage:
			// 处理音频数据
			// TODO: 实现实际的语音识别逻辑
			// 这里模拟一个识别结果
			response := ASRResponse{
				Text:       "测试识别结果",
				Confidence: 87.3,
			}

			// 将结果转换为JSON并发送
			if jsonData, err := json.Marshal(response); err == nil {
				if err := conn.WriteMessage(websocket.TextMessage, jsonData); err != nil {
					log.Printf("发送识别结果错误: %v", err)
					break
				}
			}
		}
	}
}
