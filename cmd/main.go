package main

import (
	"bufio"
	"context"
	"fmt"
	"go/types"
	"log"
	"os"
	"strings"

	"ai_dialer_mini/internal/service/asr"
	"ai_dialer_mini/internal/service/fs"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Println("AI 电话系统启动中...")

	// 加载配置
	var config types.Config
	if err := loadConfig(&config); err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 初始化服务
	fsClient := fs.New(&config.Freeswitch)
	asrService := asr.New(&config.ASR)

	// 连接FreeSWITCH
	err := fsClient.Connect()
	if err != nil {
		log.Fatalf("连接FreeSWITCH失败: %v", err)
	}
	defer fsClient.Close()

	// 创建ASR服务
	defer asrService.Stop()

	// 启动HTTP服务器
	httpServer := fs.NewHTTPServer(":8080", asrService)
	go func() {
		if err := httpServer.Start(); err != nil {
			log.Fatalf("HTTP服务器启动失败: %v", err)
		}
	}()
	defer httpServer.Stop()

	// 订阅事件
	if err := fsClient.Subscribe([]string{
		"CHANNEL_CREATE",
		"CHANNEL_ANSWER",
		"CHANNEL_HANGUP",
		"CHANNEL_DESTROY",
		"CUSTOM sofia::media",
	}); err != nil {
		log.Fatalf("订阅事件失败: %v", err)
	}

	// 启动事件处理
	ctx, cancel := context.WithCancel(context.Background())
	go handleEvents(ctx, fsClient)
	defer cancel()

	// 启动命令处理
	handleCommands(fsClient)
}

// handleCommands 处理用户输入的命令
func handleCommands(client *fs.Client) {
	reader := bufio.NewReader(os.Stdin)

	// 显示帮助信息
	showHelp()

	// 创建一个通道用于接收命令
	cmdChan := make(chan string)

	// 启动一个goroutine来读取用户输入
	go func() {
		for {
			command, err := reader.ReadString('\n')
			if err != nil {
				if err.Error() != "EOF" {
					log.Printf("[WARN] 读取命令时发生错误: %v", err)
				}
				close(cmdChan)
				return
			}
			cmdChan <- strings.TrimSpace(command)
		}
	}()

	// 主循环处理命令
	for command := range cmdChan {
		if command == "" {
			continue
		}

		parts := strings.Fields(command)
		if len(parts) == 0 {
			continue
		}

		switch parts[0] {
		case "call":
			if len(parts) != 3 {
				log.Printf("[ERROR] 用法错误: call <from> <to>")
				showHelp()
				continue
			}
			from, to := parts[1], parts[2]
			if err := client.HandleCall(from, to); err != nil {
				log.Printf("[ERROR] 发起呼叫失败: %v", err)
			}
		case "quit", "exit":
			log.Println("[INFO] 正在退出程序...")
			return
		case "help":
			showHelp()
		default:
			log.Printf("[WARN] 未知命令: %s", parts[0])
			showHelp()
		}
	}
}

// showHelp 显示帮助信息
func showHelp() {
	fmt.Println("\n可用命令:")
	fmt.Println("  help            - 显示此帮助信息")
	fmt.Println("  quit/exit       - 退出程序")
	fmt.Print("> ")
}

// handleEvents 处理FreeSWITCH事件
func handleEvents(ctx context.Context, fsClient *fs.Client) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-fsClient.GetEvents():
			log.Printf("[DEBUG] 处理事件: %s", event.EventName)

			switch event.EventName {
			case "CHANNEL_CREATE":
				log.Printf("[INFO] 通道创建: 主叫=%s, 被叫=%s, UUID=%s",
					event.CallerNumber, event.CalledNumber, event.CallID)

			case "CHANNEL_ANSWER":
				log.Printf("[INFO] 通话应答: 主叫=%s, 被叫=%s, UUID=%s",
					event.CallerNumber, event.CalledNumber, event.CallID)

			case "CHANNEL_HANGUP":
				log.Printf("[INFO] 通话挂断: 主叫=%s, 被叫=%s, UUID=%s",
					event.CallerNumber, event.CalledNumber, event.CallID)

			case "CHANNEL_DESTROY":
				log.Printf("[INFO] 通道销毁: 主叫=%s, 被叫=%s, UUID=%s", event.CallerNumber, event.CalledNumber, event.CallID)

			}
		}
	}
}
