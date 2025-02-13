package routes

import (
	"ai_dialer_mini/internal/clients/ollama"
	"ai_dialer_mini/internal/clients/xfyun"
	"ai_dialer_mini/internal/models"
	"time"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册所有路由
func RegisterRoutes(r *gin.Engine, wsService models.WSService, asrConfig xfyun.Config, ollamaConfig ollama.Config) {

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
			"time":   time.Now().Format(time.RFC3339),
		})
	})

	// 注册ASR路由
	RegisterASRRoutes(r, wsService)

	// 注册对话路由
	RegisterDialogRoutes(r, asrConfig, ollamaConfig)
}
