package ollama

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Config Ollama客户端配置
type Config struct {
	Host  string // Ollama服务器地址（完整URL）
	Model string // 使用的模型名称
}

// Client Ollama客户端
type Client struct {
	config Config
	client *http.Client
}

// GenerateRequest 生成请求参数
type GenerateRequest struct {
	Model    string  `json:"model"`              // 模型名称
	Prompt   string  `json:"prompt"`             // 提示词
	Stream   bool    `json:"stream,omitempty"`   // 是否流式输出
	Context  []int   `json:"context,omitempty"`  // 上下文
	Options  Options `json:"options,omitempty"`  // 可选参数
}

// Options 生成选项
type Options struct {
	Temperature float64 `json:"temperature,omitempty"` // 温度参数
	TopP        float64 `json:"top_p,omitempty"`      // Top-p采样
	TopK        int     `json:"top_k,omitempty"`      // Top-k采样
	MaxTokens   int     `json:"max_tokens,omitempty"` // 最大生成token数
}

// GenerateResponse 生成响应
type GenerateResponse struct {
	Model              string    `json:"model"`               // 模型名称
	CreatedAt          string    `json:"created_at"`         // 创建时间
	Response          string    `json:"response"`           // 生成的文本
	Context           []int     `json:"context,omitempty"`  // 上下文
	Done              bool      `json:"done"`               // 是否完成
	TotalDuration     int64     `json:"total_duration"`     // 总耗时(纳秒)
	LoadDuration      int64     `json:"load_duration"`      // 加载耗时(纳秒)
	PromptEvalCount   int       `json:"prompt_eval_count"`  // 提示词评估数量
	EvalCount         int       `json:"eval_count"`         // 评估数量
	EvalDuration      int64     `json:"eval_duration"`      // 评估耗时(纳秒)
}

// NewClient 创建新的Ollama客户端
func NewClient(config Config) *Client {
	return &Client{
		config: config,
		client: &http.Client{},
	}
}

// Generate 生成文本
func (c *Client) Generate(prompt string, options Options) (*GenerateResponse, error) {
	// 准备请求体
	reqBody := GenerateRequest{
		Model:   c.config.Model,
		Prompt:  prompt,
		Stream:  false,
		Options: options,
	}

	// 序列化请求体
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %v", err)
	}

	// 构建请求URL
	url := fmt.Sprintf("%s/api/generate", c.config.Host)
	
	// 创建请求
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("服务器返回错误: %s", string(body))
	}

	// 解析响应
	var response GenerateResponse
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&response); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	return &response, nil
}

// GenerateStream 流式生成文本
func (c *Client) GenerateStream(prompt string, options Options, callback func(*GenerateResponse) error) error {
	// 准备请求体
	reqBody := GenerateRequest{
		Model:   c.config.Model,
		Prompt:  prompt,
		Stream:  true,
		Options: options,
	}

	// 序列化请求体
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %v", err)
	}

	// 构建请求URL
	url := fmt.Sprintf("%s/api/generate", c.config.Host)

	// 创建请求
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("服务器返回错误: %s", string(body))
	}

	// 创建解码器
	decoder := json.NewDecoder(resp.Body)

	// 逐行读取响应
	for decoder.More() {
		var response GenerateResponse
		if err := decoder.Decode(&response); err != nil {
			return fmt.Errorf("解析响应失败: %v", err)
		}

		if err := callback(&response); err != nil {
			return fmt.Errorf("处理响应失败: %v", err)
		}

		if response.Done {
			break
		}
	}

	return nil
}
