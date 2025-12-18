# Lightweight Tunnel (轻量级内网隧道)

一个用 Go 编写的高性能、低延迟内网穿透隧道。它通过 UDP 模拟 TCP 流量（Fake-TCP）绕过防火墙，并支持服务器协同的 P2P 打洞，实现客户端间的直接互联。

---

## 🚀 核心特性

- **Fake-TCP 伪装**：基于 UDP 模拟 TCP 握手与头部，有效绕过运营商对 UDP 的限制或防火墙拦截。
- **服务器协同打洞 (PUNCH)**：服务端主动协调客户端进行双向同步打洞，显著提升 P2P 直连成功率。
- **强加密保护**：内置 AES-256-GCM 对称加密（通过 `-k` 参数），确保所有流量（包括中转与 P2P）的私密性。
- **FEC 纠错**：可选的前向纠错（Forward Error Correction）机制，在丢包严重的网络环境下保持稳定。
- **智能路由**：自动探测路径质量，优先使用 P2P 直连，失败时自动回退到服务器中转。
- **多客户端支持**：服务端支持 Hub 模式，允许多个客户端组成虚拟局域网。

---

## 🛠️ 快速开始

### 1. 启动服务端
在具有公网 IP 的服务器上运行：
```bash
sudo ./lightweight-tunnel -m server -l 0.0.0.0:9000 -t 10.0.0.1/24 -k "你的密钥"
```

### 2. 启动客户端 A
```bash
sudo ./lightweight-tunnel -m client -r <服务器IP>:9000 -t 10.0.0.2/24 -k "你的密钥"
```

### 3. 启动客户端 B
```bash
sudo ./lightweight-tunnel -m client -r <服务器IP>:9000 -t 10.0.0.3/24 -k "你的密钥"
```

**测试连接**：在客户端 A 上执行 `ping 10.0.0.3`。系统会尝试打洞，成功后流量将不经过服务器直接传输。

---

## ⚙️ 关键配置说明

### 命令行参数
| 参数 | 说明 | 示例 |
| :--- | :--- | :--- |
| `-m` | 运行模式：`server` 或 `client` | `-m client` |
| `-l` | 服务端监听地址 | `-l 0.0.0.0:9000` |
| `-r` | 客户端连接的远端服务器地址 | `-r 1.2.3.4:9000` |
| `-t` | 隧道虚拟网卡地址 | `-t 10.0.0.2/24` |
| `-k` | **对称加密密钥** (推荐必填) | `-k "mysecret"` |
| `-fec-data` | FEC 数据分片数 (默认 10) | `-fec-data 10` |
| `-fec-parity` | FEC 校验分片数 (默认 3) | `-fec-parity 3` |

### 配置文件 (JSON)
推荐使用配置文件以管理复杂设置：`./lightweight-tunnel -c config.json`
```json
{
  "mode": "client",
  "remote_addr": "1.2.3.4:9000",
  "tunnel_addr": "10.0.0.2/24",
  "key": "your-secret-key",
  "p2p_enabled": true,
  "mtu": 1400
}
```

---

## 🔍 P2P 机制与局限性

1. **协同打洞**：当新客户端上线时，服务器会向所有在线节点发送 `PUNCH` 指令。收到指令的节点会立即向新节点发起 UDP 探测，从而在 NAT 网关上“打洞”。
2. **路由优先级**：本地局域网直连 > 公网 P2P 直连 > 服务器中转。
3. **局限性**：
   - **对称 NAT (Symmetric NAT)**：在双方均为严格对称 NAT 的环境下，UDP 打洞可能失败，流量将自动回退到服务器中转。
   - **Relay 模式**：代码架构预留了多跳中继（Relay）位置，但目前尚未实现，请勿依赖。
   - **TLS 说明**：已移除原有的 `-tls` 命令行选项。如需 TLS，请在传输层外部实现，或直接使用内置的 `-k` 加密。

---

## ⚠️ 安全建议
- **务必设置密钥**：不带 `-k` 参数运行会导致流量明文传输。
- **Root 权限**：由于需要创建 TUN 虚拟网卡，程序必须以 `sudo` 或 root 权限运行。

联系与许可证
---
项目遵循 LICENSE 文件中的开源协议。有关安全问题，请参阅 `SECURITY.md`。

---
（已移除 README 中对 CLI TLS 旗标的示例；有关加密请优先使用 `-k` 对称密钥或在传输层采用外部 TLS 代理）
# 轻量级内网隧道

一个使用 Go 语言开发的轻量级内网隧道工具，使用 **UDP 传输并添加伪装 TCP 头部**来绕过防火墙，同时支持 **AES-256-GCM 加密**和 FEC 纠错功能，适用于在多个服务器之间建立安全的虚拟内网连接。

## 主要特性

- 🚀 **轻量高效** - 资源占用少，适合低配置服务器
- 🔐 **AES-256-GCM 加密** - 使用 `-k` 参数启用加密，防止未授权连接
- 🎭 **TCP 伪装** - UDP 数据包伪装成 TCP 连接，可穿透防火墙
- ⚡ **UDP 传输** - 实际使用 UDP 传输，避免 TCP-over-TCP 问题
- 🛡️ **FEC 纠错** - 自动纠正丢包，提升弱网环境下的稳定性
- 🌐 **多客户端** - 支持多个客户端同时连接，客户端之间可互相通信
- 🔄 **P2P 直连** - 支持客户端之间 P2P 直接连接，无需服务器中转
- 🧠 **智能路由** - 自动选择最优路径：P2P、中继或服务器转发
- 🌐 **网状网络** - 支持通过其他客户端中继流量，实现多跳转发
- ⚡ **高性能** - 基于 Go 协程实现高并发处理
- 🎯 **简单易用** - 支持命令行和配置文件两种方式

## 🔐 安全说明

### 推荐：使用 `-k` 参数加密

使用 `-k` 参数可以启用 **AES-256-GCM 加密**，这是推荐的安全配置：

```bash
# 服务端（启用加密）
sudo ./lightweight-tunnel -m server -l 0.0.0.0:9000 -t 10.0.0.1/24 -k "your-secret-key"

# 客户端（使用相同密钥）
sudo ./lightweight-tunnel -m client -r <服务器IP>:9000 -t 10.0.0.2/24 -k "your-secret-key"
```

**加密的优势：**
- ✅ 所有隧道流量（包括 P2P）都会被加密
- ✅ 防止未授权用户连接到您的隧道
- ✅ 防止中间人攻击和流量窃听
- ✅ 密钥不匹配的连接会被自动拒绝

**注意：** 服务端和客户端必须使用相同的密钥，否则无法通信。

### 无加密模式（仅用于测试）

如果不使用 `-k` 参数，隧道将不加密，**不建议在生产环境使用**：

```bash
# ⚠️ 警告：此模式无加密，仅用于测试
sudo ./lightweight-tunnel -m server -l 0.0.0.0:9000 -t 10.0.0.1/24
```

详见 [SECURITY.md](SECURITY.md)。

## 快速开始

### 系统要求

- Linux 系统（需要 TUN 设备支持）
- Root 权限（用于创建和配置 TUN 设备）
- Go 1.19+ （仅编译时需要）

### 安装

```bash
# 克隆仓库
git clone https://github.com/openbmx/lightweight-tunnel.git
cd lightweight-tunnel

# 编译
go build -o lightweight-tunnel ./cmd/lightweight-tunnel

# 或者直接安装
go install ./cmd/lightweight-tunnel
```

### 基本使用

#### 场景一：带加密的安全隧道（推荐）

**服务端：**
```bash
sudo ./lightweight-tunnel -m server -l 0.0.0.0:9000 -t 10.0.0.1/24 -k "my-secret-key"
```

**客户端：**
```bash
sudo ./lightweight-tunnel -m client -r <服务器IP>:9000 -t 10.0.0.2/24 -k "my-secret-key"
```

**测试连接：**
```bash
# 在客户端执行
ping 10.0.0.1
```

#### 场景二：简单的点对点连接（仅测试用，无加密）

**服务端：**
```bash
sudo ./lightweight-tunnel -m server -l 0.0.0.0:9000 -t 10.0.0.1/24
```

**客户端：**
```bash
sudo ./lightweight-tunnel -m client -r <服务器IP>:9000 -t 10.0.0.2/24
```

**⚠️ 注意：** 此模式下流量未加密，任何人都可以连接。仅建议在测试环境使用。

#### 场景三：多客户端组网（带加密）

服务端默认支持多客户端连接，所有客户端可以互相通信：

**服务端：**
```bash
sudo ./lightweight-tunnel -m server -l 0.0.0.0:9000 -t 10.0.0.1/24 -k "shared-secret"
```

**客户端 1：**
```bash
sudo ./lightweight-tunnel -m client -r <服务器IP>:9000 -t 10.0.0.2/24 -k "shared-secret"
```

**客户端 2：**
```bash
sudo ./lightweight-tunnel -m client -r <服务器IP>:9000 -t 10.0.0.3/24 -k "shared-secret"
```

**客户端 3：**
```bash
sudo ./lightweight-tunnel -m client -r <服务器IP>:9000 -t 10.0.0.4/24 -k "shared-secret"
```

连接后，客户端之间可以直接通信：
```bash
# 在客户端 1 上 ping 客户端 2
ping 10.0.0.3

# 在客户端 1 上 SSH 到客户端 3
ssh user@10.0.0.4
```

#### 场景四：P2P 直连模式（带加密）

启用 P2P 模式，客户端之间会自动尝试建立直接连接，无需通过服务器中转。P2P 流量同样会被加密：

**服务端：**
```bash
sudo ./lightweight-tunnel -m server -l 0.0.0.0:9000 -t 10.0.0.1/24 -k "p2p-secret"
```

**客户端 1（启用 P2P）：**
```bash
sudo ./lightweight-tunnel -m client -r <服务器IP>:9000 -t 10.0.0.2/24 -p2p -k "p2p-secret"
```

**客户端 2（启用 P2P）：**
```bash
sudo ./lightweight-tunnel -m client -r <服务器IP>:9000 -t 10.0.0.3/24 -p2p -k "p2p-secret"
```

P2P 连接建立后：
- 流量直接在客户端之间传输，延迟更低
- **所有 P2P 流量都会被加密**
- 减少服务器带宽和 CPU 负载
- P2P 失败时自动回退到服务器转发

**查看路由状态：**
```bash
# 日志会显示路由统计
Routing stats: 2 peers, 1 direct, 0 relay, 1 server
# 表示：2个对等节点，1个P2P直连，0个中继，1个服务器路由
```

详细文档请参阅：[P2P_ROUTING.md](P2P_ROUTING.md)

### 使用配置文件

#### 生成示例配置

```bash
./lightweight-tunnel -g config.json
# 会生成 config.json（服务端）和 config.json.client（客户端）
```

#### 服务端配置示例

```json
{
  "mode": "server",
  "local_addr": "0.0.0.0:9000",
  "tunnel_addr": "10.0.0.1/24",
  "key": "your-secret-key",
  "mtu": 1400,
  "fec_data": 10,
  "fec_parity": 3,
  "timeout": 30,
  "keepalive": 10,
  "send_queue_size": 1000,
  "recv_queue_size": 1000,
  "multi_client": true,
  "max_clients": 100,
  "client_isolation": false
}
```

**重要说明：**
- `key`：加密密钥，服务端和客户端必须相同
- `multi_client`：未指定时默认为 `true`，允许多个客户端同时连接
- 如需限制为单客户端模式，显式设置为 `false`

#### 客户端配置示例

```json
{
  "mode": "client",
  "remote_addr": "<服务器IP>:9000",
  "tunnel_addr": "10.0.0.2/24",
  "key": "your-secret-key",
  "mtu": 1400,
  "fec_data": 10,
  "fec_parity": 3,
  "timeout": 30,
  "keepalive": 10,
  "send_queue_size": 1000,
  "recv_queue_size": 1000,
  "p2p_enabled": true,
  "p2p_port": 0,
  "enable_mesh_routing": true,
  "max_hops": 3,
  "route_update_interval": 30
}
```

**加密配置说明：**
- `key`：加密密钥，必须与服务端相同

**P2P 配置说明：**
- `p2p_enabled`：启用 P2P 直连（默认：true）
- `p2p_port`：UDP 端口，0 表示自动选择
- `enable_mesh_routing`：启用网状路由（默认：true）
- `max_hops`：最大跳数（默认：3）
- `route_update_interval`：路由更新间隔秒数（默认：30）

#### 使用配置文件运行

```bash
sudo ./lightweight-tunnel -c config.json
```

## 配置说明

### 命令行参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-c` | 配置文件路径 | - |
| `-m` | 模式：server 或 client | server |
| `-l` | 监听地址（服务端） | 0.0.0.0:9000 |
| `-r` | 服务器地址（客户端） | - |
| `-t` | 隧道 IP 地址和子网掩码 | 10.0.0.1/24 |
| `-k` | **加密密钥**（推荐使用） | - |
| `-mtu` | MTU 大小 | 1400 |
| `-fec-data` | FEC 数据分片数 | 10 |
| `-fec-parity` | FEC 校验分片数 | 3 |
| `-send-queue` | 发送队列大小 | 1000 |
| `-recv-queue` | 接收队列大小 | 1000 |
| `-multi-client` | 启用多客户端支持（服务端） | true |
| `-max-clients` | 最大客户端数量（服务端） | 100 |
| `-client-isolation` | 客户端隔离模式 | false |
| `-p2p` | 启用 P2P 直连 | true |
| `-p2p-port` | P2P UDP 端口（0=自动） | 0 |
| `-mesh-routing` | 启用网状路由 | true |
| `-max-hops` | 最大跳数 | 3 |
| `-route-update` | 路由更新间隔（秒） | 30 |
| `-tls` | 启用 TLS 加密（不推荐，因为使用 UDP） | false |
| `-tls-cert` | TLS 证书文件（服务端） | - |
| `-tls-key` | TLS 私钥文件（服务端） | - |
| `-tls-skip-verify` | 跳过证书验证（客户端，不安全） | false |
| `-v` | 显示版本 | - |
| `-g` | 生成示例配置文件 | - |

### 多客户端配置选项

**Hub 模式（默认，推荐带加密）：** 所有客户端可以互相通信
```bash
sudo ./lightweight-tunnel -m server -l 0.0.0.0:9000 -t 10.0.0.1/24 -k "my-key"
```

**客户端隔离模式：** 客户端只能与服务端通信，不能互访
```bash
sudo ./lightweight-tunnel -m server -l 0.0.0.0:9000 -t 10.0.0.1/24 -k "my-key" -client-isolation
```

**单客户端模式：** 只允许一个客户端连接
```bash
sudo ./lightweight-tunnel -m server -l 0.0.0.0:9000 -t 10.0.0.1/24 -k "my-key" -multi-client=false
```

## 性能调优

### 高速网络环境

```bash
# 增大 MTU
sudo ./lightweight-tunnel -mtu 8000 ...

# 减少 FEC 开销
sudo ./lightweight-tunnel -fec-data 20 -fec-parity 2 ...
```

### 高丢包网络环境

```bash
# 增加 FEC 冗余
sudo ./lightweight-tunnel -fec-data 10 -fec-parity 5 ...

# 减小 MTU
sudo ./lightweight-tunnel -mtu 1200 ...
```

### 高带宽场景

如果看到 "Send queue full, dropping packet" 错误，增大队列：
```bash
sudo ./lightweight-tunnel -send-queue 5000 -recv-queue 5000 ...
```

建议值：
- 普通使用：1000-5000
- 高带宽隧道：5000-10000

## 常见问题

### 权限错误

**问题：** `failed to open /dev/net/tun: permission denied`

**解决：** 使用 root 权限运行
```bash
sudo ./lightweight-tunnel ...
```

### TUN 设备不存在

**问题：** `/dev/net/tun: no such file or directory`

**解决：** 加载 TUN 模块
```bash
sudo modprobe tun
```

### 无法连接服务器

**排查步骤：**
1. 检查服务端是否运行：`netstat -tlnp | grep 9000`
2. 检查防火墙：`sudo ufw allow 9000/tcp`
3. 验证服务器 IP 地址是否正确
4. 测试网络连通性：`ping <服务器IP>`

### 第二个客户端连接失败（EOF/Broken Pipe）

**问题：** 第二个客户端报错 "Network read error: EOF" 或 "write: broken pipe"

**原因：** 服务端配置为单客户端模式（`multi_client: false`）

**解决方案：**

使用 JSON 配置文件时：
```json
{
  "mode": "server",
  "local_addr": "0.0.0.0:9000",
  "tunnel_addr": "10.0.0.1/24",
  "multi_client": true,
  "max_clients": 100
}
```

使用命令行时（默认已启用）：
```bash
sudo ./lightweight-tunnel -m server -l 0.0.0.0:9000 -t 10.0.0.1/24
```

### 密钥不匹配导致解密失败

**问题：** 日志显示 "Decryption error (wrong key?)"

**原因：** 服务端和客户端使用了不同的加密密钥

**解决方案：** 确保服务端和客户端使用完全相同的 `-k` 参数值

```bash
# 服务端
sudo ./lightweight-tunnel -m server -k "same-key" ...

# 客户端
sudo ./lightweight-tunnel -m client -k "same-key" ...
```

### 端口被占用

**问题：** `bind: address already in use`

**解决：** 查找并关闭占用端口的进程
```bash
# 查找进程
sudo lsof -i :9000

# 终止进程
sudo kill -9 PID
```

## 工作原理

1. **TUN 设备：** 创建虚拟网卡，处理三层 IP 数据包
2. **UDP 传输：** 使用 UDP 作为实际传输协议，避免 TCP-over-TCP 问题
3. **TCP 伪装：** 在 UDP 包外添加伪装的 TCP 头部，穿透防火墙（仅限服务器隧道）
4. **AES-256-GCM 加密：** 使用 `-k` 参数启用，加密所有隧道流量（包括 P2P）
5. **FEC 纠错：** 添加冗余数据分片，自动恢复丢失的数据包
6. **保活机制：** 定期发送心跳包维持连接
7. **P2P 直连：** 使用 UDP 打洞技术建立客户端之间的直接连接
8. **智能路由：** 根据连接质量自动选择最优路径（P2P/中继/服务器）

## 技术原理

### UDP + 伪装 TCP 头部

本项目的核心设计借鉴了 **udp2raw** 和 **tinyfecVPN**：

- **实际传输：** UDP（避免 TCP-over-TCP 性能问题）
- **外观伪装：** 添加 TCP 头部（端口、序列号、ACK、标志位）
- **防火墙绕过：** 简单的防火墙只检查包头，会认为这是 TCP 流量
- **性能优势：** 无头部阻塞、无重传延迟、适合实时应用

### 与参考项目对比

| 特性 | 本项目 | udp2raw | tinyfecVPN |
|------|-------|---------|------------|
| 传输协议 | UDP | UDP | UDP |
| TCP 伪装 | ✅ 简单 | ✅ 完整 (raw socket) | ❌ 无 |
| 内置加密 | ✅ AES-256-GCM | ❌ 无 | ❌ 无 |
| FEC 纠错 | ✅ XOR | ❌ 无 | ✅ Reed-Solomon |
| TUN/TAP | ✅ | ❌ | ✅ |
| P2P 直连 | ✅ | ❌ | ❌ |
| 语言 | Go | C++ | C++ |
| 复杂度 | 低 | 中 | 中 |

## 架构图

```
                    服务端 (10.0.0.1)
                          │
        ┌─────────────────┼─────────────────┐
        │                 │                 │
   客户端 1           客户端 2           客户端 3
  (10.0.0.2)        (10.0.0.3)        (10.0.0.4)
        │                 │                 │
        └═════════════════╪═════════════════┘
              P2P 直连（可选）

         AES-256-GCM 加密（-k 参数）
              ↓
        FEC 纠错 + TCP 伪装
              ↓
         TUN 虚拟网卡
```

## 限制说明

- 目前仅支持 IPv4
- 需要 root 权限创建 TUN 设备
- 仅支持 Linux 系统
- P2P 流量不使用 TCP 伪装（使用纯 UDP），但仍然加密
- P2P 需要 UDP 端口支持（可能被防火墙阻止）
- TCP 伪装可能被深度包检测（DPI）识破

## 为什么不用真正的 TCP？

使用真正的 TCP 会导致 **TCP-over-TCP 问题**：

- 内层 TCP（应用流量）和外层 TCP（隧道）都会重传丢失的包
- 双重重传导致性能急剧下降（可降低到原来的 1/10）
- 延迟增加，吞吐量减少
- 不适合实时应用（游戏、VoIP 等）

使用 **UDP + 伪装 TCP 头**的优势：

- ✅ 绕过简单的防火墙（只检查端口和协议）
- ✅ 避免 TCP-over-TCP 性能问题
- ✅ 无头部阻塞，适合实时应用
- ✅ FEC 纠错可以恢复丢包，无需重传
- ❌ 可能被深度包检测（DPI）识破

## 安全建议

### 推荐配置

1. **始终使用 `-k` 参数启用加密**
   ```bash
   # 使用强密钥
   sudo ./lightweight-tunnel -m server -k "complex-random-key-123!" ...
   ```

2. **使用强密钥**
   - 密钥长度建议 16 字符以上
   - 包含字母、数字和特殊字符
   - 避免使用常见词汇或简单密码

3. **定期更换密钥**
   - 生产环境建议定期更换加密密钥
   - 更换密钥需要同时更新服务端和所有客户端

更多安全信息请参阅 [SECURITY.md](SECURITY.md)，包括：
- 运营商可见性和深度包检测（DPI）
- GFW 和网络监控考量
- 威胁模型和最佳实践

## 参考项目

- [udp2raw](https://github.com/wangyu-/udp2raw) - UDP 到 TCP 转换工具
- [tinyfecvpn](https://github.com/wangyu-/tinyfecVPN) - 带 FEC 的 VPN

## 开源协议

MIT License

## 贡献指南

欢迎提交 Pull Request 和 Issue！
