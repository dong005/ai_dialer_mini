package freeswitch

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
)

// ESLConfig ESL客户端配置
type ESLConfig struct {
	Host     string
	Port     int
	Password string
}

// ESLClient ESL客户端
type ESLClient struct {
	config   ESLConfig
	conn     net.Conn
	reader   *bufio.Reader
	handlers map[string]EventHandler
	mu       sync.RWMutex
	running  bool
}

// EventHandler 事件处理函数类型
type EventHandler func(headers map[string]string) error

// NewESLClient 创建新的ESL客户端
func NewESLClient(config ESLConfig) *ESLClient {
	return &ESLClient{
		config:   config,
		handlers: make(map[string]EventHandler),
		running:  false,
	}
}

// NewESLClientWithDefaultConfig 创建新的ESL客户端，使用默认配置
func NewESLClientWithDefaultConfig() *ESLClient {
	return NewESLClient(ESLConfig{
		Host:     "127.0.0.1",
		Port:     8021,
		Password: "ClueCon",
	})
}

// Connect 连接到FreeSWITCH
func (c *ESLClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 建立TCP连接
	addr := fmt.Sprintf("%s:%d", c.config.Host, c.config.Port)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("连接失败: %v", err)
	}

	c.conn = conn
	c.reader = bufio.NewReader(conn)

	// 读取欢迎信息
	headers, err := c.readHeaders()
	if err != nil {
		c.conn.Close()
		return fmt.Errorf("读取欢迎信息失败: %v", err)
	}

	// 验证是否是认证请求
	if headers["Content-Type"] != "auth/request" {
		c.conn.Close()
		return fmt.Errorf("未收到认证请求: %s", headers["Content-Type"])
	}

	// 发送认证
	authCmd := fmt.Sprintf("auth %s\n\n", c.config.Password)
	if _, err := c.conn.Write([]byte(authCmd)); err != nil {
		c.conn.Close()
		return fmt.Errorf("发送认证失败: %v", err)
	}

	// 读取认证响应
	headers, err = c.readHeaders()
	if err != nil {
		c.conn.Close()
		return fmt.Errorf("读取认证响应失败: %v", err)
	}

	if !strings.Contains(headers["Reply-Text"], "+OK accepted") {
		c.conn.Close()
		return fmt.Errorf("认证失败: %s", headers["Reply-Text"])
	}

	log.Println("认证成功，连接已建立")

	// 启动事件读取循环
	go c.readEventLoop()

	return nil
}

// Close 关闭连接
func (c *ESLClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.running = false
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// SubscribeEvents 订阅事件
func (c *ESLClient) SubscribeEvents() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("未连接")
	}

	// 订阅所有事件
	cmd := "event plain all\n\n"
	if _, err := c.conn.Write([]byte(cmd)); err != nil {
		return fmt.Errorf("订阅事件失败: %v", err)
	}

	// 读取订阅响应
	headers, err := c.readHeaders()
	if err != nil {
		return fmt.Errorf("读取订阅响应失败: %v", err)
	}

	if !strings.Contains(headers["Reply-Text"], "+OK") {
		return fmt.Errorf("订阅失败: %s", headers["Reply-Text"])
	}

	log.Println("事件订阅成功")
	return nil
}

// RegisterHandler 注册事件处理器
func (c *ESLClient) RegisterHandler(eventName string, handler EventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers[eventName] = handler
}

// SendCommand 发送命令
func (c *ESLClient) SendCommand(command string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return "", fmt.Errorf("未连接")
	}

	// 发送命令
	cmd := fmt.Sprintf("api %s\n\n", command)
	if _, err := c.conn.Write([]byte(cmd)); err != nil {
		return "", fmt.Errorf("发送命令失败: %v", err)
	}

	// 读取响应
	headers, err := c.readHeaders()
	if err != nil {
		return "", fmt.Errorf("读取命令响应失败: %v", err)
	}

	return headers["Reply-Text"], nil
}

// readHeaders 读取ESL头部
func (c *ESLClient) readHeaders() (map[string]string, error) {
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
func (c *ESLClient) readEventLoop() {
	c.running = true
	log.Println("开始事件读取循环")

	for c.running {
		// 读取事件头部
		headers, err := c.readHeaders()
		if err != nil {
			log.Printf("读取事件头部失败: %v\n", err)
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
					log.Printf("读取事件体失败: %v\n", err)
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

	log.Println("事件读取循环结束")
}

// handleEvent 处理单个事件
func (c *ESLClient) handleEvent(headers map[string]string) {
	// 如果有事件名称，调用对应的处理器
	if eventName, ok := headers["Event-Name"]; ok {
		c.mu.RLock()
		handler, exists := c.handlers[eventName]
		c.mu.RUnlock()

		if exists {
			if err := handler(headers); err != nil {
				log.Printf("事件处理失败: %v\n", err)
			} else {
				log.Printf("成功处理事件: %s\n", eventName)
			}
		}
	}
}
