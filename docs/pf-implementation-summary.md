# macOS PF 实施总结

## 当前状态

### ✅ 已实现的修复

1. **macOS客户端不绑定send socket**
   - 文件：pkg/rawsocket/rawsocket.go
   - 效果：避免macOS内核因"端口监听"而发送RST
   - 状态：已提交并测试

2. **服务端iptables规则**
   - 文件：pkg/iptables/iptables.go
   - 规则：`iptables -A OUTPUT -p tcp --tcp-flags RST RST --sport 9000 -j DROP`
   - 效果：DROP服务端发送的所有RST包
   - 状态：已部署

### ⚠️ 仍存在的问题

从测试结果看：
1. **RST包仍被接收**
   - 服务端日志：`Received RST from 124.126.5.85:xxxxx, closing connection`
   - 连接建立后不久就被RST关闭
   - 原因：RST可能来自客户端macOS内核或网络中间设备

2. **连接不稳定**
   - 客户端持续重连
   - 日志：`Network read error: read tcp: timeout`
   - 原因：RST导致连接频繁断开

3. **隧道功能测试失败**
   - ping 10.0.0.1：**100%丢包**
   - SOCKS5代理：**超时失败**

## PF 方案说明

### 方案1: 服务端使用DROP所有RST包（已实现）
**状态**: ✅ 已实施

**iptables规则**：
```bash
iptables -A OUTPUT -p tcp --tcp-flags RST RST --sport 9000 -j DROP
```

**效果**：
- ✅ DROP所有服务端发送的RST包
- ⚠️ 无法阻止来自网络中间设备的RST
- ⚠️ 可能影响其他TCP连接的RST（正常行为）

### 方案2: macOS客户端使用pf阻止RST发送

**目标**: 在macOS上使用pf (packet filter) 阻火墙代替Linux的iptables

**实现方式**：
1. 修改pkg/faketcp/faketcp_raw.go
2. 添加pf规则调用：
   ```bash
   # 在NewConnRaw中添加
   pfctl -e "block drop out proto tcp from any port any to any port <remote_port> flags R/RST"
   ```

**效果**：
- ✅ 阻止macOS内核发送RST包
- ✅ 更精确的规则（只阻止RST，不影响ACK等）
- ⚠️ 需要root权限

### 方案3: 增强iptables规则（待测试）

**更激进的规则**：
```bash
# 移除所有RST规则
iptables -D OUTPUT -p tcp --tcp-flags RST RST

# 添加激进DROP规则
iptables -A OUTPUT -p tcp --tcp-flags RST RST -j DROP
iptables -A OUTPUT -p tcp --tcp-flags RST RST --sport 9000 -j DROP
iptables -A OUTPUT -p tcp --tcp-flags RST RST --dport 9000 -j DROP
```

### 方案4: 连接保活优化（待实现）

**目标**：通过更频繁的keepalive减少RST触发

**实现方式**：
```go
// 降低keepalive间隔
const keepaliveInterval = 5 * time.Second  // 从30s改为5s

// 降低超时
const readTimeout = 10 * time.Second  // 从30s改为10s

// 启动keepalive定时器
func (c *ConnRaw) startKeepalive() {
	ticker := time.NewTicker(keepaliveInterval)
	go func() {
		for range ticker.C {
			c.sendKeepalive()
		}
	}
}

// keepalive包：发送带padding的ACK包
func (c *ConnRaw) sendKeepalive() error {
	padding := bytes.Repeat([]byte{0}, 20)
	err := c.rawSocket.SendPacket(c.localIP, c.localPort, c.remoteIP, c.remotePort,
		c.seqNum, c.ackNum, ACK, c.buildTCPOptions(), padding)
	if err != nil {
		return fmt.Errorf("failed to send keepalive: %v", err)
	}
	log.Printf("Sent keepalive to %s:%d", c.remoteIP, c.remotePort)
	c.lastActivity = time.Now()
}
```

## 测试结果对比

| 测试项 | 初始状态 | rawsocket修复 | iptables修复 |
|-------|---------|-------------|--------------|
| TCP三次握手 | ✅ | ✅ | ✅ |
| 连接建立 | ⚠️ 立即RST | ⚠️ 仍RST | ⚠️ 仍RST |
| 数据包收发 | ✅ | ✅ | ✅ |
| 路由添加 | ✅ | ✅ | ✅ |
| 加密通信 | ✅ | ✅ | ✅ |
| P2P功能 | ✅ | ✅ | ✅ |
| ping隧道 | ❌ | ❌ | ❌ |
| SOCKS5代理 | ❌ | ❌ | ❌ |

## 代码修改记录

### pkg/rawsocket/rawsocket.go
**修改**: 移除macOS客户端send socket的bind调用

**提交**: commit 1374b8f

### pkg/iptables/iptables.go  
**修改**: 添加DROP所有RST包规则

**提交**: 4669f48

## 下一步建议

### 立即行动
1. **简化方案**：专注于连接稳定性，不改变核心协议
2. **keepalive优化**：增加keepalive频率到5秒
3. **超时调优**：降低read timeout到10秒
4. **重连优化**：更快的重连间隔

### 长期方案
1. **pf集成**：完整实现macOS pf防火墙支持
2. **连接复用**：实现连接池减少握手
3. **多路径**：支持同时通过多个路径传输数据
4. **协议优化**：考虑使用QUIC或其他协议

## 环境信息

### 测试环境
- **客户端**: macOS (ARM64, Apple Silicon)
- **服务端**: Linux (x86_64)
- **客户端IP**: 124.126.5.85:50753
- **服务端IP**: 154.17.4.187:9000
- **隧道网络**: 10.0.0.0/24
- **SOCKS5**: 10.0.0.1:1080 (服务器上运行)

### 网络路径
```
[macOS客户端] --> [公网] --> [NAT] --> [防火墙] --> [Linux服务端]
(124.126.5.85:50753)   (154.17.4.187)    (防火墙)    (内核RST?)
```

**可能的问题点**:
1. NAT防火墙（154.17.4.187）可能识别原始TCP包为异常
2. 公网到服务器中间的网络设备可能过滤TCP特征
3. macOS内核的Raw Socket实现可能有限制
4. 服务端iptables规则可能不够精确

## 结论

### 当前进度
- ✅ **基础功能正常**：连接建立、数据传输、路由、加密
- ⚠️ **稳定性问题**：RST包导致连接不稳定
- ❌ **隧道功能失效**：ping和SOCKS5无法使用

### 建议
1. **短期**：实施连接保活优化（方案4）
2. **中期**：增加更多iptables规则或使用pf（方案2/3）
3. **长期**：重构RST处理逻辑或考虑协议变更

