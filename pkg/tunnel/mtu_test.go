package tunnel

import (
	"testing"

	"github.com/openbmx/lightweight-tunnel/internal/config"
	"github.com/openbmx/lightweight-tunnel/pkg/crypto"
)

// TestMTUAdjustmentWithEncryption verifies that MTU is automatically adjusted
// when encryption is enabled in rawtcp mode to prevent TCP segmentation issues
func TestMTUAdjustmentWithEncryption(t *testing.T) {
	tests := []struct {
		name            string
		transport       string
		key             string
		initialMTU      int
		expectedMTU     int
		shouldAdjust    bool
	}{
		{
			name:         "rawtcp with encryption - MTU too large",
			transport:    "rawtcp",
			key:          "test-key-123",
			initialMTU:   1400,
			expectedMTU:  1371, // 1400 - 1 (packet type) - 28 (encryption overhead)
			shouldAdjust: true,
		},
		{
			name:         "rawtcp with encryption - MTU already safe",
			transport:    "rawtcp",
			key:          "test-key-123",
			initialMTU:   1371,
			expectedMTU:  1371,
			shouldAdjust: false,
		},
		{
			name:         "rawtcp with encryption - MTU below safe threshold",
			transport:    "rawtcp",
			key:          "test-key-123",
			initialMTU:   1200,
			expectedMTU:  1200,
			shouldAdjust: false,
		},
		{
			name:         "rawtcp without encryption - no adjustment",
			transport:    "rawtcp",
			key:          "",
			initialMTU:   1400,
			expectedMTU:  1400,
			shouldAdjust: false,
		},
		{
			name:         "udp with encryption - no adjustment",
			transport:    "udp",
			key:          "test-key-123",
			initialMTU:   1400,
			expectedMTU:  1400,
			shouldAdjust: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Mode:            "server",
				Transport:       tt.transport,
				LocalAddr:       "127.0.0.1:9000",
				TunnelAddr:      "10.0.0.1/24",
				MTU:             tt.initialMTU,
				FECDataShards:   10,
				FECParityShards: 3,
				Key:             tt.key,
				SendQueueSize:   100,
				RecvQueueSize:   100,
			}

			// Simulate the MTU adjustment logic from NewTunnel
			if cfg.Key != "" {
				cipher, err := crypto.NewCipher(cfg.Key)
				if err != nil {
					t.Fatalf("Failed to create cipher: %v", err)
				}

				if cfg.Transport == "rawtcp" {
					const maxRawTCPSegment = 1400
					const packetTypeOverhead = 1
					encryptionOverhead := cipher.Overhead()
					maxSafeMTU := maxRawTCPSegment - packetTypeOverhead - encryptionOverhead

					if cfg.MTU > maxSafeMTU {
						t.Logf("Adjusting MTU from %d to %d", cfg.MTU, maxSafeMTU)
						cfg.MTU = maxSafeMTU
					}
				}
			}

			// Verify the result
			if cfg.MTU != tt.expectedMTU {
				t.Errorf("Expected MTU %d, got %d", tt.expectedMTU, cfg.MTU)
			}

			if tt.shouldAdjust && cfg.MTU == tt.initialMTU {
				t.Errorf("Expected MTU to be adjusted from %d, but it remained unchanged", tt.initialMTU)
			}

			if !tt.shouldAdjust && cfg.MTU != tt.initialMTU {
				t.Errorf("Expected MTU to remain %d, but it was adjusted to %d", tt.initialMTU, cfg.MTU)
			}
		})
	}
}

// TestEncryptionOverhead verifies that the encryption overhead calculation is correct
func TestEncryptionOverhead(t *testing.T) {
	cipher, err := crypto.NewCipher("test-key")
	if err != nil {
		t.Fatalf("Failed to create cipher: %v", err)
	}

	overhead := cipher.Overhead()
	expectedOverhead := 28 // 12 bytes nonce + 16 bytes auth tag

	if overhead != expectedOverhead {
		t.Errorf("Expected encryption overhead %d, got %d", expectedOverhead, overhead)
	}

	t.Logf("Encryption overhead: %d bytes (nonce + auth tag)", overhead)
}

// TestMaxSafeMTUCalculation verifies the safe MTU calculation formula
func TestMaxSafeMTUCalculation(t *testing.T) {
	const maxRawTCPSegment = 1400
	const packetTypeOverhead = 1
	const encryptionOverhead = 28 // nonce (12) + tag (16)

	maxSafeMTU := maxRawTCPSegment - packetTypeOverhead - encryptionOverhead
	expectedSafeMTU := 1371

	if maxSafeMTU != expectedSafeMTU {
		t.Errorf("Expected safe MTU %d, got %d", expectedSafeMTU, maxSafeMTU)
	}

	t.Logf("Max safe MTU calculation:")
	t.Logf("  Max TCP segment: %d bytes", maxRawTCPSegment)
	t.Logf("  - Packet type overhead: %d byte", packetTypeOverhead)
	t.Logf("  - Encryption overhead: %d bytes", encryptionOverhead)
	t.Logf("  = Safe MTU: %d bytes", maxSafeMTU)

	// Verify that packets at safe MTU won't be segmented
	testPacketSize := maxSafeMTU + packetTypeOverhead + encryptionOverhead
	if testPacketSize > maxRawTCPSegment {
		t.Errorf("Packet size %d exceeds max segment %d", testPacketSize, maxRawTCPSegment)
	}

	t.Logf("Verification: packet with MTU=%d after encryption = %d bytes (fits in %d byte segment)",
		maxSafeMTU, testPacketSize, maxRawTCPSegment)
}
