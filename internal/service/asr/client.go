package asr

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"ai_dialer_mini/internal/service/ws"
	"ai_dialer_mini/internal/types"
)

const (
	hostUrl = "wss://iat-api.xfyun.cn/v2/iat"

	// 音频状态常量
	STATUS_FIRST_FRAME    = 0
	STATUS_CONTINUE_FRAME = 1
	STATUS_LAST_FRAME     = 2
)

// XFYunConfig 讯飞云配置
type XFYunConfig struct {
	AppID     string
	APIKey    string
	APISecret string
	Host      string
	Port      int
}

// XFASRClient 讯飞ASR客户端
type XFASRClient struct {
	sync.RWMutex
	config        *XFYunConfig
	wsClient      *ws.Client
	resultHandler func(*types.ASRResult)
	ctx           context.Context
	cancel        context.CancelFunc
	resultCh      chan *types.ASRResult
	done          chan struct{}
}

// NewXFASRClient 创建新的讯飞ASR客户端
func NewXFASRClient(config *XFYunConfig) (*XFASRClient, error) {
	ctx, cancel := context.WithCancel(context.Background())

	client := &XFASRClient{
		config:   config,
		ctx:      ctx,
		cancel:   cancel,
		resultCh: make(chan *types.ASRResult, 100),
		done:     make(chan struct{}),
	}

	// 创建WebSocket客户端
	wsConfig := &ws.Config{
		Host:            config.Host,
		Port:            config.Port,
		Path:           "/v2/iat",
		WriteTimeout:    5000,
		ReadTimeout:     5000,
		BufferSize:      4096,
		Headers:         client.buildAuthHeaders(),
		MessageHandler:  client,
	}

	wsClient := ws.NewClient(wsConfig)
	client.wsClient = wsClient

	return client, nil
}

// SetResultHandler 设置结果处理函数
func (c *XFASRClient) SetResultHandler(handler func(*types.ASRResult)) {
	c.Lock()
	defer c.Unlock()
	c.resultHandler = handler
}

// Start 启动客户端
func (c *XFASRClient) Start() error {
	return c.wsClient.Connect()
}

// Stop 停止客户端
func (c *XFASRClient) Stop() {
	close(c.done)
	c.cancel()
	c.wsClient.Close()
	close(c.resultCh)
}

// SendAudio 发送音频数据
func (c *XFASRClient) SendAudio(data []byte) error {
	// 构建音频数据帧
	frame := &AudioFrame{}
	frame.Common.AppID = c.config.AppID
	frame.Business.Language = "zh_cn"
	frame.Business.Domain = "iat"
	frame.Data.Status = 1
	frame.Data.Format = "audio/L16;rate=16000"
	frame.Data.Encoding = "raw"
	frame.Data.Audio = base64.StdEncoding.EncodeToString(data)

	frameData, err := json.Marshal(frame)
	if err != nil {
		return fmt.Errorf("序列化音频帧失败: %v", err)
	}

	return c.wsClient.Send(types.WSTextMessage, frameData)
}

// GetResults 获取识别结果通道
func (c *XFASRClient) GetResults() <-chan *types.ASRResult {
	return c.resultCh
}

// HandleMessage 处理WebSocket消息
func (c *XFASRClient) HandleMessage(session types.Session, msg types.Message) error {
	if msg.Type() != types.WSTextMessage {
		return fmt.Errorf("unexpected message type: %v", msg.Type())
	}

	data, ok := msg.Data().([]byte)
	if !ok {
		return fmt.Errorf("invalid message data type")
	}

	var resp RespData
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("解析响应数据失败: %v", err)
	}

	// 处理识别结果
	if resp.Data.Status == 2 {
		decoder := &Decoder{}
		decoder.Decode(&resp.Data.Result)
		text := decoder.String()

		result := &types.ASRResult{
			Text:      text,
			Status:    types.ASRStatusDone,
			StartTime: time.Now(),
			EndTime:   time.Now(),
		}

		c.Lock()
		handler := c.resultHandler
		c.Unlock()

		if handler != nil {
			handler(result)
		}

		// 发送结果到通道
		select {
		case c.resultCh <- result:
		case <-c.done:
		}
	}

	return nil
}

// buildAuthHeaders 构建认证头部
func (c *XFASRClient) buildAuthHeaders() map[string]string {
	now := time.Now().UTC().Format(time.RFC1123)
	signature := c.buildSignature(now)

	return map[string]string{
		"Date":                   now,
		"Authorization":          signature,
		"X-Appid":                c.config.AppID,
		"X-Real-Ip":              "127.0.0.1",
		"X-Param":                buildXParam(),
		"Content-Type":           "application/json",
		"Accept":                 "application/json",
		"Connection":             "keep-alive",
		"Host":                   "iat-api.xfyun.cn",
		"Origin":                 "https://iat-api.xfyun.cn",
		"Sec-WebSocket-Protocol": "v2.iat",
	}
}

// buildSignature 构建签名
func (c *XFASRClient) buildSignature(date string) string {
	signString := fmt.Sprintf("host: %s\ndate: %s\nGET /v2/iat HTTP/1.1", "iat-api.xfyun.cn", date)
	hmacObj := hmac.New(sha256.New, []byte(c.config.APISecret))
	hmacObj.Write([]byte(signString))
	sha := base64.StdEncoding.EncodeToString(hmacObj.Sum(nil))

	authorization := fmt.Sprintf("api_key=\"%s\", algorithm=\"hmac-sha256\", headers=\"host date request-line\", signature=\"%s\"",
		c.config.APIKey, sha)

	return authorization
}

// buildXParam 构建X-Param参数
func buildXParam() string {
	param := map[string]interface{}{
		"engine_type": "sms16k",
		"aue":         "raw",
	}

	paramBytes, _ := json.Marshal(param)
	return base64.StdEncoding.EncodeToString(paramBytes)
}

type AudioFrame struct {
	Common struct {
		AppID string `json:"app_id"`
	} `json:"common"`
	Business struct {
		Language string `json:"language"`
		Domain   string `json:"domain"`
	} `json:"business"`
	Data struct {
		Status   int    `json:"status"`
		Format   string `json:"format"`
		Encoding string `json:"encoding"`
		Audio    string `json:"audio"`
	} `json:"data"`
}

type RespData struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Result string `json:"result"`
		Status int    `json:"status"`
	} `json:"data"`
}

type Decoder struct {
}

func (d *Decoder) Decode(data *string) {
	// TODO: 实现解码逻辑
}

func (d *Decoder) String() string {
	// TODO: 实现获取解码结果逻辑
	return ""
}
