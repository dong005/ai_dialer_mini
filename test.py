import socket
import time
import threading

class FreeSwitchClient:
    def __init__(self, host, port, password):
        self.host = host
        self.port = port
        self.password = password
        self.socket = None
        self.running = False

    def connect(self):
        """连接到 FreeSWITCH"""
        try:
            self.socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            self.socket.connect((self.host, self.port))
            
            # 读取欢迎信息
            auth_request = self.socket.recv(1024).decode()
            print(f"收到认证请求: {auth_request}")
            
            # 发送认证
            auth_cmd = f"auth {self.password}\n\n"
            self.socket.send(auth_cmd.encode())
            
            # 读取认证响应
            auth_response = self.socket.recv(1024).decode()
            print(f"认证响应: {auth_response}")
            
            if "Reply-Text: +OK accepted" in auth_response:
                print("认证成功!")
                return True
            else:
                print("认证失败!")
                return False
                
        except Exception as e:
            print(f"连接失败: {e}")
            return False

    def subscribe_events(self):
        """订阅所有事件"""
        try:
            cmd = "event plain all\n\n"
            self.socket.send(cmd.encode())
            response = self.socket.recv(1024).decode()
            print(f"订阅事件响应: {response}")
        except Exception as e:
            print(f"订阅事件失败: {e}")

    def read_events(self):
        """读取事件循环"""
        buffer = ""
        self.running = True
        
        while self.running:
            try:
                data = self.socket.recv(1024).decode()
                if not data:
                    break
                
                buffer += data
                
                # 处理完整的事件
                while "\n\n" in buffer:
                    event, buffer = buffer.split("\n\n", 1)
                    self.handle_event(event)
                    
            except Exception as e:
                print(f"读取事件失败: {e}")
                break

    def handle_event(self, event_text):
        """处理单个事件"""
        try:
            # 解析事件头部
            headers = {}
            for line in event_text.split("\n"):
                if ": " in line:
                    key, value = line.split(": ", 1)
                    headers[key] = value
            
            # 打印事件信息
            if "Event-Name" in headers:
                print(f"\n收到事件: {headers['Event-Name']}")
                for key, value in headers.items():
                    print(f"  {key}: {value}")
        except Exception as e:
            print(f"处理事件失败: {e}")

    def send_command(self, command):
        """发送命令到 FreeSWITCH"""
        try:
            cmd = f"api {command}\n\n"
            self.socket.send(cmd.encode())
            response = self.socket.recv(1024).decode()
            print(f"命令响应: {response}")
        except Exception as e:
            print(f"发送命令失败: {e}")

    def close(self):
        """关闭连接"""
        self.running = False
        if self.socket:
            self.socket.close()

def main():
    # FreeSWITCH 连接参数
    host = "192.168.11.180"
    port = 8021
    password = "ClueCon"

    client = FreeSwitchClient(host, port, password)
    
    try:
        # 连接到 FreeSWITCH
        if not client.connect():
            return

        # 订阅事件
        client.subscribe_events()
        
        # 启动事件读取线程
        event_thread = threading.Thread(target=client.read_events)
        event_thread.start()
        
        # 主循环等待用户输入命令
        print("\n可用命令:")
        print("  status - 查看服务器状态")
        print("  channels - 查看当前通道")
        print("  call <from> <to> - 发起呼叫")
        print("  quit - 退出程序")
        
        while True:
            cmd = input("> ").strip()
            if not cmd:
                continue
                
            if cmd == "quit":
                break
            elif cmd == "status":
                client.send_command("status")
            elif cmd == "channels":
                client.send_command("show channels")
            elif cmd.startswith("call"):
                parts = cmd.split()
                if len(parts) == 3:
                    from_num = parts[1]
                    to_num = parts[2]
                    client.send_command(f"originate user/{from_num} &bridge(user/{to_num})")
                else:
                    print("使用方法: call <from> <to>")
            else:
                print("未知命令")
                
    except KeyboardInterrupt:
        print("\n正在退出...")
    finally:
        client.close()
        event_thread.join()

if __name__ == "__main__":
    main()
