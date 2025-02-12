package handlers

import (
	"ai_dialer_mini/internal/clients/freeswitch"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册所有路由
func RegisterRoutes(r *gin.Engine, client *freeswitch.Client) {
	// 根路由
	r.GET("/", func(c *gin.Context) {
		c.String(200, "AI Dialer Mini Server Running")
	})

	// 健康检查路由
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"service": "ai_dialer_mini",
		})
	})
}
