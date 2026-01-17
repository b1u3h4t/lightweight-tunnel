# Lightweight Tunnel 低配服务器优化配置

本目录包含针对低配置服务器（单核1G内存等小型VPS）优化的配置文件模板。

## 配置文件说明

### 1. low-spec-minimal.json （最小化配置 - 服务端）
**适用场景**：1核心512MB-1GB内存，2-5个客户端
- 队列大小：500
- 最大客户端：5
- FEC：5+1（低开销）
- MTU：1200
- 禁用P2P和Mesh路由
- 预计内存占用：~40-50MB

### 2. low-spec-moderate.json （中等负载配置 - 服务端）
**适用场景**：1核心1-2GB内存，10-15个客户端
- 队列大小：1000
- 最大客户端：15
- FEC：8+2（平衡性能）
- MTU：1200
- 禁用P2P和Mesh路由
- 预计内存占用：~60-80MB

### 3. low-spec-client.json （客户端配置）
**适用场景**：低配置客户端
- 队列大小：500
- FEC：5+1
- MTU：1200
- 禁用P2P功能
- 预计内存占用：~30-40MB

## 使用方法

### 1. 复制并修改配置文件
```bash
# 复制配置文件
cp configs/low-spec-minimal.json /etc/lightweight-tunnel/config.json

# 修改配置（必须修改key和地址）
sudo nano /etc/lightweight-tunnel/config.json
```

### 2. 启动服务
```bash
# 使用配置文件启动
sudo ./lightweight-tunnel -c /etc/lightweight-tunnel/config.json
```

### 3. 作为systemd服务运行
```bash
# 安装服务
sudo make install-service \
  CONFIG_PATH=/etc/lightweight-tunnel/config.json \
  SERVICE_NAME=lightweight-tunnel-server

# 启动服务
sudo systemctl start lightweight-tunnel-server
sudo systemctl status lightweight-tunnel-server
```

## 优化说明

### 为什么这些配置适合低配服务器？

| 优化项 | 默认值 | 低配值 | 节省说明 |
|--------|--------|--------|----------|
| 队列大小 | 5000 | 500-1000 | 减少90%内存占用 |
| 最大客户端 | 100 | 5-15 | 按实际需求限制 |
| MTU | 1400 | 1200 | 减少包缓冲区大小 |
| FEC | 10+3 | 5+1 | 减少30%CPU和带宽开销 |
| P2P/Mesh | 启用 | 禁用 | 节省3个goroutine和路由表内存 |
| NAT检测 | 启用 | 禁用 | 无需STUN服务器连接 |
| Keepalive | 10秒 | 15秒 | 减少控制包开销 |

### 资源占用估算

#### 最小化配置（5客户端）
- 基础进程：~30MB
- 包缓冲池：~5MB
- 队列开销：~5MB
- Goroutines：~1MB
- **总计：~41MB**
- **可用内存：959MB（95%+空闲）**

#### 中等负载配置（15客户端）
- 基础进程：~30MB
- 包缓冲池：~15MB
- 队列开销：~15MB
- Goroutines：~3MB
- **总计：~63MB**
- **可用内存：937MB（93%+空闲）**

## 性能监控

### 查看实际资源使用
```bash
# 查看内存占用
ps aux | grep lightweight-tunnel
# 或
top -p $(pgrep lightweight-tunnel)

# 查看goroutine数量（需要启用pprof）
curl http://localhost:6060/debug/pprof/goroutine?debug=1

# 查看网络流量
sudo iftop -i tun0
```

### 性能调优建议

如果出现性能问题，可以尝试：

1. **降低客户端数量**：减少max_clients
2. **减少FEC开销**：降低fec_parity（适用于稳定网络）
3. **增加keepalive间隔**：减少控制包频率
4. **禁用加密**：仅测试用，生产环境不推荐

## 常见问题

### Q: 为什么禁用P2P？
A: P2P功能需要额外的goroutine、UDP端口监听和路由表维护，对于简单的服务器-客户端场景不必要。如果需要客户端间直连，可以启用（但会增加内存使用）。

### Q: 队列太小会不会丢包？
A: 队列大小500-1000对于大多数场景足够。系统有100ms超时重试机制，真正丢包前会等待。如果经常看到"queue full"日志，可以适当增加队列大小或减少客户端数量。

### Q: MTU 1200会影响性能吗？
A: 会略微降低吞吐量（增加包分片），但减少了每个包的内存占用。对于低配服务器，内存比吞吐量更重要。

### Q: 可以进一步降低内存吗？
A: 可以。尝试：
- 减少队列到300-400
- 降低MTU到800-1000
- 设置max_clients=2-3
- 但需要权衡可用性

## 高级配置

如果需要更细粒度的控制，可以手动调整：

```json
{
  "mode": "server",
  "local_addr": "0.0.0.0:9000",
  "tunnel_addr": "10.0.0.1/24",
  "key": "your-strong-key-here",
  "mtu": 1200,
  "fec_data": 5,
  "fec_parity": 1,
  "send_queue_size": 500,
  "recv_queue_size": 500,
  "timeout": 30,
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

## 参考资料

- 主README：[../README.md](../README.md)
- 性能调优章节：[../README.md#性能调优](../README.md#性能调优)
- 配置参数说明：[../README.md#配置参数说明](../README.md#配置参数说明)
