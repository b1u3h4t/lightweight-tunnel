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

# 查看连接状态（服务端）
sudo netstat -tulnp | grep 9000
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

- 主文档：[../README.md](../README.md)
- 配置说明：[README.md](README.md)
