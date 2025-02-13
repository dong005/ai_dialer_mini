package xfyun_test

import (
	"io/ioutil"
	"testing"
	"time"

	"ai_dialer_mini/internal/clients/xfyun"
	"ai_dialer_mini/internal/models"
)

// MockDialogService 模拟对话服务
type MockDialogService struct{}

// ProcessMessage 处理消息
func (m *MockDialogService) ProcessMessage(sessionID string, message string) (string, error) {
	return "回复", nil
}

// GetHistory 获取历史记录
func (m *MockDialogService) GetHistory(sessionID string) []models.Message {
	return nil
}

// ClearHistory 清除历史记录
func (m *MockDialogService) ClearHistory(sessionID string) {
}

func TestASRClient_ProcessAudio(t *testing.T) {
	// 创建测试配置
	config := xfyun.Config{
		AppID:             "c0de4f24",
		APIKey:            "51012a35448538a8396dc564cf050f68",
		APISecret:         "NWRhZDBkNzA5ZDQxNGMzYmQ1NWMwMWNh",
		ServerURL:         "wss://iat-api.xfyun.cn/v2/iat",
		MaxRetries:        3,
		ReconnectInterval: time.Second,
	}

	t.Logf("ASR配置: %+v", config)

	// 创建ASR客户端
	client := xfyun.NewASRClient(config, &MockDialogService{})

	// 定义测试用例
	tests := []struct {
		name    string
		file    string
		wantErr bool
	}{
		{
			name:    "Process PCM Audio File",
			file:    "../../../demo/iat_ws_go_demo/16k_10.pcm",
			wantErr: false,
		},
		{
			name:    "Process Empty Audio",
			file:    "",
			wantErr: true,
		},
		{
			name:    "Dialog History Management",
			file:    "",
			wantErr: true,
		},
	}

	// 运行测试用例
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var audioData []byte
			var err error

			if tt.file != "" {
				// 读取音频文件
				audioData, err = ioutil.ReadFile(tt.file)
				if err != nil {
					t.Fatalf("读取音频文件失败: %v", err)
				}
				t.Logf("读取音频文件成功，大小: %d 字节", len(audioData))
			}

			// 记录开始时间
			startTime := time.Now()

			// 处理音频
			result, err := client.ProcessAudio("test_session", audioData)

			// 计算处理时间
			processTime := time.Since(startTime)

			// 检查错误
			if (err != nil) != tt.wantErr {
				t.Errorf("ProcessAudio() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			t.Log("音频处理完成")
			if err == nil {
				t.Logf("最终识别结果: %s", result)
				t.Logf("处理时间: %v", processTime)
			}
		})
	}
}
