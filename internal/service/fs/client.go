package fs

import (
	"ai_dialer_mini/internal/config"
	"ai_dialer_mini/internal/service/asr"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/fiorix/go-eventsocket/eventsocket"
)

// Client FreeSWITCH ESL客户端
type Client struct {
	config *config.FreeSwitchConfig
	conn   *eventsocket.Connection
	events chan *CallEvent
}

// NewClient 创建新的FreeSWITCH客户端
func NewClient(config *config.FreeSwitchConfig) *Client {
	return &Client{
		config: config,
		events: make(chan *CallEvent, 100),
	}
}

// Connect 连接到FreeSWITCH
func (c *Client) Connect() error {
	var err error
	c.conn, err = eventsocket.Dial(fmt.Sprintf("%s:%d", c.config.Host, c.config.Port), c.config.Password)
	if err != nil {
		return fmt.Errorf("连接FreeSWITCH失败: %v", err)
	}

	// 只订阅需要的事件
	events := []string{
		"CHANNEL_CREATE",
		"CHANNEL_ANSWER",
		"CHANNEL_HANGUP",
		"CHANNEL_DESTROY",
	}
	if _, err := c.conn.Send(fmt.Sprintf("events plain %s", strings.Join(events, " "))); err != nil {
		return fmt.Errorf("订阅事件失败: %v", err)
	}

	// 启动事件处理
	go c.handleEvents()

	log.Printf("已连接到FreeSWITCH服务器 %s:%d", c.config.Host, c.config.Port)
	return nil
}

// Subscribe 订阅事件
func (c *Client) Subscribe(events []string) error {
	eventList := strings.Join(events, " ")
	_, err := c.conn.Send(fmt.Sprintf("events plain %s", eventList))
	if err != nil {
		return fmt.Errorf("订阅事件失败: %v", err)
	}
	return nil
}

// handleEvents 处理FreeSWITCH事件
func (c *Client) handleEvents() {
	for {
		ev, err := c.ReadEvent()
		if err != nil {
			log.Printf("读取事件错误: %v", err)
			c.reconnect()
			continue
		}

		// 跳过非事件消息
		if ev == nil {
			continue
		}

		eventName := ev.GetHeader("Event-Name")
		if eventName == "" {
			continue
		}

		// 只处理关心的事件类型
		switch eventName {
		case "CHANNEL_CREATE", "CHANNEL_ANSWER", "CHANNEL_HANGUP", "CHANNEL_DESTROY", "CUSTOM":
			log.Printf("收到FreeSWITCH事件: %s", eventName)
			// 打印所有事件头部
			log.Printf("事件头部信息:")
			for k := range ev.headers {
				v := ev.GetHeader(k)
				if v != "" {
					log.Printf("  %s = %s", k, v)
				}
			}

			// 获取CallID
			var callID string
			possibleFields := []string{
				"Unique-ID",
				"Channel-Call-UUID",
				"variable_call_uuid",
				"Other-Leg-Unique-ID",
				"Bridge-A-Unique-ID",
				"Bridge-B-Unique-ID",
			}

			for _, field := range possibleFields {
				if id := ev.GetHeader(field); id != "" {
					callID = id
					log.Printf("使用 %s 作为CallID: %s", field, id)
					break
				}
			}

			// 获取主叫和被叫号码
			callerNumber := ev.GetHeader("Caller-ANI")
			if callerNumber == "" {
				callerNumber = ev.GetHeader("Caller-Caller-ID-Number")
			}
			if callerNumber == "" {
				callerNumber = ev.GetHeader("variable_origination_caller_id_number")
			}

			calledNumber := ev.GetHeader("Caller-Destination-Number")
			if calledNumber == "" {
				calledNumber = ev.GetHeader("variable_dialed_number")
			}

			// 创建通话事件
			event := &CallEvent{
				EventName:    eventName,
				CallID:       callID,
				CallerNumber: callerNumber,
				CalledNumber: calledNumber,
				Headers:      make(map[string]string),
			}

			// 复制事件头部信息
			for k := range ev.headers {
				event.Headers[k] = ev.GetHeader(k)
			}

			// 发送事件到通道
			select {
			case c.events <- event:
				log.Printf("已发送事件到通道: %s", eventName)
			default:
				log.Printf("事件通道已满，丢弃事件: %s", eventName)
			}
		}
	}
}

// reconnect 重新连接
func (c *Client) reconnect() {
	for {
		log.Println("尝试重新连接FreeSWITCH...")
		err := c.Connect()
		if err == nil {
			break
		}
		log.Printf("重连失败: %v", err)
		time.Sleep(5 * time.Second)
	}
}

// Originate 发起呼叫
func (c *Client) Originate(from, to string) error {
	// 设置更多的呼叫参数
	cmd := fmt.Sprintf("originate {origination_caller_id_number=%s,origination_caller_id_name=%s,originate_timeout=30}user/%s &bridge(user/%s)",
		from, from, from, to)
	log.Printf("执行FreeSWITCH命令: %s", cmd)
	_, err := c.conn.Send("api " + cmd)
	if err != nil {
		return fmt.Errorf("发起呼叫失败: %v", err)
	}
	return nil
}

// Close 关闭连接
func (c *Client) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
	close(c.events)
}

// GetEvents 获取事件通道
func (c *Client) GetEvents() <-chan *CallEvent {
	return c.events
}

// Event FreeSWITCH事件
type Event struct {
	headers map[string]string
}

// GetHeader 获取事件头部字段
func (e *Event) GetHeader(name string) string {
	if e.headers == nil {
		return ""
	}
	return e.headers[name]
}

// ReadEvent 读取事件
func (c *Client) ReadEvent() (*Event, error) {
	ev, err := c.conn.ReadEvent()
	if err != nil {
		return nil, err
	}

	// 打印原始事件信息
	log.Printf("收到原始事件: Content-Type=%s, Event-Name=%s",
		ev.Get("Content-Type"), ev.Get("Event-Name"))

	// 创建新的事件对象
	event := &Event{
		headers: make(map[string]string),
	}

	// 复制事件头部
	for k := range ev.Header {
		v := ev.Get(k)
		if v != "" {
			event.headers[k] = v
		}
	}

	// 检查是否是有效事件
	if event.GetHeader("Event-Name") == "" {
		log.Printf("跳过无效事件")
		return nil, nil
	}

	return event, nil
}

// CallEvent 通话事件
type CallEvent struct {
	EventName    string
	CallID       string
	CallerNumber string
	CalledNumber string
	Headers      map[string]string
}

// HandleCall 发起呼叫并处理ASR
func (c *Client) HandleCall(from, to string) error {
	log.Printf("开始处理呼叫: from=%s, to=%s", from, to)

	// 发起呼叫
	err := c.Originate(from, to)
	if err != nil {
		return fmt.Errorf("发起呼叫失败: %v", err)
	}

	// 订阅更多事件类型以便调试
	log.Println("订阅所有相关事件...")
	_, err = c.conn.Send("event plain CHANNEL_CREATE CHANNEL_ANSWER CHANNEL_HANGUP CHANNEL_DESTROY CUSTOM sofia::media")
	if err != nil {
		return fmt.Errorf("订阅事件失败: %v", err)
	}

	go func() {
		for {
			ev, err := c.ReadEvent()
			if err != nil {
				log.Printf("读取事件错误: %v", err)
				return
			}

			if ev == nil {
				log.Println("收到空事件，跳过")
				continue
			}

			eventName := ev.GetHeader("Event-Name")
			log.Printf("收到事件: %s", eventName)

			if eventName == "CHANNEL_ANSWER" {
				uuid := ev.GetHeader("Unique-ID")
				log.Printf("通话已接通，UUID: %s", uuid)
				if uuid != "" {
					log.Printf("准备启动ASR，UUID: %s", uuid)
					go c.StartASR(uuid)
				}
			}
		}
	}()

	return nil
}

// StartASR 启动ASR
func (c *Client) StartASR(uuid string) {
	log.Printf("开始启动ASR，UUID: %s", uuid)

	// 订阅通道音频事件
	log.Println("订阅通道音频事件...")
	_, err := c.conn.Execute("uuid_record", fmt.Sprintf("%s start", uuid), true)
	if err != nil {
		log.Printf("订阅音频事件失败: %v", err)
		return
	}

	// 创建ASR客户端
	log.Println("创建ASR客户端...")
	asrClient := asr.NewXFYunASR(&config.ASRConfig{
		APPID:     "c0de4f24",
		APISecret: "NWRhZDBkNzA5ZDQxNGMzYmQ1NWMwMWNh",
		APIKey:    "51012a35448538a8396dc564cf050f68",
	})

	// 启动ASR会话
	log.Println("启动ASR会话...")
	err = asrClient.Start(uuid)
	if err != nil {
		log.Printf("启动ASR会话失败: %v", err)
		return
	}

	log.Printf("ASR会话启动成功，开始监听识别结果...")
	// 监听识别结果
	go func() {
		for result := range asrClient.GetResults() {
			if result.CallID == uuid {
				log.Printf("【ASR识别结果】UUID: %s, 文本: %s", uuid, result.Text)
			}
		}
	}()
}
