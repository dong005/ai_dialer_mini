// Package config 提供配置加载和管理功能
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

var globalConfig *Config

// Config 应用程序配置结构
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	FreeSWITCH FreeSWITCHConfig `yaml:"freeswitch"`
	ASR        ASRConfig        `yaml:"asr"`
	MySQL      MySQLConfig      `yaml:"mysql"`
	Redis      RedisConfig      `yaml:"redis"`
	Ollama     OllamaConfig     `yaml:"ollama"`
	WebSocket  WebSocketConfig  `yaml:"websocket"`
}

// ServerConfig HTTP服务器配置
type ServerConfig struct {
	Host string `yaml:"host"` // 服务器监听地址
	Port int    `yaml:"port"` // 服务器监听端口
}

// FreeSWITCHConfig FreeSWITCH连接配置
type FreeSWITCHConfig struct {
	Host     string `yaml:"host"`     // FreeSWITCH主机地址
	Port     int    `yaml:"port"`     // FreeSWITCH端口
	Password string `yaml:"password"` // 认证密码
}

// ASRConfig 语音识别配置
type ASRConfig struct {
	AppID             string `yaml:"app_id"`              // 应用ID
	APIKey            string `yaml:"api_key"`             // API密钥
	APISecret         string `yaml:"api_secret"`          // API密钥
	ServerURL         string `yaml:"server_url"`          // 服务器地址
	ReconnectInterval int    `yaml:"reconnect_interval"`  // 重连间隔（秒）
	MaxRetries        int    `yaml:"max_retries"`         // 最大重试次数
	SampleRate        int    `yaml:"sample_rate"`         // 采样率
}

// OllamaConfig Ollama配置
type OllamaConfig struct {
	Host      string `yaml:"host"`       // Ollama服务器地址
	Model     string `yaml:"model"`      // 模型名称
	MaxTokens int    `yaml:"max_tokens"` // 最大生成token数
}

// WebSocketConfig WebSocket配置
type WebSocketConfig struct {
	ReadBufferSize  int           `yaml:"read_buffer_size"`  // 读缓冲区大小
	WriteBufferSize int           `yaml:"write_buffer_size"` // 写缓冲区大小
	PingPeriod      time.Duration `yaml:"ping_period"`       // 心跳间隔
	PongWait        time.Duration `yaml:"pong_wait"`         // 等待Pong响应的超时时间
}

// MySQLConfig MySQL配置
type MySQLConfig struct {
	Host     string `yaml:"host"`     // MySQL主机地址
	Port     int    `yaml:"port"`     // MySQL端口
	User     string `yaml:"user"`     // MySQL用户名
	Password string `yaml:"password"` // MySQL密码
	Database string `yaml:"database"` // 数据库名
}

// RedisConfig Redis配置
type RedisConfig struct {
	Host     string `yaml:"host"`     // Redis主机地址
	Port     int    `yaml:"port"`     // Redis端口
	Password string `yaml:"password"` // Redis密码
	DB       int    `yaml:"db"`      // Redis数据库编号
}

// GetConfig 获取全局配置实例
func GetConfig() *Config {
	return globalConfig
}

// Load 从文件加载配置
func Load(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %v", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %v", err)
	}

	// 设置默认值
	if config.WebSocket.ReadBufferSize == 0 {
		config.WebSocket.ReadBufferSize = 1024
	}
	if config.WebSocket.WriteBufferSize == 0 {
		config.WebSocket.WriteBufferSize = 1024
	}
	if config.WebSocket.PingPeriod == 0 {
		config.WebSocket.PingPeriod = 30 * time.Second
	}
	if config.WebSocket.PongWait == 0 {
		config.WebSocket.PongWait = 60 * time.Second
	}

	// 验证配置
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("配置验证失败: %v", err)
	}

	// 设置全局配置
	globalConfig = &config

	return &config, nil
}

// validateConfig 验证配置是否有效
func validateConfig(config *Config) error {
	// 验证服务器配置
	if config.Server.Host == "" {
		return fmt.Errorf("服务器地址不能为空")
	}
	if config.Server.Port <= 0 {
		return fmt.Errorf("服务器端口必须大于0")
	}

	// 验证FreeSWITCH配置
	if config.FreeSWITCH.Host == "" {
		return fmt.Errorf("FreeSWITCH主机地址不能为空")
	}
	if config.FreeSWITCH.Port <= 0 {
		return fmt.Errorf("FreeSWITCH端口必须大于0")
	}
	if config.FreeSWITCH.Password == "" {
		return fmt.Errorf("FreeSWITCH密码不能为空")
	}

	// 验证ASR配置
	if config.ASR.AppID == "" {
		return fmt.Errorf("ASR应用ID不能为空")
	}
	if config.ASR.APIKey == "" {
		return fmt.Errorf("ASR API密钥不能为空")
	}
	if config.ASR.APISecret == "" {
		return fmt.Errorf("ASR API密钥不能为空")
	}
	if config.ASR.ServerURL == "" {
		return fmt.Errorf("ASR服务器地址不能为空")
	}
	if config.ASR.ReconnectInterval <= 0 {
		config.ASR.ReconnectInterval = 5 // 默认5秒
	}
	if config.ASR.MaxRetries <= 0 {
		config.ASR.MaxRetries = 3 // 默认3次
	}
	if config.ASR.SampleRate <= 0 {
		config.ASR.SampleRate = 16000 // 默认16kHz
	}

	// 验证Ollama配置
	if config.Ollama.Host == "" {
		return fmt.Errorf("Ollama服务器地址不能为空")
	}
	if config.Ollama.Model == "" {
		return fmt.Errorf("Ollama模型名称不能为空")
	}
	if config.Ollama.MaxTokens <= 0 {
		return fmt.Errorf("Ollama最大生成token数必须大于0")
	}

	return nil
}
