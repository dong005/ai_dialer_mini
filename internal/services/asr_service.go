package services

import (
	"log"

	"ai_dialer_mini/internal/clients/xfyun"
	"ai_dialer_mini/internal/config"
	"ai_dialer_mini/internal/models"
)

// ASRService 语音识别服务
type ASRService struct {
	client    *xfyun.ASRClient
	dialogSvc models.DialogService
}

// NewASRService 创建新的ASR服务实例
func NewASRService(cfg *config.Config, dialogSvc models.DialogService) *ASRService {
	// 创建ASR客户端
	client := xfyun.NewASRClient(cfg.XFYun, dialogSvc)

	return &ASRService{
		client:    client,
		dialogSvc: dialogSvc,
	}
}

// ProcessAudio 处理音频数据并返回识别结果
func (s *ASRService) ProcessAudio(sessionID string, audioData []byte) (string, error) {
	result, err := s.client.ProcessAudio(sessionID, audioData)
	if err != nil {
		log.Printf("处理音频失败: %v", err)
		return "", err
	}
	return result, nil
}

// GetDialogHistory 获取对话历史
func (s *ASRService) GetDialogHistory(sessionID string) []models.Message {
	return s.client.GetDialogHistory(sessionID)
}

// ClearDialogHistory 清除对话历史
func (s *ASRService) ClearDialogHistory(sessionID string) {
	s.client.ClearDialogHistory(sessionID)
}
