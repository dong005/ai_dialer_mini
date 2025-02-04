package asr

import (
	"ai_dialer_mini/internal/config"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// 音频数据状态
	StatusFirstFrame    = 0 // 第一帧音频
	StatusContinueFrame = 1 // 中间帧
	StatusLastFrame     = 2 // 最后一帧

	// 默认配置
	DefaultFrameSize = 1280                  // 默认帧大小
	DefaultInterval  = 40 * time.Millisecond // 默认发送间隔
)

// XFYunASR 科大讯飞ASR服务实现
type XFYunASR struct {
	config   *config.ASRConfig
	sessions sync.Map        // 会话管理，key为callID
	results  chan *ASRResult // 识别结果通道
	closed   bool
}

// session 表示一个语音识别会话
type session struct {
	conn      *websocket.Conn
	callID    string
	decoder   *decoder
	status    int
	closeChan chan struct{}
	audioChan chan []byte // 添加音频数据通道
}

// decoder 用于解码科大讯飞WebSocket返回的数据
type decoder struct {
	// 解码器状态
	status int
	// 部分结果缓存
	buffer string
}

// newDecoder 创建新的解码器
func newDecoder() *decoder {
	return &decoder{
		status: 0,
		buffer: "",
	}
}

// decode 解码一帧数据
func (d *decoder) decode(message []byte) (string, error) {
	// TODO: 实现具体的解码逻辑
	return string(message), nil
}

// NewXFYunASR 创建新的科大讯飞ASR服务实例
func NewXFYunASR(config *config.ASRConfig) *XFYunASR {
	return &XFYunASR{
		config:  config,
		results: make(chan *ASRResult, 100),
	}
}

// NewClient 创建新的ASR客户端（别名函数）
func NewClient(config config.ASRConfig) *XFYunASR {
	return NewXFYunASR(&config)
}

// Start 启动语音识别会话
func (x *XFYunASR) Start(callID string) error {
	// 检查callID是否为空
	if callID == "" {
		return fmt.Errorf("callID不能为空")
	}

	// 检查会话是否已存在
	if _, exists := x.sessions.Load(callID); exists {
		return fmt.Errorf("会话已存在: %s", callID)
	}

	// 生成鉴权URL
	wsURL := x.getWebsocketURL()
	log.Printf("科大讯飞WebSocket URL: %s", wsURL)

	// 连接WebSocket
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	// 添加必要的header
	header := http.Header{}
	date := time.Now().UTC().Format(time.RFC1123)
	header.Add("Date", date)
	header.Add("Host", "iat-api.xfyun.cn")

	conn, resp, err := dialer.Dial(wsURL, header)
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("连接科大讯飞WebSocket失败: %v, 响应: %s", err, string(body))
		}
		return fmt.Errorf("连接科大讯飞WebSocket失败: %v", err)
	}

	// 创建会话
	sess := &session{
		conn:      conn,
		callID:    callID,
		decoder:   newDecoder(),
		status:    StatusFirstFrame,
		closeChan: make(chan struct{}),
		audioChan: make(chan []byte, 100), // 添加音频通道，缓冲大小为100
	}
	x.sessions.Store(callID, sess)

	// 启动消息处理
	go x.handleMessages(sess)

	// 发送初始化参数
	initParams := map[string]interface{}{
		"common": map[string]interface{}{
			"app_id": x.config.APPID,
		},
		"business": map[string]interface{}{
			"language": "zh_cn",
			"domain":   "iat",
			"accent":   "mandarin",
			"dwa":      "wpgs",      // 开启动态修正功能
			"pd":       "telephone", // 选择电话模型
			"ptt":      0,           // 标点符号
		},
		"data": map[string]interface{}{
			"status":   0,
			"format":   "audio/L16;rate=8000",
			"encoding": "raw",
		},
	}

	if err := sess.conn.WriteJSON(initParams); err != nil {
		x.Stop(callID)
		return fmt.Errorf("发送初始化参数失败: %v", err)
	}

	log.Printf("已为通话 %s 启动ASR会话", callID)
	return nil
}

// WriteAudio 写入音频数据
func (x *XFYunASR) WriteAudio(callID string, data []byte) error {
	// 获取会话
	sessObj, ok := x.sessions.Load(callID)
	if !ok {
		return fmt.Errorf("会话不存在: %s", callID)
	}
	sess := sessObj.(*session)

	// 发送音频数据
	select {
	case sess.audioChan <- data:
		return nil
	default:
		return fmt.Errorf("音频通道已满")
	}
}

// handleMessages 处理WebSocket消息
func (x *XFYunASR) handleMessages(sess *session) {
	// 启动音频发送协程
	go x.sendAudio(sess)

	// 创建心跳定时器
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	// 处理接收到的消息
	for {
		select {
		case <-sess.closeChan:
			return
		case <-heartbeat.C:
			// 发送心跳包
			err := sess.conn.WriteJSON(map[string]interface{}{
				"data": map[string]interface{}{
					"status":   StatusContinueFrame,
					"format":   "audio/L16;rate=8000",
					"encoding": "raw",
					"audio":    "", // 空音频数据作为心跳
				},
			})
			if err != nil {
				log.Printf("发送心跳包失败: %v", err)
				x.Stop(sess.callID)
				return
			}
		default:
			// 设置读取超时
			sess.conn.SetReadDeadline(time.Now().Add(20 * time.Second))
			_, message, err := sess.conn.ReadMessage()
			if err != nil {
				if !x.closed {
					log.Printf("读取WebSocket消息失败: %v", err)
				}
				x.Stop(sess.callID)
				return
			}

			// 重置读取超时
			sess.conn.SetReadDeadline(time.Time{})

			// 解析消息
			var resp map[string]interface{}
			if err := json.Unmarshal(message, &resp); err != nil {
				log.Printf("解析消息失败: %v", err)
				continue
			}

			// 处理识别结果
			if data, ok := resp["data"].(map[string]interface{}); ok {
				if result, ok := data["result"].(map[string]interface{}); ok {
					if text, ok := result["ws"].([]interface{}); ok {
						var words []string
						for _, item := range text {
							if w, ok := item.(map[string]interface{}); ok {
								if cw, ok := w["cw"].([]interface{}); ok {
									for _, cwItem := range cw {
										if word, ok := cwItem.(map[string]interface{}); ok {
											if w, ok := word["w"].(string); ok {
												words = append(words, w)
											}
										}
									}
								}
							}
						}
						if len(words) > 0 {
							result := strings.Join(words, "")
							log.Printf("[%s] 识别结果: %s", sess.callID, result)

							// 发送结果到通道
							select {
							case x.results <- &ASRResult{
								CallID: sess.callID,
								Text:   result,
							}:
							default:
								log.Printf("结果通道已满，丢弃结果: %s", result)
							}
						}
					}
				}
			}
		}
	}
}

// sendAudio 发送音频数据
func (x *XFYunASR) sendAudio(sess *session) {
	ticker := time.NewTicker(DefaultInterval)
	defer ticker.Stop()

	buffer := make([]byte, 0, DefaultFrameSize)

	for {
		select {
		case <-sess.closeChan:
			return
		case data := <-sess.audioChan:
			buffer = append(buffer, data...)

			// 当缓冲区达到一定大小时发送
			for len(buffer) >= DefaultFrameSize {
				// 准备发送的数据
				frame := buffer[:DefaultFrameSize]
				buffer = buffer[DefaultFrameSize:]

				// 发送音频数据
				err := sess.conn.WriteJSON(map[string]interface{}{
					"data": map[string]interface{}{
						"status":   StatusContinueFrame,
						"format":   "audio/L16;rate=8000",
						"encoding": "raw",
						"audio":    base64.StdEncoding.EncodeToString(frame),
					},
				})
				if err != nil {
					log.Printf("发送音频数据失败: %v", err)
					x.Stop(sess.callID)
					return
				}
			}
		case <-ticker.C:
			// 定期检查并发送剩余数据
			if len(buffer) > 0 {
				err := sess.conn.WriteJSON(map[string]interface{}{
					"data": map[string]interface{}{
						"status":   StatusContinueFrame,
						"format":   "audio/L16;rate=8000",
						"encoding": "raw",
						"audio":    base64.StdEncoding.EncodeToString(buffer),
					},
				})
				if err != nil {
					log.Printf("发送音频数据失败: %v", err)
					x.Stop(sess.callID)
					return
				}
				buffer = buffer[:0]
			}
		}
	}
}

// Stop 停止语音识别会话
func (x *XFYunASR) Stop(callID string) error {
	// 获取会话
	sessObj, ok := x.sessions.Load(callID)
	if !ok {
		return nil
	}
	sess := sessObj.(*session)

	// 发送最后一帧
	if sess.conn != nil {
		sess.conn.WriteJSON(map[string]interface{}{
			"data": map[string]interface{}{
				"status": StatusLastFrame,
			},
		})
		sess.conn.Close()
	}

	// 关闭通道
	close(sess.closeChan)
	close(sess.audioChan)

	// 删除会话
	x.sessions.Delete(callID)
	log.Printf("已关闭通话 %s 的ASR会话", callID)
	return nil
}

// Close 关闭服务
func (x *XFYunASR) Close() error {
	x.closed = true
	x.sessions.Range(func(key, value interface{}) bool {
		x.Stop(key.(string))
		return true
	})
	close(x.results)
	return nil
}

// getWebsocketURL 生成WebSocket URL
func (x *XFYunASR) getWebsocketURL() string {
	host := "iat-api.xfyun.cn"
	path := "/v2/iat"
	date := time.Now().UTC().Format(time.RFC1123)

	// 生成签名字符串
	signString := fmt.Sprintf("host: %s\ndate: %s\nGET %s HTTP/1.1", host, date, path)
	log.Printf("签名字符串: %s", signString)

	// 计算签名
	hash := hmac.New(sha256.New, []byte(x.config.APISecret))
	hash.Write([]byte(signString))
	signature := base64.StdEncoding.EncodeToString(hash.Sum(nil))

	// 生成鉴权字符串
	authorization := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf(
		"hmac username=\"%s\", algorithm=\"hmac-sha256\", headers=\"host date request-line\", signature=\"%s\"",
		x.config.APIKey,
		signature,
	)))

	// 构建URL
	v := url.Values{}
	v.Add("authorization", authorization)
	v.Add("date", date)
	v.Add("host", host)

	return fmt.Sprintf("wss://%s%s?%s", host, path, v.Encode())
}

// GetResults 获取识别结果通道
func (x *XFYunASR) GetResults() <-chan *ASRResult {
	return x.results
}

// ASRResult 语音识别结果
type ASRResult struct {
	CallID string // 通话ID
	Text   string // 识别文本
}
