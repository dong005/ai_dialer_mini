// Package ws 提供通用的WebSocket客户端实现
package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Client WebSocket客户端基类
type Client struct {
	// WebSocket连接配置
	url      string
	headers  map[string]string
	conn     *websocket.Conn
	connLock sync.Mutex

	// 重连控制
	reconnectInterval time.Duration
	maxRetries       int
	currentRetries   int

	// 心跳控制
	heartbeatInterval time.Duration
	heartbeatMessage []byte
	lastPong        time.Time
	heartbeatTimer  *time.Timer

	// 消息处理
	handlers map[string]MessageHandler
	ctx      context.Context
	cancel   context.CancelFunc
}

// MessageHandler 消息处理函数类型
type MessageHandler func(message []byte) error

// Config WebSocket客户端配置
type Config struct {
	URL               string            // WebSocket服务器地址
	Headers           map[string]string // 自定义请求头
	ReconnectInterval time.Duration     // 重连间隔
	MaxRetries        int              // 最大重试次数
	HeartbeatInterval time.Duration    // 心跳间隔
	HeartbeatMessage  []byte           // 心跳消息内容
}

// NewClient 创建新的WebSocket客户端
func NewClient(config Config) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		url:               config.URL,
		headers:           config.Headers,
		reconnectInterval: config.ReconnectInterval,
		maxRetries:        config.MaxRetries,
		heartbeatInterval: config.HeartbeatInterval,
		heartbeatMessage:  config.HeartbeatMessage,
		handlers:          make(map[string]MessageHandler),
		ctx:              ctx,
		cancel:           cancel,
	}
}

// Connect 连接到WebSocket服务器
func (c *Client) Connect() error {
	c.connLock.Lock()
	defer c.connLock.Unlock()

	log.Printf("正在连接WebSocket服务器: %s\n", c.url)

	// 解析URL
	u, err := url.Parse(c.url)
	if err != nil {
		return fmt.Errorf("解析URL失败: %v", err)
	}

	// 建立连接
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("连接WebSocket失败: %v", err)
	}

	c.conn = conn
	c.currentRetries = 0
	c.lastPong = time.Now()

	// 设置Pong处理函数
	c.conn.SetPongHandler(func(string) error {
		c.lastPong = time.Now()
		return nil
	})

	// 启动心跳
	if c.heartbeatInterval > 0 {
		c.startHeartbeat()
	}

	// 启动接收循环
	go c.receiveLoop()

	return nil
}

// Close 关闭WebSocket连接
func (c *Client) Close() error {
	c.cancel()
	c.stopHeartbeat()

	c.connLock.Lock()
	defer c.connLock.Unlock()

	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// RegisterHandler 注册消息处理器
func (c *Client) RegisterHandler(messageType string, handler MessageHandler) {
	c.handlers[messageType] = handler
}

// SendMessage 发送消息到服务器
func (c *Client) SendMessage(message interface{}) error {
	c.connLock.Lock()
	defer c.connLock.Unlock()

	if c.conn == nil {
		return fmt.Errorf("WebSocket连接未建立")
	}

	// 将消息转换为JSON
	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("消息序列化失败: %v", err)
	}

	// 发送消息
	if err := c.conn.WriteMessage(websocket.TextMessage, jsonData); err != nil {
		go c.handleConnectionError()
		return fmt.Errorf("发送消息失败: %v", err)
	}

	return nil
}

// startHeartbeat 启动心跳
func (c *Client) startHeartbeat() {
	c.stopHeartbeat()

	c.heartbeatTimer = time.NewTimer(c.heartbeatInterval)
	go func() {
		for {
			select {
			case <-c.ctx.Done():
				return
			case <-c.heartbeatTimer.C:
				if err := c.sendHeartbeat(); err != nil {
					log.Printf("发送心跳失败: %v\n", err)
					go c.handleConnectionError()
					return
				}
				// 重置定时器
				c.heartbeatTimer.Reset(c.heartbeatInterval)
			}
		}
	}()
}

// stopHeartbeat 停止心跳
func (c *Client) stopHeartbeat() {
	if c.heartbeatTimer != nil {
		c.heartbeatTimer.Stop()
		c.heartbeatTimer = nil
	}
}

// sendHeartbeat 发送心跳消息
func (c *Client) sendHeartbeat() error {
	c.connLock.Lock()
	defer c.connLock.Unlock()

	if c.conn == nil {
		return fmt.Errorf("WebSocket连接未建立")
	}

	return c.conn.WriteMessage(websocket.PingMessage, c.heartbeatMessage)
}

// receiveLoop 接收消息循环
func (c *Client) receiveLoop() {
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			if err := c.receiveMessage(); err != nil {
				log.Printf("接收消息失败: %v\n", err)
				go c.handleConnectionError()
				return
			}
		}
	}
}

// receiveMessage 接收单条消息
func (c *Client) receiveMessage() error {
	c.connLock.Lock()
	if c.conn == nil {
		c.connLock.Unlock()
		return fmt.Errorf("WebSocket连接未建立")
	}
	c.connLock.Unlock()

	_, message, err := c.conn.ReadMessage()
	if err != nil {
		return err
	}

	// 解析消息类型
	var jsonMessage map[string]interface{}
	if err := json.Unmarshal(message, &jsonMessage); err != nil {
		return fmt.Errorf("解析消息失败: %v", err)
	}

	// 获取消息类型
	messageType, ok := jsonMessage["type"].(string)
	if !ok {
		return fmt.Errorf("消息类型无效")
	}

	// 调用对应的处理器
	if handler, exists := c.handlers[messageType]; exists {
		return handler(message)
	}

	return nil
}

// handleConnectionError 处理连接错误
func (c *Client) handleConnectionError() {
	c.connLock.Lock()
	defer c.connLock.Unlock()

	// 关闭现有连接
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}

	// 检查重试次数
	if c.maxRetries > 0 && c.currentRetries >= c.maxRetries {
		log.Printf("达到最大重试次数，停止重连\n")
		return
	}

	// 增加重试计数
	c.currentRetries++

	// 等待重连间隔
	time.Sleep(c.reconnectInterval)

	// 尝试重新连接
	go func() {
		if err := c.Connect(); err != nil {
			log.Printf("重连失败: %v\n", err)
		} else {
			log.Printf("重连成功\n")
		}
	}()
}
