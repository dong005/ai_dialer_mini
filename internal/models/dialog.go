package models

// Message 对话消息
type Message struct {
	Role    string `json:"role"`     // 消息角色：user/assistant
	Content string `json:"content"`  // 消息内容
}

// DialogResponse WebSocket响应消息
type DialogResponse struct {
	Type      string `json:"type"`       // 消息类型：text/error
	Content   string `json:"content"`    // 消息内容
	SessionID string `json:"session_id"` // 会话ID
}

// DialogService 对话服务接口
type DialogService interface {
	// ProcessMessage 处理用户消息并返回回复
	ProcessMessage(sessionID string, text string) (string, error)
	
	// GetHistory 获取对话历史
	GetHistory(sessionID string) []Message
	
	// ClearHistory 清除对话历史
	ClearHistory(sessionID string)
}
