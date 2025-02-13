package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ai_dialer_mini/internal/config"
	"ai_dialer_mini/internal/middleware"
	"ai_dialer_mini/internal/routes"
	"ai_dialer_mini/internal/services"
	"ai_dialer_mini/internal/services/ws"

	"github.com/gin-gonic/gin"
)

func main() {
	// 配置日志输出
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Println("开始初始化服务...")

	// 加载配置文件
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("加载配置文件失败: %v\n", err)
	}
	log.Println("配置文件加载成功")

	// 创建对话服务
	dialogService := services.NewDialogService(cfg)
	if dialogService == nil {
		log.Println("警告: 对话服务初始化失败")
	} else {
		log.Println("对话服务初始化成功")
	}

	// 创建WebSocket服务
	wsService := ws.NewASRServer(cfg, dialogService)
	if wsService == nil {
		log.Println("警告: WebSocket服务初始化失败")
	} else {
		log.Println("WebSocket服务初始化成功")
	}

	// 创建Gin引擎
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	log.Println("Gin引擎创建成功")

	// 注册中间件
	r.Use(middleware.Cors())
	r.Use(middleware.Logger())
	log.Println("中间件注册成功")

	// 注册所有路由
	routes.RegisterRoutes(r, wsService, cfg.XFYun, cfg.Ollama)
	log.Println("路由注册成功")

	// 创建HTTP服务器
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("正在启动服务器，监听地址: %s\n", addr)

	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	// 启动HTTP服务器
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("服务器运行出错: %v\n", err)
			os.Exit(1)
		}
	}()

	log.Println("服务器启动成功")

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("收到关闭信号，正在关闭服务器...")

	// 设置关闭超时
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("服务器关闭失败: %v\n", err)
	}

	log.Println("服务器已关闭")
}
