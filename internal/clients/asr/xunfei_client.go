// Package asr 提供语音识别客户端实现
package asr

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// ResultCallback 识别结果回调函数类型
type ResultCallback func(text string, isLast bool) error

// XunfeiClient 科大讯飞语音识别客户端
type XunfeiClient struct {
	appID     string
	apiKey    string
	apiSecret string
	hostURL   string
	conn      *websocket.Conn
	ctx       context.Context
	cancel    context.CancelFunc
	callback  ResultCallback
}

// Config 科大讯飞语音识别配置
type Config struct {
	AppID     string
	APIKey    string
	APISecret string
	HostURL   string
}

// AudioFrameStatus 音频帧状态
const (
	StatusFirstFrame    = 0 // 第一帧
	StatusContinueFrame = 1 // 中间帧
	StatusLastFrame     = 2 // 最后帧
)

// Response 科大讯飞响应数据结构
type Response struct {
	Sid     string `json:"sid"`
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
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

// NewXunfeiClient 创建新的科大讯飞语音识别客户端
func NewXunfeiClient(config Config) *XunfeiClient {
	ctx, cancel := context.WithCancel(context.Background())
	return &XunfeiClient{
		appID:     config.AppID,
		apiKey:    config.APIKey,
		apiSecret: config.APISecret,
		hostURL:   config.HostURL,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// SetResultCallback 设置识别结果回调
func (c *XunfeiClient) SetResultCallback(callback ResultCallback) {
	c.callback = callback
}

// Connect 连接到科大讯飞WebSocket服务器
func (c *XunfeiClient) Connect() error {
	d := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	// 构建认证URL
	authURL := c.assembleAuthUrl()

	// 建立WebSocket连接
	conn, resp, err := d.Dial(authURL, nil)
	if err != nil {
		if resp != nil {
			body := c.readResp(resp)
			return fmt.Errorf("dial error: %v, response: %s", err, body)
		}
		return fmt.Errorf("dial error: %v", err)
	}
	if resp.StatusCode != 101 {
		body := c.readResp(resp)
		return fmt.Errorf("unexpected status code: %d, response: %s", resp.StatusCode, body)
	}

	c.conn = conn

	// 启动消息读取循环
	go c.readLoop()

	return nil
}

// Close 关闭连接
func (c *XunfeiClient) Close() error {
	c.cancel()
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// SendAudioFrame 发送音频帧
func (c *XunfeiClient) SendAudioFrame(data []byte) error {
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	// 构建请求数据
	req := struct {
		Common struct {
			AppID string `json:"app_id"`
		} `json:"common"`
		Business struct {
			Language string `json:"language"`
			Domain   string `json:"domain"`
			Accent   string `json:"accent"`
		} `json:"business"`
		Data struct {
			Status   int    `json:"status"`
			Format   string `json:"format"`
			Audio    string `json:"audio"`
			Encoding string `json:"encoding"`
		} `json:"data"`
	}{}

	req.Common.AppID = c.appID
	req.Business.Language = "zh_cn"
	req.Business.Domain = "iat"
	req.Business.Accent = "mandarin"
	req.Data.Format = "audio/L16;rate=16000"
	req.Data.Encoding = "raw"
	req.Data.Audio = base64.StdEncoding.EncodeToString(data)
	req.Data.Status = StatusContinueFrame

	// 发送数据
	if err := c.conn.WriteJSON(req); err != nil {
		return fmt.Errorf("write frame error: %v", err)
	}

	return nil
}

// SendEndFrame 发送结束帧
func (c *XunfeiClient) SendEndFrame() error {
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	// 构建结束帧
	req := struct {
		Common struct {
			AppID string `json:"app_id"`
		} `json:"common"`
		Business struct {
			Language string `json:"language"`
			Domain   string `json:"domain"`
			Accent   string `json:"accent"`
		} `json:"business"`
		Data struct {
			Status   int    `json:"status"`
			Format   string `json:"format"`
			Audio    string `json:"audio"`
			Encoding string `json:"encoding"`
		} `json:"data"`
	}{}

	req.Common.AppID = c.appID
	req.Business.Language = "zh_cn"
	req.Business.Domain = "iat"
	req.Business.Accent = "mandarin"
	req.Data.Format = "audio/L16;rate=16000"
	req.Data.Encoding = "raw"
	req.Data.Status = StatusLastFrame

	// 发送数据
	if err := c.conn.WriteJSON(req); err != nil {
		return fmt.Errorf("write end frame error: %v", err)
	}

	return nil
}

// readLoop 读取消息循环
func (c *XunfeiClient) readLoop() {
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			resp, err := c.ReadMessage()
			if err != nil {
				fmt.Printf("读取消息错误: %v\n", err)
				continue
			}

			if resp.Code != 0 {
				fmt.Printf("服务器返回错误: %s\n", resp.Message)
				continue
			}

			// 解析识别结果
			if c.callback != nil {
				var text string
				for _, ws := range resp.Data.Result.Ws {
					for _, cw := range ws.Cw {
						text += cw.W
					}
				}
				if err := c.callback(text, resp.Data.Status == 2); err != nil {
					fmt.Printf("回调处理错误: %v\n", err)
				}
			}
		}
	}
}

// ReadMessage 读取识别结果
func (c *XunfeiClient) ReadMessage() (*Response, error) {
	if c.conn == nil {
		return nil, fmt.Errorf("not connected")
	}

	_, msg, err := c.conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("read message error: %v", err)
	}

	var resp Response
	if err := json.Unmarshal(msg, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response error: %v", err)
	}

	return &resp, nil
}

// assembleAuthUrl 创建鉴权URL
func (c *XunfeiClient) assembleAuthUrl() string {
	ul, err := url.Parse(c.hostURL)
	if err != nil {
		return ""
	}

	date := time.Now().UTC().Format(time.RFC1123)
	date = strings.Replace(date, "UTC", "GMT", -1)

	signString := fmt.Sprintf("host: %s\ndate: %s\nGET %s HTTP/1.1", ul.Host, date, ul.Path)
	signature := c.hmacWithShaTobase64("hmac-sha256", signString, c.apiSecret)

	authorization := fmt.Sprintf("api_key=\"%s\", algorithm=\"%s\", headers=\"%s\", signature=\"%s\"",
		c.apiKey, "hmac-sha256", "host date request-line", signature)

	authorization = base64.StdEncoding.EncodeToString([]byte(authorization))

	query := url.Values{}
	query.Add("authorization", authorization)
	query.Add("date", date)
	query.Add("host", ul.Host)

	return fmt.Sprintf("%s?%s", c.hostURL, query.Encode())
}

// hmacWithShaTobase64 计算HMAC-SHA256签名
func (c *XunfeiClient) hmacWithShaTobase64(algorithm, data, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(data))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// readResp 读取HTTP响应内容
func (c *XunfeiClient) readResp(resp *http.Response) string {
	if resp == nil {
		return ""
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	return string(b)
}
