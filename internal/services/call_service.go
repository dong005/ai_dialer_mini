package services

import (
	"context"
	"fmt"
	"log"

	"ai_dialer_mini/internal/clients/freeswitch"
)

// CallService FreeSWITCH 通话服务接口
type CallService interface {
	// InitiateCall 发起呼叫
	InitiateCall(ctx context.Context, fromNumber, toNumber string) (string, error)
	
	// EndCall 结束呼叫
	EndCall(ctx context.Context, callID string) error
	
	// HandleCallEvent 处理通话事件
	HandleCallEvent(ctx context.Context, eventType string, eventData map[string]string) error
}

// CallServiceImpl FreeSWITCH 通话服务实现
type CallServiceImpl struct {
	fsClient *freeswitch.ESLClient
}

// NewCallService 创建新的通话服务实例
func NewCallService(fsClient *freeswitch.ESLClient) CallService {
	service := &CallServiceImpl{
		fsClient: fsClient,
	}

	// 注册事件处理器
	fsClient.RegisterHandler("CHANNEL_CREATE", func(headers map[string]string) error {
		return service.HandleCallEvent(context.Background(), "CHANNEL_CREATE", headers)
	})

	fsClient.RegisterHandler("CHANNEL_ANSWER", func(headers map[string]string) error {
		return service.HandleCallEvent(context.Background(), "CHANNEL_ANSWER", headers)
	})

	fsClient.RegisterHandler("CHANNEL_HANGUP", func(headers map[string]string) error {
		return service.HandleCallEvent(context.Background(), "CHANNEL_HANGUP", headers)
	})

	return service
}

// InitiateCall 实现发起呼叫
func (s *CallServiceImpl) InitiateCall(ctx context.Context, fromNumber, toNumber string) (string, error) {
	// 构建originate命令
	cmd := fmt.Sprintf("originate user/%s &bridge(user/%s)", fromNumber, toNumber)
	
	// 发送命令
	resp, err := s.fsClient.SendCommand(cmd)
	if err != nil {
		return "", fmt.Errorf("发起呼叫失败: %v", err)
	}

	log.Printf("发起呼叫响应: %s", resp)
	return resp, nil
}

// EndCall 实现结束呼叫
func (s *CallServiceImpl) EndCall(ctx context.Context, callID string) error {
	// 构建hangup命令
	cmd := fmt.Sprintf("uuid_kill %s", callID)
	
	// 发送命令
	resp, err := s.fsClient.SendCommand(cmd)
	if err != nil {
		return fmt.Errorf("结束呼叫失败: %v", err)
	}

	log.Printf("结束呼叫响应: %s", resp)
	return nil
}

// HandleCallEvent 实现通话事件处理
func (s *CallServiceImpl) HandleCallEvent(ctx context.Context, eventType string, headers map[string]string) error {
	// 获取通道名称和UUID
	channelName := headers["Channel-Name"]
	uuid := headers["Unique-ID"]

	switch eventType {
	case "CHANNEL_CREATE":
		log.Printf("新通道创建 - UUID: %s, 通道: %s", uuid, channelName)
	case "CHANNEL_ANSWER":
		log.Printf("通道应答 - UUID: %s, 通道: %s", uuid, channelName)
	case "CHANNEL_HANGUP":
		hangupCause := headers["Hangup-Cause"]
		log.Printf("通道挂断 - UUID: %s, 通道: %s, 原因: %s", uuid, channelName, hangupCause)
	}

	return nil
}
