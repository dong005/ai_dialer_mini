package ws

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// Client WebSocket客户端
type Client struct {
	config *Config
	conn   *websocket.Conn
	done   chan struct{}
}

// NewClient 创建新的WebSocket客户端
func NewClient(config *Config) *Client {
	return &Client{
		config: config,
		done:   make(chan struct{}),
	}
}

// Connect 连接到WebSocket服务器
func (c *Client) Connect() error {
	// 构建URL
	url := fmt.Sprintf("ws://%s:%d%s", c.config.Host, c.config.Port, c.config.Path)
	if c.config.EnableTLS {
		url = fmt.Sprintf("wss://%s:%d%s", c.config.Host, c.config.Port, c.config.Path)
	}

	// 设置拨号器选项
	dialer := websocket.Dialer{
		HandshakeTimeout: time.Duration(c.config.HandshakeTimeout) * time.Second,
		ReadBufferSize:   c.config.BufferSize,
		WriteBufferSize:  c.config.BufferSize,
	}

	// 设置请求头
	header := http.Header{}
	if c.config.Headers != nil {
		for k, v := range c.config.Headers {
			header.Set(k, v)
		}
	}

	// 建立连接
	conn, _, err := dialer.Dial(url, header)
	if err != nil {
		return fmt.Errorf("连接WebSocket服务器失败: %v", err)
	}

	c.conn = conn
	return nil
}

// Close 关闭连接
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Send 发送消息
func (c *Client) Send(msgType MessageType, data interface{}) error {
	if c.conn == nil {
		return fmt.Errorf("WebSocket未连接")
	}

	msg := &WSMessage{
		msgType: msgType,
		data:    data,
	}

	if c.config.MessageHandler != nil {
		if err := c.config.MessageHandler.HandleMessage(nil, msg); err != nil {
			return fmt.Errorf("处理消息失败: %v", err)
		}
	}

	return c.conn.WriteJSON(msg)
}
