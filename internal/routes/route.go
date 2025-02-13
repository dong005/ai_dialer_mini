package routes

import (
	"github.com/gin-gonic/gin"
)

// InitRoutes 初始化所有路由
func InitRoutes(engine *gin.Engine) {
	// 初始化ASR路由
	InitASRRoutes(engine)
}
