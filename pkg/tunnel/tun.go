package tunnel

import (
	"fmt"
	"os"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const (
	// TUN device constants
	TUNSETIFF   = 0x400454ca
	IFF_TUN     = 0x0001
	IFF_NO_PI   = 0x1000
	IFF_MULTI_QUEUE = 0x0100
	
	// TUN I/O polling interval when device would block
	// This balances responsiveness (for signal handling) with CPU efficiency
	tunPollInterval = 10 * time.Millisecond
)

// TunDevice represents a TUN network device
type TunDevice struct {
	file   *os.File
	fd     int
	name   string
	closed int32 // atomic flag to track if device is closed
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
	fd, err := syscall.Open("/dev/net/tun", syscall.O_RDWR|syscall.O_NONBLOCK, 0)
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
			// Non-blocking read would block, check if closed and wait briefly
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
			// Non-blocking write would block, check if closed and wait briefly
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
