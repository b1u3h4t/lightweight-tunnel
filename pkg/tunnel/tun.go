package tunnel

import (
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const (
	// TUN device constants (Linux)
	TUNSETIFF       = 0x400454ca
	IFF_TUN         = 0x0001
	IFF_NO_PI       = 0x1000
	IFF_MULTI_QUEUE = 0x0100

	// macOS utun constants
	AF_SYSTEM         = 32
	AF_SYS_CONTROL    = 2
	SYSPROTO_CONTROL  = 2
	CTLIOCGINFO       = 0xc0644e03
	UTUN_OPT_IFNAME   = 2
	UTUN_CONTROL_NAME = "com.apple.net.utun_control"

	// TUN I/O polling interval when device would block.
	// The device is opened in blocking mode, so this is only a defensive fallback
	// for kernels/drivers that may still surface EAGAIN/EWOULDBLOCK occasionally.
	tunPollInterval = 10 * time.Millisecond
)

// TunDevice represents a TUN network device
type TunDevice struct {
	file   *os.File
	fd     int
	name   string
	closed int32 // atomic flag to track if device is closed
}

// ifreq structure for ioctl calls (Linux)
type ifreq struct {
	Name  [16]byte
	Flags uint16
	pad   [22]byte
}

// ctl_info structure for macOS utun
type ctlInfo struct {
	CtlID   uint32
	CtlName [96]byte
}

// sockaddr_ctl structure for macOS utun
type sockaddrCtl struct {
	ScLen     uint8
	ScFamily  uint8
	SsSysaddr uint16
	ScID      uint32
	ScUnit    uint32
	Reserved  [5]uint32
}

// CreateTUN creates a new TUN device
func CreateTUN(name string) (*TunDevice, error) {
	if runtime.GOOS == "darwin" {
		return createTUNmacOS(name)
	}
	return createTUNLinux(name)
}

// createTUNLinux creates a TUN device on Linux
func createTUNLinux(name string) (*TunDevice, error) {
	// Open TUN device in blocking mode using syscall to avoid Go's runtime poller.
	// Blocking I/O ensures packets are delivered immediately without the sleep-based
	// polling overhead that would add per-packet latency.
	fd, err := syscall.Open("/dev/net/tun", syscall.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open /dev/net/tun: %v", err)
	}

	// Prepare ifreq structure
	var ifr ifreq
	copy(ifr.Name[:], []byte(name))
	ifr.Flags = IFF_TUN | IFF_NO_PI

	// Create TUN device using ioctl
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(TUNSETIFF), uintptr(unsafe.Pointer(&ifr)))
	if errno != 0 {
		syscall.Close(fd)
		return nil, fmt.Errorf("failed to create TUN device: %v", errno)
	}

	// Create os.File from file descriptor for compatibility and automatic closing
	file := os.NewFile(uintptr(fd), "/dev/net/tun")

	// Get actual device name (may differ if name was in use)
	actualName := string(ifr.Name[:])
	// Trim null bytes
	for i, b := range actualName {
		if b == 0 {
			actualName = actualName[:i]
			break
		}
	}

	return &TunDevice{
		file:   file,
		fd:     fd,
		name:   actualName,
		closed: 0,
	}, nil
}

// createTUNmacOS creates a utun device on macOS
func createTUNmacOS(name string) (*TunDevice, error) {
	// Create a socket for utun control
	fd, err := syscall.Socket(AF_SYSTEM, syscall.SOCK_DGRAM, SYSPROTO_CONTROL)
	if err != nil {
		return nil, fmt.Errorf("failed to create utun socket: %v", err)
	}

	// Get control ID for utun
	// The ctlInfo structure: first 4 bytes are ctl_id (uint32), rest is name
	var ctlInfoBuf [96]byte
	copy(ctlInfoBuf[4:], []byte(UTUN_CONTROL_NAME))
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(CTLIOCGINFO), uintptr(unsafe.Pointer(&ctlInfoBuf[0])))
	if errno != 0 {
		syscall.Close(fd)
		return nil, fmt.Errorf("failed to get utun control info: %v", errno)
	}

	// Extract control ID from the buffer (first 4 bytes, little-endian)
	ctlID := *(*uint32)(unsafe.Pointer(&ctlInfoBuf[0]))

	// Prepare socket address for utun
	var addr sockaddrCtl
	addr.ScLen = uint8(unsafe.Sizeof(addr))
	addr.ScFamily = AF_SYSTEM
	addr.SsSysaddr = AF_SYS_CONTROL
	addr.ScID = ctlID
	// Extract unit number from name if provided (e.g., "utun5" -> 5)
	// If name is empty or doesn't contain a number, use 0 and let system assign
	addr.ScUnit = 0
	if name != "" {
		// Try to extract number from name like "utun5" or "5"
		var unit uint32
		if n, _ := fmt.Sscanf(name, "utun%d", &unit); n == 1 {
			addr.ScUnit = unit + 1 // macOS uses 1-based indexing internally
		} else if n, _ := fmt.Sscanf(name, "%d", &unit); n == 1 {
			addr.ScUnit = unit + 1
		}
	}

	// Connect to utun control
	_, _, errno = syscall.RawSyscall(syscall.SYS_CONNECT, uintptr(fd),
		uintptr(unsafe.Pointer(&addr)), uintptr(unsafe.Sizeof(addr)))
	if errno != 0 {
		syscall.Close(fd)
		return nil, fmt.Errorf("failed to connect to utun: %v", errno)
	}

	// Get the actual interface name using getsockopt
	var ifName [16]byte
	ifNameLen := uint32(len(ifName))
	_, _, errno = syscall.Syscall6(syscall.SYS_GETSOCKOPT, uintptr(fd),
		uintptr(SYSPROTO_CONTROL), uintptr(UTUN_OPT_IFNAME),
		uintptr(unsafe.Pointer(&ifName[0])), uintptr(unsafe.Pointer(&ifNameLen)), 0)
	
	actualName := ""
	if errno == 0 && ifNameLen > 0 {
		// Trim null bytes and get the actual name
		for i, b := range ifName {
			if b == 0 {
				actualName = string(ifName[:i])
				break
			}
		}
		if actualName == "" && ifNameLen > 0 {
			if ifNameLen > uint32(len(ifName)) {
				ifNameLen = uint32(len(ifName))
			}
			actualName = string(ifName[:ifNameLen-1])
		}
	}
	
	// Fallback: if we couldn't get the name, use the unit number
	// But only if ScUnit > 0 (to avoid overflow)
	if actualName == "" {
		if addr.ScUnit > 0 {
			actualName = fmt.Sprintf("utun%d", addr.ScUnit-1)
		} else {
			// System assigned, try to find it via ifconfig (last resort)
			actualName = "utun0" // Default fallback
		}
	}

	// Create os.File from file descriptor for compatibility and automatic closing
	file := os.NewFile(uintptr(fd), fmt.Sprintf("/dev/%s", actualName))

	return &TunDevice{
		file:   file,
		fd:     fd,
		name:   actualName,
		closed: 0,
	}, nil
}

// Read reads a packet from the TUN device
func (t *TunDevice) Read(buf []byte) (int, error) {
	// Check if device is already closed
	if atomic.LoadInt32(&t.closed) != 0 {
		return 0, syscall.EBADF
	}

	for {
		n, err := syscall.Read(t.fd, buf)
		if err == nil {
			return n, nil
		}

		if err == syscall.EINTR {
			// Check again if device was closed while we were interrupted
			if atomic.LoadInt32(&t.closed) != 0 {
				return 0, syscall.EBADF
			}
			continue
		}

		if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
			// Defensive: some drivers may still return EAGAIN in rare cases even
			// though we open the fd in blocking mode. Briefly back off to avoid
			// busy-waiting while remaining responsive to shutdown.
			if atomic.LoadInt32(&t.closed) != 0 {
				return 0, syscall.EBADF
			}
			// Use a short sleep to avoid busy-waiting while still being responsive
			// to shutdown signals. The tunPollInterval provides a good balance between
			// CPU efficiency and shutdown responsiveness.
			time.Sleep(tunPollInterval)
			// Check again after sleep
			if atomic.LoadInt32(&t.closed) != 0 {
				return 0, syscall.EBADF
			}
			continue
		}

		return n, err
	}
}

// Write writes a packet to the TUN device
func (t *TunDevice) Write(buf []byte) (int, error) {
	// Check if device is already closed
	if atomic.LoadInt32(&t.closed) != 0 {
		return 0, syscall.EBADF
	}

	for {
		n, err := syscall.Write(t.fd, buf)
		if err == nil {
			return n, nil
		}

		if err == syscall.EINTR {
			// Check again if device was closed while we were interrupted
			if atomic.LoadInt32(&t.closed) != 0 {
				return 0, syscall.EBADF
			}
			continue
		}

		if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
			// Defensive: some drivers may still return EAGAIN in rare cases even
			// though we open the fd in blocking mode. Briefly back off to avoid
			// busy-waiting while remaining responsive to shutdown.
			if atomic.LoadInt32(&t.closed) != 0 {
				return 0, syscall.EBADF
			}
			// Use a short sleep to avoid busy-waiting while still being responsive
			// to shutdown signals. The tunPollInterval provides a good balance between
			// CPU efficiency and shutdown responsiveness.
			time.Sleep(tunPollInterval)
			// Check again after sleep
			if atomic.LoadInt32(&t.closed) != 0 {
				return 0, syscall.EBADF
			}
			continue
		}

		return n, err
	}
}

// Close closes the TUN device
func (t *TunDevice) Close() error {
	// Mark as closed atomically
	if !atomic.CompareAndSwapInt32(&t.closed, 0, 1) {
		// Already closed
		return nil
	}

	// Close the file descriptor - this will unblock any pending Read/Write
	return t.file.Close()
}

// Name returns the device name
func (t *TunDevice) Name() string {
	return t.name
}
