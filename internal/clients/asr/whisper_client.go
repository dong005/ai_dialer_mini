package asr

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"time"

	"ai_dialer_mini/internal/clients/ws"
	"ai_dialer_mini/internal/models"
)

// WhisperClient 实现与 ASR 服务器的 WebSocket 通信
type WhisperClient struct {
	wsClient *ws.Client
	grammar  string
}

// NewWhisperClient 创建新的 Whisper 客户端
func NewWhisperClient(serverURL string) *WhisperClient {
	config := ws.Config{
		URL:               serverURL,
		ReconnectInterval: 5 * time.Second,
		MaxRetries:        3,
		HeartbeatInterval: 30 * time.Second,
		HeartbeatMessage:  []byte(`{"type":"ping"}`),
	}

	client := &WhisperClient{
		wsClient: ws.NewClient(config),
	}

	// 注册消息处理器
	client.wsClient.RegisterHandler("result", client.handleResult)

	return client
}

// Connect 连接到 ASR 服务器
func (c *WhisperClient) Connect() error {
	return c.wsClient.Connect()
}

// Close 关闭连接
func (c *WhisperClient) Close() error {
	return c.wsClient.Close()
}

// SetGrammar 设置语法
func (c *WhisperClient) SetGrammar(grammar string) error {
	c.grammar = grammar
	req := models.WhisperRequest{
		Grammar: grammar,
	}
	return c.wsClient.SendMessage(req)
}

// SendAudioFrame 发送音频帧
func (c *WhisperClient) SendAudioFrame(audio []byte) error {
	req := models.WhisperRequest{
		Data: struct {
			Status   int    `json:"status"`
			Format   string `json:"format"`
			Audio    string `json:"audio"`
			Encoding string `json:"encoding"`
		}{
			Status:   1, // 中间帧
			Format:   "audio/L16;rate=16000",
			Audio:    base64.StdEncoding.EncodeToString(audio),
			Encoding: "raw",
		},
	}
	return c.wsClient.SendMessage(req)
}

// SendEndFrame 发送结束帧
func (c *WhisperClient) SendEndFrame() error {
	req := models.WhisperRequest{
		Data: struct {
			Status   int    `json:"status"`
			Format   string `json:"format"`
			Audio    string `json:"audio"`
			Encoding string `json:"encoding"`
		}{
			Status:   2, // 结束帧
			Format:   "audio/L16;rate=16000",
			Audio:    "",
			Encoding: "raw",
		},
	}
	return c.wsClient.SendMessage(req)
}

// handleResult 处理识别结果
func (c *WhisperClient) handleResult(message []byte) error {
	var resp models.WhisperResponse
	if err := json.Unmarshal(message, &resp); err != nil {
		return err
	}

	log.Printf("收到识别结果: %s", resp.Text)
	return nil
}
