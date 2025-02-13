package models

import "github.com/gin-gonic/gin"

// WSService WebSocket服务接口
type WSService interface {
	// HandleConnection 处理WebSocket连接
	HandleConnection(c *gin.Context)
	
	// ProcessAudio 处理音频数据
	ProcessAudio(sessionID string, data []byte) (string, error)
}
