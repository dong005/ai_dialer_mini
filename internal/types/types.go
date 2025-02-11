// Package types 定义基本类型
package types

import "time"

// MessageType 消息类型
type MessageType int

// Message 消息接口
type Message interface {
	Type() MessageType
	Data() interface{}
}

// Session 会话接口
type Session interface {
	ID() string
	Send(msg Message) error
	Close() error
}

// MessageHandler 消息处理器
type MessageHandler interface {
	HandleMessage(session Session, msg Message) error
}

// WebSocketConfig WebSocket配置
type WebSocketConfig struct {
	Host string
	Port int
	Path string
}

// 定义WebSocket消息类型常量
const (
	WSTextMessage MessageType = iota
	WSBinaryMessage
	WSCloseMessage
	WSPingMessage
	WSPongMessage
)

// AudioFrameStatus 音频帧状态
type AudioFrameStatus int

// 定义音频帧状态常量
const (
	AudioFrameStatusNormal AudioFrameStatus = iota
	AudioFrameStatusEnd
	AudioFrameStatusError
)

// CallStatus 通话状态
type CallStatus int

// 定义通话状态常量
const (
	CallStatusIdle CallStatus = iota
	CallStatusRinging
	CallStatusAnswered
	CallStatusHangup
	CallStatusError
)

// ASRStatus 语音识别状态
type ASRStatus int

// 定义语音识别状态常量
const (
	ASRStatusIdle ASRStatus = iota
	ASRStatusBusy
	ASRStatusDone
	ASRStatusError
)

// ASRResult 语音识别结果
type ASRResult struct {
	CallID    string    // 通话ID
	Text      string    // 识别文本
	Status    ASRStatus // 识别状态
	StartTime time.Time // 开始时间
	EndTime   time.Time // 结束时间
	Duration  float64   // 持续时间(秒)
	Error     error     // 错误信息
}
