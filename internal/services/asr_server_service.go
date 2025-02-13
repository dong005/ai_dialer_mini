package services

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

// ASRServerService 处理语音识别的WebSocket服务器
type ASRServerService struct {
	upgrader websocket.Upgrader
	mu       sync.Mutex
	grammars map[*websocket.Conn]string
}

// NewASRServerService 创建新的ASR服务器实例
func NewASRServerService() *ASRServerService {
	return &ASRServerService{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		grammars: make(map[*websocket.Conn]string),
	}
}

// ServeHTTP 处理WebSocket连接
func (s *ASRServerService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("升级WebSocket连接失败: %v", err)
		return
	}
	defer conn.Close()

	for {
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			log.Printf("读取消息失败: %v", err)
			s.mu.Lock()
			delete(s.grammars, conn)
			s.mu.Unlock()
			return
		}

		// 处理语法设置请求
		if messageType == websocket.TextMessage {
			var grammar ASRGrammar
			if err := json.Unmarshal(p, &grammar); err != nil {
				log.Printf("解析语法设置失败: %v", err)
				continue
			}

			s.mu.Lock()
			s.grammars[conn] = grammar.Grammar
			s.mu.Unlock()
			continue
		}

		// 处理音频数据
		if messageType == websocket.BinaryMessage {
			// TODO: 实现音频处理逻辑
			response := ASRResponse{
				Text:       "识别结果示例",
				Confidence: 0.95,
			}

			responseJSON, err := json.Marshal(response)
			if err != nil {
				log.Printf("序列化响应失败: %v", err)
				continue
			}

			if err := conn.WriteMessage(websocket.TextMessage, responseJSON); err != nil {
				log.Printf("发送响应失败: %v", err)
				return
			}
		}
	}
}
