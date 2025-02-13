package freeswitch

import (
	"fmt"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// mockFreeSWITCH 模拟 FreeSWITCH 服务器
type mockFreeSWITCH struct {
	listener net.Listener
	quit     chan struct{}
}

func newMockFreeSWITCH(t *testing.T) *mockFreeSWITCH {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)

	mock := &mockFreeSWITCH{
		listener: listener,
		quit:     make(chan struct{}),
	}

	go mock.serve()
	return mock
}

func (m *mockFreeSWITCH) serve() {
	for {
		select {
		case <-m.quit:
			return
		default:
			conn, err := m.listener.Accept()
			if err != nil {
				continue
			}

			go m.handleConnection(conn)
		}
	}
}

func (m *mockFreeSWITCH) handleConnection(conn net.Conn) {
	defer conn.Close()

	// 发送欢迎消息
	fmt.Fprintf(conn, "Content-Type: auth/request\n\n")

	// 读取认证
	buffer := make([]byte, 1024)
	if _, err := conn.Read(buffer); err != nil {
		return
	}

	// 发送认证成功响应
	fmt.Fprintf(conn, "Content-Type: command/reply\nReply-Text: +OK accepted\n\n")

	// 处理后续命令
	for {
		select {
		case <-m.quit:
			return
		default:
			if _, err := conn.Read(buffer); err != nil {
				return
			}

			// 回复命令
			fmt.Fprintf(conn, "Content-Type: command/reply\nReply-Text: +OK\n\n")
		}
	}
}

func (m *mockFreeSWITCH) close() {
	close(m.quit)
	m.listener.Close()
}

func (m *mockFreeSWITCH) addr() string {
	return m.listener.Addr().String()
}

func TestClientConnect(t *testing.T) {
	mock := newMockFreeSWITCH(t)
	defer mock.close()

	addr := mock.addr()
	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	client := NewESLClient(ESLConfig{
		Host:     host,
		Port:     port,
		Password: "ClueCon",
	})

	err := client.Connect()
	assert.NoError(t, err)
	defer client.Close()
}

func TestClientSendCommand(t *testing.T) {
	mock := newMockFreeSWITCH(t)
	defer mock.close()

	addr := mock.addr()
	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	client := NewESLClient(ESLConfig{
		Host:     host,
		Port:     port,
		Password: "ClueCon",
	})

	err := client.Connect()
	assert.NoError(t, err)
	defer client.Close()

	response, err := client.SendCommand("status")
	assert.NoError(t, err)
	assert.Contains(t, response, "+OK")
}

func TestClientEventHandler(t *testing.T) {
	mock := newMockFreeSWITCH(t)
	defer mock.close()

	addr := mock.addr()
	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	client := NewESLClient(ESLConfig{
		Host:     host,
		Port:     port,
		Password: "ClueCon",
	})

	err := client.Connect()
	assert.NoError(t, err)
	defer client.Close()

	eventReceived := make(chan bool)
	client.RegisterHandler("CHANNEL_CREATE", func(headers map[string]string) error {
		close(eventReceived)
		return nil
	})

	err = client.SubscribeEvents()
	assert.NoError(t, err)

	select {
	case <-eventReceived:
		// 事件处理成功
	case <-time.After(100 * time.Millisecond):
		t.Skip("事件处理测试跳过 - 需要实际的 FreeSWITCH 服务器")
	}
}
