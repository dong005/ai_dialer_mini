package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"ai_dialer_mini/internal/config"
	"ai_dialer_mini/internal/service/asr"
	"ai_dialer_mini/internal/service/fs"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Println("AI 电话系统启动中...")

	// 加载配置
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 连接FreeSWITCH
	fsClient := fs.NewClient(cfg.FreeSWITCH)
	err = fsClient.Connect()
	if err != nil {
		log.Fatalf("连接FreeSWITCH失败: %v", err)
	}
	defer fsClient.Close()

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

	// 创建ASR服务
	asrService := asr.NewXFYunASR(cfg.ASR)
	defer asrService.Close()

	// 启动事件处理
	ctx, cancel := context.WithCancel(context.Background())
	go handleEvents(ctx, fsClient, asrService)
	defer cancel()

	// 启动命令处理
	handleCommands(fsClient)
}

// handleCommands 处理用户输入的命令
func handleCommands(client *fs.Client) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("可用命令:")
	fmt.Println("  call <from> <to> - 发起呼叫")
	fmt.Println("  quit/exit - 退出程序")

	for {
		fmt.Print("> ")
		command, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("读取命令失败: %v", err)
			continue
		}

		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}

		parts := strings.Fields(command)
		switch parts[0] {
		case "call":
			if len(parts) != 3 {
				log.Printf("用法: call <from> <to>")
				continue
			}
			from, to := parts[1], parts[2]
			if err := client.HandleCall(from, to); err != nil {
				log.Printf("发起呼叫失败: %v", err)
			}
		case "quit", "exit":
			return
		case "help":
			fmt.Println("可用命令:")
			fmt.Println("  call <from> <to> - 发起呼叫")
			fmt.Println("  quit/exit - 退出程序")
		default:
			log.Printf("未知命令: %s", command)
		}
	}
}

// handleEvents 处理FreeSWITCH事件
func handleEvents(ctx context.Context, fsClient *fs.Client, asrClient *asr.XFYunASR) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-fsClient.GetEvents():
			log.Printf("收到事件: %s", event.EventName)

			switch event.EventName {
			case "CHANNEL_CREATE":
				log.Printf("通道创建: 主叫=%s, 被叫=%s", event.CallerNumber, event.CalledNumber)

			case "CHANNEL_ANSWER":
				log.Printf("通话应答: 主叫=%s, 被叫=%s", event.CallerNumber, event.CalledNumber)

				// 启动ASR会话
				if err := asrClient.Start(event.CallID); err != nil {
					log.Printf("启动ASR会话失败: %v", err)
				} else {
					log.Printf("ASR会话启动成功: callID=%s", event.CallID)
				}

			case "CHANNEL_HANGUP":
				log.Printf("通话挂断: 主叫=%s, 被叫=%s", event.CallerNumber, event.CalledNumber)
				asrClient.Stop(event.CallID)

			case "CHANNEL_DESTROY":
				log.Printf("通道销毁: 主叫=%s, 被叫=%s", event.CallerNumber, event.CalledNumber)

			case "CUSTOM":
				// 处理媒体事件
				if event.Headers["Event-Subclass"] == "sofia::media" {
					// 获取音频数据
					if audioData := event.Headers["Media-Data"]; audioData != "" {
						// 发送到ASR服务
						if err := asrClient.WriteAudio(event.CallID, []byte(audioData)); err != nil {
							log.Printf("发送音频数据失败: %v", err)
						}
					}
				}
			}
		case result := <-asrClient.GetResults():
			// 处理ASR结果
			if result != nil {
				log.Printf("【ASR识别结果】通话ID: %s, 文本: %s", result.CallID, result.Text)
			}
		}
	}
}
