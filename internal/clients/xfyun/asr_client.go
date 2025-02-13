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
	config      Config
	conn        *websocket.Conn
	callback    func(string, bool) error
	mu          sync.Mutex
	retryCount  int
	decoder     *Decoder
}

// NewWSClient 创建新的WebSocket客户端
func NewWSClient(config Config) *WSClient {
	return &WSClient{
		config:  config,
		decoder: &Decoder{},
	}
}

// Connect 连接WebSocket服务器
func (c *WSClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return nil
	}

	// 生成握手参数
	handshakeParams := c.generateHandshakeParams()
	if handshakeParams == "" {
		return fmt.Errorf("生成握手参数失败")
	}
	
	url := fmt.Sprintf("%s?%s", c.config.ServerURL, handshakeParams)
	log.Printf("正在连接WebSocket服务器: %s", url)

	// 建立连接
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}
	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		c.retryCount++
		if c.retryCount > c.config.MaxRetries {
			return fmt.Errorf("连接失败，已达到最大重试次数: %v", err)
		}
		log.Printf("连接失败，将在 %v 后重试: %v", c.config.ReconnectInterval, err)
		time.Sleep(c.config.ReconnectInterval)
		return c.Connect()
	}

	log.Printf("WebSocket连接成功")
	c.retryCount = 0
	c.conn = conn

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
func (c *WSClient) SendAudio(data []byte, status int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		if err := c.Connect(); err != nil {
			return fmt.Errorf("重新连接失败: %v", err)
		}
	}

	// 将音频数据转换为Base64
	base64Data := base64.StdEncoding.EncodeToString(data)

	// 构建消息
	frame := Frame{}
	
	// 只在第一帧时发送common和business信息
	if status == STATUS_FIRST_FRAME {
		frame.Common.AppID = c.config.AppID
		frame.Business.Language = "zh_cn"
		frame.Business.Domain = "iat"
		frame.Business.Accent = "mandarin"
	}

	frame.Data.Status = status
	frame.Data.Format = "audio/L16;rate=16000"
	frame.Data.Audio = base64Data

	// 序列化消息
	message, err := json.Marshal(frame)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %v", err)
	}

	log.Printf("发送音频帧，状态: %d, 大小: %d 字节", status, len(data))

	// 发送消息
	if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
		c.conn = nil // 连接可能已断开，标记为nil以便下次重连
		return fmt.Errorf("发送消息失败: %v", err)
	}

	return nil
}

// receiveMessages 接收消息
func (c *WSClient) receiveMessages() {
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			log.Printf("读取消息失败: %v", err)
			c.handleError(err)
			return
		}

		log.Printf("收到原始消息: %s", string(message))

		var resp Response
		if err := json.Unmarshal(message, &resp); err != nil {
			log.Printf("解析消息失败: %v", err)
			c.handleError(err)
			continue
		}

		// 检查响应状态
		if resp.Code != 0 {
			log.Printf("服务器错误: %s", resp.Message)
			c.handleError(fmt.Errorf("服务器错误: %s", resp.Message))
			continue
		}

		// 解码结果
		c.decoder.Decode(&resp.Data.Result)
		text := c.decoder.String()
		log.Printf("解析识别结果: %s, 状态: %d, pgs: %s", text, resp.Data.Status, resp.Data.Result.Pgs)

		// 只有在pgs为"rpl"时才更新最终结果
		if resp.Data.Result.Pgs == "rpl" {
			if c.callback != nil {
				isEnd := resp.Data.Status == STATUS_LAST_FRAME
				if err := c.callback(text, isEnd); err != nil {
					log.Printf("回调函数执行失败: %v", err)
					c.handleError(err)
				}
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

// generateHandshakeParams 生成握手参数
func (c *WSClient) generateHandshakeParams() string {
	// 生成RFC1123格式的日期
	now := time.Now()
	date := now.UTC().Format(time.RFC1123)

	// 提取host
	u, err := url.Parse(c.config.ServerURL)
	if err != nil {
		log.Printf("解析URL失败: %v", err)
		return ""
	}
	host := u.Host

	// 签名原文
	signString := fmt.Sprintf("host: %s\ndate: %s\nGET /v2/iat HTTP/1.1", host, date)

	// 使用HMAC-SHA256计算签名
	mac := hmac.New(sha256.New, []byte(c.config.APISecret))
	mac.Write([]byte(signString))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	// 生成authorization
	authString := fmt.Sprintf("api_key=\"%s\", algorithm=\"hmac-sha256\", headers=\"host date request-line\", signature=\"%s\"", c.config.APIKey, signature)

	// 构建查询参数
	params := url.Values{}
	params.Set("authorization", base64.StdEncoding.EncodeToString([]byte(authString)))
	params.Set("date", date)
	params.Set("host", host)

	return params.Encode()
}

// Frame WebSocket帧
type Frame struct {
	Common struct {
		AppID string `json:"app_id"`
	} `json:"common"`
	Business struct {
		Language string `json:"language"`
		Domain   string `json:"domain"`
		Accent   string `json:"accent"`
	} `json:"business"`
	Data struct {
		Status int    `json:"status"`
		Format string `json:"format"`
		Audio  string `json:"audio"`
	} `json:"data"`
}

// Decoder 解析返回数据
type Decoder struct {
	results []*Result
}

// Decode 解码结果
func (d *Decoder) Decode(result *Result) {
	if len(d.results) <= result.Sn {
		d.results = append(d.results, make([]*Result, result.Sn-len(d.results)+1)...)
	}
	if result.Pgs == "rpl" {
		for i := result.Rg[0]; i <= result.Rg[1]; i++ {
			d.results[i] = nil
		}
	}
	d.results[result.Sn] = result
}

// String 获取完整识别结果
func (d *Decoder) String() string {
	var r string
	for _, v := range d.results {
		if v == nil {
			continue
		}
		r += v.String()
	}
	return r
}

// Result 识别结果
type Result struct {
	Ls  bool   `json:"ls"`
	Rg  []int  `json:"rg"`
	Sn  int    `json:"sn"`
	Pgs string `json:"pgs"`
	Ws  []Ws   `json:"ws"`
}

// String 获取单个结果文本
func (t *Result) String() string {
	var wss string
	for _, v := range t.Ws {
		wss += v.String()
	}
	return wss
}

// Ws 词信息
type Ws struct {
	Bg int  `json:"bg"`
	Cw []Cw `json:"cw"`
}

// String 获取词文本
func (w *Ws) String() string {
	var wss string
	for _, v := range w.Cw {
		wss += v.W
	}
	return wss
}

// Cw 字信息
type Cw struct {
	Sc int    `json:"sc"`
	W  string `json:"w"`
}

// Response 响应结构体
type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Sid     string `json:"sid"`
	Data    struct {
		Status int    `json:"status"`
		Result Result `json:"result"`
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
	if len(audioData) == 0 {
		return "", fmt.Errorf("音频数据为空")
	}

	log.Printf("开始处理音频数据，大小: %d 字节", len(audioData))

	// 创建结果通道
	resultChan := make(chan string, 1)
	errChan := make(chan error, 1)
	var finalResult string

	// 设置回调函数
	c.wsClient.SetCallback(func(text string, isEnd bool) error {
		if text != "" {
			finalResult = text
			log.Printf("实时识别结果: %s", text)
		}
		if isEnd {
			log.Printf("识别完成，最终结果: %s", finalResult)
			resultChan <- finalResult
		}
		return nil
	})

	// 连接WebSocket服务器
	log.Printf("连接WebSocket服务器: %s", c.wsClient.config.ServerURL)
	if err := c.wsClient.Connect(); err != nil {
		return "", fmt.Errorf("连接WebSocket服务器失败: %v", err)
	}
	defer c.wsClient.Close()

	// 分帧发送音频数据
	frameSize := 1280 // 每帧大小
	interval := 40 * time.Millisecond // 发送间隔
	
	// 计算总的处理时间
	totalFrames := (len(audioData) + frameSize - 1) / frameSize
	totalDuration := time.Duration(totalFrames) * interval
	timeout := totalDuration + 10*time.Second // 额外加10秒用于处理
	
	log.Printf("音频总帧数: %d, 预计处理时间: %v, 超时时间: %v", totalFrames, totalDuration, timeout)
	
	// 创建发送完成通道
	sendDone := make(chan bool)
	
	go func() {
		defer close(sendDone)
		for i := 0; i < len(audioData); i += frameSize {
			end := i + frameSize
			if end > len(audioData) {
				end = len(audioData)
			}
			
			// 确定帧状态
			var status int
			if i == 0 {
				status = STATUS_FIRST_FRAME
				log.Printf("发送第一帧...")
			} else if end == len(audioData) {
				status = STATUS_LAST_FRAME
				log.Printf("发送最后一帧...")
			} else {
				status = STATUS_CONTINUE_FRAME
			}
			
			// 发送音频帧
			frame := audioData[i:end]
			if err := c.wsClient.SendAudio(frame, status); err != nil {
				log.Printf("发送音频帧失败: %v", err)
				errChan <- fmt.Errorf("发送音频数据失败: %v", err)
				return
			}
			
			// 控制发送速率
			time.Sleep(interval)
		}
		log.Printf("音频数据发送完成")
	}()

	// 等待结果
	select {
	case <-sendDone:
		// 等待最终结果
		select {
		case result := <-resultChan:
			log.Printf("成功获取识别结果")
			return result, nil
		case err := <-errChan:
			log.Printf("处理音频出错: %v", err)
			return "", err
		case <-time.After(5 * time.Second): // 等待5秒钟最终结果
			log.Printf("等待最终结果超时")
			return finalResult, nil
		}
	case err := <-errChan:
		log.Printf("处理音频出错: %v", err)
		return "", err
	case <-time.After(timeout):
		log.Printf("处理音频超时")
		return "", fmt.Errorf("处理音频超时")
	}
}

// GetDialogHistory 获取对话历史
func (c *ASRClient) GetDialogHistory(sessionID string) []models.Message {
	return c.dialogSvc.GetHistory(sessionID)
}

// ClearDialogHistory 清除对话历史
func (c *ASRClient) ClearDialogHistory(sessionID string) {
	c.dialogSvc.ClearHistory(sessionID)
}

// GetWSClient 获取WebSocket客户端
func (c *ASRClient) GetWSClient() *WSClient {
	return c.wsClient
}
