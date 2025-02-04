package config

import "errors"

// 配置相关错误
var (
    ErrEmptyHost     = errors.New("FreeSWITCH主机地址不能为空")
    ErrEmptyPort     = errors.New("FreeSWITCH端口不能为空")
    ErrEmptyPassword = errors.New("FreeSWITCH密码不能为空")
    ErrEmptyAppID    = errors.New("科大讯飞AppID不能为空")
    ErrEmptyAPIKey   = errors.New("科大讯飞APIKey不能为空")
    ErrEmptyAPISecret = errors.New("科大讯飞APISecret不能为空")
)
