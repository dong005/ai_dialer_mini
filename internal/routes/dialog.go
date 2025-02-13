package routes

import (
	"ai_dialer_mini/internal/clients/ollama"
	"ai_dialer_mini/internal/clients/xfyun"
	"ai_dialer_mini/internal/handlers"

	"github.com/gin-gonic/gin"
)

// RegisterDialogRoutes 注册对话相关路由
func RegisterDialogRoutes(r *gin.Engine, asrConfig xfyun.Config, ollamaConfig ollama.Config) {
	// 创建处理器
	dialogHandler := handlers.NewDialogHandler(asrConfig, ollamaConfig)

	// 注册WebSocket路由
	r.GET("/", dialogHandler.HandleWebSocket)
}
