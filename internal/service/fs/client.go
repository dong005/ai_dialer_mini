package fs

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"ai_dialer_mini/internal/types"
	"github.com/fiorix/go-eventsocket/eventsocket"
)

// Client FreeSWITCH客户端
type Client struct {
	conn      *eventsocket.Connection
	events    chan *types.Event
	audioChan chan *types.AudioData
	config    *types.FSConfig
	done      chan struct{}
	started   bool
	sync.RWMutex
}

// NewClient 创建新的FreeSWITCH客户端
func NewClient(config *types.FSConfig) *Client {
	return &Client{
		events:    make(chan *types.Event, 100),
		audioChan: make(chan *types.AudioData, 1000),
		config:    config,
		done:      make(chan struct{}),
		started:   false,
	}
}

// Connect 连接到FreeSWITCH
func (c *Client) Connect() error {
	c.Lock()
	defer c.Unlock()

	if c.started {
		return nil
	}

	// 连接FreeSWITCH
	if err := c.connect(); err != nil {
		return err
	}

	// 启动事件处理
	go c.handleEvents()

	c.started = true
	return nil
}

// Start 启动客户端
func (c *Client) Start() error {
	c.Lock()
	defer c.Unlock()

	if c.started {
		return nil
	}

	// 连接FreeSWITCH
	if err := c.connect(); err != nil {
		return err
	}

	// 启动事件处理
	go c.handleEvents()

	c.started = true
	return nil
}

// handleEvents 处理事件循环
func (c *Client) handleEvents() {
	reader := bufio.NewReader(c.conn)
	defer close(c.events)

	for {
		select {
		case <-c.done:
			return
		default:
			event, err := c.readEvent(reader)
			if err != nil {
				if err == io.EOF {
					return
				}
				log.Printf("[ERROR] 读取事件失败: %v", err)
				continue
			}

			// 发送事件到通道
			select {
			case c.events <- event:
				log.Printf("[DEBUG] 事件已发送到通道: %s", event.EventName)
			default:
				log.Printf("[WARN] 事件通道已满，丢弃事件: %s", event.EventName)
			}
		}
	}
}

// readEvent 读取单个事件
func (c *Client) readEvent(reader *bufio.Reader) (*types.Event, error) {
	var headers []string

	// 读取事件头
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		headers = append(headers, line)
	}

	// 解析Content-Length
	contentLength := 0
	for _, header := range headers {
		if strings.HasPrefix(header, "Content-Length: ") {
			length := strings.TrimPrefix(header, "Content-Length: ")
			contentLength, _ = strconv.Atoi(length)
			break
		}
	}

	// 读取事件体
	if contentLength > 0 {
		bodyBytes := make([]byte, contentLength)
		_, err := io.ReadFull(reader, bodyBytes)
		if err != nil {
			return nil, err
		}
	}

	// 解析事件
	event := &types.Event{
		Headers: make(map[string]string),
	}

	// 解析所有头部字段
	for _, header := range headers {
		parts := strings.SplitN(header, ": ", 2)
		if len(parts) != 2 {
			continue
		}
		key, value := parts[0], parts[1]
		event.Headers[key] = value

		// 提取关键字段
		switch key {
		case "Event-Name":
			event.EventName = value
		case "Unique-ID":
			event.CallID = value
		case "Caller-Caller-ID-Number":
			event.CallerNumber = value
		case "Caller-Destination-Number":
			event.CalledNumber = value
		}
	}

	return event, nil
}

// connect 连接到FreeSWITCH
func (c *Client) connect() error {
	var err error
	maxRetries := 3
	retryDelay := time.Second

	for i := 0; i < maxRetries; i++ {
		log.Printf("[INFO] 尝试连接FreeSWITCH服务器 %s:%d", c.config.Host, c.config.Port)

		// 建立TCP连接
		addr := fmt.Sprintf("%s:%d", c.config.Host, c.config.Port)
		c.conn, err = net.DialTimeout("tcp", addr, 5*time.Second)
		if err != nil {
			log.Printf("[ERROR] 连接失败: %v, 重试中...", err)
			time.Sleep(retryDelay)
			continue
		}

		// 读取欢迎信息
		resp, err := c.readResponse()
		if err != nil {
			log.Printf("[ERROR] 读取欢迎信息失败: %v", err)
			c.conn.Close()
			time.Sleep(retryDelay)
			continue
		}
		log.Printf("[DEBUG] 收到欢迎信息: %s", resp)

		// 发送认证
		auth := fmt.Sprintf("auth %s\n\n", c.config.Password)
		_, err = c.conn.Write([]byte(auth))
		if err != nil {
			log.Printf("[ERROR] 发送认证信息失败: %v", err)
			c.conn.Close()
			time.Sleep(retryDelay)
			continue
		}

		// 读取认证响应
		resp, err = c.readResponse()
		if err != nil {
			log.Printf("[ERROR] 读取认证响应失败: %v", err)
			c.conn.Close()
			time.Sleep(retryDelay)
			continue
		}
		log.Printf("[DEBUG] 认证响应: %s", resp)

		if !strings.Contains(resp, "+OK accepted") {
			log.Printf("[ERROR] 认证失败: %s", resp)
			c.conn.Close()
			time.Sleep(retryDelay)
			continue
		}

		log.Printf("[INFO] 已成功连接到FreeSWITCH服务器 %s:%d", c.config.Host, c.config.Port)
		return nil
	}

	return fmt.Errorf("连接FreeSWITCH服务器失败，已重试%d次", maxRetries)
}

// send 发送命令
func (c *Client) send(cmd string) error {
	_, err := c.conn.Write([]byte(cmd))
	return err
}

// readResponse 读取响应
func (c *Client) readResponse() (string, error) {
	var response strings.Builder
	reader := bufio.NewReader(c.conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("读取响应失败: %v", err)
		}
		response.WriteString(line)

		// 如果是认证响应，需要继续读取直到遇到空行
		if strings.HasPrefix(line, "Content-Type: auth/request") {
			continue
		}

		// 检查错误响应
		if strings.HasPrefix(line, "-ERR") {
			return response.String(), fmt.Errorf("收到错误响应: %s", strings.TrimSpace(line))
		}

		// 空行表示响应结束
		if line == "\n" {
			break
		}
	}

	return response.String(), nil
}

// SendCommand 发送FreeSWITCH命令
func (c *Client) SendCommand(cmd string) error {
	if err := c.send(cmd + "\n\n"); err != nil {
		return err
	}
	resp, err := c.readResponse()
	if err != nil {
		return err
	}
	log.Printf("命令响应: %s", resp)
	return nil
}

// Close 关闭连接
func (c *Client) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
	close(c.done)
}

// GetEvents 获取事件通道
func (c *Client) GetEvents() <-chan *types.Event {
	return c.events
}

// Subscribe 订阅事件
func (c *Client) Subscribe(events []string) error {
	cmd := fmt.Sprintf("events plain %s\n\n", strings.Join(events, " "))
	return c.send(cmd)
}

// Originate 发起呼叫
func (c *Client) Originate(from, to string) error {
	// 设置更多的呼叫参数
	cmd := fmt.Sprintf("api originate {origination_caller_id_number=%s,origination_caller_id_name=%s,originate_timeout=30}user/%s &bridge(user/%s)",
		from, from, from, to)
	log.Printf("执行FreeSWITCH命令: %s", cmd)
	return c.SendCommand(cmd)
}

// HandleCall 发起呼叫并处理ASR
func (c *Client) HandleCall(from, to string) error {
	log.Printf("开始处理呼叫: from=%s, to=%s", from, to)

	// 发起呼叫
	err := c.Originate(from, to)
	if err != nil {
		return fmt.Errorf("发起呼叫失败: %v", err)
	}

	// 订阅通道事件
	if err := c.Subscribe([]string{
		"CHANNEL_CREATE",
		"CHANNEL_ANSWER",
		"CHANNEL_HANGUP",
		"CHANNEL_DESTROY",
		"CUSTOM",
		"sofia::register",
		"sofia::unregister",
		"sofia::media",
	}); err != nil {
		return fmt.Errorf("订阅事件失败: %v", err)
	}

	// 启动事件监听
	go func() {
		for event := range c.events {
			if event.EventName == "CHANNEL_ANSWER" {
				// 获取通道UUID
				uuid := event.CallID
				if uuid != "" {
					log.Printf("通话已接通，开始处理音频流 UUID: %s", uuid)

					// 启动音频流
					cmd := fmt.Sprintf("api uuid_audio_stream %s start ws://192.168.11.2:8080/ws?uuid=%s mono 8000", uuid, uuid)
					if err := c.SendCommand(cmd); err != nil {
						log.Printf("启动音频流失败: %v", err)
						continue
					}

					// 获取命令结果
					setCmd := fmt.Sprintf("set api_result=${uuid_audio_stream(%s start ws://192.168.11.2:8080/ws?uuid=%s mono 8000)}", uuid, uuid)
					if err := c.SendCommand(setCmd); err != nil {
						log.Printf("设置音频流参数失败: %v", err)
						continue
					}

					log.Printf("音频流已启动 UUID: %s", uuid)
				}
			}
		}
	}()

	return nil
}

// StartASR 启动ASR
func (c *Client) StartASR(uuid string) {
	log.Printf("开始ASR处理: uuid=%s", uuid)

	// 发送命令开始录音
	cmd := fmt.Sprintf("uuid_record %s start /tmp/%s.wav", uuid, uuid)
	if err := c.SendCommand(cmd); err != nil {
		log.Printf("开始录音失败: %v", err)
		return
	}
}
