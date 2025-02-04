# AI Dialer Mini

这是一个基于FreeSWITCH和科大讯飞ASR的简单呼叫系统，可以实现实时语音转文字功能。

## 功能特性

- 支持FreeSWITCH呼叫控制
- 集成科大讯飞实时语音识别（ASR）
- 实时显示通话语音转文字结果
- 支持多通道音频处理

## 系统要求

- Windows 11操作系统
- FreeSWITCH服务器
- MySQL数据库
- Redis服务器
- Go 1.x或更高版本

## 配置说明

### FreeSWITCH配置
- 服务器地址：192.168.11.161
- 外网IP：111.61.208.207
- 端口：8021
- 密码：ClueCon

### 科大讯飞ASR配置
- APPID：c0de4f24
- APISecret：NWRhZDBkNzA5ZDQxNGMzYmQ1NWMwMWNh
- APIKey：51012a35448538a8396dc564cf050f68

### MySQL配置
- 地址：127.0.0.1
- 数据库名：ai_dialer

## 项目结构

```
ai_dialer_mini/
├── cmd/
│   └── main.go            # 主程序入口
├── internal/
│   ├── config/           # 配置相关
│   │   ├── config.go
│   │   └── freeswitch.go
│   └── service/
│       ├── asr/          # ASR服务
│       │   ├── types.go
│       │   └── xfyun.go
│       └── fs/           # FreeSWITCH服务
│           ├── client.go
│           └── types.go
└── README.md
```

## 使用说明

1. 启动程序:
   ```
   go run cmd/main.go
   ```

2. 启动程序后，会显示可用命令列表：
   ```
   可用命令:
     call <from> <to> - 发起呼叫
     quit/exit - 退出程序
   ```

3. 发起呼叫：
   ```
   > call 1000 1004
   ```
   这将从分机1000呼叫分机1004

4. 当通话建立后，系统会自动启动ASR会话，实时识别通话内容并显示在控制台

5. 使用quit或exit命令退出程序

## 注意事项

1. 确保FreeSWITCH服务器已正确配置并运行
2. 确保科大讯飞ASR服务配置正确
3. 确保MySQL和Redis服务正常运行
4. 宿主机防火墙已关闭，以确保网络连接正常

## 技术支持

如有问题，请参考：
- FreeSWITCH文档：https://freeswitch.org/confluence/
- 科大讯飞语音听写（流式版）WebAPI文档：https://www.xfyun.cn/doc/asr/voicedictation/API.html