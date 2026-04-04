//go:build !darwin || !cgo

package rawsocket

import "net"

func darwinPcapBackendAvailable() bool { return false }

func initializeDarwinPcap(rs *RawSocket) {}

func recvPacketDarwinPcap(rs *RawSocket, buf []byte) (srcIP net.IP, srcPort uint16, dstIP net.IP, dstPort uint16, seq, ack uint32, flags uint8, payload []byte, handled bool, err error) {
	return nil, 0, nil, 0, 0, 0, 0, nil, false, nil
}

func closeDarwinPcap(rs *RawSocket) {}
