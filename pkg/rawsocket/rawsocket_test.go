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
