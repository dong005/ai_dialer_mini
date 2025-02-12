package asr

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewXunfeiClient(t *testing.T) {
	config := Config{
		AppID:     "test_app_id",
		APIKey:    "test_api_key",
		APISecret: "test_api_secret",
		HostURL:   "wss://iat-api.xfyun.cn/v2/iat",
	}

	client := NewXunfeiClient(config)
	assert.NotNil(t, client)
	assert.Equal(t, config.AppID, client.appID)
	assert.Equal(t, config.APIKey, client.apiKey)
	assert.Equal(t, config.APISecret, client.apiSecret)
	assert.Equal(t, config.HostURL, client.hostURL)
	assert.NotNil(t, client.ctx)
}

func TestXunfeiClientConnect(t *testing.T) {
	// 这里我们需要一个模拟的WebSocket服务器来测试连接
	// 在实际测试中，你可能需要使用httptest.NewServer和gorilla/websocket来创建一个测试服务器
	t.Skip("需要模拟WebSocket服务器的测试")
}

func TestXunfeiClientSendAudioFrame(t *testing.T) {
	// 这里我们需要一个模拟的WebSocket服务器来测试音频帧发送
	t.Skip("需要模拟WebSocket服务器的测试")
}

func TestXunfeiClientReadMessage(t *testing.T) {
	// 这里我们需要一个模拟的WebSocket服务器来测试消息读取
	t.Skip("需要模拟WebSocket服务器的测试")
}

func TestXunfeiASR(t *testing.T) {
	// 创建客户端配置
	config := Config{
		AppID:     "c0de4f24",
		APIKey:    "51012a35448538a8396dc564cf050f68",
		APISecret: "NWRhZDBkNzA5ZDQxNGMzYmQ1NWMwMWNh",
		HostURL:   "wss://iat-api.xfyun.cn/v2/iat",
	}

	// 创建客户端
	client := NewXunfeiClient(config)

	// 设置识别结果回调
	var result string
	client.SetResultCallback(func(text string, isLast bool) error {
		result += text
		fmt.Printf("收到识别结果: %s, 是否最后一个: %v\n", text, isLast)
		return nil
	})

	// 连接到服务器
	if err := client.Connect(); err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer client.Close()

	// 读取音频文件
	audioData, err := os.ReadFile("../../../demo/iat_ws_go_demo/16k_10.pcm")
	if err != nil {
		t.Fatalf("读取音频文件失败: %v", err)
	}

	// 发送音频数据
	frameSize := 1280 // 每帧40ms的音频数据
	for i := 0; i < len(audioData); i += frameSize {
		end := i + frameSize
		if end > len(audioData) {
			end = len(audioData)
		}

		frame := audioData[i:end]
		if err := client.SendAudioFrame(frame); err != nil {
			t.Errorf("发送音频帧失败: %v", err)
		}

		// 每帧间隔40ms
		time.Sleep(40 * time.Millisecond)
	}

	// 发送结束标志
	if err := client.SendEndFrame(); err != nil {
		t.Errorf("发送结束帧失败: %v", err)
	}

	// 等待最终结果
	time.Sleep(2 * time.Second)

	// 验证结果
	if result == "" {
		t.Error("没有收到识别结果")
	} else {
		t.Logf("最终识别结果: %s", result)
	}
}
