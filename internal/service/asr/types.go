// Package asr 语音识别服务包
package asr

import (
	"strings"

	"ai_dialer_mini/internal/types"
)

// AudioFormat 音频格式
type AudioFormat int

const (
	AudioFormatPCM AudioFormat = iota // PCM格式
	AudioFormatWAV                    // WAV格式
)

// AudioData 音频数据
type AudioData struct {
	Data      []byte    // 音频数据
	Timestamp int64     // 时间戳
	Channel   int       // 通道号
	Format    AudioFormat // 音频格式
	SampleRate int      // 采样率
}

// SessionStatus 会话状态
type SessionStatus int

const (
	SessionStatusIdle    SessionStatus = iota // 空闲
	SessionStatusBusy                         // 忙碌
	SessionStatusClosed                       // 关闭
)

// ASRSession 语音识别会话
type ASRSession struct {
	ID        string        // 会话ID
	CallID    string        // 通话ID
	Status    SessionStatus // 会话状态
	WSSession types.Session // WebSocket会话
}

// ASRService 语音识别服务接口
type ASRService interface {
	Start() error
	Stop()
	CreateSession(id string) *ASRSession
	RemoveSession(id string)
	GetSession(id string) (*ASRSession, bool)
	HandleMessage(session types.Session, msg types.Message) error
}

// RespData 响应数据结构
type RespData struct {
	Sid     string `json:"sid"`
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    Data   `json:"data"`
}

// Data 响应数据
type Data struct {
	Result Result `json:"result"`
	Status int    `json:"status"`
}

// Result 识别结果
type Result struct {
	Ls  bool   `json:"ls"`
	Rg  []int  `json:"rg"`
	Sn  int    `json:"sn"`
	Pgs string `json:"pgs"`
	Ws  []Ws   `json:"ws"`
}

// Ws 词语
type Ws struct {
	Bg int  `json:"bg"`
	Cw []Cw `json:"cw"`
}

// Cw 词语详情
type Cw struct {
	Sc int    `json:"sc"`
	W  string `json:"w"`
}

// Decoder 用于解析返回数据
type Decoder struct {
	results []*Result
}

// Decode 解析识别结果
func (d *Decoder) Decode(result *Result) {
	d.results = append(d.results, result)
}

// String 获取识别结果文本
func (d *Decoder) String() string {
	var text strings.Builder
	for _, result := range d.results {
		for _, ws := range result.Ws {
			for _, cw := range ws.Cw {
				text.WriteString(cw.W)
			}
		}
	}
	return text.String()
}
