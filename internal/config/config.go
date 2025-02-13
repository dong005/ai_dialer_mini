// Package config 提供配置加载和管理功能
package config

import (
	"fmt"
	"os"
	"time"

	"ai_dialer_mini/internal/clients/ollama"
	"ai_dialer_mini/internal/clients/xfyun"

	"gopkg.in/yaml.v3"
)

var globalConfig *Config

// Config 应用程序配置结构
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	FreeSWITCH FreeSWITCHConfig `yaml:"freeswitch"`
	XFYun      xfyun.Config    `yaml:"xfyun"`
	Ollama     ollama.Config   `yaml:"ollama"`
	WebSocket  WebSocketConfig  `yaml:"websocket"`
	MySQL      MySQLConfig      `yaml:"mysql"`
	Redis      RedisConfig      `yaml:"redis"`
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

// WebSocketConfig WebSocket配置
type WebSocketConfig struct {
	ReadBufferSize  int           `yaml:"read_buffer_size"`  // 读缓冲区大小
	WriteBufferSize int           `yaml:"write_buffer_size"` // 写缓冲区大小
	PingPeriod      time.Duration `yaml:"ping_period"`       // 心跳间隔
	PongWait        time.Duration `yaml:"pong_wait"`         // 等待Pong响应的超时时间
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
	if config.Server.Port <= 0 {
		return fmt.Errorf("服务器端口必须大于0")
	}

	// 验证WebSocket配置
	if config.WebSocket.ReadBufferSize <= 0 {
		return fmt.Errorf("WebSocket读缓冲区大小必须大于0")
	}
	if config.WebSocket.WriteBufferSize <= 0 {
		return fmt.Errorf("WebSocket写缓冲区大小必须大于0")
	}

	return nil
}
