package xfyun

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

	"ai_dialer_mini/internal/models"
	"github.com/gorilla/websocket"
)

const (
	STATUS_FIRST_FRAME    = 0
	STATUS_CONTINUE_FRAME = 1
	STATUS_LAST_FRAME     = 2
)

// Config 科大讯飞ASR配置
type Config struct {
	AppID             string
	APIKey            string
	APISecret         string
	ServerURL         string
	ReconnectInterval time.Duration
	MaxRetries        int
	SampleRate        int
}

// WSClient WebSocket客户端
type WSClient struct {
	config     Config
	conn       *websocket.Conn
	callback   func(string, bool) error
	mu         sync.Mutex
	retryCount int
}

// NewWSClient 创建新的WebSocket客户端
func NewWSClient(config Config) *WSClient {
	return &WSClient{
		config: config,
	}
}

// Connect 连接WebSocket服务器
func (c *WSClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return nil
	}

	// 构建鉴权URL
	authURL, err := c.buildAuthURL()
	if err != nil {
		return fmt.Errorf("构建鉴权URL失败: %v", err)
	}

	// 连接WebSocket服务器
	conn, _, err := websocket.DefaultDialer.Dial(authURL, nil)
	if err != nil {
		c.retryCount++
		if c.retryCount > c.config.MaxRetries {
			return fmt.Errorf("连接失败，已达到最大重试次数: %v", err)
		}
		time.Sleep(c.config.ReconnectInterval)
		return c.Connect()
	}

	c.conn = conn
	c.retryCount = 0

	// 启动消息接收协程
	go c.receiveMessages()

	return nil
}

// Close 关闭连接
func (c *WSClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}

// SetCallback 设置回调函数
func (c *WSClient) SetCallback(callback func(string, bool) error) {
	c.mu.Lock()
	c.callback = callback
	c.mu.Unlock()
}

// SendAudio 发送音频数据
func (c *WSClient) SendAudio(data []byte, isEnd bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("连接未建立")
	}

	// 构建帧数据
	status := STATUS_CONTINUE_FRAME
	if len(data) == 0 {
		status = STATUS_FIRST_FRAME
	} else if isEnd {
		status = STATUS_LAST_FRAME
	}

	frame := Frame{
		Common: Common{
			App_id: c.config.AppID,
		},
		Business: Business{
			Language: "zh_cn",
			Domain:   "iat",
			Accent:   "mandarin",
			Format:   "audio/L16;rate=16000",
			Vad_eos:  5000,
		},
		Data: Data{
			Status: status,
			Audio:  base64.StdEncoding.EncodeToString(data),
		},
	}

	// 发送数据
	return c.conn.WriteJSON(frame)
}

// receiveMessages 接收消息
func (c *WSClient) receiveMessages() {
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			c.handleError(err)
			return
		}

		var response Response
		if err := json.Unmarshal(message, &response); err != nil {
			log.Printf("解析响应失败: %v", err)
			continue
		}

		// 处理错误码
		if response.Code != 0 {
			log.Printf("服务器返回错误: %s", response.Msg)
			continue
		}

		// 提取识别结果
		var result string
		for _, ws := range response.Data.Result.Ws {
			for _, cw := range ws.Cw {
				result += cw.W
			}
		}

		// 调用回调函数
		if c.callback != nil {
			isEnd := response.Data.Status == 2
			if err := c.callback(result, isEnd); err != nil {
				log.Printf("回调函数执行失败: %v", err)
			}
		}
	}
}

// handleError 处理错误
func (c *WSClient) handleError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
		log.Printf("WebSocket连接异常关闭: %v", err)
	}

	// 关闭连接
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}

	// 尝试重连
	if c.retryCount < c.config.MaxRetries {
		c.retryCount++
		go func() {
			time.Sleep(c.config.ReconnectInterval)
			if err := c.Connect(); err != nil {
				log.Printf("重连失败: %v", err)
			}
		}()
	}
}

// buildAuthURL 构建鉴权URL
func (c *WSClient) buildAuthURL() (string, error) {
	baseURL := c.config.ServerURL
	now := time.Now().UTC().Format(time.RFC1123)
	now = strings.Replace(now, "UTC", "GMT", -1)

	signString := fmt.Sprintf("host: %s\ndate: %s\nGET /v2/iat HTTP/1.1", strings.Split(baseURL, "://")[1], now)
	signature := c.buildAuthorization(signString)

	authorization := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("api_key=\"%s\", algorithm=\"hmac-sha256\", headers=\"host date request-line\", signature=\"%s\"",
		c.config.APIKey, signature)))

	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}

	q := u.Query()
	q.Set("authorization", authorization)
	q.Set("date", now)
	q.Set("host", u.Host)
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// buildAuthorization 构建鉴权信息
func (c *WSClient) buildAuthorization(data string) string {
	mac := hmac.New(sha256.New, []byte(c.config.APISecret))
	mac.Write([]byte(data))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// Frame WebSocket帧
type Frame struct {
	Common   Common   `json:"common"`
	Business Business `json:"business"`
	Data     Data     `json:"data"`
}

// Common 公共信息
type Common struct {
	App_id string `json:"app_id"`
}

// Business 业务信息
type Business struct {
	Language string `json:"language"`
	Domain   string `json:"domain"`
	Accent   string `json:"accent"`
	Format   string `json:"format"`
	Vad_eos  int    `json:"vad_eos"`
}

// Data 数据信息
type Data struct {
	Status int    `json:"status"`
	Audio  string `json:"audio"`
}

// Response WebSocket响应
type Response struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Result struct {
			Ws []struct {
				Cw []struct {
					W string `json:"w"`
				} `json:"cw"`
			} `json:"ws"`
		} `json:"result"`
		Status int `json:"status"`
	} `json:"data"`
}

// ASRClient 科大讯飞ASR客户端
type ASRClient struct {
	config    Config
	wsClient  *WSClient
	dialogSvc models.DialogService
}

// NewASRClient 创建新的ASR客户端
func NewASRClient(config Config, dialogSvc models.DialogService) *ASRClient {
	return &ASRClient{
		config:    config,
		wsClient:  NewWSClient(config),
		dialogSvc: dialogSvc,
	}
}

// ProcessAudio 处理音频数据并返回识别结果
func (c *ASRClient) ProcessAudio(sessionID string, audioData []byte) (string, error) {
	var result string
	var resultErr error
	var wg sync.WaitGroup
	wg.Add(1)

	c.wsClient.SetCallback(func(text string, isEnd bool) error {
		if text != "" {
			result = text
			// 处理ASR结果，获取AI回复
			aiResponse, err := c.dialogSvc.ProcessMessage(sessionID, text)
			if err != nil {
				log.Printf("处理对话失败: %v", err)
			} else {
				log.Printf("AI回复: %s", aiResponse)
			}
		}
		if isEnd {
			wg.Done()
		}
		return nil
	})

	if err := c.wsClient.Connect(); err != nil {
		return "", fmt.Errorf("连接失败: %v", err)
	}

	if err := c.wsClient.SendAudio(audioData, true); err != nil {
		resultErr = fmt.Errorf("发送音频失败: %v", err)
		wg.Done()
	}

	wg.Wait()

	if resultErr != nil {
		return "", resultErr
	}

	return result, nil
}

// GetDialogHistory 获取对话历史
func (c *ASRClient) GetDialogHistory(sessionID string) []models.Message {
	return c.dialogSvc.GetHistory(sessionID)
}

// ClearDialogHistory 清除对话历史
func (c *ASRClient) ClearDialogHistory(sessionID string) {
	c.dialogSvc.ClearHistory(sessionID)
}
