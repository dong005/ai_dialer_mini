package config

import (
	"fmt"
	"io/ioutil"

	"gopkg.in/yaml.v2"
)

// Config 应用配置结构
type Config struct {
	FreeSWITCH *FSConfig        `yaml:"freeswitch"` // FreeSWITCH配置
	ASR        *ASRConfig       `yaml:"asr"`        // ASR配置
	WebSocket  *WebSocketConfig `yaml:"websocket"`  // WebSocket配置
}

// FSConfig FreeSWITCH配置
type FSConfig struct {
	Host     string `yaml:"host"`     // 主机地址
	Port     int    `yaml:"port"`     // 端口
	Password string `yaml:"password"` // 密码
}

// ASRConfig 语音识别配置
type ASRConfig struct {
	AppID     string `yaml:"app_id"`     // 应用ID
	APIKey    string `yaml:"api_key"`    // API密钥
	APISecret string `yaml:"api_secret"` // API密钥
	HostURL   string `yaml:"host_url"`   // 服务地址
}

// WebSocketConfig WebSocket配置
type WebSocketConfig struct {
	Host             string `yaml:"host"`              // 服务器主机
	Port             int    `yaml:"port"`              // 服务器端口
	Path             string `yaml:"path"`              // WebSocket路径
	HandshakeTimeout int    `yaml:"handshake_timeout"` // 握手超时时间(秒)
	WriteTimeout     int    `yaml:"write_timeout"`     // 写超时时间(秒)
	ReadTimeout      int    `yaml:"read_timeout"`      // 读超时时间(秒)
	PingInterval     int    `yaml:"ping_interval"`     // 心跳间隔(秒)
	BufferSize       int    `yaml:"buffer_size"`       // 缓冲区大小
	EnableTLS        bool   `yaml:"enable_tls"`        // 是否启用TLS
	CertFile         string `yaml:"cert_file"`         // TLS证书文件
	KeyFile          string `yaml:"key_file"`          // TLS密钥文件
}

// Load 从YAML文件加载配置
func Load(filename string) (*Config, error) {
	var config Config

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %v", err)
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %v", err)
	}

	return &config, nil
}

// NewConfig 创建新的配置实例
func NewConfig() *Config {
	return &Config{
		FreeSWITCH: &FSConfig{
			Host:     "localhost",
			Port:     8021,
			Password: "ClueCon",
		},
		ASR: &ASRConfig{
			AppID:     "",
			APIKey:    "",
			APISecret: "",
			HostURL:   "wss://iat-api.xfyun.cn/v2/iat",
		},
		WebSocket: &WebSocketConfig{
			Host:             "localhost",
			Port:             8080,
			Path:             "/ws",
			HandshakeTimeout: 10,
			WriteTimeout:     10,
			ReadTimeout:      10,
			PingInterval:     30,
			BufferSize:       4096,
			EnableTLS:        false,
		},
	}
}

// Validate 验证所有配置
func (c *Config) Validate() error {
	if c.FreeSWITCH == nil {
		return fmt.Errorf("FreeSWITCH配置不能为空")
	}
	if c.ASR == nil {
		return fmt.Errorf("ASR配置不能为空")
	}
	if c.WebSocket == nil {
		return fmt.Errorf("WebSocket配置不能为空")
	}
	return nil
}
