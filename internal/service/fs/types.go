package fs

import (
	"net/http"
	"sync"

	"ai_dialer_mini/internal/types"
)

// CallStatus 通话状态
type CallStatus = types.CallStatus

// Event FreeSWITCH事件
type Event struct {
	Name   string
	Data   map[string]string
	CallID string
}

// FSConfig FreeSWITCH配置
type FSConfig struct {
	Host     string
	Port     int
	Password string
}

// AudioData 音频数据
type AudioData struct {
	Data       []byte
	Channel    int
	Timestamp  int64
	SampleRate int
}

// FSClient FreeSWITCH客户端
type FSClient struct {
	sync.RWMutex
	config       *FSConfig
	conn         *eventsocket.Conn
	eventHandler func(event *Event)
	audioHandler func(audio *AudioData)
	isConnected  bool
}

// FSService FreeSWITCH服务接口
type FSService interface {
	// Connect 连接到FreeSWITCH
	Connect() error
	// Disconnect 断开连接
	Disconnect() error
	// SendCommand 发送命令
	SendCommand(cmd string) (string, error)
	// SetEventHandler 设置事件处理器
	SetEventHandler(handler func(event *Event))
	// SetAudioHandler 设置音频处理器
	SetAudioHandler(handler func(audio *AudioData))
	// StartCall 发起呼叫
	StartCall(from, to string) error
	// EndCall 结束呼叫
	EndCall(callID string) error
	// IsConnected 检查连接状态
	IsConnected() bool
}
