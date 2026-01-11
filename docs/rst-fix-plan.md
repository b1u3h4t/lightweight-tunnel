# RST 问题优化方案 - 4个方案逐个测试

## 背景
- 问题：服务端持续收到RST包并关闭连接
- 根本原因：macOS客户端Socket绑定导致内核发送RST

## 方案列表

### 方案1: 服务端DROP所有RST包（最激进）
**原理**: 禁止服务端发送任何RST包

**修改**: pkg/iptables/iptables.go

**iptables规则**:
```bash
# 移除现有规则
iptables -D OUTPUT -p tcp --tcp-flags RST RST --sport 9000 2>/dev/null
iptables -D OUTPUT -p tcp --tcp-flags RST RST --dport 9000 2>/dev/null

# 添加激进规则
iptables -A OUTPUT -p tcp --tcp-flags RST -j DROP
iptables -A OUTPUT -p tcp --tcp-flags RST RST --sport 9000 -j DROP
iptables -A OUTPUT -p tcp --tcp-flags RST RST --dport 9000 -j DROP
```

**优点**: 最简单，直接阻止所有RST
**缺点**: 可能影响其他TCP连接的RST（正常行为）

---

### 方案2: 完全禁用macOS客户端iptables
**原理**: macOS客户端根本不使用iptables，依赖libpcap回退

**修改**: pkg/iptables/iptables.go

**代码**:
```go
func (m *IPTablesManager) AddRuleForPort(port uint16, isServer bool) error {
	if runtime.GOOS == "darwin" {
		// macOS: 完全跳过所有iptables操作
		// 依赖libpcap处理所有包
		log.Printf("macOS: skipping ALL iptables operations, relying on libpcap")
		return nil
	}
	// Linux: 原有逻辑
	...
}
```

**优点**: 避免iptables相关的任何问题
**缺点**: libpcap需要处理所有TCP状态

---

### 方案3: 服务端精确匹配四元组
**原理**: 只DROP特定四元组（五元组）的RST

**修改**: pkg/faketcp/faketcp_raw.go - 为每个连接添加精确规则

**代码**:
```go
// 为每个连接添加DROP规则
func (c *ConnRaw) addConnectionSpecificRules() error {
	if c.isListener {
		return nil
	}
	
	// 获取远程地址
	rule := fmt.Sprintf(
		"OUTPUT -p tcp --tcp-flags RST RST -d %s --dport %d -j DROP",
		c.remoteIP.String(),
		c.remotePort,
	)
	
	cmd := exec.Command("iptables", strings.Split(rule, " ")...)
	if err := cmd.Run(); err != nil {
		log.Printf("Failed to add connection rule: %v", err)
	}
	
	log.Printf("Added connection-specific rule: %s", rule)
}

func (c *ConnRaw) removeConnectionSpecificRules() error {
	// 清理连接特定规则
	rule := fmt.Sprintf(
		"OUTPUT -p tcp --tcp-flags RST RST -d %s --dport %d -j DROP",
		c.remoteIP.String(),
		c.remotePort,
	)
	
	cmd := exec.Command("iptables", strings.Split(rule, " ")...)
	cmd.Run() // 忽略错误
}
```

**优点**: 精确控制，只影响特定连接
**缺点**: 连接数量多时规则太多

---

### 方案4: 调整连接保活和超时
**原理**: 增加keepalive发送频率，减少超时

**修改**: pkg/faketcp/faketcp_raw.go

**参数调整**:
```go
// 减少超时
const handshakeTimeout = 15 * time.Second  // 从30s改为15s

// 增加keepalive
const keepaliveInterval = 10 * time.Second  // 从30s改为10s

// 增加重连间隔
const maxBackoff = 8 * time.Second  // 从32s改为8s
```

**代码**:
```go
// 在recvLoop中添加keepalive逻辑
func (c *ConnRaw) sendKeepalive() error {
	// 发送ACK包keepalive
	keepalive := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0} // 20字节padding
	
	err := c.rawSocket.SendPacket(c.localIP, c.localPort, c.remoteIP, c.remotePort,
		c.ackNum, c.ackNum, ACK|c.seqNum, nil, keepalive)
	if err != nil {
		return fmt.Errorf("failed to send keepalive: %v", err)
	}
	
	log.Printf("Sent keepalive to %s:%d", c.remoteIP, c.remotePort)
	return nil
}

// 启动keepalive定时器
func (c *ConnRaw) startKeepalive() {
	ticker := time.NewTicker(keepaliveInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.sendKeepalive()
		}
	}
}
```

**优点**: 减少超时，保持连接活跃
**缺点**: 增加少量开销

---

## 测试顺序

1. 方案1：激进DROP RST
2. 方案2：macOS完全不用iptables
3. 方案3：精确匹配四元组
4. 方案4：增加保活和调整超时

每个方案测试：
- 启动服务器
- 启动客户端
- 观察日志中的RST
- 测试ping 10.0.0.1
- 测试SOCKS5代理
- 记录结果
