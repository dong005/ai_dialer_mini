package fs

import (
	"ai_dialer_mini/internal/service/asr"
	"ai_dialer_mini/internal/service/ws"
	"log"
	"net/http"
)

// WebSocketHandler WebSocket处理器
type WebSocketHandler struct {
	// ASR服务实例
	asrService *asr.XFYunASR
	// WebSocket服务
	wsService *ws.WebSocketService
	// 用于跟踪是否已经处理过WAV文件头
	processedHeaders map[string]bool
}

// NewWebSocketHandler 创建新的WebSocket处理器
func NewWebSocketHandler(asrService *asr.XFYunASR) *WebSocketHandler {
	handler := &WebSocketHandler{
		asrService:       asrService,
		processedHeaders: make(map[string]bool),
	}

	// 创建WebSocket服务
	wsConfig := ws.NewConfig()
	wsConfig.BufferSize = 16384 // 增加缓冲区大小以适应音频数据

	handler.wsService = ws.NewWebSocketService(wsConfig, handler.handleMessage)
	return handler
}

// ServeHTTP 处理HTTP请求
func (h *WebSocketHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	uuid := r.URL.Query().Get("uuid")
	rate := r.URL.Query().Get("rate")
	codec := r.URL.Query().Get("codec")
	log.Printf("[DEBUG] 收到WebSocket请求: 路径=%s, UUID=%s, 采样率=%s, 编码=%s", 
		r.URL.Path, uuid, rate, codec)
	
	h.wsService.HandleWebSocket(w, r)
}

// handleMessage 处理WebSocket消息
func (h *WebSocketHandler) handleMessage(message *ws.Message) error {
	session := message.Session
	sessionID := session.ID

	// 只处理二进制消息
	if message.Type != ws.BinaryMessage {
		log.Printf("[WARN] 收到非二进制消息，会话ID: %s, 类型: %d，跳过处理", 
			sessionID, message.Type)
		return nil
	}

	// 检查数据长度
	if len(message.Data) == 0 {
		log.Printf("[WARN] 收到空数据，会话ID: %s，跳过处理", sessionID)
		return nil
	}

	// 记录接收到的数据大小和会话信息
	log.Printf("[DEBUG] 收到音频数据，会话ID: %s, 大小: %d字节, 前16字节: % x", 
		sessionID, len(message.Data), message.Data[:min(16, len(message.Data))])

	// 检查数据是否是WAV文件头
	if !h.processedHeaders[sessionID] && h.isWAVHeader(message.Data) {
		log.Printf("[DEBUG] 检测到WAV文件头，会话ID: %s，跳过处理WAV头", sessionID)
		h.processedHeaders[sessionID] = true
		return nil
	}

	// 处理音频数据
	if err := h.processAudioData(sessionID, message.Data); err != nil {
		log.Printf("[ERROR] 处理音频数据失败，会话ID: %s, 错误: %v", sessionID, err)
		return err
	}

	return nil
}

// isWAVHeader 检查数据是否是WAV文件头
func (h *WebSocketHandler) isWAVHeader(data []byte) bool {
	if len(data) < 12 {
		return false
	}

	// WAV文件头标识
	riffID := string(data[0:4])
	waveID := string(data[8:12])

	return riffID == "RIFF" && waveID == "WAVE"
}

// processAudioData 处理音频数据
func (h *WebSocketHandler) processAudioData(sessionID string, data []byte) error {
	// 记录音频数据的前几个字节，用于调试
	log.Printf("[DEBUG] 处理音频数据，会话ID: %s，前8字节: % x，总大小: %d字节", 
		sessionID, data[:min(8, len(data))], len(data))

	// 发送到ASR服务
	if err := h.asrService.WriteAudio(sessionID, data); err != nil {
		log.Printf("[ERROR] 发送音频数据到ASR服务失败: %v", err)
		return err
	}

	log.Printf("[DEBUG] 音频数据已发送到ASR服务，会话ID: %s", sessionID)
	return nil
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
