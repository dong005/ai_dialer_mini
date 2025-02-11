import os
import json
import time
import logging
import asyncio
import websockets
import edge_tts
import io
import wave
import numpy as np
from scipy import signal

# 设置日志
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s',
    handlers=[
        logging.FileHandler("edge_tts_server.log", mode='w', encoding='utf-8'),
        logging.StreamHandler()
    ]
)
logger = logging.getLogger(__name__)

class EdgeTTSEngine:
    def __init__(self):
        # 音频参数配置
        self.voice = "zh-CN-XiaoxiaoNeural"  # 中文女声
        self.rate = "+10%"  # 语速
        self.volume = "+10%"  # 音量
        self.sample_rate = 16000  # 设置采样率为16kHz

    async def synthesize(self, text):
        """使用Edge TTS合成语音"""
        try:
            logger.info(f"开始调用Edge TTS，文本: {text}")
            
            # 使用Edge TTS合成语音
            communicate = edge_tts.Communicate(text, self.voice, rate=self.rate, volume=self.volume)
            audio_data = b""
            async for chunk in communicate.stream():
                if chunk["type"] == "audio":
                    audio_data += chunk["data"]
            
            # 将音频数据转换为16位PCM格式
            import soundfile as sf
            import io
            import numpy as np
            from scipy import signal
            
            # 使用soundfile读取音频数据
            with io.BytesIO(audio_data) as audio_stream:
                data, samplerate = sf.read(audio_stream)
                
            # 确保数据是单声道的
            if len(data.shape) > 1:
                data = data.mean(axis=1)
            
            # 重采样到8kHz
            if samplerate != self.sample_rate:
                number_of_samples = round(len(data) * float(self.sample_rate) / samplerate)
                data = signal.resample(data, number_of_samples)
            
            # 标准化音量
            data = np.int16(data * 32767)
            
            # 转换为字节
            audio_data = data.tobytes()
            
            logger.info(f"音频合成完成，总长度: {len(audio_data)} 字节")
            return audio_data

        except Exception as e:
            logger.error(f"TTS合成异常: {str(e)}")
            raise

class TTSServer:
    def __init__(self):
        self.sample_rate = 16000  # 设置采样率为16kHz
        self.channels = 1        # 单声道
        self.sample_width = 2    # 16位采样
        self.tts_engine = EdgeTTSEngine()
        self.client_count = 0    # 客户端计数器
        self.client_buffers = {}  # 存储每个客户端的完整文本
        
    async def process_text(self, text, client_id):
        """将文本转换为语音"""
        try:
            logger.info(f"[Client {client_id}] Processing TTS text: {text}")
            
            # 调用TTS引擎合成语音
            audio_data = await self.tts_engine.synthesize(text)
            
            logger.info(f"[Client {client_id}] TTS processing completed, generated audio size: {len(audio_data)} bytes")
            return audio_data
            
        except Exception as e:
            logger.error(f"[Client {client_id}] Error processing TTS: {str(e)}", exc_info=True)
            raise

    async def handle_client(self, websocket, path):
        """处理WebSocket客户端连接"""
        self.client_count += 1
        client_id = f"client_{self.client_count}"
        
        try:
            logger.info(f"[Client {client_id}] New client connected from path: {path}")
            
            async for message in websocket:
                try:
                    # 解析消息，支持JSON格式和纯文本
                    if isinstance(message, bytes):
                        try:
                            text = message.decode('utf-8').strip()
                        except UnicodeDecodeError:
                            logger.error(f"[Client {client_id}] Received invalid binary data")
                            continue
                    else:
                        text = message.strip()
                    
                    # 尝试解析JSON格式
                    try:
                        data = json.loads(text)
                        if isinstance(data, dict) and 'text' in data:
                            text = data['text']
                            uuid = data.get('uuid', '')
                        else:
                            logger.warning(f"[Client {client_id}] Invalid JSON format, using raw text")
                    except json.JSONDecodeError:
                        uuid = ''  # 非JSON格式时使用空UUID
                    
                    logger.info(f"[Client {client_id}] Received text: {text}")
                    
                    # 处理文本并生成语音
                    audio_data = await self.process_text(text, client_id)
                    
                    # 发送音频数据
                    if uuid:  # 如果有UUID，发送JSON响应
                        response = {
                            'uuid': uuid,
                            'audio_data': audio_data.hex()  # 将二进制数据转换为十六进制字符串
                        }
                        await websocket.send(json.dumps(response))
                        logger.info(f"[Client {client_id}] Sent JSON response with UUID {uuid}, audio size: {len(audio_data)} bytes")
                    else:  # 否则直接发送音频数据
                        await websocket.send(audio_data)
                        logger.info(f"[Client {client_id}] Sent raw audio data: {len(audio_data)} bytes")
                    
                except Exception as e:
                    logger.error(f"[Client {client_id}] Error processing message: {str(e)}", exc_info=True)
                    try:
                        # 发送错误消息回客户端
                        error_msg = f"Error: {str(e)}".encode('utf-8')
                        await websocket.send(error_msg)
                    except:
                        pass
                    
        except websockets.exceptions.ConnectionClosed:
            logger.info(f"[Client {client_id}] Client disconnected")
        except Exception as e:
            logger.error(f"[Client {client_id}] Unexpected error: {str(e)}")
        finally:
            logger.info(f"[Client {client_id}] Connection closed")

async def main():
    """主函数"""
    server = TTSServer()
    # 设置WebSocket服务器参数
    server_config = {
        'max_size': 10 * 1024 * 1024,  # 10MB的最大消息大小
        'ping_interval': None,  # 禁用ping以避免超时
        'ping_timeout': None,
        'close_timeout': 10  # 10秒关闭超时
    }
    
    async with websockets.serve(server.handle_client, "0.0.0.0", 8089, **server_config):
        logger.info("TTS服务器启动成功，等待连接...")
        await asyncio.Future()  # 运行直到被中断

if __name__ == "__main__":
    print("正在启动Edge TTS服务器...")
    print(f"服务器将监听在 ws://0.0.0.0:8089")
    
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        print("\n服务器已停止")
    except Exception as e:
        print(f"服务器发生错误: {str(e)}")
