package services

import (
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// WSService WebSocket服务
type WSService struct {
	clients    map[*websocket.Conn]bool
	broadcast  chan []byte
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	mu         sync.RWMutex
}

// NewWSService 创建新的WebSocket服务实例
func NewWSService() *WSService {
	return &WSService{
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan []byte),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
	}
}

// Run 启动WebSocket服务
func (s *WSService) Run() {
	for {
		select {
		case client := <-s.register:
			s.mu.Lock()
			s.clients[client] = true
			s.mu.Unlock()

		case client := <-s.unregister:
			s.mu.Lock()
			if _, ok := s.clients[client]; ok {
				delete(s.clients, client)
				client.Close()
			}
			s.mu.Unlock()

		case message := <-s.broadcast:
			s.mu.RLock()
			for client := range s.clients {
				if err := client.WriteMessage(websocket.TextMessage, message); err != nil {
					log.Printf("发送消息失败: %v", err)
					client.Close()
					delete(s.clients, client)
				}
			}
			s.mu.RUnlock()
		}
	}
}

// HandleConnection 处理WebSocket连接
func (s *WSService) HandleConnection(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("升级WebSocket连接失败: %v", err)
		return
	}

	// 注册新客户端
	s.register <- conn

	// 处理连接关闭
	defer func() {
		s.unregister <- conn
	}()

	// 读取消息
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("读取消息错误: %v", err)
			}
			break
		}

		// 广播消息
		s.broadcast <- message
	}
}

// Broadcast 广播消息给所有客户端
func (s *WSService) Broadcast(message []byte) {
	s.broadcast <- message
}
