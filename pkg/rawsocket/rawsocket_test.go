package rawsocket

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
)

func TestBuildIPHeaderUsesProvidedSourceIP(t *testing.T) {
	t.Parallel()

	providedSrc := net.ParseIP("10.2.0.12").To4()
	dst := net.ParseIP("123.118.98.255").To4()

	header := BuildIPHeader(providedSrc, dst, IPPROTO_TCP, TCPHeaderSize)
	if len(header) < IPHeaderSize {
		t.Fatalf("short IP header: %d", len(header))
	}

	gotSrc := net.IP(header[12:16]).To4()
	gotDst := net.IP(header[16:20]).To4()

	if !gotSrc.Equal(providedSrc) {
		t.Fatalf("source IP mismatch: got %v want %v", gotSrc, providedSrc)
	}
	if !gotDst.Equal(dst) {
		t.Fatalf("destination IP mismatch: got %v want %v", gotDst, dst)
	}

	totalLen := binary.BigEndian.Uint16(header[2:4])
	if totalLen != uint16(IPHeaderSize+TCPHeaderSize) {
		t.Fatalf("unexpected total length: got %d want %d", totalLen, IPHeaderSize+TCPHeaderSize)
	}

	if bytes.Equal(header[12:16], net.ParseIP("49.232.146.200").To4()) {
		t.Fatal("header unexpectedly used listener/public source IP")
	}
}

func TestShouldUseDarwinPcapInterface(t *testing.T) {
	t.Parallel()

	v4Addr := []net.Addr{&net.IPNet{IP: net.ParseIP("192.168.1.8"), Mask: net.CIDRMask(24, 32)}}
	v6Only := []net.Addr{&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)}}

	tests := []struct {
		name  string
		iface string
		flags net.Flags
		addrs []net.Addr
		want  bool
	}{
		{name: "accept en0 with ipv4", iface: "en0", flags: net.FlagUp, addrs: v4Addr, want: true},
		{name: "reject utun", iface: "utun5", flags: net.FlagUp, addrs: v4Addr, want: false},
		{name: "reject loopback", iface: "lo0", flags: net.FlagUp | net.FlagLoopback, addrs: v4Addr, want: false},
		{name: "reject bridge", iface: "bridge0", flags: net.FlagUp, addrs: v4Addr, want: false},
		{name: "reject v6 only", iface: "en7", flags: net.FlagUp, addrs: v6Only, want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := shouldUseDarwinPcapInterface(tc.iface, tc.flags, tc.addrs)
			if got != tc.want {
				t.Fatalf("shouldUseDarwinPcapInterface(%q) = %v, want %v", tc.iface, got, tc.want)
			}
		})
	}
}

func TestShouldRequireDarwinPcap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		goos             string
		backendAvailable bool
		want             bool
	}{
		{name: "darwin requires backend", goos: "darwin", backendAvailable: false, want: true},
		{name: "darwin with backend is allowed", goos: "darwin", backendAvailable: true, want: false},
		{name: "linux does not require darwin backend", goos: "linux", backendAvailable: false, want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := shouldRequireDarwinPcap(tc.goos, tc.backendAvailable)
			if got != tc.want {
				t.Fatalf("shouldRequireDarwinPcap(%q, %v) = %v, want %v", tc.goos, tc.backendAvailable, got, tc.want)
			}
		})
	}
}

func TestDarwinRawTCPUnsupportedError(t *testing.T) {
	t.Parallel()

	err := darwinRawTCPUnsupportedError()
	if err == nil {
		t.Fatal("expected error")
	}

	msg := err.Error()
	if !bytes.Contains([]byte(msg), []byte("cgo + libpcap")) {
		t.Fatalf("error message %q does not mention cgo + libpcap", msg)
	}
	if !bytes.Contains([]byte(msg), []byte("CGO_ENABLED=0")) {
		t.Fatalf("error message %q does not mention CGO_ENABLED=0", msg)
	}
}
