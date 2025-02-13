import json
import time
import hmac
import base64
import hashlib
import asyncio
import logging
import numpy as np
import websockets
from datetime import datetime
from urllib.parse import urlencode, quote
from wsgiref.handlers import format_date_time
from time import mktime

# 设置日志
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s %(levelname)s %(message)s',
    datefmt='%Y-%m-%d %H:%M:%S'
)
logger = logging.getLogger(__name__)

# 定义音频帧状态
STATUS_FIRST_FRAME = 0  # 第一帧的标识
STATUS_CONTINUE_FRAME = 1  # 中间帧标识
STATUS_LAST_FRAME = 2  # 最后一帧的标识

class XFYunASRServer:
    def __init__(self):
        # 科大讯飞API配置
        self.APPID = "c0de4f24"
        self.APISecret = "NWRhZDBkNzA5ZDQxNGMzYmQ1NWMwMWNh"
        self.APIKey = "51012a35448538a8396dc564cf050f68"
        
        # 科大讯飞实时语音识别接口地址
        self.xf_url = "wss://ws-api.xfyun.cn/v2/iat"
        
        # WebSocket服务器配置
        self.host = "0.0.0.0"
        self.port = 8088

        # 公共参数(common)
        self.CommonArgs = {"app_id": self.APPID}
        # 业务参数(business)
        self.BusinessArgs = {
            "domain": "iat",            # 领域
            "language": "zh_cn",        # 语言
            "accent": "mandarin",       # 方言，这里选择普通话
            "vinfo": 1,                 # 返回音频信息
            "vad_eos": 5000,           # 后端点检测时间5秒，延长等待时间
            "dwa": "wpgs",             # 开启动态修正功能
            "pd": "game",              # 领域个性化参数：游戏
            "ptt": 1,                  # 添加标点符号
            "nunum": 1                 # 将数字转化为汉字
        }
        
    def create_url(self):
        """生成科大讯飞WebSocket URL"""
        # 生成RFC1123格式的时间戳
        now = datetime.now()
        date = format_date_time(mktime(now.timetuple()))

        # 拼接字符串
        signature_origin = "host: " + "ws-api.xfyun.cn" + "\n"
        signature_origin += "date: " + date + "\n"
        signature_origin += "GET " + "/v2/iat " + "HTTP/1.1"

        # 进行hmac-sha256进行加密
        signature_sha = hmac.new(self.APISecret.encode('utf-8'),
                               signature_origin.encode('utf-8'),
                               digestmod=hashlib.sha256).digest()
        signature_sha = base64.b64encode(signature_sha).decode()

        authorization_origin = f'api_key="{self.APIKey}", algorithm="hmac-sha256", headers="host date request-line", signature="{signature_sha}"'
        authorization = base64.b64encode(authorization_origin.encode('utf-8')).decode()

        # 将请求的鉴权参数组合为字典
        v = {
            "authorization": authorization,
            "date": date,
            "host": "ws-api.xfyun.cn"
        }
        return self.xf_url + '?' + urlencode(v)

    async def forward_to_xfyun(self, client_websocket, xf_websocket):
        """将客户端的音频数据转发给讯飞"""
        status = STATUS_FIRST_FRAME  # 音频的状态信息
        silent_frames = 0  # 记录静音帧数
        ENERGY_THRESHOLD = 30  # 能量阈值
        MAX_SILENT_FRAMES = 20  # 最大连续静音帧数（约2.5秒）
        
        try:
            async for message in client_websocket:
                if isinstance(message, bytes):
                    # 计算音频能量
                    audio_data = np.frombuffer(message, dtype=np.int16)
                    energy = np.mean(np.abs(audio_data))
                    
                    # 如果能量太低，可能是静音
                    if energy < ENERGY_THRESHOLD:
                        silent_frames += 1
                        logger.info(f"接收到音频数据: {len(message)} 字节, 能量值: {energy:.2f}, 静音帧数: {silent_frames}")
                    else:
                        if silent_frames > 0:
                            logger.info(f"检测到语音活动，重置静音帧计数")
                        silent_frames = 0
                        logger.info(f"接收到音频数据: {len(message)} 字节, 能量值: {energy:.2f}")
                    
                    # 将PCM音频数据转换为Base64编码
                    audio_base64 = base64.b64encode(message).decode()

                    try:
                        # 第一帧处理
                        if status == STATUS_FIRST_FRAME:
                            d = {
                                "common": self.CommonArgs,
                                "business": self.BusinessArgs,
                                "data": {
                                    "status": 0,
                                    "format": "audio/L16;rate=8000",
                                    "audio": audio_base64,
                                    "encoding": "raw"
                                }
                            }
                            status = STATUS_CONTINUE_FRAME
                        # 中间帧处理
                        elif status == STATUS_CONTINUE_FRAME:
                            d = {
                                "data": {
                                    "status": 1,
                                    "format": "audio/L16;rate=8000",
                                    "audio": audio_base64,
                                    "encoding": "raw"
                                }
                            }
                        
                        await xf_websocket.send(json.dumps(d))
                        
                        # 只有在连续静音超过阈值时才发送结束帧
                        if silent_frames >= MAX_SILENT_FRAMES:
                            logger.info(f"检测到较长静音 ({silent_frames} 帧)，准备发送结束帧...")
                            d = {
                                "data": {
                                    "status": 2,
                                    "format": "audio/L16;rate=8000",
                                    "audio": "",
                                    "encoding": "raw"
                                }
                            }
                            await xf_websocket.send(json.dumps(d))
                            status = STATUS_FIRST_FRAME  # 重置状态
                            silent_frames = 0  # 重置静音计数
                            
                    except websockets.exceptions.ConnectionClosed:
                        logger.info("与讯飞服务器的连接已关闭")
                        break
                    
                elif isinstance(message, str):
                    try:
                        data = json.loads(message)
                        if data.get("eof") == "true":
                            # 发送结束帧
                            logger.info("收到结束标记，等待最后的识别结果...")
                            d = {
                                "data": {
                                    "status": 2,
                                    "format": "audio/L16;rate=8000",
                                    "audio": "",
                                    "encoding": "raw"
                                }
                            }
                            await xf_websocket.send(json.dumps(d))
                            status = STATUS_FIRST_FRAME  # 重置状态
                    except json.JSONDecodeError:
                        pass
        except websockets.exceptions.ConnectionClosed:
            logger.info("客户端连接已关闭")
        except Exception as e:
            logger.error(f"转发音频数据时出错: {str(e)}")

    async def receive_from_xfyun(self, client_websocket, xf_websocket):
        """接收讯飞的识别结果并转发给客户端"""
        current_sentence = ""  # 保存当前的完整句子
        try:
            async for message in xf_websocket:
                logger.info(f"收到讯飞原始结果: {message}")
                data = json.loads(message)
                
                if "code" in data:
                    code = data["code"]
                    if code != 0:
                        error_msg = data.get("message", "未知错误")
                        logger.error(f"讯飞服务器返回错误: {error_msg}")
                        error_response = {
                            "status": "error",
                            "message": error_msg
                        }
                        await client_websocket.send(json.dumps(error_response, ensure_ascii=False))
                    else:
                        try:
                            data_result = data["data"]["result"]
                            
                            # 处理识别结果
                            if "ws" in data_result:
                                result = ""
                                for ws in data_result["ws"]:
                                    for cw in ws["cw"]:
                                        result += cw["w"]
                                
                                # 更新当前句子
                                if not result.startswith("。") and not result.startswith("，"):
                                    current_sentence = result
                                else:
                                    current_sentence += result
                                
                                # 判断是否是最后一个结果
                                is_last = data_result.get("ls", False)
                                logger.info(f"实时识别结果: {current_sentence} {'[最终结果]' if is_last else '[中间结果]'}")
                                
                                # 发送识别结果给客户端
                                response = {
                                    "status": "success",
                                    "message": current_sentence,
                                    "is_final": is_last
                                }
                                try:
                                    await client_websocket.send(json.dumps(response, ensure_ascii=False))
                                except websockets.exceptions.ConnectionClosed:
                                    logger.info("客户端连接已关闭")
                                    break
                                
                                # 如果是最终结果，重置当前句子
                                if is_last:
                                    current_sentence = ""
                                    
                        except Exception as e:
                            logger.error(f"解析识别结果时出错: {str(e)}")
                            logger.error(f"错误的数据结构: {data}")
                        
        except websockets.exceptions.ConnectionClosed:
            logger.info("与讯飞服务器的连接已关闭")
        except Exception as e:
            logger.error(f"接收讯飞结果时出错: {str(e)}")

    async def handle_client(self, websocket):
        """处理客户端连接"""
        client_id = id(websocket)
        try:
            logger.info(f"[Client {client_id}] New WebSocket connection")
            url = self.create_url()
            logger.info(f"连接讯飞服务器URL: {url}")
            
            async with websockets.connect(url) as xf_websocket:
                logger.info("已连接到讯飞WebSocket服务器")
                await asyncio.gather(
                    self.forward_to_xfyun(websocket, xf_websocket),
                    self.receive_from_xfyun(websocket, xf_websocket)
                )
        except websockets.exceptions.ConnectionClosed:
            logger.info(f"[Client {client_id}] WebSocket connection closed")
        except Exception as e:
            logger.error(f"[Client {client_id}] Error handling request: {str(e)}", exc_info=True)
            try:
                error_response = {
                    "status": "error",
                    "message": str(e)
                }
                await websocket.send(json.dumps(error_response, ensure_ascii=False))
            except:
                pass
        finally:
            logger.info(f"[Client {client_id}] Connection terminated")

async def main():
    """主函数"""
    server = XFYunASRServer()
    
    async def debug_handler(websocket):
        """包装处理函数以捕获异常"""
        try:
            await server.handle_client(websocket)
        except Exception as e:
            logger.error(f"Error handling request: {str(e)}", exc_info=True)
            raise
    
    async with websockets.serve(
        debug_handler, 
        server.host, 
        server.port,
        ping_interval=None,  # 禁用ping以避免与FreeSWITCH的问题
        ping_timeout=None
    ) as ws_server:
        logger.info(f"XFYun ASR WebSocket server started on {server.host}:{server.port}")
        await asyncio.Future()  # 保持服务器运行

if __name__ == "__main__":
    asyncio.run(main())
