package models

// ASRService ASR服务接口
type ASRService interface {
	// ProcessAudio 处理音频数据并返回识别结果
	ProcessAudio(sessionID string, audioData []byte) (string, error)
	
	// GetDialogHistory 获取对话历史
	GetDialogHistory(sessionID string) []Message
	
	// ClearDialogHistory 清除对话历史
	ClearDialogHistory(sessionID string)
}
