package freeswitch

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ai_dialer_mini/internal/clients/ws"
)

// FSWSConfig FreeSWITCH WebSocket客户端配置
type FSWSConfig struct {
	URL      string
	Password string
}

// FSWSClient FreeSWITCH WebSocket客户端
type FSWSClient struct {
	*ws.Client
	handlers map[string]FSEventHandler
}

// FSEventHandler FreeSWITCH事件处理函数类型
type FSEventHandler func(event map[string]interface{}) error

// NewFSWSClient 创建新的FreeSWITCH WebSocket客户端
func NewFSWSClient(config FSWSConfig) *FSWSClient {
	wsConfig := ws.Config{
		URL: config.URL,
		Headers: map[string]string{
			"Password": config.Password,
		},
		ReconnectInterval: 5 * time.Second,
		MaxRetries:       3,
		HeartbeatInterval: 30 * time.Second,
		HeartbeatMessage: []byte("ping"),
	}

	client := &FSWSClient{
		Client:   ws.NewClient(wsConfig),
		handlers: make(map[string]FSEventHandler),
	}

	// 注册默认消息处理器
	client.Client.RegisterHandler("event", func(message []byte) error {
		// 解析事件
		var event map[string]interface{}
		if err := json.Unmarshal(message, &event); err != nil {
			return fmt.Errorf("解析事件失败: %v", err)
		}

		// 获取事件名称
		eventName, ok := event["Event-Name"].(string)
		if !ok {
			return fmt.Errorf("事件名称无效")
		}

		// 调用对应的处理器
		if handler, ok := client.handlers[eventName]; ok {
			if err := handler(event); err != nil {
				log.Printf("处理事件失败: %v", err)
			}
		}
		return nil
	})

	return client
}

// RegisterHandler 注册事件处理器
func (c *FSWSClient) RegisterHandler(eventName string, handler FSEventHandler) {
	c.handlers[eventName] = handler
}

// UnregisterHandler 注销事件处理器
func (c *FSWSClient) UnregisterHandler(eventName string) {
	delete(c.handlers, eventName)
}

// SendCommand 发送命令
func (c *FSWSClient) SendCommand(command string) error {
	return c.Client.SendMessage(command)
}

// SendEvent 发送事件
func (c *FSWSClient) SendEvent(event map[string]interface{}) error {
	return c.Client.SendMessage(event)
}
