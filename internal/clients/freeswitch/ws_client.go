// Package freeswitch 实现与FreeSWITCH的客户端通信
package freeswitch

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
)

// Client FreeSWITCH 客户端
type Client struct {
	host     string
	port     int
	password string
	conn     net.Conn
	reader   *bufio.Reader
	mu       sync.Mutex
	handlers map[string]EventHandler
	running  bool
}

// Config FreeSWITCH 客户端配置
type Config struct {
	Host     string
	Port     int
	Password string
}

// EventHandler 事件处理函数类型
type EventHandler func(headers map[string]string) error

// NewClient 创建新的FreeSWITCH客户端
func NewClient(config Config) *Client {
	return &Client{
		host:     config.Host,
		port:     config.Port,
		password: config.Password,
		handlers: make(map[string]EventHandler),
	}
}

// Connect 连接到FreeSWITCH服务器并进行认证
func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 建立TCP连接
	addr := fmt.Sprintf("%s:%d", c.host, c.port)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("connect error: %v", err)
	}

	c.conn = conn
	c.reader = bufio.NewReader(conn)

	// 读取欢迎信息
	headers, err := c.readHeaders()
	if err != nil {
		c.conn.Close()
		return fmt.Errorf("read welcome message error: %v", err)
	}

	// 验证是否是认证请求
	if headers["Content-Type"] != "auth/request" {
		c.conn.Close()
		return fmt.Errorf("unexpected content type: %s", headers["Content-Type"])
	}

	// 发送认证
	authCmd := fmt.Sprintf("auth %s\n\n", c.password)
	if _, err := c.conn.Write([]byte(authCmd)); err != nil {
		c.conn.Close()
		return fmt.Errorf("send auth command error: %v", err)
	}

	// 读取认证响应
	headers, err = c.readHeaders()
	if err != nil {
		c.conn.Close()
		return fmt.Errorf("read auth response error: %v", err)
	}

	if !strings.Contains(headers["Reply-Text"], "+OK accepted") {
		c.conn.Close()
		return fmt.Errorf("authentication failed: %s", headers["Reply-Text"])
	}

	fmt.Println("认证成功，连接已建立")

	// 启动事件读取循环
	go c.readEventLoop()

	return nil
}

// Close 关闭连接
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.running = false
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// SubscribeEvents 订阅事件
func (c *Client) SubscribeEvents() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	// 订阅所有事件
	cmd := "event plain all\n\n"
	if _, err := c.conn.Write([]byte(cmd)); err != nil {
		return fmt.Errorf("subscribe events error: %v", err)
	}

	// 读取订阅响应
	headers, err := c.readHeaders()
	if err != nil {
		return fmt.Errorf("read subscribe response error: %v", err)
	}

	if !strings.Contains(headers["Reply-Text"], "+OK") {
		return fmt.Errorf("subscribe failed: %s", headers["Reply-Text"])
	}

	fmt.Println("事件订阅成功")
	return nil
}

// RegisterHandler 注册事件处理器
func (c *Client) RegisterHandler(eventName string, handler EventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers[eventName] = handler
}

// SendCommand 发送命令到FreeSWITCH
func (c *Client) SendCommand(command string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return "", fmt.Errorf("not connected")
	}

	// 发送命令
	if _, err := c.conn.Write([]byte(command + "\n\n")); err != nil {
		return "", fmt.Errorf("send command error: %v", err)
	}

	// 读取响应
	headers, err := c.readHeaders()
	if err != nil {
		return "", fmt.Errorf("read command response error: %v", err)
	}

	return headers["Reply-Text"], nil
}

// readHeaders 读取ESL头部
func (c *Client) readHeaders() (map[string]string, error) {
	headers := make(map[string]string)
	for {
		line, err := c.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if idx := strings.Index(line, ": "); idx != -1 {
			key := line[:idx]
			value := line[idx+2:]
			headers[key] = value
		}
	}
	return headers, nil
}

// readEventLoop 读取事件循环
func (c *Client) readEventLoop() {
	c.running = true
	fmt.Println("开始事件读取循环")

	for c.running {
		// 读取事件头部
		headers, err := c.readHeaders()
		if err != nil {
			fmt.Printf("读取事件头部错误: %v\n", err)
			break
		}

		// 如果有Content-Length，读取事件体
		if lenStr, ok := headers["Content-Length"]; ok {
			var contentLength int
			fmt.Sscanf(lenStr, "%d", &contentLength)
			if contentLength > 0 {
				body := make([]byte, contentLength)
				_, err := c.reader.Read(body)
				if err != nil {
					fmt.Printf("读取事件体错误: %v\n", err)
					break
				}
				// 将事件体解析为头部
				for _, line := range strings.Split(string(body), "\n") {
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}
					if idx := strings.Index(line, ": "); idx != -1 {
						key := line[:idx]
						value := line[idx+2:]
						headers[key] = value
					}
				}
			}
		}

		// 处理事件
		go c.handleEvent(headers)
	}

	fmt.Println("事件读取循环结束")
}

// handleEvent 处理单个事件
func (c *Client) handleEvent(headers map[string]string) {
	// 如果有事件名称，调用对应的处理器
	if eventName, ok := headers["Event-Name"]; ok {
		c.mu.Lock()
		handler, exists := c.handlers[eventName]
		c.mu.Unlock()

		if exists {
			if err := handler(headers); err != nil {
				fmt.Printf("事件处理错误: %v\n", err)
			} else {
				fmt.Printf("成功处理事件: %s\n", eventName)
			}
		}
	}
}
