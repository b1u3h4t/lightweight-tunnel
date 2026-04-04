# 低配服务器快速参考指南

## 命令行快速启动

### 服务端（最小化配置）
适用于：1核512MB-1GB，2-5个客户端

```bash
sudo ./lightweight-tunnel \
  -m server \
  -l 0.0.0.0:9000 \
  -t 10.0.0.1/24 \
  -k "your-secret-key" \
  -mtu 1200 \
  -fec-data 5 -fec-parity 1 \
  -send-queue 500 -recv-queue 500 \
  -max-clients 5 \
  -p2p=false \
  -nat-detection=false
```

### 服务端（中等负载）
适用于：1核1-2GB，10-15个客户端

```bash
sudo ./lightweight-tunnel \
  -m server \
  -l 0.0.0.0:9000 \
  -t 10.0.0.1/24 \
  -k "your-secret-key" \
  -mtu 1200 \
  -fec-data 8 -fec-parity 2 \
  -send-queue 1000 -recv-queue 1000 \
  -max-clients 15 \
  -p2p=false \
  -nat-detection=false
```

### 客户端（低配）

```bash
sudo ./lightweight-tunnel \
  -m client \
  -r <服务器IP>:9000 \
  -t 10.0.0.2/24 \
  -k "your-secret-key" \
  -mtu 1200 \
  -fec-data 5 -fec-parity 1 \
  -send-queue 500 -recv-queue 500 \
  -p2p=false \
  -nat-detection=false
```

## 使用配置文件（推荐）

### 1. 复制配置模板

```bash
# 最小化配置（2-5客户端）
cp configs/low-spec-minimal.json /etc/lightweight-tunnel/config.json

# 或中等负载（10-15客户端）
cp configs/low-spec-moderate.json /etc/lightweight-tunnel/config.json

# 客户端
cp configs/low-spec-client.json /etc/lightweight-tunnel/config.json
```

### 2. 修改配置

```bash
sudo nano /etc/lightweight-tunnel/config.json
```

**必须修改的字段**：
- `key`: 改为强密钥（使用 `openssl rand -base64 32` 生成）
- `remote_addr`（客户端）: 改为实际服务器地址

### 3. 启动

```bash
sudo ./lightweight-tunnel -c /etc/lightweight-tunnel/config.json
```

## 参数对比表

| 参数 | 默认值 | 最小化 | 中等负载 | 说明 |
|-----|--------|--------|----------|------|
| MTU | 1400 | 1200 | 1200 | 减少包大小 |
| FEC data | 10 | 5 | 8 | 数据分片 |
| FEC parity | 3 | 1 | 2 | 校验分片 |
| Send queue | 5000 | 500 | 1000 | 发送队列 |
| Recv queue | 5000 | 500 | 1000 | 接收队列 |
| Max clients | 100 | 5 | 15 | 最大客户端 |
| P2P | true | false | false | P2P直连 |
| NAT detect | true | false | false | NAT检测 |

## 内存占用估算

| 配置 | 基础 | 缓冲区 | 队列 | Goroutines | 总计 | 可用 |
|-----|------|--------|------|-----------|------|------|
| 默认 | 30MB | 50MB+ | 400MB+ | 10MB | ~500MB | <50% |
| 最小化 | 30MB | 5MB | 5MB | 1MB | **~41MB** | **96%** |
| 中等 | 30MB | 15MB | 15MB | 3MB | **~63MB** | **94%** |

## 监控命令

```bash
# 查看内存占用
ps aux | grep lightweight-tunnel

# 实时监控
top -p $(pgrep lightweight-tunnel)

# 查看网络流量
sudo iftop -i tun0

# rawtcp 模式不要用 netstat/ss 看 9000 是否 LISTEN
# 应直接抓物理网卡上的 TCP/9000 收发
sudo tcpdump -nn -i eth0 'tcp port 9000'
```

## NAT 云主机 / EIP / 公网映射说明

如果云主机公网 IP 不是直接挂在网卡上，而是通过云厂商 NAT / EIP 映射到实例，请这样配置服务端：

- `local_addr`: 公网地址，例如 `49.232.146.200:9000`
- `reply_source_ip`: 实例网卡内网地址，例如 `10.2.0.12`

示例：

```json
{
  "mode": "server",
  "local_addr": "49.232.146.200:9000",
  "reply_source_ip": "10.2.0.12",
  "tunnel_addr": "100.0.0.1/24",
  "key": "your-strong-password-here-32-chars-minimum",
  "mtu": 0,
  "enable_nat_detection": true,
  "enable_xdp": true,
  "enable_kernel_tune": true
}
```

验证时请抓 `eth0`，看到下面这种握手才算配置生效：

```text
<client-ip>.<port> > 10.2.0.12.9000: Flags [S]
10.2.0.12.9000 > <client-ip>.<port>: Flags [S.]
<client-ip>.<port> > 10.2.0.12.9000: Flags [.]
```

## 不要把 tunnel_addr 设成公网或 CGNAT 地址

`tunnel_addr` 只应使用 RFC1918 私网段，例如：

- `10.233.0.1/24`
- `10.233.0.20/24`

不要使用：

- `100.0.0.1/24`
- 任何公网地址
- `100.64.0.0/10` 共享地址段

否则一旦系统没有把路由正确装到 `tun0`，你的测试流量会直接走物理网卡，得到一个完全错误的“隧道 RTT”。

测试前先确认：

```bash
ip route get 10.233.0.1
```

结果必须包含：

```text
10.233.0.1 dev tun0 ...
```

## 性能调优建议

### 如果内存不足
1. 减少 max_clients
2. 降低队列大小到 300-400
3. 减少 MTU 到 800-1000
4. 禁用更多功能（确认不需要时）

### 如果 CPU 使用率高
1. 降低 FEC parity（如果网络稳定）
2. 增加 keepalive 间隔到 20-30 秒
3. 禁用 kernel-tune（可选）

### 如果丢包严重
1. 增加 FEC parity（但会增加 CPU 和带宽）
2. 检查网络质量
3. 可能需要更高配置的服务器

## 故障排查

### macOS rawtcp：握手成功但 ping 丢包很高

如果 macOS 客户端已经能握手成功，但 `ping 10.233.0.1` 仍然高丢包，先不要改 FEC。保持现有 `fec_data` / `fec_parity` 不变，优先检查本机是否在向 rawtcp 流量发 TCP RST。

首次使用前先做一次 PF bootstrap：

```bash
sudo ./lightweight-tunnel -install-macos-pf
sudo pfctl -sr | grep lightweight-tunnel
```

必须先看到：

```text
anchor "lightweight-tunnel/*" all
```

操作步骤：

```bash
# 1) 确认 PF 已启用
sudo pfctl -s info
sudo pfctl -E

# 2) 从日志或抓包找到 rawtcp 客户端本地源端口，下面以 22535 为例

# 3) 加载只针对这条连接的 RST 拦截规则
cat <<'EOF' | sudo pfctl -a lightweight-tunnel/client-22535 -f -
block drop out quick proto tcp from any port 22535 to 49.232.146.200 port 9000 flags R/R
EOF

# 4) 验证规则
sudo pfctl -a lightweight-tunnel/client-22535 -s rules

# 5) 抓包确认客户端外网网卡上不再持续出现 RST
sudo tcpdump -nn -i en0 'tcp[tcpflags] & tcp-rst != 0 and host 49.232.146.200 and port 9000'
```

删除规则：

```bash
sudo pfctl -a lightweight-tunnel/client-22535 -F rules
```

### 启动失败："permission denied"
```bash
# 需要 root 权限
sudo ./lightweight-tunnel ...
```

### 连接失败
```bash
# 1. 检查防火墙
sudo ufw allow 9000/tcp
sudo ufw allow 9000/udp

# 2. 测试连通性
ping <服务器IP>
nc -zv <服务器IP> 9000

# 3. 查看日志
journalctl -xe
```

### 队列满错误
```bash
# 增加队列大小
-send-queue 1000 -recv-queue 1000

# 或减少客户端数量
-max-clients 3
```

## 完整示例

### 服务端配置文件（/etc/lightweight-tunnel/server.json）

```json
{
  "mode": "server",
  "local_addr": "0.0.0.0:9000",
  "tunnel_addr": "10.0.0.1/24",
  "key": "your-strong-password-here-32-chars-minimum",
  "mtu": 1200,
  "fec_data": 5,
  "fec_parity": 1,
  "send_queue_size": 500,
  "recv_queue_size": 500,
  "keepalive": 15,
  "max_clients": 5,
  "multi_client": true,
  "client_isolation": false,
  "p2p_enabled": false,
  "enable_mesh_routing": false,
  "enable_nat_detection": false,
  "enable_xdp": true,
  "enable_kernel_tune": true
}
```

### 客户端配置文件（/etc/lightweight-tunnel/client.json）

```json
{
  "mode": "client",
  "remote_addr": "1.2.3.4:9000",
  "tunnel_addr": "10.0.0.2/24",
  "key": "your-strong-password-here-32-chars-minimum",
  "mtu": 1200,
  "fec_data": 5,
  "fec_parity": 1,
  "send_queue_size": 500,
  "recv_queue_size": 500,
  "keepalive": 15,
  "p2p_enabled": false,
  "enable_mesh_routing": false,
  "enable_nat_detection": false,
  "enable_xdp": true,
  "enable_kernel_tune": true
}
```

### Systemd 服务安装

```bash
# 编译
make build

# 安装服务
sudo make install-service \
  CONFIG_PATH=/etc/lightweight-tunnel/server.json \
  SERVICE_NAME=lightweight-tunnel-server

# 启动
sudo systemctl start lightweight-tunnel-server
sudo systemctl enable lightweight-tunnel-server

# 查看状态
sudo systemctl status lightweight-tunnel-server

# 查看日志
sudo journalctl -u lightweight-tunnel-server -f
```

## 更多信息

- macOS rawtcp 运行需要原生 Darwin 构建并启用 `cgo + libpcap`。Linux 上 `GOOS=darwin CGO_ENABLED=0` 交叉编译出来的 Darwin 二进制仅用于编译验证，不能实际完成握手。
- 主文档：[../README.md](../README.md)
- 配置说明：[README.md](README.md)
