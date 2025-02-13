package services

import (
	"sync"
	"time"

	"ai_dialer_mini/internal/clients/ollama"
	"ai_dialer_mini/internal/config"
	"ai_dialer_mini/internal/models"
)

// DialogContext 对话上下文
type DialogContext struct {
	SessionID     string
	History      []models.Message
	LastActivity time.Time
	mu           sync.RWMutex
}

// DialogService 处理对话服务
type DialogService struct {
	ollamaClient *ollama.Client
	sessions     map[string]*DialogContext
	mu           sync.RWMutex
}

// NewDialogService 创建新的对话服务
func NewDialogService(cfg *config.Config) *DialogService {
	ollamaConfig := ollama.Config{
		Host:  cfg.Ollama.Host,
		Model: cfg.Ollama.Model,
	}
	return &DialogService{
		ollamaClient: ollama.NewClient(ollamaConfig),
		sessions:     make(map[string]*DialogContext),
	}
}

// getOrCreateSession 获取或创建会话
func (s *DialogService) getOrCreateSession(sessionID string) *DialogContext {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ctx, exists := s.sessions[sessionID]; exists {
		ctx.LastActivity = time.Now()
		return ctx
	}

	ctx := &DialogContext{
		SessionID:    sessionID,
		History:     make([]models.Message, 0),
		LastActivity: time.Now(),
	}
	s.sessions[sessionID] = ctx
	return ctx
}

// ProcessMessage 处理用户消息
func (s *DialogService) ProcessMessage(sessionID string, text string) (string, error) {
	ctx := s.getOrCreateSession(sessionID)
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	// 添加用户消息到历史记录
	userMsg := models.Message{
		Role:    "user",
		Content: text,
	}
	ctx.History = append(ctx.History, userMsg)

	// 构建提示词
	prompt := s.buildPromptFromHistory(ctx.History)

	// 调用Ollama生成回复
	options := ollama.Options{
		Temperature: 0.7,
		MaxTokens:   2048,
	}
	response, err := s.ollamaClient.Generate(prompt, options)
	if err != nil {
		return "", err
	}

	// 添加助手回复到历史记录
	assistantMsg := models.Message{
		Role:    "assistant",
		Content: response.Response,
	}
	ctx.History = append(ctx.History, assistantMsg)

	return response.Response, nil
}

// buildPromptFromHistory 从历史记录构建提示词
func (s *DialogService) buildPromptFromHistory(history []models.Message) string {
	var prompt string
	for _, msg := range history {
		switch msg.Role {
		case "user":
			prompt += "用户: " + msg.Content + "\n"
		case "assistant":
			prompt += "助手: " + msg.Content + "\n"
		}
	}
	return prompt
}

// GetHistory 获取对话历史
func (s *DialogService) GetHistory(sessionID string) []models.Message {
	ctx := s.getOrCreateSession(sessionID)
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()

	history := make([]models.Message, len(ctx.History))
	copy(history, ctx.History)
	return history
}

// ClearHistory 清除对话历史
func (s *DialogService) ClearHistory(sessionID string) {
	ctx := s.getOrCreateSession(sessionID)
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	ctx.History = make([]models.Message, 0)
}
