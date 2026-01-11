package rawsocket

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

const (
	// Protocol numbers
	IPPROTO_TCP = 6
	IPPROTO_RAW = 255

	// IP header flags
	IP_DF = 0x4000 // Don't fragment

	// TCP header size
	TCPHeaderSize = 20
	// IP header size
	IPHeaderSize = 20
)

// RawSocket represents a raw socket for sending/receiving raw IP packets
type RawSocket struct {
	fd         int
	sendFd     int // Separate socket for sending on macOS
	localIP    net.IP
	localPort  uint16
	remoteIP   net.IP
	remotePort uint16
	isServer   bool

	// macOS-specific: use libpcap for receiving
	pcapHandle *pcap.Handle
	pcapMu     sync.Mutex
	pcapPacket chan []byte
}

// NewRawSocket creates a new raw socket
func NewRawSocket(localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, isServer bool) (*RawSocket, error) {
	// Create raw socket for receiving (IPPROTO_TCP)
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_TCP)
	if err != nil {
		return nil, fmt.Errorf("failed to create raw socket: %v (需要root权限)", err)
	}

	// On macOS, we use a different approach: don't set IP_HDRINCL for sending
	// This allows the kernel to build the IP header automatically
	sendFd := fd // Default: use same socket
	if runtime.GOOS == "darwin" {
		// Create a separate socket for sending without IP_HDRINCL
		// This allows macOS kernel to build IP header automatically
		sendFd, err = syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_TCP)
		if err != nil {
			sendFd = fd // Fallback to receive socket
		} else {
			// Do NOT set IP_HDRINCL - let kernel build IP header
			// On macOS, DON'T bind the send socket to a specific IP/port
			// Binding causes the kernel to send RST packets when it sees
			// raw TCP packets from a port it thinks should be "listening"
			// Instead, let the kernel choose the source address/port automatically
			//
			// NOTE: The source IP will be determined by the kernel based on routing
			// The source port will be random (as expected for outgoing connections)
		}
	}

	// Set IP_HDRINCL on receive socket
	// On macOS, we should NOT set IP_HDRINCL for receiving - it prevents receiving packets
	// The kernel needs to process the packets first, then we can receive them
	if runtime.GOOS == "darwin" {
		// On macOS, don't set IP_HDRINCL for receiving - this allows kernel to process packets
		// and then pass them to us
	} else {
		// On Linux, we need IP_HDRINCL for receiving
		err = syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_HDRINCL, 1)
		if err != nil {
			syscall.Close(fd)
			if sendFd != fd {
				syscall.Close(sendFd)
			}
			return nil, fmt.Errorf("failed to set IP_HDRINCL: %v", err)
		}
	}

	// Set socket to non-blocking mode for better control
	if err := syscall.SetNonblock(fd, false); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("failed to set non-blocking: %v", err)
	}

	// On macOS, try different binding strategies to receive packets
	if runtime.GOOS == "darwin" {
		// Try binding to INADDR_ANY first to receive all packets
		anyAddr := syscall.SockaddrInet4{
			Port: 0,
		}
		// INADDR_ANY = 0.0.0.0
		anyAddr.Addr[0] = 0
		anyAddr.Addr[1] = 0
		anyAddr.Addr[2] = 0
		anyAddr.Addr[3] = 0

		if err := syscall.Bind(fd, &anyAddr); err != nil {
			// If binding to INADDR_ANY fails, try binding to local IP
			if localIP != nil {
				addr := syscall.SockaddrInet4{
					Port: 0,
				}
				copy(addr.Addr[:], localIP.To4())
				syscall.Bind(fd, &addr) // Ignore error
			}
		}
	} else if localIP != nil {
		// On Linux, bind to local IP
		addr := syscall.SockaddrInet4{
			Port: 0,
		}
		copy(addr.Addr[:], localIP.To4())
		syscall.Bind(fd, &addr) // Ignore error
	}

	rs := &RawSocket{
		fd:         fd,
		sendFd:     sendFd,
		localIP:    localIP,
		localPort:  localPort,
		remoteIP:   remoteIP,
		remotePort: remotePort,
		isServer:   isServer,
		pcapPacket: make(chan []byte, 100),
	}

	// On macOS, try to use libpcap for receiving packets
	// This can bypass the raw socket limitation
	if runtime.GOOS == "darwin" {
		// Try to open pcap handle
		handle, err := pcap.OpenLive("any", 65535, true, pcap.BlockForever)
		if err == nil {
			// Set filter to capture TCP packets
			// For client: capture packets from server (src port = remotePort) to us (dst port = localPort)
			// For server: capture packets to our port (dst port = localPort)
			var filter string
			if !isServer && remotePort != 0 {
				// Client mode: capture packets from server port to our local port
				filter = fmt.Sprintf("tcp and (dst port %d or src port %d)", localPort, remotePort)
			} else {
				// Server mode: capture packets to our port
				filter = fmt.Sprintf("tcp port %d", localPort)
			}

			if err := handle.SetBPFFilter(filter); err == nil {
				rs.pcapHandle = handle
				// Start pcap receiver in background
				go rs.pcapReceiver()
				log.Printf("✅ pcap receiver started with filter: %s", filter)
			} else {
				log.Printf("⚠️  Failed to set pcap filter '%s': %v", filter, err)
				handle.Close()
			}
		} else {
			log.Printf("⚠️  Failed to open pcap handle: %v (raw socket will be used)", err)
		}
	}

	return rs, nil
}

// pcapReceiver receives packets using libpcap (macOS workaround)
func (rs *RawSocket) pcapReceiver() {
	if rs.pcapHandle == nil {
		return
	}

	packetSource := gopacket.NewPacketSource(rs.pcapHandle, rs.pcapHandle.LinkType())
	for packet := range packetSource.Packets() {
		// Extract IP and TCP layers
		ipLayer := packet.Layer(layers.LayerTypeIPv4)
		tcpLayer := packet.Layer(layers.LayerTypeTCP)

		if ipLayer != nil && tcpLayer != nil {
			ip, _ := ipLayer.(*layers.IPv4)
			tcp, _ := tcpLayer.(*layers.TCP)

			// Filter for our connection
			// For server: accept packets to our port
			// For client: accept packets from server to our local port
			// During handshake/reconnection, we may not have remoteIP set yet, so be more lenient
			matches := false
			if rs.isServer {
				matches = uint16(tcp.DstPort) == rs.localPort
			} else {
				// Client mode: check if packet is from server to us
				// Check: packet to our local port AND (from remote IP/port OR from any IP with remote port)
				if uint16(tcp.DstPort) == rs.localPort {
					if rs.remoteIP != nil && rs.remotePort != 0 {
						// Strict match: from server IP and port
						matches = ip.SrcIP.Equal(rs.remoteIP) && uint16(tcp.SrcPort) == rs.remotePort
					} else if rs.remotePort != 0 {
						// During handshake: accept any packet to our local port from server port
						// This allows us to receive SYN-ACK during handshake even if remoteIP not set yet
						matches = uint16(tcp.SrcPort) == rs.remotePort
					} else {
						// If remotePort is 0, accept any packet to our local port (shouldn't happen in normal operation)
						matches = true
					}
				} else if rs.remotePort != 0 && uint16(tcp.SrcPort) == rs.remotePort {
					// Also accept packets from server port (in case destination port changed)
					matches = ip.SrcIP.Equal(rs.remoteIP) || rs.remoteIP == nil
				}
			}

			if matches {
				// Build packet data (IP header + TCP header + payload)
				packetData := make([]byte, 0, len(ip.Contents)+len(tcp.Contents)+len(tcp.Payload))
				packetData = append(packetData, ip.Contents...)
				packetData = append(packetData, tcp.Contents...)
				packetData = append(packetData, tcp.Payload...)

				// Send to channel (non-blocking)
				select {
				case rs.pcapPacket <- packetData:
					// Successfully queued
				default:
					// Channel full, drop packet (shouldn't happen often with buffer size 100)
				}
			} else {
				// Debug: log filtered packets during handshake (first few only)
				// This helps diagnose why SYN-ACK might not be received
				if !rs.isServer && rs.remotePort != 0 && uint16(tcp.DstPort) == rs.localPort {
					// This is a packet to our port but filtered - log for debugging
					// Only log SYN-ACK packets to reduce noise
					if tcp.SYN && tcp.ACK {
						log.Printf("🔍 pcapReceiver: Filtered SYN-ACK from %s:%d to %s:%d (expected from %s:%d)",
							ip.SrcIP, tcp.SrcPort, ip.DstIP, tcp.DstPort, rs.remoteIP, rs.remotePort)
					}
				}
			}
		}
	}
}

// BuildIPHeader constructs an IPv4 header
func BuildIPHeader(srcIP, dstIP net.IP, protocol uint8, payloadLen int) []byte {
	header := make([]byte, IPHeaderSize)

	// Version (4 bits) + IHL (4 bits)
	header[0] = 0x45 // Version 4, IHL 5 (20 bytes)

	// Type of Service
	header[1] = 0

	// Total Length
	totalLen := IPHeaderSize + payloadLen
	binary.BigEndian.PutUint16(header[2:4], uint16(totalLen))

	// Identification (can be random or incremental)
	binary.BigEndian.PutUint16(header[4:6], uint16(12345)) // Simple ID

	// Flags (3 bits) + Fragment Offset (13 bits)
	binary.BigEndian.PutUint16(header[6:8], IP_DF) // Don't fragment

	// TTL
	header[8] = 64

	// Protocol
	header[9] = protocol

	// Checksum (will be calculated later)
	header[10] = 0
	header[11] = 0

	// Source IP
	copy(header[12:16], srcIP.To4())

	// Destination IP
	copy(header[16:20], dstIP.To4())

	// Calculate and set checksum
	checksum := CalculateChecksum(header)
	binary.BigEndian.PutUint16(header[10:12], checksum)

	return header
}

// BuildTCPHeader constructs a TCP header
func BuildTCPHeader(srcPort, dstPort uint16, seq, ack uint32, flags uint8, window uint16, options []byte) []byte {
	// Calculate header length including options
	optLen := len(options)
	// Pad options to 4-byte boundary
	if optLen%4 != 0 {
		padding := 4 - (optLen % 4)
		options = append(options, make([]byte, padding)...)
		optLen = len(options)
	}

	headerLen := TCPHeaderSize + optLen
	header := make([]byte, headerLen)

	// Source port
	binary.BigEndian.PutUint16(header[0:2], srcPort)

	// Destination port
	binary.BigEndian.PutUint16(header[2:4], dstPort)

	// Sequence number
	binary.BigEndian.PutUint32(header[4:8], seq)

	// Acknowledgment number
	binary.BigEndian.PutUint32(header[8:12], ack)

	// Data offset (4 bits) + Reserved (4 bits)
	dataOffset := uint8(headerLen / 4)
	header[12] = dataOffset << 4

	// Flags
	header[13] = flags

	// Window size
	binary.BigEndian.PutUint16(header[14:16], window)

	// Checksum (will be calculated later)
	header[16] = 0
	header[17] = 0

	// Urgent pointer
	header[18] = 0
	header[19] = 0

	// Options
	if optLen > 0 {
		copy(header[TCPHeaderSize:], options)
	}

	return header
}

// CalculateTCPChecksum calculates TCP checksum with pseudo header
func CalculateTCPChecksum(srcIP, dstIP net.IP, tcpHeader, payload []byte) uint16 {
	// Build pseudo header
	pseudoHeader := make([]byte, 12)
	copy(pseudoHeader[0:4], srcIP.To4())
	copy(pseudoHeader[4:8], dstIP.To4())
	pseudoHeader[8] = 0
	pseudoHeader[9] = IPPROTO_TCP
	tcpLen := len(tcpHeader) + len(payload)
	binary.BigEndian.PutUint16(pseudoHeader[10:12], uint16(tcpLen))

	// Combine pseudo header + TCP header + payload
	data := make([]byte, len(pseudoHeader)+len(tcpHeader)+len(payload))
	copy(data[0:], pseudoHeader)
	copy(data[len(pseudoHeader):], tcpHeader)
	copy(data[len(pseudoHeader)+len(tcpHeader):], payload)

	return CalculateChecksum(data)
}

// CalculateChecksum calculates Internet checksum
func CalculateChecksum(data []byte) uint16 {
	var sum uint32

	// Add 16-bit words
	for i := 0; i < len(data)-1; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(data[i : i+2]))
	}

	// Add odd byte if present
	if len(data)%2 == 1 {
		sum += uint32(data[len(data)-1]) << 8
	}

	// Fold 32-bit sum to 16 bits
	for sum>>16 != 0 {
		sum = (sum & 0xFFFF) + (sum >> 16)
	}

	// Return one's complement
	return ^uint16(sum)
}

// SendPacket sends a raw IP packet with TCP header and payload
func (rs *RawSocket) SendPacket(srcIP net.IP, srcPort uint16, dstIP net.IP, dstPort uint16,
	seq, ack uint32, flags uint8, tcpOptions, payload []byte) error {

	// Build TCP header (without checksum first)
	tcpHeader := BuildTCPHeader(srcPort, dstPort, seq, ack, flags, 65535, tcpOptions)

	// Use the local IP from the socket if available, otherwise use the provided srcIP
	ipSrc := srcIP
	if rs.localIP != nil {
		ipSrc = rs.localIP
	}

	// Calculate TCP checksum (needed even if kernel builds IP header)
	checksum := CalculateTCPChecksum(ipSrc, dstIP, tcpHeader, payload)
	binary.BigEndian.PutUint16(tcpHeader[16:18], checksum)

	// On macOS, if sendFd doesn't have IP_HDRINCL, send only TCP header + payload
	// The kernel will automatically build the IP header
	var packet []byte
	if runtime.GOOS == "darwin" && rs.sendFd != rs.fd {
		// macOS: send only TCP header + payload (kernel builds IP header)
		packet = make([]byte, len(tcpHeader)+len(payload))
		copy(packet[0:], tcpHeader)
		copy(packet[len(tcpHeader):], payload)
	} else {
		// Linux or same socket: build full IP packet with IP_HDRINCL
		ipHeader := BuildIPHeader(ipSrc, dstIP, IPPROTO_TCP, len(tcpHeader)+len(payload))
		packet = make([]byte, len(ipHeader)+len(tcpHeader)+len(payload))
		copy(packet[0:], ipHeader)
		copy(packet[len(ipHeader):], tcpHeader)
		copy(packet[len(ipHeader)+len(tcpHeader):], payload)
	}

	// Send packet
	addr := syscall.SockaddrInet4{
		Port: 0, // Port is in TCP header
	}
	copy(addr.Addr[:], dstIP.To4())

	// Use sendFd for sending (separate socket on macOS without IP_HDRINCL)
	err := syscall.Sendto(rs.sendFd, packet, 0, &addr)
	if err != nil {
		return fmt.Errorf("failed to send packet: %v", err)
	}

	return nil
}

// RecvPacket receives a raw IP packet and extracts TCP header and payload
func (rs *RawSocket) RecvPacket(buf []byte) (srcIP net.IP, srcPort uint16, dstIP net.IP, dstPort uint16,
	seq, ack uint32, flags uint8, payload []byte, err error) {

	// On macOS, try to use libpcap first (if available)
	if runtime.GOOS == "darwin" && rs.pcapHandle != nil {
		select {
		case packetData := <-rs.pcapPacket:
			// Parse packet from pcap
			if len(packetData) < IPHeaderSize+TCPHeaderSize {
				return nil, 0, nil, 0, 0, 0, 0, nil, fmt.Errorf("packet too small: %d bytes", len(packetData))
			}

			// Copy to buf if it fits
			if len(packetData) <= len(buf) {
				copy(buf, packetData)
			}

			// Parse IP header
			ipHeader := packetData[:IPHeaderSize]
			ihl := (ipHeader[0] & 0x0F) * 4
			if int(ihl) > len(packetData) {
				return nil, 0, nil, 0, 0, 0, 0, nil, fmt.Errorf("invalid IP header length")
			}

			protocol := ipHeader[9]
			if protocol != IPPROTO_TCP {
				return nil, 0, nil, 0, 0, 0, 0, nil, fmt.Errorf("not a TCP packet")
			}

			srcIP = net.IPv4(ipHeader[12], ipHeader[13], ipHeader[14], ipHeader[15])
			dstIP = net.IPv4(ipHeader[16], ipHeader[17], ipHeader[18], ipHeader[19])

			// Parse TCP header
			tcpStart := int(ihl)
			if len(packetData) < tcpStart+TCPHeaderSize {
				return nil, 0, nil, 0, 0, 0, 0, nil, fmt.Errorf("packet too small for TCP header")
			}

			tcpHeader := packetData[tcpStart : tcpStart+TCPHeaderSize]
			srcPort = binary.BigEndian.Uint16(tcpHeader[0:2])
			dstPort = binary.BigEndian.Uint16(tcpHeader[2:4])
			seq = binary.BigEndian.Uint32(tcpHeader[4:8])
			ack = binary.BigEndian.Uint32(tcpHeader[8:12])
			dataOffset := (tcpHeader[12] >> 4) * 4
			flags = tcpHeader[13]

			// Extract payload
			payloadStart := tcpStart + int(dataOffset)
			if payloadStart < len(packetData) {
				payload = make([]byte, len(packetData)-payloadStart)
				copy(payload, packetData[payloadStart:])
			}

			return srcIP, srcPort, dstIP, dstPort, seq, ack, flags, payload, nil
		case <-time.After(100 * time.Millisecond):
			// Timeout - fall through to raw socket
		}
	}

	// Fall back to raw socket (or use it on Linux)
	n, _, err := syscall.Recvfrom(rs.fd, buf, 0)
	if err != nil {
		// On macOS, EAGAIN/EWOULDBLOCK is common - kernel processed the packet
		if runtime.GOOS == "darwin" {
			if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
				// This is expected on macOS - kernel processed the packet
				// Return a timeout-like error that can be handled by the caller
				return nil, 0, nil, 0, 0, 0, 0, nil, fmt.Errorf("packet not available (macOS kernel processed it)")
			}
			// Other errors on macOS
			return nil, 0, nil, 0, 0, 0, 0, nil, fmt.Errorf("failed to receive packet on macOS: %v", err)
		}
		return nil, 0, nil, 0, 0, 0, 0, nil, fmt.Errorf("failed to receive packet: %v", err)
	}

	if n < IPHeaderSize+TCPHeaderSize {
		return nil, 0, nil, 0, 0, 0, 0, nil, fmt.Errorf("packet too small: %d bytes", n)
	}

	// Parse IP header
	ipHeader := buf[:IPHeaderSize]
	ihl := (ipHeader[0] & 0x0F) * 4
	if int(ihl) > n {
		return nil, 0, nil, 0, 0, 0, 0, nil, fmt.Errorf("invalid IP header length")
	}

	protocol := ipHeader[9]
	if protocol != IPPROTO_TCP {
		return nil, 0, nil, 0, 0, 0, 0, nil, fmt.Errorf("not a TCP packet")
	}

	srcIP = net.IPv4(ipHeader[12], ipHeader[13], ipHeader[14], ipHeader[15])
	dstIP = net.IPv4(ipHeader[16], ipHeader[17], ipHeader[18], ipHeader[19])

	// Parse TCP header
	tcpStart := int(ihl)
	if n < tcpStart+TCPHeaderSize {
		return nil, 0, nil, 0, 0, 0, 0, nil, fmt.Errorf("packet too small for TCP header")
	}

	tcpHeader := buf[tcpStart : tcpStart+TCPHeaderSize]
	srcPort = binary.BigEndian.Uint16(tcpHeader[0:2])
	dstPort = binary.BigEndian.Uint16(tcpHeader[2:4])
	seq = binary.BigEndian.Uint32(tcpHeader[4:8])
	ack = binary.BigEndian.Uint32(tcpHeader[8:12])
	dataOffset := (tcpHeader[12] >> 4) * 4
	flags = tcpHeader[13]

	// Extract payload
	payloadStart := tcpStart + int(dataOffset)
	if payloadStart < n {
		payload = make([]byte, n-payloadStart)
		copy(payload, buf[payloadStart:n])
	}

	return srcIP, srcPort, dstIP, dstPort, seq, ack, flags, payload, nil
}

// SetReadTimeout sets read timeout for the socket
func (rs *RawSocket) SetReadTimeout(sec, usec int64) error {
	tv := syscall.Timeval{
		Sec:  sec,
		Usec: int32(usec),
	}
	return syscall.SetsockoptTimeval(rs.fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &tv)
}

// SetWriteTimeout sets write timeout for the socket
func (rs *RawSocket) SetWriteTimeout(sec, usec int64) error {
	tv := syscall.Timeval{
		Sec:  sec,
		Usec: int32(usec),
	}
	return syscall.SetsockoptTimeval(rs.fd, syscall.SOL_SOCKET, syscall.SO_SNDTIMEO, &tv)
}

// Close closes the raw socket
func (rs *RawSocket) Close() error {
	var err error

	// Close pcap handle if used
	if rs.pcapHandle != nil {
		rs.pcapHandle.Close()
		rs.pcapHandle = nil
	}

	if rs.sendFd != rs.fd {
		err = syscall.Close(rs.sendFd)
	}
	if closeErr := syscall.Close(rs.fd); closeErr != nil && err == nil {
		err = closeErr
	}
	return err
}

// GetLocalAddr returns local address
func (rs *RawSocket) GetLocalAddr() string {
	if rs.localIP == nil {
		return fmt.Sprintf("0.0.0.0:%d", rs.localPort)
	}
	return fmt.Sprintf("%s:%d", rs.localIP.String(), rs.localPort)
}

// GetRemoteAddr returns remote address
func (rs *RawSocket) GetRemoteAddr() string {
	if rs.remoteIP == nil {
		return ""
	}
	return fmt.Sprintf("%s:%d", rs.remoteIP.String(), rs.remotePort)
}

// GetFD returns the file descriptor
func (rs *RawSocket) GetFD() int {
	return rs.fd
}

// SetSocketOption sets a socket option
func (rs *RawSocket) SetSocketOption(level, name int, value interface{}) error {
	switch v := value.(type) {
	case int:
		return syscall.SetsockoptInt(rs.fd, level, name, v)
	case []byte:
		return syscall.SetsockoptString(rs.fd, level, name, string(v))
	default:
		return fmt.Errorf("unsupported option type")
	}
}

// GetSocketOption gets a socket option
func (rs *RawSocket) GetSocketOption(level, name int) (int, error) {
	return syscall.GetsockoptInt(rs.fd, level, name)
}

// LocalIP returns the local IP address
func (rs *RawSocket) LocalIP() net.IP {
	return rs.localIP
}

// LocalPort returns the local port
func (rs *RawSocket) LocalPort() uint16 {
	return rs.localPort
}

// RemoteIP returns the remote IP address
func (rs *RawSocket) RemoteIP() net.IP {
	return rs.remoteIP
}

// RemotePort returns the remote port
func (rs *RawSocket) RemotePort() uint16 {
	return rs.remotePort
}

// SetRemoteAddr sets the remote address
func (rs *RawSocket) SetRemoteAddr(ip net.IP, port uint16) {
	rs.remoteIP = ip
	rs.remotePort = port
}

var _ = unsafe.Sizeof(0) // For future use
