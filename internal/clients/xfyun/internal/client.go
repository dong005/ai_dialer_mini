package internal

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
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
	authURL, err := c.getAuthURL()
	if err != nil {
		return fmt.Errorf("生成鉴权URL失败: %v", err)
	}

	// 创建WebSocket连接
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	conn, resp, err := dialer.Dial(authURL, nil)
	if err != nil {
		if resp != nil {
			body, _ := ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			return fmt.Errorf("连接失败: HTTP %d - %s - %v", resp.StatusCode, string(body), err)
		}
		return fmt.Errorf("连接失败: %v", err)
	}
	if resp.StatusCode != 101 {
		body, _ := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		return fmt.Errorf("连接失败: HTTP %d - %s", resp.StatusCode, string(body))
	}

	c.conn = conn
	c.isRunning = true
	c.retryCount = 0

	// 启动读取循环
	go c.readLoop()

	return nil
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
				"audio":    base64.StdEncoding.EncodeToString(data),
				"encoding": "raw",
			},
		}
	} else if retryCount == 0 {
		// 第一帧，包含完整配置
		frame = map[string]interface{}{
			"common": map[string]interface{}{
				"app_id": c.config.AppID,
			},
			"business": map[string]interface{}{
				"language": "zh_cn",
				"domain":   "iat",
				"accent":   "mandarin",
				"dwa":     "wpgs", // 开启动态修正功能
				"vad_eos": 3000,   // 后端点检测时间，单位是毫秒
			},
			"data": map[string]interface{}{
				"status":   STATUS_FIRST_FRAME,
				"format":   fmt.Sprintf("audio/L16;rate=%d", c.config.SampleRate),
				"audio":    base64.StdEncoding.EncodeToString(data),
				"encoding": "raw",
			},
		}
		c.retryCount++
	} else {
		// 中间帧
		frame = map[string]interface{}{
			"data": map[string]interface{}{
				"status":   STATUS_CONTINUE_FRAME,
				"format":   fmt.Sprintf("audio/L16;rate=%d", c.config.SampleRate),
				"audio":    base64.StdEncoding.EncodeToString(data),
				"encoding": "raw",
			},
		}
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(frame)
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

	type RespData struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Sid     string `json:"sid"`
		Data    struct {
			Status int `json:"status"`
			Result struct {
				Ws []struct {
					Cw []struct {
						W string `json:"w"`
					} `json:"cw"`
				} `json:"ws"`
			} `json:"result"`
		} `json:"data"`
	}

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

		var resp RespData
		if err := json.Unmarshal(message, &resp); err != nil {
			log.Printf("解析响应失败: %v", err)
			continue
		}

		// 检查响应码
		if resp.Code != 0 {
			log.Printf("科大讯飞返回错误: %s", resp.Message)
			if c.callback != nil {
				c.callback("", true)
			}
			return
		}

		// 处理响应
		var text strings.Builder
		for _, ws := range resp.Data.Result.Ws {
			for _, cw := range ws.Cw {
				text.WriteString(cw.W)
			}
		}
		recognizedText := text.String()
		if recognizedText != "" {
			log.Printf("识别到文本: %s", recognizedText)
		}

		// 检查是否是最终结果
		isEnd := resp.Data.Status == 2

		if c.callback != nil {
			if err := c.callback(recognizedText, isEnd); err != nil {
				log.Printf("回调处理错误: %v", err)
			}
		}

		// 如果是最终结果，关闭连接
		if isEnd {
			return
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
