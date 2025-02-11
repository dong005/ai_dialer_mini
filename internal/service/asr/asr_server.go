package asr

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// 科大讯飞ASR配置
	hostURL string = "wss://iat-api.xfyun.cn/v2/iat"
	appID   string = "c0de4f24"
	apiSecret string = "NWRhZDBkNzA5ZDQxNGMzYmQ1NWMwMWNh"
	apiKey   string = "51012a35448538a8396dc564cf050f68"

	// 音频状态常量
	statusFirstFrame    int = 0
	statusContinueFrame int = 1
	statusLastFrame     int = 2
)

// ASRServer 处理音频流并进行实时语音识别
type ASRServer struct {
	sync.Mutex
	wsConn     *websocket.Conn
	ctx        context.Context
	cancel     context.CancelFunc
	frameSize  int // 每帧音频大小
	resultChan chan string
}

// NewASRServer 创建新的ASR服务器实例
func NewASRServer() (*ASRServer, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// 建立WebSocket连接
	d := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}
	conn, resp, err := d.Dial(assembleAuthUrl(hostURL, apiKey, apiSecret), nil)
	if err != nil {
		cancel()
		if resp != nil {
			return nil, fmt.Errorf("websocket连接失败: %s %v", readResp(resp), err)
		}
		return nil, fmt.Errorf("websocket连接失败: %v", err)
	}

	server := &ASRServer{
		wsConn:     conn,
		ctx:        ctx,
		cancel:     cancel,
		frameSize:  1280,
		resultChan: make(chan string, 100),
	}

	// 启动goroutine接收识别结果
	go server.receiveResult()

	return server, nil
}

// StartServer 启动HTTP服务器接收音频流
func (s *ASRServer) StartServer(addr string) error {
	http.HandleFunc("/stream", s.handleAudioStream)
	return http.ListenAndServe(addr, nil)
}

// handleAudioStream 处理音频流请求
func (s *ASRServer) handleAudioStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只支持POST请求", http.StatusMethodNotAllowed)
		return
	}

	status := statusFirstFrame
	buffer := make([]byte, s.frameSize)

	for {
		n, err := r.Body.Read(buffer)
		if err != nil {
			if err == io.EOF {
				// 发送最后一帧
				s.sendFrame(buffer[:n], statusLastFrame)
				break
			}
			fmt.Printf("读取音频流错误: %v\n", err)
			break
		}

		// 发送音频帧
		if status == statusFirstFrame {
			s.sendFrame(buffer[:n], statusFirstFrame)
			status = statusContinueFrame
		} else {
			s.sendFrame(buffer[:n], statusContinueFrame)
		}
	}
}

// sendFrame 发送音频帧到科大讯飞
func (s *ASRServer) sendFrame(audioData []byte, status int) error {
	s.Lock()
	defer s.Unlock()

	frameData := map[string]interface{}{
		"common": map[string]interface{}{
			"app_id": appID,
		},
		"business": map[string]interface{}{
			"language": "zh_cn",
			"domain":   "iat",
			"accent":   "mandarin",
		},
		"data": map[string]interface{}{
			"status": status,
			"format": "audio/L16;rate=16000",
			"audio":  base64.StdEncoding.EncodeToString(audioData),
		},
	}

	// 只在第一帧发送common和business参数
	if status != statusFirstFrame {
		delete(frameData, "common")
		delete(frameData, "business")
	}

	return s.wsConn.WriteJSON(frameData)
}

// receiveResult 接收并处理识别结果
func (s *ASRServer) receiveResult() {
	decoder := &Decoder{}

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			_, msg, err := s.wsConn.ReadMessage()
			if err != nil {
				fmt.Printf("读取消息错误: %v\n", err)
				continue
			}

			var resp RespData
			err = json.Unmarshal(msg, &resp)
			if err != nil {
				fmt.Printf("解析响应错误: %v\n", err)
				continue
			}

			if resp.Code != 0 {
				fmt.Printf("识别错误: %s\n", resp.Message)
				continue
			}

			decoder.Decode(&resp.Data.Result)
			result := decoder.String()
			if result != "" {
				fmt.Printf("识别结果: %s\n", result)
				s.resultChan <- result
			}
		}
	}
}

// Close 关闭ASR服务器
func (s *ASRServer) Close() {
	s.cancel()
	if s.wsConn != nil {
		s.wsConn.Close()
	}
	close(s.resultChan)
}

// assembleAuthUrl 创建鉴权URL
func assembleAuthUrl(hosturl string, apiKey, apiSecret string) string {
	ul, err := url.Parse(hosturl)
	if err != nil {
		fmt.Printf("url解析失败: %v\n", err)
		return ""
	}
	now := time.Now().Unix()
	param := url.Values{}
	param.Set("host", ul.Host)
	param.Set("date", fmt.Sprintf("%d", now))

	signString := fmt.Sprintf("host: %s\ndate: %d\nGET %s HTTP/1.1",
		ul.Host, now, ul.Path)
	signature := hmacWithShaTobase64(signString, apiSecret)

	authorization := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("api_key=\"%s\", algorithm=\"%s\", headers=\"%s\", signature=\"%s\"",
		apiKey, "hmac-sha256", "host date request-line", signature)))

	param.Set("authorization", authorization)
	return hosturl + "?" + param.Encode()
}

// hmacWithShaTobase64 HMAC-SHA256加密
func hmacWithShaTobase64(data, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(data))
	encodeData := mac.Sum(nil)
	return base64.StdEncoding.EncodeToString(encodeData)
}

// readResp 读取HTTP响应
func readResp(resp *http.Response) string {
	if resp == nil {
		return ""
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("读取响应失败: %v\n", err)
		return ""
	}
	return fmt.Sprintf("code=%d,body=%s", resp.StatusCode, string(b))
}

type RespData struct {
	Sid     string `json:"sid"`
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    Data   `json:"data"`
}

type Data struct {
	Result Result `json:"result"`
	Status int    `json:"status"`
}

type Result struct {
	Ls  bool   `json:"ls"`
	Rg  []int  `json:"rg"`
	Sn  int    `json:"sn"`
	Pgs string `json:"pgs"`
	Ws  []Ws   `json:"ws"`
}

type Ws struct {
	Bg int  `json:"bg"`
	Cw []Cw `json:"cw"`
}

type Cw struct {
	Sc int    `json:"sc"`
	W  string `json:"w"`
}

type Decoder struct {
	results []*Result
}

func (d *Decoder) Decode(result *Result) {
	d.results = append(d.results, result)
}

func (d *Decoder) String() string {
	var text strings.Builder
	for _, result := range d.results {
		for _, ws := range result.Ws {
			for _, cw := range ws.Cw {
				text.WriteString(cw.W)
			}
		}
	}
	return text.String()
}
