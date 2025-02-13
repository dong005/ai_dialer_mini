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

	"ai_dialer_mini/internal/clients/freeswitch"
	"ai_dialer_mini/internal/config"
	"ai_dialer_mini/internal/handlers"
	"ai_dialer_mini/internal/middleware"
	"ai_dialer_mini/internal/services"
	"ai_dialer_mini/internal/services/ws"

	"github.com/gin-gonic/gin"
)

func main() {
	// 配置日志输出
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	// 加载配置文件
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("加载配置文件失败: %v\n", err)
	}

	// 创建FreeSWITCH ESL客户端配置
	fsConfig := freeswitch.ESLConfig{
		Host:     cfg.FreeSWITCH.Host,
		Port:     cfg.FreeSWITCH.Port,
		Password: cfg.FreeSWITCH.Password,
	}

	// 创建FreeSWITCH ESL客户端
	fsClient := freeswitch.NewESLClient(fsConfig)

	// 连接到FreeSWITCH
	if err := fsClient.Connect(); err != nil {
		log.Fatalf("连接FreeSWITCH失败: %v\n", err)
	}
	defer fsClient.Close()

	// 订阅事件
	if err := fsClient.SubscribeEvents(); err != nil {
		log.Fatalf("订阅事件失败: %v\n", err)
	}

	// 创建对话服务
	dialogService := services.NewDialogService(cfg)

	// 创建WebSocket服务
	wsService := ws.NewASRServer(cfg, dialogService)

	// 创建Gin引擎
	r := gin.Default()

	// 注册中间件
	r.Use(middleware.Cors())
	r.Use(middleware.Logger())

	// 注册路由处理器
	handlers.RegisterHandlers(r, wsService)

	// 创建HTTP服务器
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: r,
	}

	// 启动HTTP服务器
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("启动服务器失败: %v\n", err)
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// 优雅关闭服务器
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("服务器关闭失败: %v\n", err)
	}
}
