package handlers

import (
	"ai_dialer_mini/internal/models"
	"github.com/gin-gonic/gin"
)

// RegisterHandlers 注册所有路由和处理器
func RegisterHandlers(r *gin.Engine, wsService models.WSService) {
	// 根路由
	r.GET("/", func(c *gin.Context) {
		c.String(200, "AI Dialer Mini Server Running")
	})

	// 健康检查路由
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
			"time":   c.Request.URL.Query().Get("time"),
		})
	})

	// WebSocket路由
	r.GET("/ws", func(c *gin.Context) {
		wsService.HandleConnection(c)
	})
}
