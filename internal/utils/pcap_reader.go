package utils

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

// PCAPReader 用于读取和解析PCAP文件
type PCAPReader struct {
	filename string
	handle   *pcap.Handle
}

// NewPCAPReader 创建新的PCAP读取器
func NewPCAPReader(filename string) (*PCAPReader, error) {
	// 打开PCAP文件
	handle, err := pcap.OpenOffline(filename)
	if err != nil {
		return nil, fmt.Errorf("打开PCAP文件失败: %v", err)
	}

	return &PCAPReader{
		filename: filename,
		handle:   handle,
	}, nil
}

// Close 关闭PCAP读取器
func (r *PCAPReader) Close() {
	if r.handle != nil {
		r.handle.Close()
	}
}

// reopenHandle 重新打开PCAP文件句柄
func (r *PCAPReader) reopenHandle() error {
	if r.handle != nil {
		r.handle.Close()
	}

	handle, err := pcap.OpenOffline(r.filename)
	if err != nil {
		return fmt.Errorf("重新打开PCAP文件失败: %v", err)
	}

	r.handle = handle
	return nil
}

// ExtractWebSocketHandshake 提取WebSocket握手信息
func (r *PCAPReader) ExtractWebSocketHandshake() (*WebSocketHandshake, error) {
	if err := r.reopenHandle(); err != nil {
		return nil, fmt.Errorf("重新打开PCAP文件失败: %v", err)
	}

	packetSource := gopacket.NewPacketSource(r.handle, r.handle.LinkType())
	
	packetCount := 0
	for packet := range packetSource.Packets() {
		packetCount++
		fmt.Printf("处理数据包 #%d\n", packetCount)

		// 获取TCP层
		tcpLayer := packet.Layer(layers.LayerTypeTCP)
		if tcpLayer == nil {
			// 打印数据包的所有层
			fmt.Printf("数据包 #%d 的层: ", packetCount)
			for _, layer := range packet.Layers() {
				fmt.Printf("%s ", layer.LayerType())
			}
			fmt.Println()

			// 打印原始数据
			fmt.Printf("数据包 #%d 的原始数据: %x\n", packetCount, packet.Data())

			continue
		}

		// 获取TCP负载
		tcp, ok := tcpLayer.(*layers.TCP)
		if !ok || len(tcp.Payload) == 0 {
			fmt.Printf("数据包 #%d TCP层为空或没有负载\n", packetCount)
			continue
		}

		// 打印TCP负载
		fmt.Printf("数据包 #%d TCP负载: %s\n", packetCount, string(tcp.Payload))

		// 检查是否为HTTP GET请求
		if strings.Contains(string(tcp.Payload), "GET") && 
		   strings.Contains(string(tcp.Payload), "HTTP/1.1") && 
		   strings.Contains(string(tcp.Payload), "Upgrade: websocket") {
			fmt.Printf("找到WebSocket握手数据包\n")

			// 尝试解析HTTP请求
			handshake, err := parseWebSocketHandshake(string(tcp.Payload))
			if err != nil {
				fmt.Printf("解析握手失败: %v\n", err)
				continue
			}

			return handshake, nil
		}
	}

	fmt.Printf("处理完成，共处理 %d 个数据包\n", packetCount)
	return nil, nil
}

// ReadWebSocketFrames 读取WebSocket数据帧
func (r *PCAPReader) ReadWebSocketFrames() ([][]byte, error) {
	if err := r.reopenHandle(); err != nil {
		return nil, fmt.Errorf("重新打开PCAP文件失败: %v", err)
	}

	var frames [][]byte
	packetSource := gopacket.NewPacketSource(r.handle, r.handle.LinkType())
	
	for packet := range packetSource.Packets() {
		// 获取原始数据
		data := packet.Data()
		if len(data) < 54 { // 以太网(14) + IP(20) + TCP(20)的最小长度
			continue
		}

		// 跳过以太网头部
		data = data[14:]

		// 验证IP头部
		if len(data) < 20 || (data[0]>>4) != 4 { // 只处理IPv4
			continue
		}

		// 获取IP头部长度
		ipHeaderLen := (data[0] & 0x0F) * 4
		if len(data) < int(ipHeaderLen) {
			continue
		}

		// 验证是TCP协议
		if data[9] != 6 { // TCP protocol number
			continue
		}

		// 跳过IP头部
		data = data[ipHeaderLen:]

		// 验证TCP头部
		if len(data) < 20 {
			continue
		}

		// 获取TCP头部长度
		tcpHeaderLen := (data[12] >> 4) * 4
		if len(data) < int(tcpHeaderLen) {
			continue
		}

		// 跳过TCP头部
		data = data[tcpHeaderLen:]

		// 尝试在原始数据中查找WebSocket帧
		for i := 0; i < len(data)-2; i++ {
			// 检查是否为WebSocket帧的起始
			// 第一个字节的FIN位应该为1，RSV1-3位应该为0，opcode应该是文本或二进制
			if (data[i]&0x80 != 0) && (data[i]&0x70 == 0) && (data[i]&0x0F == 0x1 || data[i]&0x0F == 0x2) {
				opcode := data[i] & 0x0F
				if opcode != 0x1 && opcode != 0x2 {
					continue
				}

				// 获取payload长度
				payloadLen := int(data[i+1] & 0x7F)
				headerLen := 2

				if len(data) < i+headerLen {
					continue
				}

				// 处理扩展长度
				if payloadLen == 126 {
					if len(data) < i+4 {
						continue
					}
					payloadLen = int(data[i+2])<<8 | int(data[i+3])
					headerLen += 2
				} else if payloadLen == 127 {
					if len(data) < i+10 {
						continue
					}
					payloadLen = 0
					for j := 0; j < 8; j++ {
						payloadLen = payloadLen<<8 | int(data[i+2+j])
					}
					headerLen += 8
				}

				// 验证payload长度是否合理
				if payloadLen <= 0 || payloadLen > 65535 {
					continue
				}

				// 检查掩码位
				masked := (data[i+1] & 0x80) != 0
				if masked {
					headerLen += 4
				}

				// 确保有足够的数据
				if len(data) < i+headerLen+payloadLen {
					continue
				}

				// 提取帧数据
				frameData := make([]byte, payloadLen)
				copy(frameData, data[i+headerLen:i+headerLen+payloadLen])

				// 如果数据被掩码，则解码
				if masked {
					maskKey := data[i+headerLen-4 : i+headerLen]
					for j := 0; j < payloadLen; j++ {
						frameData[j] ^= maskKey[j%4]
					}
				}

				// 验证数据是否是有效的UTF-8文本（如果是文本帧）
				if opcode == 0x1 && !utf8.Valid(frameData) {
					continue
				}

				// 验证数据不是全零或全相同字节
				if len(frameData) > 0 {
					allSame := true
					firstByte := frameData[0]
					for _, b := range frameData[1:] {
						if b != firstByte {
							allSame = false
							break
						}
					}
					if allSame {
						continue
					}
				}

				// 验证数据不是全空白字符
				if len(frameData) > 0 {
					allWhitespace := true
					for _, b := range frameData {
						if b != ' ' && b != '\t' && b != '\n' && b != '\r' {
							allWhitespace = false
							break
						}
					}
					if allWhitespace {
						continue
					}
				}

				frames = append(frames, frameData)
				// 跳过已处理的数据
				i += headerLen + payloadLen - 1
			}
		}
	}

	return frames, nil
}

// WebSocketHandshake WebSocket握手信息
type WebSocketHandshake struct {
	Path      string
	Headers   map[string]string
	Protocol  string
	Key       string
	Version   string
}

// parseWebSocketHandshake 解析WebSocket握手信息
func parseWebSocketHandshake(data string) (*WebSocketHandshake, error) {
	lines := strings.Split(data, "\r\n")
	if len(lines) < 1 {
		return nil, fmt.Errorf("无效的HTTP请求")
	}

	// 解析请求行
	requestLine := strings.Split(lines[0], " ")
	if len(requestLine) != 3 || requestLine[0] != "GET" {
		return nil, fmt.Errorf("无效的HTTP请求行")
	}

	handshake := &WebSocketHandshake{
		Path:    requestLine[1],
		Headers: make(map[string]string),
	}

	// 解析请求头
	for _, line := range lines[1:] {
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) != 2 {
			continue
		}
		key := parts[0]
		value := parts[1]
		handshake.Headers[key] = value

		// 提取特定的WebSocket头部
		switch key {
		case "Sec-WebSocket-Protocol":
			handshake.Protocol = value
		case "Sec-WebSocket-Key":
			handshake.Key = value
		case "Sec-WebSocket-Version":
			handshake.Version = value
		}
	}

	return handshake, nil
}
