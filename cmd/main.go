// Package main 程序入口
package main

import (
	"context"
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
	"ai_dialer_mini/internal/servers/ws"

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

	// 创建FreeSWITCH客户端配置
	fsConfig := freeswitch.Config{
		Host:     cfg.FreeSWITCH.Host,
		Port:     cfg.FreeSWITCH.Port,
		Password: cfg.FreeSWITCH.Password,
	}

	// 创建FreeSWITCH客户端
	client := freeswitch.NewClient(fsConfig)

	// 注册事件处理器
	client.RegisterHandler("CHANNEL_CREATE", func(headers map[string]string) error {
		log.Printf("新通道创建: %s\n", headers["Channel-Name"])
		return nil
	})

	client.RegisterHandler("CHANNEL_ANSWER", func(headers map[string]string) error {
		log.Printf("通道应答: %s\n", headers["Channel-Name"])
		return nil
	})

	client.RegisterHandler("CHANNEL_HANGUP", func(headers map[string]string) error {
		log.Printf("通道挂断: %s, 原因: %s\n", headers["Channel-Name"], headers["Hangup-Cause"])
		return nil
	})

	// 连接到FreeSWITCH
	log.Println("正在连接到FreeSWITCH...")
	if err := client.Connect(); err != nil {
		log.Fatalf("连接FreeSWITCH失败: %v\n", err)
	}
	defer client.Close()

	// 订阅事件
	log.Println("正在订阅FreeSWITCH事件...")
	if err := client.SubscribeEvents(); err != nil {
		log.Fatalf("订阅事件失败: %v\n", err)
	}

	// 创建Gin引擎
	engine := gin.Default()

	// 注册中间件
	middleware.RegisterMiddleware(engine)

	// 注册路由
	handlers.RegisterRoutes(engine, client)

	// 创建并启动 ASR 服务器
	asrServer := ws.NewASRServer()
	engine.GET("/asr", func(c *gin.Context) {
		asrServer.ServeHTTP(c.Writer, c.Request)
	})

	// 创建HTTP服务器
	srv := &http.Server{
		Addr:    cfg.Server.Address,
		Handler: engine,
	}

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("HTTP服务器正在启动，监听地址: %s\n", cfg.Server.Address)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP服务器启动失败: %v\n", err)
		}
	}()

	<-quit
	log.Println("正在关闭服务器...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("服务器关闭失败: %v\n", err)
	}

	log.Println("服务器已关闭")
}
