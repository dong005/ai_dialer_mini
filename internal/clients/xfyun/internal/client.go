package internal

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	STATUS_FIRST_FRAME    = 0
	STATUS_CONTINUE_FRAME = 1
	STATUS_LAST_FRAME     = 2
)

// Config 科大讯飞WebSocket客户端配置
type Config struct {
	AppID             string
	APIKey            string
	APISecret         string
	ServerURL         string
	ReconnectInterval time.Duration
	MaxRetries        int
	SampleRate        int // 音频采样率，支持8000或16000
}

// WSClient 科大讯飞WebSocket客户端
type WSClient struct {
	config     Config
	conn       *websocket.Conn
	callback   func(string, bool) error
	mu         sync.RWMutex  // 用于保护isRunning和result
	writeMu    sync.Mutex    // 用于保护WebSocket写入操作
	isRunning  bool
	retryCount int
	result     string // 存储识别结果
}

// NewWSClient 创建新的WS客户端
func NewWSClient(config Config) *WSClient {
	if config.ReconnectInterval == 0 {
		config.ReconnectInterval = 5 * time.Second
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.SampleRate == 0 {
		config.SampleRate = 16000
	}
	return &WSClient{
		config:    config,
		isRunning: false,
	}
}

// SetCallback 设置回调函数
func (c *WSClient) SetCallback(callback func(string, bool) error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.callback = callback
}

// Connect 连接到服务器
func (c *WSClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isRunning {
		return nil
	}

	// 生成鉴权URL
	authUrl, err := c.getAuthURL()
	if err != nil {
		return fmt.Errorf("生成鉴权URL失败: %v", err)
	}

	// 设置连接超时
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	// 添加重试逻辑
	var lastError error
	for i := 0; i <= c.config.MaxRetries; i++ {
		if i > 0 {
			time.Sleep(c.config.ReconnectInterval)
		}

		conn, _, err := dialer.Dial(authUrl, nil)
		if err == nil {
			c.conn = conn
			c.isRunning = true
			c.retryCount = i

			// 启动心跳检测
			go func() {
				ticker := time.NewTicker(5 * time.Second)
				defer ticker.Stop()

				for {
					c.mu.RLock()
					if !c.isRunning {
						c.mu.RUnlock()
						return
					}
					c.mu.RUnlock()

					c.writeMu.Lock()
					err := c.conn.WriteMessage(websocket.PingMessage, nil)
					c.writeMu.Unlock()

					if err != nil {
						log.Printf("发送心跳失败: %v", err)
						c.mu.Lock()
						c.isRunning = false
						c.mu.Unlock()
						return
					}

					time.Sleep(5 * time.Second)
				}
			}()

			go c.readLoop()
			return nil
		}
		lastError = err
	}

	return fmt.Errorf("WebSocket连接失败，已重试%d次: %v", c.config.MaxRetries, lastError)
}

// Close 关闭连接
func (c *WSClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.isRunning {
		return nil
	}

	c.isRunning = false
	if c.conn != nil {
		c.writeMu.Lock()
		defer c.writeMu.Unlock()
		return c.conn.Close()
	}
	return nil
}

// SendAudio 发送音频数据
func (c *WSClient) SendAudio(data []byte, isEnd bool) error {
	c.mu.RLock()
	if !c.isRunning || c.conn == nil {
		c.mu.RUnlock()
		return fmt.Errorf("连接未建立")
	}
	retryCount := c.retryCount
	c.mu.RUnlock()

	var frame map[string]interface{}
	if isEnd {
		// 结束帧
		frame = map[string]interface{}{
			"data": map[string]interface{}{
				"status":   STATUS_LAST_FRAME,
				"format":   fmt.Sprintf("audio/L16;rate=%d", c.config.SampleRate),
				"audio":    "",
				"encoding": "raw",
			},
		}
	} else {
		// 音频数据帧
		frame = map[string]interface{}{
			"data": map[string]interface{}{
				"status":   STATUS_CONTINUE_FRAME,
				"format":   fmt.Sprintf("audio/L16;rate=%d", c.config.SampleRate),
				"audio":    base64.StdEncoding.EncodeToString(data),
				"encoding": "raw",
			},
		}
		// 只在第一帧添加common和business字段
		if retryCount == 0 {
			frame["common"] = map[string]interface{}{
				"app_id": c.config.AppID,
			}
			frame["business"] = map[string]interface{}{
				"language": "zh_cn",
				"domain":   "iat",
				"accent":   "mandarin",
				"dwa":     "wpgs",
				"vad_eos": 3000,
			}
			// 修改为第一帧状态
			frame["data"].(map[string]interface{})["status"] = STATUS_FIRST_FRAME
		}
	}

	frameData, err := json.Marshal(frame)
	if err != nil {
		return fmt.Errorf("编码音频数据失败: %v", err)
	}

	c.writeMu.Lock()
	err = c.conn.WriteMessage(websocket.TextMessage, frameData)
	c.writeMu.Unlock()

	if err != nil {
		c.mu.Lock()
		c.isRunning = false
		c.mu.Unlock()
		return fmt.Errorf("发送音频数据失败: %v", err)
	}

	return nil
}

// readLoop 读取响应数据
func (c *WSClient) readLoop() {
	defer func() {
		c.mu.Lock()
		c.isRunning = false
		c.mu.Unlock()
		c.writeMu.Lock()
		c.conn.Close()
		c.writeMu.Unlock()
		log.Println("readLoop退出")
	}()

	for {
		c.mu.RLock()
		if !c.isRunning {
			c.mu.RUnlock()
			return
		}
		c.mu.RUnlock()

		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("读取消息错误: %v", err)
			}
			return
		}

		log.Printf("收到原始消息: %s", string(message))

		var response map[string]interface{}
		if err := json.Unmarshal(message, &response); err != nil {
			log.Printf("解析响应失败: %v", err)
			continue
		}

		// 检查响应码
		if code, ok := response["code"].(float64); ok && code != 0 {
			msg, _ := response["message"].(string)
			log.Printf("科大讯飞返回错误: %s", msg)
			continue
		}

		// 处理响应
		if data, ok := response["data"].(map[string]interface{}); ok {
			if result, ok := data["result"].(map[string]interface{}); ok {
				if ws, ok := result["ws"].([]interface{}); ok && len(ws) > 0 {
					var recognizedText strings.Builder
					for _, item := range ws {
						if cw, ok := item.(map[string]interface{}); ok {
							if words, ok := cw["cw"].([]interface{}); ok {
								for _, word := range words {
									if w, ok := word.(map[string]interface{}); ok {
										if t, ok := w["w"].(string); ok {
											recognizedText.WriteString(t)
										}
									}
								}
							}
						}
					}
					text := recognizedText.String()
					log.Printf("识别到文本: %s", text)

					// 检查是否是最终结果
					isEnd := false
					if status, ok := data["status"].(float64); ok {
						isEnd = status == 2
					}

					c.mu.Lock()
					c.result = text
					c.mu.Unlock()

					if c.callback != nil {
						if err := c.callback(text, isEnd); err != nil {
							log.Printf("回调处理错误: %v", err)
						}
					} else {
						log.Printf("回调函数未设置")
					}
				}
			}
		}
	}
}

// getAuthURL 生成鉴权URL
func (c *WSClient) getAuthURL() (string, error) {
	baseURL := c.config.ServerURL
	now := time.Now()

	urlStruct, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("解析URL失败: %v", err)
	}

	date := now.UTC().Format(time.RFC1123)
	signString := fmt.Sprintf("host: %s\ndate: %s\nGET %s HTTP/1.1",
		urlStruct.Host, date, urlStruct.Path)

	mac := hmac.New(sha256.New, []byte(c.config.APISecret))
	mac.Write([]byte(signString))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	authorization := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf(
		"api_key=\"%s\", algorithm=\"hmac-sha256\", headers=\"host date request-line\", signature=\"%s\"",
		c.config.APIKey, signature)))

	v := url.Values{}
	v.Add("authorization", authorization)
	v.Add("date", date)
	v.Add("host", urlStruct.Host)

	if strings.Contains(baseURL, "?") {
		return baseURL + "&" + v.Encode(), nil
	}
	return baseURL + "?" + v.Encode(), nil
}
