// Package config 提供配置加载和管理功能
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config 应用程序配置结构
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	FreeSWITCH FreeSWITCHConfig `yaml:"freeswitch"`
	ASR        ASRConfig        `yaml:"asr"`
	MySQL      MySQLConfig      `yaml:"mysql"`
	Redis      RedisConfig      `yaml:"redis"`
}

// ServerConfig HTTP服务器配置
type ServerConfig struct {
	Address string `yaml:"address"` // 服务器监听地址，如 ":8080"
}

// FreeSWITCHConfig FreeSWITCH连接配置
type FreeSWITCHConfig struct {
	Host     string `yaml:"host"`     // FreeSWITCH主机地址
	Port     int    `yaml:"port"`     // FreeSWITCH端口
	Password string `yaml:"password"` // 认证密码
}

// ASRConfig 语音识别配置
type ASRConfig struct {
	Provider  string `yaml:"provider"`   // 提供商：xfyun, aliyun等
	AppID     string `yaml:"app_id"`     // 应用ID
	APIKey    string `yaml:"api_key"`    // API密钥
	APISecret string `yaml:"api_secret"` // API密钥
	ServerURL string `yaml:"server_url"` // 服务器地址
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

// Load 从配置文件加载配置
func Load(configPath string) (*Config, error) {
	// 获取配置文件的绝对路径
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("获取配置文件绝对路径失败: %v", err)
	}

	// 读取配置文件
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %v", err)
	}

	// 解析YAML
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %v", err)
	}

	// 验证配置
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("配置验证失败: %v", err)
	}

	return &config, nil
}

// validateConfig 验证配置是否有效
func validateConfig(config *Config) error {
	// 验证服务器配置
	if config.Server.Address == "" {
		return fmt.Errorf("服务器地址不能为空")
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
	if config.ASR.Provider == "" {
		return fmt.Errorf("ASR提供商不能为空")
	}
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

	// 验证MySQL配置
	if config.MySQL.Host == "" {
		return fmt.Errorf("MySQL主机地址不能为空")
	}
	if config.MySQL.Port <= 0 {
		return fmt.Errorf("MySQL端口必须大于0")
	}
	if config.MySQL.User == "" {
		return fmt.Errorf("MySQL用户名不能为空")
	}
	if config.MySQL.Database == "" {
		return fmt.Errorf("MySQL数据库名不能为空")
	}

	// 验证Redis配置
	if config.Redis.Host == "" {
		return fmt.Errorf("Redis主机地址不能为空")
	}
	if config.Redis.Port <= 0 {
		return fmt.Errorf("Redis端口必须大于0")
	}
	if config.Redis.DB < 0 {
		return fmt.Errorf("Redis数据库编号不能为负数")
	}

	return nil
}
