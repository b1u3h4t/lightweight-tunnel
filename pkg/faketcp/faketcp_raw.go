package faketcp

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"log"
	"math/big"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/openbmx/lightweight-tunnel/pkg/iptables"
	"github.com/openbmx/lightweight-tunnel/pkg/rawsocket"
)

const (
	rawRecvQueueSize = 4096
	keepaliveInterval   = 5 * time.Second  // 方案4：5秒keepalive（从30秒改为5秒）
	keepalivePacketSize = 20              // 20字节padding的keepalive包
	staleConnectionTimeout = 5 * time.Second // 方案4：5秒空闲超时（从15秒改为5秒）
)

type ConnRaw struct {
	rawSocket     *rawsocket.RawSocket
	localIP       net.IP
	localPort     uint16
	remoteIP      net.IP
	remotePort    uint16
	srcPort       uint16
	dstPort       uint16
	seqNum        uint32
	ackNum        uint32
	mu            sync.Mutex
	isConnected   bool
	recvQueue     chan []byte
	closed        int32
	closeOnce     sync.Once
	iptablesMgr   *iptables.IPTablesManager
	stopCh        chan struct{}
	wg            sync.WaitGroup
	isListener    bool
	ownsResources bool
	lastActivity  time.Time
	// 方案4: macOS客户端keepalive相关字段
	hasKeepalive      bool
	keepaliveStop     chan struct{}
	keepaliveTimer    *time.Timer
}

func NewConnRaw(localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, isClient bool) (*ConnRaw, error) {
	isn, err := randomUint32()
	if err != nil {
		return nil, err
	}

	// Create raw socket
	rawSock, err := rawsocket.NewRawSocket(localIP, localPort, remoteIP, remotePort, !isClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create raw socket: %v", err)
	}

	// Create iptables manager
	iptablesMgr := iptables.NewIPTablesManager()
	if err := iptablesMgr.AddRuleForPort(localPort, !isClient); err != nil {
		rawSock.Close()
		return nil, fmt.Errorf("failed to add iptables rule: %v", err)
	}

	conn := &ConnRaw{
		rawSocket:     rawSock,
		localIP:       localIP,
		localPort:     localPort,
			remoteIP:      remoteIP,
		remotePort:    remotePort,
		srcPort:       localPort,
		dstPort:       remotePort,
		seqNum:        isn,
		ackNum:        0,
		isConnected:   false,
		recvQueue:     make(chan []byte, rawRecvQueueSize),
		iptablesMgr:   iptablesMgr,
		stopCh:        make(chan struct{}),
		wg:            sync.WaitGroup{},
		isListener:    false,
		ownsResources: true,
		lastActivity:  time.Now(),
		hasKeepalive:      false,
		keepaliveStop:     make(chan struct{}),
		keepaliveTimer:    time.NewTimer(staleConnectionTimeout),
	}

	return conn, nil
}

func (c *ConnRaw) performHandshake(timeout time.Duration) error {
	tcpOptions := c.buildTCPOptions()
	maxRetries := 3
	retryInterval := 500 * time.Millisecond

	log.Printf("Starting handshake to %s:%d (local: %s:%d)", c.remoteIP, c.remotePort, c.localIP, c.localPort)

	for retry := 0; retry < maxRetries; retry++ {
		if retry > 0 {
			log.Printf("Handshake retry %d/%d", retry+1, maxRetries)
			time.Sleep(retryInterval)
		}

		err := c.rawSocket.SendPacket(c.localIP, c.localPort, c.remoteIP, c.remotePort,
			c.seqNum, 0, SYN, tcpOptions, nil)
		if err != nil {
			log.Printf("Failed to send SYN: %v", err)
			continue
		}

		deadline := time.Now().Add(timeout / time.Duration(maxRetries))
		log.Printf("Waiting for SYN-ACK until %v", deadline)
		packetCount := 0

		for time.Now().Before(deadline) {
			select {
			case data := <-c.recvQueue:
				packetCount++
				if len(data) < TCPHeaderSize {
					log.Printf("Received packet too small: %d bytes", len(data))
					continue
				}
				hdr := parseTCPHeader(data)
				if hdr == nil {
					log.Printf("Failed to parse TCP header from %d bytes", len(data))
					continue
				}
				log.Printf("Received packet: flags=0x%02x (SYN=%v, ACK=%v), src=%d, dst=%d, seq=%d, ack=%d",
					hdr.Flags, (hdr.Flags&SYN) != 0, (hdr.Flags&ACK) != 0,
					hdr.SrcPort, hdr.DstPort, hdr.SeqNum, hdr.AckNum)

				if hdr.Flags&(SYN|ACK) == (SYN | ACK) {
					log.Printf("Received SYN-ACK! seq=%d, ack=%d", hdr.SeqNum, hdr.AckNum)
					c.seqNum++
					c.ackNum = hdr.SeqNum + 1

					// Send ACK
					log.Printf("Sending ACK (seq=%d, ack=%d)", c.seqNum, c.ackNum)
					err = c.rawSocket.SendPacket(c.localIP, c.localPort, c.remoteIP, c.remotePort,
						c.seqNum, c.ackNum, ACK, c.buildTCPOptions(), nil)
					if err != nil {
						return fmt.Errorf("failed to send ACK: %v", err)
					}

					c.mu.Lock()
					c.isConnected = true
					c.mu.Unlock()

					log.Printf("Handshake completed successfully!")

					// 清空recvQueue中的握手包
					for {
						select {
						case <-c.recvQueue:
						default:
							return
					}
					}
				}

			case <-time.After(200 * time.Millisecond):
				continue
			}

			log.Printf("Handshake attempt %d/%d timed out (received %d packets total)", retry+1, packetCount)
		}

		return fmt.Errorf("handshake timeout after %d retries", maxRetries)
}

func (c *ConnRaw) startKeepalive() {
	if !c.hasKeepalive {
		return
	}

	c.hasKeepalive = true
	c.keepaliveStop = make(chan struct{})

	ticker := time.NewTicker(keepaliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.keepaliveStop:
			log.Printf("macOS keepalive stopped")
			return
		case <-ticker.C:
			if c.isConnected {
				if err := c.sendKeepalive(); err != nil {
					log.Printf("keepalive send failed: %v", err)
				}
			}
		}
	}
}

func (c *ConnRaw) sendKeepalive() error {
	// 构造20字节的keepalive包（padding填充）
	padding := make([]byte, keepalivePacketSize)

	err := c.rawSocket.SendPacket(c.localIP, c.srcPort, c.remoteIP, c.dstPort,
		c.seqNum, c.ackNum, ACK|c.buildTCPOptions(), padding)
	if err != nil {
		return fmt.Errorf("failed to send keepalive: %v", err)
	}

	c.mu.Lock()
	c.lastActivity = time.Now()
	c.mu.Unlock()

	log.Printf("Sent keepalive to %s:%d", c.remoteIP, c.remotePort)
	return nil
}

func (c *ConnRaw) stopKeepaliveTimer() {
	if c.keepaliveTimer != nil {
		c.keepaliveTimer.Stop()
		c.keepaliveTimer = nil
	}
}

func (c *ConnRaw) stopKeepalive() {
	if c.hasKeepalive && c.keepaliveStop != nil {
		close(c.keepaliveStop)
		c.hasKeepalive = false
		c.stopKeepaliveTimer()
		log.Printf("Stopped keepalive")
	}
}

func (c *ConnRaw) startKeepaliveTimer() {
	if c.hasKeepalive {
		return
	}

	c.keepaliveTimer = time.NewTimer(staleConnectionTimeout)
}

func (c *ConnRaw) Close() error {
	// 方案4: 停止keepalive
	if c.hasKeepalive && c.keepaliveStop != nil {
		c.stopKeepalive()
	}

	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return nil
	}

	// Stop receive loop
	close(c.stopCh)
	c.wg.Wait()

	// Only client connections close socket and remove iptables rules
	if c.ownsResources {
		if err := c.rawSocket.Close(); err != nil {
			log.Printf("Error closing raw socket: %v", err)
		}
		if err := c.iptablesMgr.RemoveAllRules(); err != nil {
			log.Printf("Error removing iptables rules: %v", err)
		}
	}

	// Close receive queue
	c.closeOnce.Do(func() {
		close(c.recvQueue)
	})

	return nil
}
