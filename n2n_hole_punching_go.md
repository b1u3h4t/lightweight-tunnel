n2n — 多节点互访与高成功率 NAT 打洞（Go 实现指南）

概述
- 本文档把 n2n 的关键机制抽象为可移植到 Go 的实现要点，目标是实现多节点（Edge）互访与尽可能高的 NAT 打洞成功率。
- 参考代码位置：`include/n2n.h`、`src/edge.c`、`src/supernode.c`、`src/wire.c`、`src/tuntap_linux.c`。

一、核心理念与组成
- TAP/TUN（本地虚拟接口）：Edge 在本机创建 TAP（L2）或 TUN（L3），以读写原始帧/分组。实现参考：`src/tuntap_linux.c`（Linux 实现）。
- Wire 层（报文封装/编解码）：将 TAP 的以太帧封装进自定义 N2N 报文（带 community、cookie、MAC、socket info 等字段），发送到目标端。核心实现：`src/wire.c`。
- Supernode（引导 / 目录服务）：edge 向 supernode 周期性注册（REGISTER），supernode 保存外部观察到的 UDP/TCP endpoint（即 NAT 外映射），并在需要时返回对等端信息以促成 P2P 打洞。核心实现：`src/supernode.c` 与 `src/edge.c` 中的注册逻辑。
- Transform（加密/压缩）：在 payload 层支持 AES/ChaCha20/LZO/ZSTD 等，保证机密性与可选压缩。

二、多节点互访（High-level 流程）
1. Edge 启动并创建 TAP；绑定一个本地 UDP 端口 socket（重要：该 socket 的源端口应被用于发送注册包与后续 P2P 包，以保持 NAT 映射一致）。
2. Edge 周期性向配置的 supernode 发送 `REGISTER`（包含 community, cookie, srcMac, dstMac, socket 描述等），以让 supernode 记录 edge 的公网 endpoint（source IP:port）。
3. 当某 edge 需要与另一路 edge 通信：
   - 若已知对端 overlay IP/MAC，edge 会向 supernode 查询（或 supernode 在收到注册后主动下发），supernode 返回 `PEER_INFO`（包含对端观察到的外网/内网 socket）。
   - 收到 `PEER_INFO` 后，发起方与被发起方会开始向彼此的外部 endpoint 发送 UDP 小包（可能同时也向内网地址/端口尝试），通过同时发送触发 NAT 的映射建立或更新（即 UDP 打洞）。
4. 一旦包能直接双向到达，双方进入直接 P2P 通信路径，继续用相同端口发送 overlay 数据包（封装的以太帧）。
5. 若 P2P 未成功，Flow 会回退到经 supernode 中继（Supernode relay，通常较慢），或使用 TCP 穿透路径。

三、提升打洞成功率的 n2n 策略（从代码与设计层面总结）
- 稳定固定源端口：edge 使用单一 UDP 端口发送注册并用于 P2P。保持源端口/本地 socket 不变是成功穿透的首要条件（见 `edge` 的绑定行为）。
- 周期注册与 NAT 保活：edge 周期性发送 REGISTER（打洞与保持映射）。注册间隔与超时时间能影响 NAT 映射寿命与穿透成功率。实战上需频繁但不过载（默认实现使用较短间隔，参见 `REGISTER` 发送逻辑）。
- 同步发送（simultaneous open）：supernode 将双方外部 endpoint 互相通知后，双方立即开始向对方发送包，形成“同时发送”以破除对称 NAT 的阻碍。
- 发送多个目标（外网 & 私网 & supernode）：在收到对端信息后，先向对端的外网地址发送，同时尝试向其私网地址（若可得）和通过 supernode 转发地址发送，增加命中任意 NAT 映射的概率。
- 适度重试与幂等性：持续发送短小探测包（短间隔、多次）直到建立连接或超时。
- NAT 类型适配：对称 NAT 比较难打洞，n2n 倾向于通过 supernode 协助（转发或 TCP 连接）作为 fallback。
- 使用 header / payload 加密与 cookie 验证：防止伪造与中间人，确保双方用相同 community/key 时才接受解包。

四、在 Go 中实现的关键点与建议
1. TAP/TUN：使用成熟库创建虚拟接口并读写帧
   - 推荐库：`github.com/songgao/water`（跨平台 TUN/TAP），或 `golang.zx2c4.com/wireguard/tun`。
   - 读写循环需高吞吐、低延迟，建议使用带缓冲的 goroutine 池处理 I/O 与封包操作。

2. UDP Socket 绑定策略（关键）
   - 在本地创建并 bind 到固定本地端口（0.0.0.0:port 或 指定接口地址:port），并用该 socket 既发送注册包也发送后续 P2P 数据。保证发送的源端口与 supernode 注册时一致。
   - 在 Go 中：`net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: port})`。

3. Wire 编码/解码（与 n2n 兼容或自定义）
   - 定义与 `src/wire.c` 等效的报文结构（common header、REGISTER、PEER_INFO、PACKET），注意字节序与字段长度（MAC 6 字节、community 固定长度等）。
   - 示例：用 `encoding/binary`（BigEndian）读写固定字段，逐字段 memcpy 处理 MAC 与可变 payload。

4. 注册与 peer 信息交换（逻辑）
   - 实现 `sendRegister()`：构造 REGISTER 报文并用固定 UDP socket 发送到 supernode（host:port）。
   - 周期性发送：使用 `time.Ticker`，间隔应小于常见 NAT 映射超时时间（例如 20-60 秒内常见），可从 n2n 源码中获取默认值并微调。
   - 处理 supernode 回复：当接收到 `PEER_INFO`，提取对端外部 socket 信息并调用 `punchPeer()`。

5. 打洞逻辑：`punchPeer()`（实施细节）
   - 参数：peer 外网 endpoint（IP:port），可选私网 endpoint。
   - 行为：在短时间窗口内（例如 1-2 秒）以小间隔（如 20-200ms）连续向 peer 的外网地址发送 N 次（例如 6-20 次）小探测包；同时向私网地址发送探测（若可达），并向 supernode 发送 keepalive/告知（增加中继备选）。
   - 发送源：必须使用与注册相同的 UDP socket（相同源端口）。
   - 接收与验证：收到来自对端的探测/数据后，根据报文头 cookie/community 等验证身份，然后将对端加入 peer table，进入 P2P 通道。

6. 超时与回退
   - 如果在 T 秒内未成功，则切换到经 supernode 中继或尝试 TCP 连接。
   - 对称 NAT 与企业 NAT 场景建议优先使用 supernode relay（或 TURN-like 中继）。

五、Go 示例代码片段（伪代码，重点展示流程）

```go
// 简化结构体（示例）
type CommonHeader struct {
    Version uint8
    TTL     uint8
    Flags   uint16
    Community [64]byte // 与 n2n 源码的 N2N_COMMUNITY_SIZE 对齐
}

// 绑定 UDP（固定本地端口）
func bindUDP(port int) (*net.UDPConn, error) {
    laddr := &net.UDPAddr{IP: net.IPv4zero, Port: port}
    return net.ListenUDP("udp4", laddr)
}

// 发送注册包
func sendRegister(conn *net.UDPConn, supernode *net.UDPAddr, myMac [6]byte, myIP net.IP, community string) error {
    // 构造 common + register fields (cookie, macs, sock info, ip/subnet etc.)
    // 使用 bytes.Buffer + binary.Write (BigEndian)
    // 注意：字段长度/顺序需与目标 supernode 一致
    // 然后 conn.WriteToUDP(buf.Bytes(), supernode)
    return nil
}

// 打洞
func punchPeer(conn *net.UDPConn, peerAddr *net.UDPAddr) {
    payload := []byte("n2n-punch")
    for i := 0; i < 12; i++ {
        conn.WriteToUDP(payload, peerAddr)
        time.Sleep(100 * time.Millisecond)
    }
}
```

六、参数建议与调优
- 注册间隔：根据 NAT 行为，典型值 10–30 秒；对超短寿命 NAT（如某些移动运营商）可降到 5–10 秒，但注意带宽/负载。
- 探测频率与次数：20–200ms 间隔，6–20 次。过多会浪费带宽，过少则降低成功率。
- 并发限制：在大量 peer 同时建立时限制并发打洞 goroutine 数量，防止端口/带宽耗尽。

七、日志与可观测性
- 记录：注册发送/收到时间、supernode 返回的 peer endpoints、每次 punch 成功/失败事件与 RTT。
- 报表：统计打洞成功率（成功/尝试），便于调整参数。

八、兼容与安全注意
- 若要与原生 n2n 节点互通，必须严格实现 wire 报文格式（参见 `src/wire.c` 的 encode/decode 实现）。
- 使用 header/payload 加密时保证密钥协商或配置一致。cookie/timestamp 校验用于防重放和消息有效性判定（参见 `time_stamp` 等函数）。

九、测试建议（在开发时）
- 本地 NAT 仿真：在多台虚拟机/容器上部署 edge 与 supernode，模拟不同 NAT 类型（对称、端口保留型、地址限制等）。
- 抓包验证：使用 `tcpdump`/`wireshark` 在 supernode 与 edge 端口抓包，确认 REGISTER、PEER_INFO、PACKET 的字段与端口一致性。
- 指标：统计注册成功数、打洞成功数、回退到 relay 的次数。

十、参考代码位置
- 头与接口：`include/n2n.h`。
- Edge 主逻辑（注册/打洞/处理 TAP）：`src/edge.c`。
- Supernode（peer 列表/分配/回应）：`src/supernode.c`。
- Wire 编解码：`src/wire.c`。
- Linux TAP：`src/tuntap_linux.c`。

附：若需要，我可以：
- 提供一个更完整的 Go 实现样板（含 TAP 读写、wire 编解码、注册/打洞完整流程与小型测试 harness）；或
- 提取并逐字段翻译 `src/wire.c` 中的报文格式为 Go 的 struct/读写函数，以确保与 n2n 完全兼容。


---
文档生成于仓库 `docs/n2n_hole_punching_go.md`，如需我现在生成 Go 样板代码，请回复“生成样板”。
