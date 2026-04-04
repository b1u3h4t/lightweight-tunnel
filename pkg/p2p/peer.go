package p2p

import (
	"net"
	"sync"
	"time"

	"github.com/openbmx/lightweight-tunnel/pkg/nat"
)

// PeerInfo contains information about a peer
type PeerInfo struct {
	TunnelIP          net.IP        // Tunnel IP address (e.g., 10.0.0.2)
	PublicAddr        string        // Public address for P2P (IP:Port)
	LocalAddr         string        // Local address behind NAT
	NATType           nat.NATType   // NAT type of this peer
	LastSeen          time.Time     // Last time we received data from this peer
	Latency           time.Duration // Measured latency to this peer
	PacketLoss        float64       // Packet loss rate (0.0 - 1.0)
	Connected         bool          // Whether P2P connection is established
	ThroughServer     bool          // Whether currently routing through server
	IsLocalConnection bool          // Whether connection is via local network (not NAT)
	RelayPeers        []net.IP      // List of peers that can relay to this peer
	// Quality monitoring fields — sequence-number based loss tracking
	sendSeq          uint64    // Monotonic sequence number for sent packets
	recvCount        uint64    // Number of packets received in this measurement window
	recvMaxSeq       uint64    // Highest sequence number observed from remote peer
	recvMinSeq       uint64    // Lowest sequence number observed in this window (0 = unset)
	lastQualityCheck time.Time // Last time quality was checked
	mu               sync.RWMutex
}

// NewPeerInfo creates a new peer information structure
func NewPeerInfo(tunnelIP net.IP) *PeerInfo {
	return &PeerInfo{
		TunnelIP:   tunnelIP,
		NATType:    nat.NATUnknown,
		LastSeen:   time.Now(),
		RelayPeers: make([]net.IP, 0),
	}
}

// UpdateLatency updates the latency measurement
func (p *PeerInfo) UpdateLatency(latency time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Latency = latency
	p.LastSeen = time.Now()
}

// UpdatePacketLoss updates the packet loss rate
func (p *PeerInfo) UpdatePacketLoss(loss float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.PacketLoss = loss
}

// RecordPacketSent increments the send sequence counter and returns the
// sequence number that should be embedded in the packet header.
func (p *PeerInfo) RecordPacketSent() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sendSeq++
	return p.sendSeq
}

// RecordPacketReceived records an incoming packet with the given remote
// sequence number. If seq is 0 (legacy peer without sequence numbers),
// we fall back to simple counting.
func (p *PeerInfo) RecordPacketReceived() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.recvCount++
	p.LastSeen = time.Now()
}

// RecordPacketReceivedWithSeq records an incoming packet with a known
// remote sequence number for accurate loss calculation.
func (p *PeerInfo) RecordPacketReceivedWithSeq(seq uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.recvCount++
	if seq > p.recvMaxSeq {
		p.recvMaxSeq = seq
	}
	if p.recvMinSeq == 0 || seq < p.recvMinSeq {
		p.recvMinSeq = seq
	}
	p.LastSeen = time.Now()
}

// CalculatePacketLoss calculates packet loss based on sequence number gaps.
// If sequence numbers are available (recvMaxSeq > 0), loss is calculated as:
//
//	expectedCount = recvMaxSeq - recvMinSeq + 1
//	loss = 1 - (recvCount / expectedCount)
//
// This measures true unidirectional receive loss regardless of traffic asymmetry.
// If no sequence numbers are available, returns the last known loss value.
func (p *PeerInfo) CalculatePacketLoss() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.recvMaxSeq > 0 && p.recvMinSeq > 0 && p.recvMaxSeq >= p.recvMinSeq {
		// Sequence-number based: count how many packets in the range we actually got
		expectedCount := p.recvMaxSeq - p.recvMinSeq + 1
		if expectedCount == 0 {
			return 0.0
		}
		actual := p.recvCount
		if actual > expectedCount {
			actual = expectedCount // cap at 100% (duplicates don't mean negative loss)
		}
		loss := 1.0 - float64(actual)/float64(expectedCount)
		if loss < 0 {
			loss = 0
		}
		p.PacketLoss = loss
		return loss
	}

	// No sequence data available — cannot calculate meaningful loss
	return p.PacketLoss
}

// ResetPacketCounters resets packet statistics (called periodically)
func (p *PeerInfo) ResetPacketCounters() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.recvCount = 0
	p.recvMaxSeq = 0
	p.recvMinSeq = 0
	p.lastQualityCheck = time.Now()
}

// SetNATType sets the NAT type for this peer
func (p *PeerInfo) SetNATType(natType nat.NATType) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.NATType = natType
}

// GetNATType returns the NAT type for this peer
func (p *PeerInfo) GetNATType() nat.NATType {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.NATType
}

// SetConnected marks the peer as connected via P2P
func (p *PeerInfo) SetConnected(connected bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Connected = connected
	if connected {
		p.ThroughServer = false
	}
}

// SetLocalConnection marks whether the connection is via local network
func (p *PeerInfo) SetLocalConnection(isLocal bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.IsLocalConnection = isLocal
}

// SetThroughServer marks traffic as going through server
func (p *PeerInfo) SetThroughServer(through bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ThroughServer = through
}

// GetQualityScore returns a quality score for this peer (0-150, higher is better)
// Score above 100 indicates excellent quality (typically local connections)
// Score 80-100 is good quality P2P
// Score 50-80 is acceptable
// Score below 50 indicates poor quality that may need attention
func (p *PeerInfo) GetQualityScore() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Quality scoring constants
	const (
		latencyPenaltyDivisor    = 10   // Divide latency by this to get penalty groups
		latencyPenaltyMultiplier = 5    // Multiply penalty groups by this value
		packetLossPenaltyScale   = 1000 // Scale packet loss to penalty points
		p2pQualityBonus          = 20   // Bonus points for P2P connection
		localConnectionBonus     = 30   // Bonus points for local network connection (highest priority)
		serverRoutePenalty       = 30   // Penalty points for server routing
	)

	// Base score
	score := 100

	// Deduct for latency (every 10ms reduces score by 5)
	latencyPenalty := int(p.Latency.Milliseconds() / latencyPenaltyDivisor * latencyPenaltyMultiplier)
	score -= latencyPenalty

	// Deduct for packet loss (1% loss = 10 points)
	lossPenalty := int(p.PacketLoss * packetLossPenaltyScale)
	score -= lossPenalty

	// Bonus for direct P2P connection
	if p.Connected {
		score += p2pQualityBonus
	}

	// Extra bonus for local network connection (highest priority)
	// This ensures local connections are preferred over public NAT traversal
	if p.IsLocalConnection {
		score += localConnectionBonus
	}

	// Penalty for going through server
	if p.ThroughServer {
		score -= serverRoutePenalty
	}

	// Ensure score is in valid range
	// Allow scores above 100 for local connection bonus to ensure proper prioritization
	if score < 0 {
		score = 0
	}
	if score > 150 {
		score = 150
	}

	return score
}

// IsStale checks if peer information is stale
func (p *PeerInfo) IsStale(timeout time.Duration) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return time.Since(p.LastSeen) > timeout
}

// Clone creates a copy of peer info for safe reading
func (p *PeerInfo) Clone() *PeerInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	clone := &PeerInfo{
		TunnelIP:          p.TunnelIP,
		PublicAddr:        p.PublicAddr,
		LocalAddr:         p.LocalAddr,
		NATType:           p.NATType,
		LastSeen:          p.LastSeen,
		Latency:           p.Latency,
		PacketLoss:        p.PacketLoss,
		Connected:         p.Connected,
		ThroughServer:     p.ThroughServer,
		IsLocalConnection: p.IsLocalConnection,
		RelayPeers:        make([]net.IP, len(p.RelayPeers)),
		sendSeq:           p.sendSeq,
		recvCount:         p.recvCount,
		recvMaxSeq:        p.recvMaxSeq,
		recvMinSeq:        p.recvMinSeq,
		lastQualityCheck:  p.lastQualityCheck,
	}
	copy(clone.RelayPeers, p.RelayPeers)

	return clone
}
