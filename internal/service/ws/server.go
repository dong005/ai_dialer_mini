package ws

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// WebSocketService WebSocket服务
type WebSocketService struct {
	sync.RWMutex
	config    *Config
	upgrader  websocket.Upgrader
	sessions  map[string]*WSSession
	done      chan struct{}
}

// NewWebSocketService 创建新的WebSocket服务
func NewWebSocketService(config *Config) *WebSocketService {
	if config == nil {
		config = &Config{
			Host:             "localhost",
			Port:             8080,
			Path:             "/ws",
			HandshakeTimeout: 10,
			WriteTimeout:     10,
			ReadTimeout:      10,
			PingInterval:     30,
			BufferSize:       4096,
		}
	}

	service := &WebSocketService{
		config: config,
		upgrader: websocket.Upgrader{
			HandshakeTimeout: time.Duration(config.HandshakeTimeout) * time.Second,
			ReadBufferSize:   config.BufferSize,
			WriteBufferSize:  config.BufferSize,
			CheckOrigin: func(r *http.Request) bool {
				return true // 允许所有来源
			},
		},
		sessions: make(map[string]*WSSession),
		done:     make(chan struct{}),
	}

	return service
}

// Start 启动WebSocket服务
func (s *WebSocketService) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	http.HandleFunc(s.config.Path, s.handleWebSocket)

	server := &http.Server{
		Addr:              addr,
		ReadTimeout:       time.Duration(s.config.ReadTimeout) * time.Second,
		WriteTimeout:      time.Duration(s.config.WriteTimeout) * time.Second,
		ReadHeaderTimeout: time.Duration(s.config.HandshakeTimeout) * time.Second,
	}

	go func() {
		var err error
		if s.config.EnableTLS {
			err = server.ListenAndServeTLS(s.config.CertFile, s.config.KeyFile)
		} else {
			err = server.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			fmt.Printf("WebSocket服务器错误: %v\n", err)
		}
	}()

	return nil
}

// Stop 停止WebSocket服务
func (s *WebSocketService) Stop() {
	close(s.done)
	s.Lock()
	defer s.Unlock()
	for _, session := range s.sessions {
		session.Close()
	}
}

// handleWebSocket 处理WebSocket连接
func (s *WebSocketService) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Printf("升级WebSocket连接失败: %v\n", err)
		return
	}

	sessionID := uuid.New().String()
	session := &WSSession{
		id:      sessionID,
		conn:    conn,
		handler: s.config.MessageHandler,
	}

	s.Lock()
	s.sessions[sessionID] = session
	s.Unlock()

	go s.handleSession(session)
}

// handleSession 处理WebSocket会话
func (s *WebSocketService) handleSession(session *WSSession) {
	defer func() {
		s.Lock()
		delete(s.sessions, session.id)
		s.Unlock()
		session.Close()
	}()

	for {
		select {
		case <-s.done:
			return
		default:
			var msg WSMessage
			if err := session.conn.ReadJSON(&msg); err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					fmt.Printf("读取消息错误: %v\n", err)
				}
				return
			}

			if session.handler != nil {
				if err := session.handler.HandleMessage(session, &msg); err != nil {
					fmt.Printf("处理消息错误: %v\n", err)
				}
			}
		}
	}
}
