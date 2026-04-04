package tunnel

import "encoding/binary"

const (
	tlsVersionMajor       = 0x03
	tlsRecordHandshake    = 0x16
	tlsRecordChangeCipher = 0x14
	tlsRecordApplication  = 0x17
	tlsRecordAlert        = 0x15
	asciiPrintableMin     = 0x20
	asciiPrintableMax     = 0x7E
)

var encryptedServicePorts = map[uint16]struct{}{
	443:   {},
	8443:  {},
	853:   {},
	2053:  {},
	3389:  {},
	8388:  {},
	8389:  {},
	10086: {},
	10443: {},
}

func portLikelyEncrypted(port uint16) bool {
	_, ok := encryptedServicePorts[port]
	return ok
}

// isLikelyEncryptedTraffic inspects the inner IP packet to detect TLS- or AEAD-
// protected traffic (e.g., Shadowsocks/vmess/vless). When detected, we can skip
// outer AES to avoid double encryption.
func isLikelyEncryptedTraffic(ipPacket []byte) bool {
	if len(ipPacket) < IPv4MinHeaderLen || ipPacket[0]>>4 != IPv4Version {
		return false
	}

	ihl := int(ipPacket[0]&0x0F) * 4
	if len(ipPacket) < ihl+4 {
		return false
	}

	proto := ipPacket[9]
	payload := ipPacket[ihl:]

	switch proto {
	case 6: // TCP
		if len(payload) < 20 {
			return false
		}
		srcPort := binary.BigEndian.Uint16(payload[0:2])
		dstPort := binary.BigEndian.Uint16(payload[2:4])
		dataOffset := int(payload[12]>>4) * 4
		if dataOffset > len(payload) {
			return false
		}
		appPayload := payload[dataOffset:]
		return portLikelyEncrypted(srcPort) || portLikelyEncrypted(dstPort) ||
			looksLikeTLS(appPayload) || looksLikeAEADProxy(appPayload)
	case 17: // UDP (QUIC or Shadowsocks)
		if len(payload) < 8 {
			return false
		}
		srcPort := binary.BigEndian.Uint16(payload[0:2])
		dstPort := binary.BigEndian.Uint16(payload[2:4])
		appPayload := payload[8:]
		return portLikelyEncrypted(srcPort) || portLikelyEncrypted(dstPort) ||
			looksLikeQUIC(appPayload) || looksLikeAEADProxy(appPayload)
	default:
		return false
	}
}

func looksLikeTLS(payload []byte) bool {
	if len(payload) < 5 {
		return false
	}
	recordType := payload[0]
	versionMajor := payload[1]
	versionMinor := payload[2]
	if versionMajor != tlsVersionMajor {
		return false
	}
	if recordType == tlsRecordHandshake || recordType == tlsRecordChangeCipher || recordType == tlsRecordApplication || recordType == tlsRecordAlert {
		switch versionMinor {
		case 0x00, 0x01, 0x02, 0x03, 0x04:
			return true
		default:
			return false
		}
	}
	return false
}

func looksLikeQUIC(payload []byte) bool {
	if len(payload) < 5 {
		return false
	}
	first := payload[0]

	// QUIC long header: bit 7 set AND bit 6 set (fixed bit = 1 in QUIC v1).
	// Then bytes 1-4 contain the version. Check for known QUIC versions.
	if first&0xC0 == 0xC0 {
		// Verify QUIC version field (bytes 1-4)
		version := uint32(payload[1])<<24 | uint32(payload[2])<<16 | uint32(payload[3])<<8 | uint32(payload[4])
		switch version {
		case 0x00000001, // QUIC v1 (RFC 9000)
			0x6B3343CF,                                                         // QUIC v2 (RFC 9369)
			0xFF000000 | 29, 0xFF000000 | 30, 0xFF000000 | 31, 0xFF000000 | 32, // Draft versions 29-32
			0xFF000000 | 33, 0xFF000000 | 34: // Draft versions 33-34
			return true
		}
		// Version negotiation (version = 0) is also QUIC
		if version == 0 {
			return true
		}
		return false
	}

	// QUIC short header: bit 7 clear, bit 6 set (fixed bit = 1).
	// Short headers are used after the handshake completes, so the connection
	// is definitely encrypted. Require minimum payload length for confidence.
	if first&0xC0 == 0x40 && len(payload) > 20 {
		return true
	}

	return false
}

func looksLikeAEADProxy(payload []byte) bool {
	if len(payload) < 8 {
		return false
	}

	// Exclude well-known binary protocols that are NOT encrypted:
	// DNS responses start with transaction ID + flags (bit 15 = QR=1 for response)
	if len(payload) >= 12 && payload[2]&0x80 != 0 {
		// Looks like a DNS response — not encrypted
		return false
	}
	// DHCP: starts with op=1 (request) or op=2 (reply), htype=1 (Ethernet), hlen=6
	if len(payload) >= 4 && (payload[0] == 0x01 || payload[0] == 0x02) && payload[1] == 0x01 && payload[2] == 0x06 {
		return false
	}

	// Shadowsocks/VMess/VLESS commonly start with address type (1/3/4) or random nonce bytes.
	atyp := payload[0]
	switch atyp {
	case 0x01:
		return len(payload) >= 1+4+2 // IPv4 addr + port
	case 0x04:
		return len(payload) >= 1+16+2 // IPv6 addr + port
	case 0x03:
		domainLen := int(payload[1])
		if domainLen == 0 || domainLen > 253 {
			// Invalid domain length for SOCKS-style address — probably not AEAD proxy
			return false
		}
		return len(payload) >= 2+domainLen+2
	default:
		// High-entropy heuristic: check if most of the sampled bytes are non-printable.
		// Use a larger sample (16 bytes) and require a higher threshold (75%) to reduce
		// false positives on binary protocols like DNS, NTP, SNMP, etc.
		nonPrintable := 0
		checkLen := len(payload)
		if checkLen > 16 {
			checkLen = 16
		}
		// Require 75% non-printable (was 50%+1) to classify as encrypted
		threshold := checkLen*3/4 + 1
		for i := 0; i < checkLen; i++ {
			if payload[i] < asciiPrintableMin || payload[i] > asciiPrintableMax {
				nonPrintable++
			}
		}
		return nonPrintable >= threshold
	}
}

func isPlainPassThroughPacket(packet []byte) bool {
	if len(packet) < 1 || packet[0] != PacketTypeData {
		return false
	}
	return isLikelyEncryptedTraffic(packet[1:])
}
