//go:build darwin && cgo

package rawsocket

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

func darwinPcapBackendAvailable() bool { return true }

func darwinTCPFlagsString(tcp *layers.TCP) string {
	flags := make([]string, 0, 8)
	if tcp.FIN {
		flags = append(flags, "FIN")
	}
	if tcp.SYN {
		flags = append(flags, "SYN")
	}
	if tcp.RST {
		flags = append(flags, "RST")
	}
	if tcp.PSH {
		flags = append(flags, "PSH")
	}
	if tcp.ACK {
		flags = append(flags, "ACK")
	}
	if tcp.URG {
		flags = append(flags, "URG")
	}
	if tcp.ECE {
		flags = append(flags, "ECE")
	}
	if tcp.CWR {
		flags = append(flags, "CWR")
	}
	if len(flags) == 0 {
		return "NONE"
	}
	return strings.Join(flags, "|")
}

func initializeDarwinPcap(rs *RawSocket) {
	device := selectDarwinPcapDevice(rs.localIP, rs.remoteIP, rs.remotePort)
	if device == "" {
		log.Printf("⚠️  Failed to find a usable macOS pcap device (raw socket will be used)")
		return
	}

	inactive, err := pcap.NewInactiveHandle(device)
	if err != nil {
		log.Printf("⚠️  Failed to create pcap handle for %s: %v (raw socket will be used)", device, err)
		return
	}
	defer inactive.CleanUp()

	if err := inactive.SetSnapLen(65535); err != nil {
		log.Printf("⚠️  Failed to set pcap snaplen on %s: %v (raw socket will be used)", device, err)
		return
	}
	if err := inactive.SetPromisc(true); err != nil {
		log.Printf("⚠️  Failed to set pcap promisc on %s: %v (raw socket will be used)", device, err)
		return
	}
	if err := inactive.SetImmediateMode(true); err != nil {
		log.Printf("⚠️  Failed to enable pcap immediate mode on %s: %v (continuing)", device, err)
	}
	if err := inactive.SetTimeout(200 * time.Millisecond); err != nil {
		log.Printf("⚠️  Failed to set pcap timeout on %s: %v (raw socket will be used)", device, err)
		return
	}

	handle, err := inactive.Activate()
	if err != nil {
		log.Printf("⚠️  Failed to activate pcap handle on %s: %v (raw socket will be used)", device, err)
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
	log.Printf("✅ pcap receiver started on %s with filter: %s (local=%s remote=%s:%d)", device, filter, rs.localIP, rs.remoteIP, rs.remotePort)
}

func (rs *RawSocket) pcapReceiverDarwin(handle *pcap.Handle) {
	var packetCount uint64
	var decodeFailures uint64
	for {
		packetData, _, err := handle.ReadPacketData()
		if err != nil {
			if err == pcap.NextErrorTimeoutExpired {
				continue
			}
			if err == pcap.NextErrorNotActivated || err == pcap.NextErrorNoMorePackets {
				return
			}
			log.Printf("⚠️  Darwin pcap read error on %s:%d -> %s:%d: %v", rs.localIP, rs.localPort, rs.remoteIP, rs.remotePort, err)
			continue
		}

		packet := gopacket.NewPacket(packetData, handle.LinkType(), gopacket.NoCopy)
		ipLayer := packet.Layer(layers.LayerTypeIPv4)
		tcpLayer := packet.Layer(layers.LayerTypeTCP)
		if ipLayer == nil || tcpLayer == nil {
			decodeFailures++
			if decodeFailures <= 5 || decodeFailures%100 == 0 {
				log.Printf("📉 Darwin pcap decode miss count=%d local=%s:%d remote=%s:%d linkType=%s packetLen=%d",
					decodeFailures, rs.localIP, rs.localPort, rs.remoteIP, rs.remotePort, handle.LinkType(), len(packetData))
			}
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
			filtered := atomic.AddUint64(&rs.pcapFilterDropCount, 1)
			if !rs.isServer && rs.remotePort != 0 && uint16(tcp.DstPort) == rs.localPort && tcp.SYN && tcp.ACK {
				log.Printf("🔍 pcapReceiver: Filtered SYN-ACK from %s:%d to %s:%d (expected from %s:%d)", ip.SrcIP, tcp.SrcPort, ip.DstIP, tcp.DstPort, rs.remoteIP, rs.remotePort)
			} else if filtered <= 5 || filtered%100 == 0 {
				log.Printf("📉 pcap filter drop count=%d local=%s:%d remote=%s:%d saw=%s:%d -> %s:%d flags=%s payload=%d",
					filtered,
					rs.localIP, rs.localPort,
					rs.remoteIP, rs.remotePort,
					ip.SrcIP, tcp.SrcPort,
					ip.DstIP, tcp.DstPort,
					darwinTCPFlagsString(tcp),
					len(tcp.Payload),
				)
			}
			continue
		}

		packetData := make([]byte, 0, len(ip.Contents)+len(tcp.Contents)+len(tcp.Payload))
		packetData = append(packetData, ip.Contents...)
		packetData = append(packetData, tcp.Contents...)
		packetData = append(packetData, tcp.Payload...)
		packetCount++
		if packetCount <= 5 || packetCount%500 == 0 {
			log.Printf("📦 Darwin pcap accepted count=%d local=%s:%d remote=%s:%d flags=%s payload=%d",
				packetCount, rs.localIP, rs.localPort, rs.remoteIP, rs.remotePort, darwinTCPFlagsString(tcp), len(tcp.Payload))
		}

		select {
		case rs.pcapPacket <- packetData:
		default:
			drops := atomic.AddUint64(&rs.pcapDropCount, 1)
			if drops <= 5 || drops%100 == 0 {
				log.Printf("📉 pcap channel drop count=%d local=%s:%d remote=%s:%d payload=%d flags=%s",
					drops,
					rs.localIP, rs.localPort,
					rs.remoteIP, rs.remotePort,
					len(tcp.Payload),
					darwinTCPFlagsString(tcp),
				)
			}
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
