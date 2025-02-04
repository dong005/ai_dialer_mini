package asr

// AudioData 音频数据
type AudioData struct {
	Data      []byte // 音频数据
	Channel   int    // 通道号
	Timestamp int64  // 时间戳
}

// ASRService 语音识别服务接口
type ASRService interface {
	// Start 启动语音识别会话
	Start(callID string) error

	// Stop 停止语音识别会话
	Stop(callID string) error

	// ProcessAudio 处理音频数据
	ProcessAudio(callID string, audio *AudioData) error

	// GetResults 获取识别结果通道
	GetResults() <-chan *ASRResult

	// Close 关闭服务
	Close() error
}
