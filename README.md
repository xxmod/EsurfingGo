# EsurfingGo

> 中国电信天翼校园网（ESurfing）自动认证拨号客户端的 Go 语言实现。由于我无法测试各个学校的情况，出现问题时，欢迎提交issue并贴上日志。

## 功能

- 自动检测强制门户（Captive Portal）并完成认证
- 自动心跳保活，断线自动重连
- 跨平台编译（Windows / Linux / macOS）
- Go编译后为单文件且无须依赖，便于路由器部署
- **本项目同时有安卓手机端版本[EsurfingGo-Android](https://github.com/xxmod/EsurfingGo-Android)**

## 使用

[Release](https://github.com/xxmod/EsurfingGo/releases/latest)中有最新版本下载，可以直接下载使用

```bash
esurfing -u <用户名> -p <密码> [-s <短信验证码>]
```

### 参数

| 参数                   | 说明                   |
| ---------------------- | ---------------------- |
| `-u` / `-user`     | 登录用户名             |
| `-p` / `-password` | 登录密码               |
| `-s` / `-sms`      | 预填短信验证码（可选） |

### 示例

```bash
# 基本登录
esurfing -u 13800138000 -p mypassword

# 携带短信验证码
esurfing -u 13800138000 -p mypassword -s 123456
```

程序启动后会自动检测网络状态，完成认证并保持连接。按 `Ctrl+C` 安全退出。

## 🚀 部署方法

### 1. 直接运行

```bash
./esurfing -u <用户名> -p <密码>
```

在进行后面的部署之前请先运行一次确保程序在你当前的网络环境可用，如不可用，可以贴上日志发issue

### 2. 后台服务部署（Linux Systemd）

创建服务文件 `/etc/systemd/system/esurfing.service`：

````ini
[Unit]
Description=EsuringGo Campus Network Authenticator
After=network.target

[Service]
Type=simple
ExecStart= #应用路径
Restart=always
User=root

[Install]
WantedBy=multi-user.target
````

启用服务：

```bash
sudo systemctl daemon-reload
sudo systemctl enable esurfing #开机自启
sudo systemctl start esurfing #启动程序
sudo systemctl status esurfing #查看状态
```

### 3. 路由器部署（OpenWrt / DD-WRT）

将编译好的二进制文件上传至路由器 `/usr/bin/`，并添加开机启动脚本：

```bash
# 添加执行权限
chmod +x /usr/bin/esurfing #此处的esurfing根据你release或者编译的名字更改

# 编辑rc.local配置自启动
nano /etc/rc.local
#最后一行添加 /usr/bin/esurfing -u <用户名> -p <密码>
#ctrl+o保存 ctrl+x关闭编辑器
```

若为梅林固件，可以在/jffs/scripts/添加services-start

```bash
#!/bin/sh
#此脚本为梅林专用
i=0
while [ $i -le 30 ]; do
    success_start_service=$(nvram get success_start_service)
    if [ "$success_start_service" == "1" ]; then
        break
    fi
    i=$(($i+1))
    sleep 1
done

logger -t "CustomScript" "My services-start script executed successfully."

sleep 5

<外部存储位置>/start.sh > /tmp/home/root/Esurfinglog.txt 2>&1 & 
#上文start.sh为与esurfing相同的目录，内容为启动命令，如./esurfing-linux-armv5 -u 123123 -p 212121

exit 0

```

start.sh可参考我的写法

```bash
#!/bin/sh
while true;do
    <绝对路径>/esurfing-linux-arm5 -u 123321 -p 212121
done
```

## 🛠️ 构建方法

### 1. 基础构建（推荐）

```bash
go build -o esurfing .
```

生成的 `esurfing.exe`（Windows）或 `esurfing`（Linux/macOS）即为可执行文件。

### 2. 跨平台编译脚本

项目提供自动化构建脚本，支持多平台打包：

- **Windows (PowerShell)**:

  ```powershell
  .\build.ps1
  ```
- **Linux / macOS (Bash)**:

  ```bash
  chmod +x build.sh
  ./build.sh
  ```

> 📌 参考 `EsurfingDialer` 项目的构建逻辑，脚本会自动设置 `CGO_ENABLED=0` 以确保静态链接，避免运行时依赖。

### 3. 旧设备兼容构建

针对 ARMv5 架构设备，需指定环境变量：

```bash
GOARM=5 CGO_ENABLED=0 GOOS=linux GOARCH=arm go build -o esurfing-arm5 .
```

## 🧪 测试验证

构建后建议运行完整测试套件：

```bash
go test ./... -v
```

特别关注 `cipher/` 模块的加解密一致性测试，确保协议兼容性。

## 项目结构

```
├── main.go              # 入口
├── client.go            # 认证客户端主逻辑
├── session.go           # 会话与加密管理
├── states.go            # 全局状态（线程安全）
├── constants.go         # 常量定义
├── cipher/              # 加密算法实现
│   ├── cipher.go        #   工厂函数
│   ├── keydata.go       #   密钥数据
│   ├── aescbc.go        #   AES-CBC
│   ├── aesecb.go        #   AES-ECB
│   ├── desedecbc.go     #   3DES-CBC
│   ├── desedeecb.go     #   3DES-ECB
│   ├── sm4cbc.go        #   SM4-CBC
│   ├── sm4ecb.go        #   SM4-ECB
│   ├── modxtea.go       #   ModXTEA
│   ├── modxteaxteaiv.go #   ModXTEA-XTEAIV
│   └── zuc.go           #   ZUC-128
├── network/             # 网络模块
│   ├── client.go        #   HTTP 客户端
│   └── connectivity.go  #   门户检测
├── utils/               # 工具函数
│   └── utils.go
└── model/               # 数据模型
    └── model.go
```

## 依赖

- [gmsm](https://github.com/emmansun/gmsm) — 国密 SM4 / ZUC 算法
- [google/uuid](https://github.com/google/uuid) — UUID 生成

## 许可证

MIT

---

> 项目结构与协议解析逻辑继承自  `Rsplwe/EsurfingDialer`，但采用更清晰的状态机与工厂模式重构，便于维护与扩展。如果喜欢这个项目，请帮忙点个star
