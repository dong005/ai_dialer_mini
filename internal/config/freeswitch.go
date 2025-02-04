package config

// FreeSwitchConfig FreeSWITCH配置结构
type FreeSwitchConfig struct {
	Host     string `yaml:"host"`     // FreeSWITCH服务器地址
	Port     int    `yaml:"port"`     // ESL端口
	Password string `yaml:"password"` // ESL密码
}

// NewFreeSwitchConfig 创建新的FreeSWITCH配置
func NewFreeSwitchConfig() *FreeSwitchConfig {
	return &FreeSwitchConfig{
		Host:     "192.168.11.161",
		Port:     8021,
		Password: "ClueCon",
	}
}

// Validate 验证FreeSWITCH配置
func (c *FreeSwitchConfig) Validate() error {
	if c.Host == "" {
		return ErrEmptyHost
	}
	if c.Port <= 0 {
		return ErrEmptyPort
	}
	if c.Password == "" {
		return ErrEmptyPassword
	}
	return nil
}
