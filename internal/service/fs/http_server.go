package fs

import (
	"ai_dialer_mini/internal/config"
	"ai_dialer_mini/internal/service/asr"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// 存储ASR会话
var asrSessions sync.Map

// HTTPServer HTTP服务器
type HTTPServer struct {
	server     *http.Server
	wsHandler  *WebSocketHandler
	asrService *asr.XFYunASR
}

// NewHTTPServer 创建HTTP服务器
func NewHTTPServer(addr string, asrService *asr.XFYunASR) *HTTPServer {
	wsHandler := NewWebSocketHandler(asrService)

	mux := http.NewServeMux()
	mux.Handle("/ws", wsHandler)
	mux.HandleFunc("/audio", wsHandler.handleAudio)
	mux.HandleFunc("/asr", wsHandler.handleASR)
	mux.HandleFunc("/stream/send", wsHandler.handleSendStream)
	mux.HandleFunc("/stream/recv", wsHandler.handleRecvStream)
	mux.HandleFunc("/stream/callee", wsHandler.handleCalleeStream)

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	return &HTTPServer{
		server:     server,
		wsHandler:  wsHandler,
		asrService: asrService,
	}
}

// Start 启动服务器
func (s *HTTPServer) Start() error {
	log.Printf("[INFO] 正在启动HTTP服务器，监听地址: %s", s.server.Addr)

	if err := s.server.ListenAndServe(); err != http.ErrServerClosed {
		log.Printf("[ERROR] HTTP服务器错误: %v", err)
		return err
	}

	return nil
}

// Stop 停止服务器
func (s *HTTPServer) Stop() error {
	log.Printf("[INFO] 正在停止HTTP服务器")
	return s.server.Shutdown(context.Background())
}

// GetWebSocketHandler 获取WebSocket处理器实例
func (s *HTTPServer) GetWebSocketHandler() *WebSocketHandler {
	return s.wsHandler
}

// handleAudio 处理ASR识别结果
func handleAudio(w http.ResponseWriter, r *http.Request) {
	log.Printf("[DEBUG] 开始处理ASR识别结果: Method=%s, URL=%s", r.Method, r.URL.String())

	// 获取UUID
	uuid := r.URL.Query().Get("uuid")
	if uuid == "" {
		log.Printf("[ERROR] 未收到UUID参数")
		http.Error(w, "Missing UUID", http.StatusBadRequest)
		return
	}

	log.Printf("[INFO] 收到ASR识别结果请求: UUID=%s, Method=%s, ContentType=%s, ContentLength=%d",
		uuid, r.Method, r.Header.Get("Content-Type"), r.ContentLength)

	// 根据不同的HTTP方法处理
	switch r.Method {
	case "POST", "PUT":
		// 读取ASR识别结果
		resultData, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Printf("[ERROR] 读取ASR识别结果失败: %v", err)
			http.Error(w, "Failed to read ASR result", http.StatusInternalServerError)
			return
		}
		defer r.Body.Close()

		// 检查结果数据是否为空
		if len(resultData) == 0 {
			log.Printf("[WARN] 收到空的ASR识别结果")
			w.WriteHeader(http.StatusOK)
			return
		}

		// 打印ASR识别结果
		asrText := string(resultData)
		log.Printf("[INFO] 通话UUID=%s 的ASR识别结果: %s", uuid, asrText)

		// 这里可以添加对ASR结果的进一步处理
		// TODO: 根据需要处理ASR结果

	case "GET":
		// 处理GET请求，这可能是FreeSWITCH的初始化请求
		log.Printf("[DEBUG] 收到GET请求，可能是FreeSWITCH的初始化请求")
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("OK"))
		return

	default:
		log.Printf("[ERROR] 不支持的HTTP方法: %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 返回成功响应
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("OK"))
}

// handleASR 处理科大讯飞ASR识别请求
func handleASR(w http.ResponseWriter, r *http.Request, uuid string) {
	log.Printf("[DEBUG] 开始处理科大讯飞ASR识别请求: Method=%s, URL=%s, UUID=%s", r.Method, r.URL.String(), uuid)

	// 读取请求体中的音频数据
	audioData, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("[ERROR] 读取音频数据失败: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	// 检查是否存在ASR会话
	_, ok := asrSessions.Load(uuid)
	if !ok {
		// 创建新的ASR会话
		asrConfig := config.NewASRConfig()
		asrClient := asr.NewXFYunASR(asrConfig)
		if err := asrClient.Start(uuid); err != nil {
			log.Printf("[ERROR] 启动ASR会话失败: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		asrSessions.Store(uuid, asrClient)
	}

	// 处理音频数据
	if err := processASRAudio(uuid, audioData); err != nil {
		log.Printf("[ERROR] 处理音频数据失败: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// processASRAudio 处理ASR音频数据
func processASRAudio(uuid string, audioData []byte) error {
	session, ok := asrSessions.Load(uuid)
	if !ok {
		return fmt.Errorf("未找到UUID=%s的ASR会话", uuid)
	}

	xfAsr, ok := session.(*asr.XFYunASR)
	if !ok {
		return fmt.Errorf("ASR会话类型错误")
	}

	// 发送音频数据到科大讯飞进行识别
	if err := xfAsr.WriteAudio(uuid, audioData); err != nil {
		return fmt.Errorf("处理音频数据失败: %v", err)
	}

	return nil
}

// processAudioData 处理音频数据
func processAudioData(uuid string, audioData []byte) error {
	// 获取或创建ASR会话
	asrClientInterface, ok := asrSessions.Load(uuid)
	if !ok {
		log.Printf("[INFO] 创建新的ASR会话: UUID=%s", uuid)
		// 创建新的ASR会话
		asrConfig := config.NewASRConfig()
		asrClient := asr.NewXFYunASR(asrConfig)

		// 启动ASR会话
		if err := asrClient.Start(uuid); err != nil {
			log.Printf("[ERROR] 启动ASR会话失败: %v", err)
			return err
		}

		asrClientInterface = asrClient
		asrSessions.Store(uuid, asrClient)

		// 启动结果监听
		go func() {
			for result := range asrClient.GetResults() {
				// 只处理有文本内容的结果
				if len(result.Text) > 0 {
					log.Printf("[INFO] ASR识别结果: UUID=%s, 文本=%s", result.CallID, result.Text)
				}
			}
		}()
	}

	asrClient := asrClientInterface.(*asr.XFYunASR)

	// 分析音频数据
	if len(audioData) > 0 {
		// 检查音频数据的有效性
		var nonZeroCount int
		for _, b := range audioData {
			if b != 0 {
				nonZeroCount++
			}
		}
		log.Printf("[DEBUG] 音频数据分析: 总字节数=%d, 非零字节数=%d, 有效率=%.2f%%",
			len(audioData), nonZeroCount, float64(nonZeroCount)*100/float64(len(audioData)))

		// 发送音频数据到ASR
		if err := asrClient.WriteAudio(uuid, audioData); err != nil {
			log.Printf("[ERROR] 发送音频数据到ASR失败: %v", err)
			return err
		}
		log.Printf("[DEBUG] 成功发送音频数据到ASR: UUID=%s, 数据大小=%d", uuid, len(audioData))
	} else {
		log.Printf("[WARN] 收到空的音频数据: UUID=%s", uuid)
	}

	return nil
}

// handleSendStream 处理主叫方发送的音频流
func handleSendStream(w http.ResponseWriter, r *http.Request) {
	caller := r.URL.Query().Get("caller")
	log.Printf("[DEBUG] 收到主叫方音频流请求: caller=%s", caller)

	// 读取音频数据
	data, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[ERROR] 读取音频数据失败: %v", err)
		http.Error(w, "读取音频数据失败", http.StatusInternalServerError)
		return
	}

	log.Printf("[DEBUG] 收到音频数据: caller=%s, size=%d bytes", caller, len(data))
	w.WriteHeader(http.StatusOK)
}

// handleRecvStream 处理发送给主叫方的音频流
func handleRecvStream(w http.ResponseWriter, r *http.Request) {
	log.Printf("[DEBUG] 收到接收音频流请求")
	w.WriteHeader(http.StatusOK)
}

// handleCalleeStream 处理被叫方的音频流
func handleCalleeStream(w http.ResponseWriter, r *http.Request) {
	leg := r.URL.Query().Get("leg")
	log.Printf("[DEBUG] 收到被叫方音频流请求: leg=%s", leg)

	// 读取音频数据
	data, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[ERROR] 读取音频数据失败: %v", err)
		http.Error(w, "读取音频数据失败", http.StatusInternalServerError)
		return
	}

	log.Printf("[DEBUG] 收到音频数据: leg=%s, size=%d bytes", leg, len(data))
	w.WriteHeader(http.StatusOK)
}
