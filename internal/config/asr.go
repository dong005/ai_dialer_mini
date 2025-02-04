package config

// ASRConfig 科大讯飞ASR配置
type ASRConfig struct {
    APPID     string // 应用ID
    APISecret string // API密钥
    APIKey    string // API密钥
}

// NewASRConfig 创建新的ASR配置
func NewASRConfig() *ASRConfig {
    return &ASRConfig{
        APPID:     "c0de4f24",
        APISecret: "NWRhZDBkNzA5ZDQxNGMzYmQ1NWMwMWNh",
        APIKey:    "51012a35448538a8396dc564cf050f68",
    }
}

// Validate 验证ASR配置
func (c *ASRConfig) Validate() error {
    if c.APPID == "" {
        return ErrEmptyAppID
    }
    if c.APIKey == "" {
        return ErrEmptyAPIKey
    }
    if c.APISecret == "" {
        return ErrEmptyAPISecret
    }
    return nil
}
