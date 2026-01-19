# Lightweight Tunnel

<div align="center">

**轻量级内网穿透与虚拟组网工具**

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go 1.19+](https://img.shields.io/badge/Go-1.19+-blue.svg)](https://golang.org)
[![Linux](https://img.shields.io/badge/Platform-Linux-green.svg)](https://www.linux.org/)

[快速开始](#快速开始) • [配置说明](#配置说明) • [性能调优](#性能调优) • [故障排查](#故障排查)

</div>

---

## 项目简介

基于 Go 的专业内网穿透和虚拟组网工具，核心特性：

- **真实 TCP 伪装**：使用 Raw Socket 构造完整 TCP/IP 包，绕过防火墙和 DPI 检测
- **高性能**：避免 TCP-over-TCP 问题，FEC 前向纠错，优化队列管理
- **安全加密**：AES-256-GCM 端到端加密
- **智能路由**：P2P 直连 + NAT 穿透 + Mesh 路由
- **资源友好**：支持低配 VPS（1核1G），内存占用低至 40MB

### 适用场景

- 企业分支内网互联
- 远程访问家庭 NAS/服务器  
- 游戏联机加速
- 开发测试环境
- 绕过网络限制

---

## 系统要求

| 项目 | 要求 |
|-----|------|
| 操作系统 | Linux（内核 2.6+） |
| 权限 | Root（Raw Socket 和 TUN 设备） |
| 内存 | 最低 64MB，推荐 128MB+ |
| CPU | 单核即可 |
| 网络 | 至少一端有公网 IP 或端口转发 |

---

## 快速开始

### 安装

**方法 1：从源码编译**
```bash
git clone https://github.com/openbmx/lightweight-tunnel.git
cd lightweight-tunnel
go build -o lightweight-tunnel ./cmd/lightweight-tunnel
sudo cp lightweight-tunnel /usr/local/bin/
```

**方法 2：使用 Makefile**
```bash
make build    # 编译到 bin/lightweight-tunnel
make install  # 安装依赖
```

### 基本使用

**服务端**（有公网 IP 的机器）
```bash
sudo ./lightweight-tunnel \
  -m server \
  -l 0.0.0.0:9000 \
  -t 10.0.0.1/24 \
  -k "your-secret-key"
```

**客户端**
```bash
sudo ./lightweight-tunnel \
  -m client \
  -r <服务器IP>:9000 \
  -t 10.0.0.2/24 \
  -k "your-secret-key"
```

**验证连接**
```bash
ping 10.0.0.1  # 在客户端 ping 服务器
ping 10.0.0.2  # 在服务端 ping 客户端
```

### 验证模式（低 CPU 开销）

适用于可信网络或已有应用层加密（HTTPS/TLS）的场景：

**服务端**
```bash
sudo ./lightweight-tunnel \
  -m server \
  -l 0.0.0.0:9000 \
  -t 10.0.0.1/24 \
  -k "your-secret-key" \
  -encrypt-after-auth
```

**客户端**
```bash
sudo ./lightweight-tunnel \
  -m client \
  -r <服务器IP>:9000 \
  -t 10.0.0.2/24 \
  -k "your-secret-key" \
  -encrypt-after-auth
```

**特点**：
- 连接时通过加密密钥验证身份
- 验证通过后，数据包不加密（控制包仍加密）
- CPU 开销大幅降低，适合高流量场景
- 延迟更低，吞吐量更高

### 低配服务器部署（1核1G）

使用优化配置模板：
```bash
# 服务端（支持 2-5 个客户端）
sudo ./lightweight-tunnel -c configs/low-spec-minimal.json

# 客户端
sudo ./lightweight-tunnel -c configs/low-spec-client.json
```

详见：[configs/README.md](configs/README.md)

---

## 核心技术

### 真实 TCP 伪装

传统方案：UDP 包添加假 TCP 头
```
[UDP Header (协议17)] + [伪造TCP头] → 易被识别
```

本项目：Raw Socket 构造真实 TCP
```
[IP Header (协议6)] + [真实TCP Header] → 完美伪装
```

**技术实现**：
- 完整 TCP 三次握手（SYN/SYN-ACK/ACK）
- 真实序列号和确认号
- 正确的 TCP 选项（MSS、SACK、Window Scale、Timestamp）
- 自动管理 iptables 规则防止内核 RST

**效果**：可绕过 TCP-only 防火墙和 DPI 深度包检测

### FEC 前向纠错

避免 TCP-over-TCP 重传灾难，使用 Reed-Solomon 编码：
```
原始数据: [D1][D2]...[D10]
编码后:   [D1][D2]...[D10][P1][P2][P3]
丢包恢复: 可恢复最多 3 个丢失包
```

**配置建议**：

| 网络环境 | fec_data | fec_parity | 可恢复丢包率 | 带宽开销 |
|---------|----------|-----------|-------------|---------|
| 良好 (<1%) | 20 | 2 | 9% | 10% |
| 一般 (1-3%) | 10 | 3 | 23% | 30% |
| 较差 (3-10%) | 10 | 5 | 33% | 50% |
| 低配/弱网 | 5 | 1 | 17% | 20% |

### P2P 直连

**连接流程**：
```
1. 客户端注册    A → [Server] ← B
2. 交换地址      A ← [Server] → B
3. 同时打洞      A ──UDP打洞──→ B
4. 直连建立      A ←───P2P────→ B
```

**NAT 兼容性**：
- 完全锥形 NAT：99% 成功率
- 限制/端口限制：90-95%
- 对称 NAT：70-80%（端口预测）

**路由优先级**：本地网络 > P2P 直连 > 服务器中转

---

## 配置说明

### 命令行参数

**基础参数**
```
-m string      运行模式：server 或 client
-l string      监听地址（服务端）
-r string      服务器地址（客户端）
-t string      隧道 IP（CIDR 格式，如 10.0.0.2/24）
-k string      加密密钥（强烈推荐）
```

**性能参数**
```
-mtu int              MTU 大小（0=自动检测，默认 1400）
-fec-data int         FEC 数据分片（默认 10）
-fec-parity int       FEC 校验分片（默认 3）
-send-queue int       发送队列大小（默认 5000）
-recv-queue int       接收队列大小（默认 5000）
```

**功能开关**
```
-p2p                  启用 P2P（默认 true）
-xdp                  启用 XDP 加速（默认 true）
-kernel-tune          启用内核调优（默认 true）
-nat-detection        启用 NAT 检测（默认 true）
-encrypt-after-auth   仅验证模式（默认 false）
```

**加密模式说明**
- 默认模式（`-encrypt-after-auth=false`）：每个数据包都加密，最安全但 CPU 开销较高
- 验证模式（`-encrypt-after-auth=true`）：连接时验证身份，之后数据包不加密
  - ✅ 优势：CPU 开销低，传输速度快，延迟更低
  - ⚠️  注意：数据包不加密，适用于可信网络或已有其他加密层（如 TLS）
  - 🔒 安全：控制包仍加密，初始验证使用密钥，IP 绑定防止欺骗

**服务端专用**
```
-multi-client         启用多客户端（默认 true）
-max-clients int      最大客户端数（默认 100）
-client-isolation     客户端隔离（默认 false）
```

**其他**
```
-c string    使用配置文件
-g string    生成示例配置
-v           显示版本
```

### 配置文件

**生成模板**
```bash
./lightweight-tunnel -g config.json
# 生成 config.json (服务端) 和 config.json.client (客户端)
```

**服务端示例**
```json
{
  "mode": "server",
  "local_addr": "0.0.0.0:9000",
  "tunnel_addr": "10.0.0.1/24",
  "key": "your-strong-key",
  "mtu": 0,
  "max_clients": 100,
  "enable_xdp": true,
  "enable_kernel_tune": true
}
```

**客户端示例**
```json
{
  "mode": "client",
  "remote_addr": "server-ip:9000",
  "tunnel_addr": "10.0.0.2/24",
  "key": "your-strong-key",
  "mtu": 0,
  "p2p_enabled": true
}
```

**使用配置文件**
```bash
sudo ./lightweight-tunnel -c config.json
```

### Systemd 服务

```bash
# 安装服务
sudo make install-service \
  CONFIG_PATH=/etc/lightweight-tunnel/config.json \
  SERVICE_NAME=lightweight-tunnel-server

# 管理服务
sudo systemctl start lightweight-tunnel-server
sudo systemctl status lightweight-tunnel-server
sudo systemctl enable lightweight-tunnel-server

# 查看日志
sudo journalctl -u lightweight-tunnel-server -f
```

---

## 性能调优

### 低配服务器优化（1核1G）

**资源占用对比**

| 配置 | 队列 | 客户端 | FEC | 内存占用 | 可用内存 |
|-----|------|--------|-----|---------|---------|
| 默认 | 5000 | 100 | 10+3 | ~500MB+ | <50% |
| 最小化 | 500 | 5 | 5+1 | ~40MB | 96% |
| 中等负载 | 1000 | 15 | 8+2 | ~60MB | 94% |

**最小化配置（2-5客户端）**
```bash
sudo ./lightweight-tunnel \
  -m server \
  -l 0.0.0.0:9000 \
  -t 10.0.0.1/24 \
  -k "your-key" \
  -mtu 1200 \
  -fec-data 5 -fec-parity 1 \
  -send-queue 500 -recv-queue 500 \
  -max-clients 5 \
  -p2p=false \
  -nat-detection=false
```

**中等负载（10-15客户端）**
```bash
sudo ./lightweight-tunnel -c configs/low-spec-moderate.json
```

**优化效果**：
- 内存占用：从 500MB+ 降至 40-60MB
- CPU 使用：避免 P2P/Mesh 路由开销
- 带宽开销：FEC 从 30% 降至 20%

### 网络环境适配

**高速稳定网络**
```bash
-mtu 1400 \
-fec-data 20 -fec-parity 2 \
-send-queue 10000 -recv-queue 10000
```

**弱网/高丢包环境**
```bash
-mtu 1200 \
-fec-data 10 -fec-parity 5 \
-send-queue 2000 -recv-queue 2000
```

**移动网络**
```bash
-mtu 0  # 启用自动检测
```

### 大规模部署（50+客户端）

使用配置文件设置：
```json
{
  "broadcast_throttle_ms": 1000,
  "enable_incremental_update": true,
  "max_peer_info_batch_size": 10,
  "route_advert_interval": 300,
  "p2p_keepalive_interval": 25
}
```

**优化效果**：
- 广播流量减少 ~80%
- 总体控制流量减少 ~60-70%

### 连接健康监控与自动恢复

**问题**：运营商可能主动导致长连接"假死"（连接未断开但无法传输数据）

**解决方案**：
- 自动 keepalive（默认 5 秒间隔）：双向发送心跳包检测连接状态
- 空闲超时检测（默认 15 秒）：超过阈值自动断开重连
- 快速故障恢复：检测到连接异常立即重连，保证服务连续性

**配置参数**：
```json
{
  "keepalive": 5,              // Keepalive间隔（秒），建议 3-10
  "timeout": 30                // 连接超时（秒）
}
```

**特点**：
- 自动检测并恢复"假死"连接
- 支持网络切换（4G/5G/WiFi）自动重连
- 断线重连期间数据缓存在队列中，恢复后继续传输

---

## 故障排查

### 连接问题

**客户端无法连接服务器**
```bash
# 1. 检查服务端运行
sudo netstat -tulnp | grep 9000

# 2. 测试连通性
ping <服务器IP>
nc -zv <服务器IP> 9000

# 3. 检查防火墙
sudo ufw allow 9000/tcp
sudo ufw allow 9000/udp

# 4. 查看日志
sudo journalctl -u lightweight-tunnel-server -n 50
```

**密钥错误**
```
错误：Decryption error (wrong key?)
解决：确保服务端和客户端使用完全相同的 -k 参数
```

### 权限问题

**Raw Socket 需要 root**
```bash
# 方法 1：使用 sudo
sudo ./lightweight-tunnel ...

# 方法 2：授予 capabilities
sudo setcap cap_net_raw,cap_net_admin=eip ./lightweight-tunnel
./lightweight-tunnel ...
```

**TUN 设备不存在**
```bash
# 加载 TUN 模块
sudo modprobe tun

# 开机自动加载
echo "tun" | sudo tee -a /etc/modules
```

### 性能问题

**队列满错误**
```
错误：Send queue full, dropping packet
解决：增加队列大小或减少客户端数量
```
```bash
-send-queue 10000 -recv-queue 10000
```

**P2P 连接失败**
- 双方均为对称 NAT → 自动回退服务器中转
- 防火墙阻止 UDP → 检查并开放 P2P 端口
- 不影响使用，仅延迟略高

### 监控命令

```bash
# 查看内存占用
ps aux | grep lightweight-tunnel
top -p $(pgrep lightweight-tunnel)

# 查看网络流量
sudo iftop -i tun0
ip -s link show tun0

# 查看路由表
ip route

# 查看服务状态
sudo systemctl status lightweight-tunnel-server
```

---

## 安全建议

### 密钥管理

**生成强密钥**
```bash
# 使用 OpenSSL
openssl rand -base64 32

# 或使用 /dev/urandom
head -c 32 /dev/urandom | base64
```

**保护配置文件**
```bash
sudo chmod 600 /etc/lightweight-tunnel/config.json
sudo chown root:root /etc/lightweight-tunnel/config.json
```

### 防火墙配置

```bash
# Ubuntu/Debian
sudo ufw allow 9000/tcp
sudo ufw allow 9000/udp

# CentOS/RHEL
sudo firewall-cmd --add-port=9000/tcp --permanent
sudo firewall-cmd --add-port=9000/udp --permanent
sudo firewall-cmd --reload
```

### 安全限制

**可以防御**：
- ISP 流量内容窥探（加密）
- DPI 协议识别（TCP 伪装）
- 未授权连接（密钥认证）
- 中间人攻击（GCM 认证加密）

**不能防御**：
- 高级流量分析（行为特征）
- 端点被入侵
- 密钥泄露

---

## 高级功能

### 动态密钥轮换

服务端自动生成新密钥并推送给客户端：
```bash
sudo ./lightweight-tunnel \
  -m server \
  -k "initial-key" \
  -config-push-interval 600  # 每 10 分钟轮换
```

### 路由宣告

向对端宣告本地网段：
```bash
sudo ./lightweight-tunnel \
  -m client \
  -r <服务器IP>:9000 \
  -t 10.0.0.2/24 \
  -routes "192.168.1.0/24,192.168.2.0/24"
```

服务端会自动接收并安装路由。

### 多客户端组网

服务端启用多客户端：
```bash
sudo ./lightweight-tunnel \
  -m server \
  -multi-client \
  -max-clients 100
```

客户端可以相互通信（除非启用 client-isolation）。

---

## 技术架构

### 项目结构

```
lightweight-tunnel/
├── cmd/lightweight-tunnel/   # 主程序入口
├── internal/config/          # 配置管理
├── pkg/
│   ├── crypto/              # AES-256-GCM 加密
│   ├── faketcp/             # Raw Socket TCP 伪装
│   ├── fec/                 # Reed-Solomon 纠错
│   ├── p2p/                 # P2P 连接管理
│   ├── nat/                 # NAT 检测（STUN）
│   ├── routing/             # 智能路由表
│   ├── tunnel/              # 隧道核心逻辑
│   ├── xdp/                 # eBPF/XDP 加速
│   └── iptables/            # 防火墙规则管理
├── configs/                  # 配置模板
└── Makefile
```

### 核心组件

- **Raw Socket**：构造真实 TCP/IP 包
- **FEC**：前向纠错避免重传
- **XDP 加速**：缓存流分类决策
- **P2P Manager**：NAT 穿透和直连
- **路由表**：智能路径选择
- **加密层**：AES-256-GCM 端到端

---

## 参考资源

### 相关项目
- [udp2raw](https://github.com/wangyu-/udp2raw) - UDP 伪装 TCP
- [tinyfecVPN](https://github.com/wangyu-/tinyfecVPN) - FEC VPN
- [n2n](https://github.com/ntop/n2n) - P2P VPN

### 技术文档
- [Go 语言官方文档](https://go.dev/doc/)
- [Linux Raw Socket](https://man7.org/linux/man-pages/man7/raw.7.html)
- [TCP/IP 协议 RFC 793](https://www.rfc-editor.org/rfc/rfc793)
- [Reed-Solomon 纠错码](https://en.wikipedia.org/wiki/Reed%E2%80%93Solomon_error_correction)

---

## 开源协议

MIT License - 详见 [LICENSE](LICENSE)

---

## 联系方式

- **Issues**：[提交问题](https://github.com/openbmx/lightweight-tunnel/issues)
- **Pull Requests**：[贡献代码](https://github.com/openbmx/lightweight-tunnel/pulls)
- **Discussions**：[讨论区](https://github.com/openbmx/lightweight-tunnel/discussions)

---

## 更新日志

### v1.0.0 (当前版本)

**核心功能**：
- Raw Socket 真实 TCP 伪装
- 多客户端 Hub 模式
- P2P 直连和 NAT 穿透
- AES-256-GCM 加密
- 自动 MTU 检测
- FEC 前向纠错
- 自动重连机制

**性能优化**：
- 队列从 1000 增至 5000
- 改进 P2P 连接
- 优化重连策略
- 低配服务器支持

---

<div align="center">

**感谢使用 Lightweight Tunnel**

如果有帮助，请给个 ⭐ Star

Made with ❤️ by the Lightweight Tunnel Team

[⬆ 返回顶部](#lightweight-tunnel)

</div>
