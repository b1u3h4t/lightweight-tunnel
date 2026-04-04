package tunnel

import (
	"fmt"
	"log"
	"net"
	"runtime"
	"syscall"
	"time"
)

const (
	// MTU discovery constants
	minMTU          = 576  // IPv4 minimum MTU
	maxMTU          = 1500 // Standard Ethernet MTU
	conservativeMTU = 1200 // Conservative MTU for uncertain cases
)

// MTUDiscovery handles adaptive MTU detection
type MTUDiscovery struct {
	remoteAddr string
	currentMTU int
}

// NewMTUDiscovery creates a new MTU discovery instance
func NewMTUDiscovery(remoteAddr string, initialMTU int) *MTUDiscovery {
	return &MTUDiscovery{
		remoteAddr: remoteAddr,
		currentMTU: initialMTU,
	}
}

// DiscoverOptimalMTU performs MTU path discovery using binary search
// Returns the optimal MTU for the network path
func (m *MTUDiscovery) DiscoverOptimalMTU() (int, error) {
	log.Printf("🔍 开始自适应MTU探测...")
	log.Printf("   目标地址: %s", m.remoteAddr)
	log.Printf("   初始MTU: %d", m.currentMTU)

	// Parse remote address
	host, _, err := net.SplitHostPort(m.remoteAddr)
	if err != nil {
		return m.currentMTU, fmt.Errorf("invalid remote address: %v", err)
	}

	// Resolve IP address
	ips, err := net.LookupIP(host)
	if err != nil {
		return m.currentMTU, fmt.Errorf("failed to resolve host: %v", err)
	}
	if len(ips) == 0 {
		return m.currentMTU, fmt.Errorf("no IP addresses found for host")
	}

	targetIP := ips[0].String()
	log.Printf("   解析地址: %s", targetIP)

	// Binary search for optimal MTU
	low := minMTU
	high := maxMTU
	optimal := minMTU

	attempts := 0
	maxAttempts := 10

	for low <= high && attempts < maxAttempts {
		attempts++
		testMTU := (low + high) / 2

		log.Printf("   [%d/%d] 测试 MTU: %d", attempts, maxAttempts, testMTU)

		if m.testMTU(targetIP, testMTU) {
			// MTU works, try larger
			optimal = testMTU
			low = testMTU + 1
			log.Printf("   ✅ MTU %d 可用", testMTU)
		} else {
			// MTU too large, try smaller
			high = testMTU - 1
			log.Printf("   ❌ MTU %d 过大", testMTU)
		}
	}

	// Account for IP header (20 bytes) and protocol overhead
	// For rawtcp mode with encryption: need to reserve space for packet type (1 byte) + encryption overhead (28 bytes)
	const ipHeaderSize = 20
	const tcpHeaderSize = 20
	const packetTypeOverhead = 1
	const encryptionOverhead = 28

	// Calculate safe MTU for tunnel payload
	safeMTU := optimal - ipHeaderSize - tcpHeaderSize - packetTypeOverhead - encryptionOverhead

	// Ensure we don't go below minimum
	if safeMTU < 500 {
		safeMTU = 500
	}

	// Cap at reasonable maximum for rawtcp mode
	if safeMTU > 1371 {
		safeMTU = 1371 // Safe maximum for rawtcp + encryption
	}

	log.Printf("✅ MTU探测完成")
	log.Printf("   路径MTU: %d", optimal)
	log.Printf("   隧道MTU: %d (已扣除协议开销)", safeMTU)

	return safeMTU, nil
}

// testMTU tests if a specific MTU size can traverse the network path.
// It sends a UDP packet of the specified size with the DF (Don't Fragment) flag set.
// If the packet is too large for any link on the path, the OS returns EMSGSIZE
// (or the packet is silently dropped). We use a short timeout to detect this.
func (m *MTUDiscovery) testMTU(targetIP string, mtu int) bool {
	// We need to subtract IP header (20) and UDP header (8) from the link MTU
	// to get the maximum UDP payload size.
	const ipHeaderLen = 20
	const udpHeaderLen = 8
	payloadSize := mtu - ipHeaderLen - udpHeaderLen
	if payloadSize <= 0 {
		return false
	}

	// Use a high ephemeral port unlikely to conflict
	addr := net.JoinHostPort(targetIP, "33434")
	raddr, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return mtu <= conservativeMTU
	}

	// Create a UDP socket and set IP_MTU_DISCOVER to IP_PMTUDISC_DO (DF bit)
	conn, err := net.DialUDP("udp4", nil, raddr)
	if err != nil {
		return mtu <= conservativeMTU
	}
	defer conn.Close()

	// Set DF flag via socket option so the kernel refuses to fragment.
	// On macOS, IP_DONTFRAG (IP option 67) is used instead of IP_MTU_DISCOVER.
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return mtu <= conservativeMTU
	}

	var setsockoptErr error
	rawConn.Control(func(fd uintptr) {
		if runtime.GOOS == "darwin" {
			// macOS: IP_DONTFRAG = 67
			const IP_DONTFRAG = 67
			setsockoptErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, IP_DONTFRAG, 1)
		} else {
			// Linux: IP_MTU_DISCOVER = 10, IP_PMTUDISC_DO = 2 (force DF)
			setsockoptErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, syscall.IP_MTU_DISCOVER, syscall.IP_PMTUDISC_DO)
		}
	})
	if setsockoptErr != nil {
		// If we can't set DF, fall back to conservative estimate
		return mtu <= conservativeMTU
	}

	// Set a short write deadline — we only care about the send result
	conn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))

	// Send a payload of the target size. The kernel will return EMSGSIZE
	// if the local interface MTU is smaller than the packet (with DF set).
	payload := make([]byte, payloadSize)
	_, err = conn.Write(payload)
	if err != nil {
		// EMSGSIZE means the packet is too large for the path
		return false
	}

	// Packet was accepted by the kernel — the local MTU allows it.
	// For intermediate links, we rely on ICMP "Fragmentation Needed" messages
	// being processed by the kernel's PMTU cache. A second write after a short
	// delay tests whether the kernel learned of a smaller path MTU.
	time.Sleep(100 * time.Millisecond)
	conn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
	_, err = conn.Write(payload)
	if err != nil {
		return false
	}

	return true
}

// GetRecommendedMTU returns a recommended MTU based on common network types
func GetRecommendedMTU(networkType string) int {
	switch networkType {
	case "ethernet":
		return 1371 // Safe for rawtcp + encryption over standard Ethernet
	case "pppoe":
		return 1343 // PPPoE reduces MTU by 8 bytes, then account for overhead
	case "mobile":
		return 1200 // Conservative for mobile networks
	case "vpn":
		return 1300 // Account for VPN overhead
	case "wifi":
		return 1371 // Usually same as Ethernet
	default:
		return 1371 // Safe default
	}
}

// AutoDetectNetworkType attempts to detect the network type
func AutoDetectNetworkType() string {
	// Simple heuristic based on available interfaces
	// In production, this could be more sophisticated

	ifaces, err := net.Interfaces()
	if err != nil {
		return "ethernet"
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		name := iface.Name

		// Check for common interface name patterns
		if len(name) >= 2 {
			prefix := name[:2]
			switch prefix {
			case "wl", "ww": // wlan, wwan
				return "wifi"
			case "pp": // ppp
				return "pppoe"
			case "et", "en": // eth, ens, enp
				return "ethernet"
			}
		}
	}

	return "ethernet" // Default
}
