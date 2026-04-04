//go:build darwin && cgo

package rawsocket

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"sync/atomic"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

func darwinPcapBackendAvailable() bool { return true }

func initializeDarwinPcap(rs *RawSocket) {
	device := selectDarwinPcapDevice()
	if device == "" {
		log.Printf("⚠️  Failed to find a usable macOS pcap device (raw socket will be used)")
		return
	}

	handle, err := pcap.OpenLive(device, 65535, true, pcap.BlockForever)
	if err != nil {
		log.Printf("⚠️  Failed to open pcap handle: %v (raw socket will be used)", err)
		return
	}

	var filter string
	if !rs.isServer && rs.remotePort != 0 {
		filter = fmt.Sprintf("tcp and (dst port %d or src port %d)", rs.localPort, rs.remotePort)
	} else {
		filter = fmt.Sprintf("tcp port %d", rs.localPort)
	}

	if err := handle.SetBPFFilter(filter); err != nil {
		log.Printf("⚠️  Failed to set pcap filter '%s': %v", filter, err)
		handle.Close()
		return
	}

	rs.pcapHandle = handle
	go rs.pcapReceiverDarwin(handle)
	log.Printf("✅ pcap receiver started on %s with filter: %s", device, filter)
}

func (rs *RawSocket) pcapReceiverDarwin(handle *pcap.Handle) {
	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	for packet := range packetSource.Packets() {
		ipLayer := packet.Layer(layers.LayerTypeIPv4)
		tcpLayer := packet.Layer(layers.LayerTypeTCP)
		if ipLayer == nil || tcpLayer == nil {
			continue
		}

		ip, _ := ipLayer.(*layers.IPv4)
		tcp, _ := tcpLayer.(*layers.TCP)

		matches := false
		if rs.isServer {
			matches = uint16(tcp.DstPort) == rs.localPort
		} else {
			if uint16(tcp.DstPort) == rs.localPort {
				if rs.remoteIP != nil && rs.remotePort != 0 {
					matches = ip.SrcIP.Equal(rs.remoteIP) && uint16(tcp.SrcPort) == rs.remotePort
				} else if rs.remotePort != 0 {
					matches = uint16(tcp.SrcPort) == rs.remotePort
				} else {
					matches = true
				}
			} else if rs.remotePort != 0 && uint16(tcp.SrcPort) == rs.remotePort {
				matches = ip.SrcIP.Equal(rs.remoteIP) || rs.remoteIP == nil
			}
		}

		if !matches {
			if !rs.isServer && rs.remotePort != 0 && uint16(tcp.DstPort) == rs.localPort && tcp.SYN && tcp.ACK {
				log.Printf("🔍 pcapReceiver: Filtered SYN-ACK from %s:%d to %s:%d (expected from %s:%d)", ip.SrcIP, tcp.SrcPort, ip.DstIP, tcp.DstPort, rs.remoteIP, rs.remotePort)
			}
			continue
		}

		packetData := make([]byte, 0, len(ip.Contents)+len(tcp.Contents)+len(tcp.Payload))
		packetData = append(packetData, ip.Contents...)
		packetData = append(packetData, tcp.Contents...)
		packetData = append(packetData, tcp.Payload...)

		select {
		case rs.pcapPacket <- packetData:
		default:
			atomic.AddUint64(&rs.pcapDropCount, 1)
		}
	}
}

func recvPacketDarwinPcap(rs *RawSocket, buf []byte) (srcIP net.IP, srcPort uint16, dstIP net.IP, dstPort uint16, seq, ack uint32, flags uint8, payload []byte, handled bool, err error) {
	handle, ok := rs.pcapHandle.(*pcap.Handle)
	if !ok || handle == nil {
		return nil, 0, nil, 0, 0, 0, 0, nil, false, nil
	}

	select {
	case packetData := <-rs.pcapPacket:
		if len(packetData) < IPHeaderSize+TCPHeaderSize {
			return nil, 0, nil, 0, 0, 0, 0, nil, true, fmt.Errorf("packet too small: %d bytes", len(packetData))
		}
		if len(packetData) <= len(buf) {
			copy(buf, packetData)
		}

		ipHeader := packetData[:IPHeaderSize]
		ihl := (ipHeader[0] & 0x0F) * 4
		if int(ihl) > len(packetData) {
			return nil, 0, nil, 0, 0, 0, 0, nil, true, fmt.Errorf("invalid IP header length")
		}
		if ipHeader[9] != IPPROTO_TCP {
			return nil, 0, nil, 0, 0, 0, 0, nil, true, fmt.Errorf("not a TCP packet")
		}

		srcIP = net.IPv4(ipHeader[12], ipHeader[13], ipHeader[14], ipHeader[15])
		dstIP = net.IPv4(ipHeader[16], ipHeader[17], ipHeader[18], ipHeader[19])
		tcpStart := int(ihl)
		if len(packetData) < tcpStart+TCPHeaderSize {
			return nil, 0, nil, 0, 0, 0, 0, nil, true, fmt.Errorf("packet too small for TCP header")
		}

		tcpHeader := packetData[tcpStart : tcpStart+TCPHeaderSize]
		srcPort = binary.BigEndian.Uint16(tcpHeader[0:2])
		dstPort = binary.BigEndian.Uint16(tcpHeader[2:4])
		seq = binary.BigEndian.Uint32(tcpHeader[4:8])
		ack = binary.BigEndian.Uint32(tcpHeader[8:12])
		dataOffset := (tcpHeader[12] >> 4) * 4
		flags = tcpHeader[13]
		payloadStart := tcpStart + int(dataOffset)
		if payloadStart < len(packetData) {
			payload = make([]byte, len(packetData)-payloadStart)
			copy(payload, packetData[payloadStart:])
		}
		return srcIP, srcPort, dstIP, dstPort, seq, ack, flags, payload, true, nil
	case <-time.After(100 * time.Millisecond):
		return nil, 0, nil, 0, 0, 0, 0, nil, false, nil
	}
}

func closeDarwinPcap(rs *RawSocket) {
	handle, ok := rs.pcapHandle.(*pcap.Handle)
	if ok && handle != nil {
		handle.Close()
	}
	rs.pcapHandle = nil
}
