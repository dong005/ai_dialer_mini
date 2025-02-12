// Package xfyun 实现与科大讯飞的WebSocket客户端通信
package xfyun

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"ai_dialer_mini/internal/clients/ws"
)

// WSClient 科大讯飞WebSocket客户端
type WSClient struct {
	*ws.Client
	appID     string
	apiKey    string
	apiSecret string
	callback  ASRCallback
}

// ASRCallback 语音识别结果回调函数类型
type ASRCallback func(result string, isEnd bool) error

// Config 科大讯飞WebSocket客户端配置
type Config struct {
	AppID             string
	APIKey            string
	APISecret         string
	ServerURL         string
	ReconnectInterval time.Duration
	MaxRetries        int
}

// NewWSClient 创建新的科大讯飞WebSocket客户端
func NewWSClient(config Config) *WSClient {
	// 生成鉴权URL
	authURL := generateAuthURL(config)

	// 创建通用WebSocket客户端配置
	wsConfig := ws.Config{
		URL:               authURL,
		Headers:           make(map[string]string),
		ReconnectInterval: config.ReconnectInterval,
		MaxRetries:        config.MaxRetries,
		HeartbeatInterval: 30 * time.Second,
		HeartbeatMessage:  []byte(`{"type":"heartbeat"}`),
	}

	client := &WSClient{
		Client:    ws.NewClient(wsConfig),
		appID:     config.AppID,
		apiKey:    config.APIKey,
		apiSecret: config.APISecret,
	}

	// 注册消息处理器
	client.RegisterHandler("result", client.handleResult)

	return client
}

// SetCallback 设置语音识别结果回调函数
func (c *WSClient) SetCallback(callback ASRCallback) {
	c.callback = callback
}

// SendAudio 发送音频数据
func (c *WSClient) SendAudio(data []byte, isEnd bool) error {
	frame := map[string]interface{}{
		"type": "audio",
		"data": base64.StdEncoding.EncodeToString(data),
		"end":  isEnd,
	}
	return c.SendMessage(frame)
}

// handleResult 处理识别结果
func (c *WSClient) handleResult(message []byte) error {
	if c.callback == nil {
		return nil
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Result  string `json:"result"`
			Status  int    `json:"status"`
			EndFlag int    `json:"end_flag"`
		} `json:"data"`
	}

	if err := json.Unmarshal(message, &result); err != nil {
		return fmt.Errorf("解析识别结果失败: %v", err)
	}

	if result.Code != 0 {
		return fmt.Errorf("识别错误: %s", result.Message)
	}

	return c.callback(result.Data.Result, result.Data.EndFlag == 1)
}

// generateAuthURL 生成带鉴权信息的WebSocket URL
func generateAuthURL(config Config) string {
	baseURL := config.ServerURL
	now := time.Now().Unix()
	expireTime := now + 60 // 1分钟有效期

	signOrigin := fmt.Sprintf("host=%s&date=%d&expires=%d&key=%s",
		baseURL, now, expireTime, config.APIKey)

	// 使用HMAC-SHA256生成签名
	h := hmac.New(sha256.New, []byte(config.APISecret))
	h.Write([]byte(signOrigin))
	signature := base64.StdEncoding.EncodeToString(h.Sum(nil))

	// 构建认证URL
	authURL := fmt.Sprintf("%s?appid=%s&date=%d&expires=%d&signature=%s",
		baseURL,
		url.QueryEscape(config.AppID),
		now,
		expireTime,
		url.QueryEscape(signature),
	)

	return authURL
}
