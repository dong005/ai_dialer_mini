package ollama_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ai_dialer_mini/internal/clients/ollama"
)

func TestClient_Generate(t *testing.T) {
	// 创建测试服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 检查请求方法
		if r.Method != "POST" {
			t.Errorf("期望POST请求，实际收到%s", r.Method)
		}

		// 检查请求路径
		if r.URL.Path != "/api/generate" {
			t.Errorf("期望路径/api/generate，实际收到%s", r.URL.Path)
		}

		// 检查Content-Type
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("期望Content-Type为application/json，实际收到%s", r.Header.Get("Content-Type"))
		}

		// 解析请求体
		var req ollama.GenerateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("解析请求体失败: %v", err)
		}

		// 返回模拟响应
		resp := ollama.GenerateResponse{
			Model:            "test-model",
			CreatedAt:       time.Now().Format(time.RFC3339),
			Response:        "这是一个测试响应",
			Done:            true,
			TotalDuration:   1000000000, // 1秒
			LoadDuration:    100000000,  // 100毫秒
			PromptEvalCount: 10,
			EvalCount:       20,
			EvalDuration:    500000000, // 500毫秒
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// 创建客户端配置
	config := ollama.Config{
		Host:  server.URL,
		Model: "test-model",
	}

	// 创建客户端
	client := ollama.NewClient(config)

	// 测试用例
	tests := []struct {
		name        string
		prompt      string
		options     ollama.Options
		wantErr     bool
		wantContain string
	}{
		{
			name:   "基本生成测试",
			prompt: "你好",
			options: ollama.Options{
				Temperature: 0.7,
				TopP:       0.9,
				TopK:       40,
				MaxTokens:  100,
			},
			wantErr:     false,
			wantContain: "这是一个测试响应",
		},
	}

	// 运行测试用例
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := client.Generate(tt.prompt, tt.options)
			if (err != nil) != tt.wantErr {
				t.Errorf("Generate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				if resp.Response != tt.wantContain {
					t.Errorf("Generate() = %v, want %v", resp.Response, tt.wantContain)
				}
				if !resp.Done {
					t.Error("Generate() response not done")
				}
				if resp.Model != config.Model {
					t.Errorf("Generate() model = %v, want %v", resp.Model, config.Model)
				}
			}
		})
	}
}

func TestClient_GenerateStream(t *testing.T) {
	// 创建测试服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 检查请求方法和路径
		if r.Method != "POST" || r.URL.Path != "/api/generate" {
			t.Errorf("无效的请求: %s %s", r.Method, r.URL.Path)
			return
		}

		// 设置响应头
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("不支持流式响应")
			return
		}

		// 发送流式响应
		responses := []ollama.GenerateResponse{
			{
				Model:     "test-model",
				Response: "第一部分",
				Done:     false,
			},
			{
				Model:     "test-model",
				Response: "第二部分",
				Done:     false,
			},
			{
				Model:     "test-model",
				Response: "最后部分",
				Done:     true,
			},
		}

		for _, resp := range responses {
			data, err := json.Marshal(resp)
			if err != nil {
				t.Errorf("序列化响应失败: %v", err)
				return
			}
			w.Write(data)
			w.Write([]byte("\n"))
			flusher.Flush()
			time.Sleep(100 * time.Millisecond) // 模拟延迟
		}
	}))
	defer server.Close()

	// 创建客户端配置和客户端
	config := ollama.Config{
		Host:  server.URL,
		Model: "test-model",
	}
	client := ollama.NewClient(config)

	// 测试流式生成
	var responses []string
	err := client.GenerateStream("测试流式生成", ollama.Options{}, func(resp *ollama.GenerateResponse) error {
		responses = append(responses, resp.Response)
		return nil
	})

	// 验证结果
	if err != nil {
		t.Errorf("GenerateStream() error = %v", err)
	}
	if len(responses) != 3 {
		t.Errorf("期望收到3个响应，实际收到%d个", len(responses))
	}
	expectedResponses := []string{"第一部分", "第二部分", "最后部分"}
	for i, want := range expectedResponses {
		if i >= len(responses) {
			t.Errorf("缺少响应#%d", i+1)
			continue
		}
		if responses[i] != want {
			t.Errorf("响应#%d = %v, want %v", i+1, responses[i], want)
		}
	}
}

func TestClient_GenerateErrors(t *testing.T) {
	// 创建测试服务器处理错误情况
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 返回500错误
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("服务器内部错误"))
	}))
	defer server.Close()

	// 创建客户端配置和客户端
	config := ollama.Config{
		Host:  server.URL,
		Model: "test-model",
	}
	client := ollama.NewClient(config)

	// 测试错误处理
	_, err := client.Generate("测试错误处理", ollama.Options{})
	if err == nil {
		t.Error("期望收到错误，但没有收到")
	}

	// 测试无效的服务器地址
	invalidClient := ollama.NewClient(ollama.Config{
		Host:  "http://invalid-server",
		Model: "test-model",
	})
	_, err = invalidClient.Generate("测试无效服务器", ollama.Options{})
	if err == nil {
		t.Error("期望收到错误，但没有收到")
	}
}
