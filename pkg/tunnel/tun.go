package tunnel

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const (
	// TUN device constants
	TUNSETIFF   = 0x400454ca
	IFF_TUN     = 0x0001
	IFF_NO_PI   = 0x1000
	IFF_MULTI_QUEUE = 0x0100
)

// TunDevice represents a TUN network device
type TunDevice struct {
	file *os.File
	fd   int
	name string
}

// ifreq structure for ioctl calls
type ifreq struct {
	Name  [16]byte
	Flags uint16
	pad   [22]byte
}

// CreateTUN creates a new TUN device
func CreateTUN(name string) (*TunDevice, error) {
	// Open TUN device using syscall to avoid Go's runtime poller
	// This prevents "not pollable" errors on some systems/kernels
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
		file: file,
		fd:   fd,
		name: actualName,
	}, nil
}

// Read reads a packet from the TUN device
func (t *TunDevice) Read(buf []byte) (int, error) {
	for {
		n, err := syscall.Read(t.fd, buf)
		if err == syscall.EINTR {
			continue
		}
		return n, err
	}
}

// Write writes a packet to the TUN device
func (t *TunDevice) Write(buf []byte) (int, error) {
	for {
		n, err := syscall.Write(t.fd, buf)
		if err == syscall.EINTR {
			continue
		}
		return n, err
	}
}

// Close closes the TUN device
func (t *TunDevice) Close() error {
	return t.file.Close()
}

// Name returns the device name
func (t *TunDevice) Name() string {
	return t.name
}
