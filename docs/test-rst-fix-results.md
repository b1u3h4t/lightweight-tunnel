# macOS RST 问题修复 - 测试报告

## 测试日期
2026-01-11

## 问题描述

### 初始问题
客户端和服务端TCP三次握手成功后，服务端不断收到RST包并关闭连接，导致连接不稳定和持续重连。

### 根本原因分析

#### 问题 1: 服务端iptables规则不正确
- **原始规则**: `iptables -A OUTPUT -p tcp --tcp-flags RST RST --sport 9000 -j DROP`
- **问题**: 只DROP源端口为9000的RST包
- **实际情况**: 客户端使用随机源端口，服务端发送的RST源端口是9000，但目标端口是客户端的随机端口
- **结果**: iptables规则无法匹配到RST包

#### 问题 2: macOS客户端Socket绑定问题
- **原始行为**: macOS客户端将send socket绑定到特定IP/Port
- **问题**: 绑定后，macOS内核会监控该端口并发送RST
- **结果**: 当macOS看到"自己端口"上的原始TCP包时，会发送RST

## 实施的修复

### 修复 1: macOS客户端不绑定send socket
**文件**: `pkg/rawsocket/rawsocket.go`

**修改**:
```go
// 之前
if localIP != nil {
    addr := syscall.SockaddrInet4{
        Port: 0,
    }
    copy(addr.Addr[:], localIP.To4())
    syscall.Bind(sendFd, &addr)
}

// 之后
// On macOS, DON'T bind to a specific IP/port
// Let kernel choose source address/port automatically
// This prevents macOS from sending RST when it sees raw TCP packets
```

**原理**: 
- 不绑定后，内核会自动选择源地址和随机端口
- 这样就不会因为看到"自己端口"的原始包而发送RST
- 类似Linux上的UDP客户端行为

### 修复 2: 服务端iptables规则匹配目标端口
**文件**: `pkg/iptables/iptables.go`

**修改**:
```go
// 之前
rule = fmt.Sprintf("OUTPUT -p tcp --tcp-flags RST RST --sport %d -j DROP", port)

// 之后  
rule = fmt.Sprintf("OUTPUT -p tcp --dport %d --tcp-flags RST RST -j DROP", port)
```

**原理**:
- 服务端只关心目标端口是9000的RST包
- DROP所有发往9000的RST包（无论源端口是什么）

## 测试结果

### 成功验证

| 测试项 | 修复前 | 修复后 | 状态 |
|-------|-------|--------|------|
| TCP三次握手 | ✅ 成功 | ✅ 成功 | 改进 |
| 连接建立 | ⚠️ 立即RST | ⚠️ 仍RST | 需进一步修复 |
| 数据包收发 | ✅ 正常 | ✅ 正常 | 通过 |
| 路由添加 | ✅ 成功 | ✅ 成功 | 通过 |
| 加密通信 | ✅ 正常 | ✅ 正常 | 通过 |
| P2P功能 | ✅ 正常 | ✅ 正常 | 通过 |
| 自动重连 | ✅ 正常 | ✅ 正常 | 通过 |
| SOCKS5代理 | ❌ 超时 | ❌ 超时 | 未解决 |
| ping隧道 | ❌ 超时 | ❌ 超时 | 未解决 |

### 持续问题

1. **RST包仍被接收**
   - 服务端日志显示: `Received RST from 124.126.5.85:xxxxx, closing connection`
   - 连接建立后不久就被RST关闭
   - 原因: RST可能来自客户端macOS内核或网络中间设备

2. **连接不稳定**
   - 客户端持续重连
   - 日志显示: `Network read error: read tcp: timeout, attempting reconnection...`
   - 重连循环导致无法稳定使用

## 待解决的方案

### 方案 1: 增强服务端iptables规则
```bash
# 添加更全面的RST DROP规则
iptables -A OUTPUT -p tcp --tcp-flags RST RST -j DROP
iptables -A OUTPUT -p tcp --tcp-flags SYN,RST SYN,RST -j DROP
```

### 方案 2: 客户端使用netfilter/iptables规则
虽然macOS使用pf，但可以尝试在客户端也添加规则防止发送RST。

### 方案 3: 增加连接保活
减少RST触发的可能性：
- 增加keepalive包发送频率
- 减少超时时间
- 更频繁地发送数据包保持连接活跃

### 方案 4: 使用连接复用
减少握手和RST：
- 使用长连接而不是频繁重连
- 连接断开后才重新建立

## 代码更改汇总

### 文件: pkg/rawsocket/rawsocket.go
```diff
@@ -58,7 +58,7 @@
 	} else {
-		// Do NOT set IP_HDRINCL - let kernel build IP header
-		// Bind to local IP if available
-		if localIP != nil {
-			addr := syscall.SockaddrInet4{
-				Port: 0,
-			}
-			copy(addr.Addr[:], localIP.To4())
-			syscall.Bind(sendFd, &addr)
-		}
-	}
+		// Do NOT set IP_HDRINCL - let kernel build IP header
+		// On macOS, DON'T bind to a specific IP/port
+		// Binding causes kernel to send RST when it sees raw TCP packets
+		// from a port it thinks should be "listening"
+		//
+		// NOTE: The source IP will be determined by kernel based on routing
+		// The source port will be random (as expected for outgoing connections)
 }
```

### 文件: pkg/iptables/iptables.go
```diff
@@ -37,9 +37,9 @@
 	var rule string
 	if isServer {
-		// Server: drop RST packets sent by kernel for incoming connections on this port
-		rule = fmt.Sprintf("OUTPUT -p tcp --tcp-flags RST RST --sport %d -j DROP", port)
+		// Server: drop RST packets in response to raw TCP packets
+		// Match TCP packets destined to port 9000 with RST flag set
+		// This catches RST packets kernel sends when it doesn't recognize our raw TCP sessions
+		rule = fmt.Sprintf("OUTPUT -p tcp --dport %d --tcp-flags RST RST -j DROP", port)
 	} else {
```

## 测试日志

### 服务端日志关键行
```
2026/01/11 02:40:09 Client connected: 64.49.46.146:27057
2026/01/11 02:40:09 Sent public address to client
2026/01/11 02:40:11 Received and stored peer info from client
2026/01/11 02:40:11 Client registered with IP: 10.0.0.5 (total clients: 1)
2026/01/11 02:40:14 Received and stored peer info from client: 10.0.0.5|64.49.46.146:52997
2026/01/11 02:40:14 Stored peer info for 10.0.0.5, ready for on-demand P2P
2026/01/11 02:40:19 Client connected: 124.126.5.85:27927
2026/01/11 02:40:19 Sent public address 124.126.5.85:27927 to client
2026/01/11 02:40:25 Received and stored peer info from client: 10.0.0.2|124.126.5.85:64582|192.168.1.7:64582|2
2026/01/11 02:40:25 Client registered with IP: 10.0.0.2 (total clients: 2)
2026/01/11 02:40:25 Stored peer info for 10.0.0.2, ready for on-demand P2P
2026/01/11 02:40:25 Received RST from 124.126.5.85:27927, closing connection
```

### 客户端日志关键行
```
2026/01/11 10:40:19 Handshake completed successfully!
2026/01/11 10:40:19 Raw TCP connection established: 192.168.1.7:27927 -> 154.17.4.187:9000
2026/01/11 10:40:19 Connected to server: 192.168.1.7:27927 -> 154.17.4.187:9000
2026/01/11 10:40:19 P2P manager listening on UDP port 52088
2026/01/11 10:40:19 Tunnel started in client mode
2026/01/11 10:40:19 Applied peer route 10.0.0.1/24 via utun3
2026/01/11 10:47:23 Network read error: read tcp: timeout, attempting reconnection...
2026/01/11 10:47:23 Attempting to reconnect to server at 154.17.4.187:9000 (backoff 1s)
2026/01/11 10:47:23 Starting handshake to 154.17.4.187:9000 (local: 192.168.1.7:52815)
```

## 结论

### 部分成功
1. ✅ TCP三次握手成功
2. ✅ 连接初始建立
3. ✅ 数据包收发正常
4. ✅ 路由添加正确
5. ✅ 加密通信正常
6. ✅ P2P功能正常

### 仍存在问题
1. ❌ RST包导致连接不稳定
2. ❌ 持续超时和重连循环
3. ❌ SOCKS5代理无法使用（连接超时）
4. ❌ ping隧道失败

### 建议下一步
1. **完全禁用RST** - 服务端添加更激进的iptables规则DROP所有RST
2. **连接保活优化** - 增加keepalive频率
3. **添加连接复用** - 减少重新握手
4. **调试RST来源** - 抓包确认RST来源（内核、网络、中间设备）

## 文件状态

已修改的文件:
- pkg/rawsocket/rawsocket.go (macOS socket绑定修复)
- pkg/iptables/iptables.go (服务端iptables规则修复)

待测试:
- 服务端增强iptables规则
- 客户端连接保活优化
- 连接复用实现
