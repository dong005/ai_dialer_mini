package config

import (
	"os"

	"gopkg.in/yaml.v2"
)

// Config 应用配置结构
type Config struct {
	FreeSWITCH *FreeSwitchConfig `yaml:"freeswitch"` // FreeSWITCH配置
	ASR        *ASRConfig        `yaml:"asr"`        // ASR配置
}

// Load 从YAML文件加载配置
func Load(filename string) (*Config, error) {
	cfg := NewConfig()

	// 读取配置文件
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	// 解析YAML
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// 验证配置
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// NewConfig 创建新的配置实例
func NewConfig() *Config {
	cfg := &Config{
		FreeSWITCH: NewFreeSwitchConfig(),
		ASR:        NewASRConfig(),
	}
	return cfg
}

// Validate 验证所有配置
func (c *Config) Validate() error {
	// 验证FreeSWITCH配置
	if err := c.FreeSWITCH.Validate(); err != nil {
		return err
	}

	// 验证ASR配置
	if err := c.ASR.Validate(); err != nil {
		return err
	}

	return nil
}
