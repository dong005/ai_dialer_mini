package routes

import (
	"ai_dialer_mini/internal/config"
	"ai_dialer_mini/internal/models"
	"ai_dialer_mini/internal/services"
	"ai_dialer_mini/internal/services/ws"

	"github.com/gin-gonic/gin"
)

// InitASRRoutes 初始化ASR相关路由
func InitASRRoutes(engine *gin.Engine) {
	cfg := config.GetConfig()
	dialogService := services.NewDialogService(cfg)
	wsService := ws.NewASRServer(cfg, dialogService)

	// 注册路由
	RegisterASRRoutes(engine, wsService)
}

// RegisterASRRoutes 注册ASR相关路由
func RegisterASRRoutes(r *gin.Engine, wsService models.WSService) {
	// 注册WebSocket路由
	r.GET("/ws", func(c *gin.Context) {
		wsService.HandleConnection(c)
	})
}
