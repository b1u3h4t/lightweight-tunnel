package faketcp

import (
	"fmt"
	"net"
	"time"

	"github.com/openbmx/lightweight-tunnel/pkg/iptables"
	"github.com/openbmx/lightweight-tunnel/pkg/rawsocket"
)

// Mode represents the fake TCP mode
type Mode int

const (
	// ModeUDP uses UDP socket with fake TCP headers in payload (原实现)
	ModeUDP Mode = iota
	// ModeRaw uses raw sockets with real TCP headers (真正的TCP伪装，类似udp2raw)
	ModeRaw
)

var (
	// CurrentMode is the current fake TCP mode (default: UDP)
	CurrentMode = ModeUDP
	// EnableRawSocket enables raw socket mode globally
	EnableRawSocket = false
)

// SetMode sets the fake TCP mode
func SetMode(mode Mode) {
	CurrentMode = mode
	EnableRawSocket = (mode == ModeRaw)
}

// GetMode returns the current mode
func GetMode() Mode {
	return CurrentMode
}

// DialAuto automatically selects the appropriate Dial function based on mode
func DialAuto(remoteAddr string, timeout time.Duration) (interface{}, error) {
	if EnableRawSocket {
		return DialRaw(remoteAddr, timeout)
	}
	return Dial(remoteAddr, timeout)
}

// ListenAuto automatically selects the appropriate Listen function based on mode
func ListenAuto(addr string) (interface{}, error) {
	if EnableRawSocket {
		return ListenRaw(addr)
	}
	return Listen(addr)
}

// ConnAdapter is a unified interface for both UDP and Raw socket connections
type ConnAdapter interface {
	WritePacket(data []byte) error
	ReadPacket() ([]byte, error)
	Close() error
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	SetDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
}

// ListenerAdapter is a unified interface for both UDP and Raw socket listeners
type ListenerAdapter interface {
	Accept() (ConnAdapter, error)
	Close() error
	Addr() net.Addr
}

// Ensure both types implement the interfaces
var _ ConnAdapter = (*Conn)(nil)
var _ ConnAdapter = (*ConnRaw)(nil)

// UDPListener wraps Listener to implement ListenerAdapter
type UDPListener struct {
	*Listener
}

// Accept wraps the UDP listener Accept
func (l *UDPListener) Accept() (ConnAdapter, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// RawListener wraps ListenerRaw to implement ListenerAdapter
type RawListener struct {
	*ListenerRaw
}

// Accept wraps the Raw listener Accept
func (l *RawListener) Accept() (ConnAdapter, error) {
	conn, err := l.ListenerRaw.Accept()
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// DialWithMode creates a connection using specified mode
func DialWithMode(remoteAddr string, timeout time.Duration, mode Mode) (ConnAdapter, error) {
	if mode == ModeRaw {
		return DialRaw(remoteAddr, timeout)
	}
	return Dial(remoteAddr, timeout)
}

// ListenWithMode creates a listener using specified mode
func ListenWithMode(addr string, mode Mode) (ListenerAdapter, error) {
	return ListenWithModeAndReplySource(addr, "", mode)
}

func ListenWithModeAndReplySource(addr string, replySourceIP string, mode Mode) (ListenerAdapter, error) {
	if mode == ModeRaw {
		listener, err := ListenRawWithReplySource(addr, replySourceIP)
		if err != nil {
			return nil, err
		}
		return &RawListener{listener}, nil
	}
	listener, err := Listen(addr)
	if err != nil {
		return nil, err
	}
	return &UDPListener{listener}, nil
}

// ModeString returns a string representation of the mode
func ModeString(mode Mode) string {
	switch mode {
	case ModeUDP:
		return "UDP (fake TCP headers in payload)"
	case ModeRaw:
		return "Raw Socket (real TCP packets with kernel RST suppression)"
	default:
		return fmt.Sprintf("Unknown mode (%d)", mode)
	}
}

// CheckRawSocketSupport checks if raw socket mode is supported
func CheckRawSocketSupport() error {
	// Try to create a test raw socket
	testSock, err := rawsocket.NewRawSocket(net.IPv4(127, 0, 0, 1), 12345, net.IPv4(127, 0, 0, 1), 54321, true)
	if err != nil {
		return fmt.Errorf("raw socket not supported: %v (需要root权限)", err)
	}
	testSock.Close()

	// Check iptables availability
	if err := iptables.CheckIPTablesAvailable(); err != nil {
		return fmt.Errorf("firewall RST suppression backend not available: %v", err)
	}

	return nil
}
