// Package ws WebSocket服务包
package ws

import (
	"sync"

	"ai_dialer_mini/internal/types"
	"github.com/gorilla/websocket"
)

// WSMessage WebSocket消息实现
type WSMessage struct {
	msgType types.MessageType
	data    interface{}
}

func (m *WSMessage) Type() types.MessageType {
	return m.msgType
}

func (m *WSMessage) Data() interface{} {
	return m.data
}

// WSSession WebSocket会话实现
type WSSession struct {
	sync.RWMutex
	id      string
	conn    *websocket.Conn
	handler types.MessageHandler
}

func (s *WSSession) ID() string {
	return s.id
}

func (s *WSSession) Send(msg types.Message) error {
	s.RLock()
	defer s.RUnlock()
	return s.conn.WriteJSON(msg)
}

func (s *WSSession) Close() error {
	s.Lock()
	defer s.Unlock()
	return s.conn.Close()
}

// Config WebSocket配置
type Config struct {
	Host             string            // 服务器主机
	Port             int               // 服务器端口
	Path             string            // WebSocket路径
	HandshakeTimeout int               // 握手超时时间(秒)
	WriteTimeout     int               // 写超时时间(秒)
	ReadTimeout      int               // 读超时时间(秒)
	PingInterval     int               // 心跳间隔(秒)
	BufferSize       int               // 缓冲区大小
	EnableTLS        bool              // 是否启用TLS
	CertFile         string            // TLS证书文件
	KeyFile          string            // TLS密钥文件
	Headers          map[string]string // 自定义头部
	MessageHandler   types.MessageHandler // 消息处理器
}

// MessageType 消息类型
type MessageType = types.MessageType

const (
	WSTextMessage   MessageType = types.WSTextMessage   // 文本消息
	WSBinaryMessage MessageType = types.WSBinaryMessage // 二进制消息
)

// Message WebSocket消息
type Message = types.Message

// Session WebSocket会话
type Session = types.Session

// MessageHandler 消息处理器
type MessageHandler = types.MessageHandler

// NewConfig 创建默认WebSocket配置
func NewConfig() *Config {
	return &Config{
		Host:             "0.0.0.0",
		Port:             8080,
		Path:             "/ws",
		HandshakeTimeout: 10,
		WriteTimeout:     10,
		ReadTimeout:      10,
		PingInterval:     10,
		BufferSize:       4096,
		EnableTLS:        false,
		CertFile:         "",
		KeyFile:          "",
		Headers:          nil,
		MessageHandler:   nil,
	}
}
