package routes

import (
	"ai_dialer_mini/internal/config"
	"ai_dialer_mini/internal/handlers"
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
	handlers.RegisterHandlers(engine, wsService)
}
