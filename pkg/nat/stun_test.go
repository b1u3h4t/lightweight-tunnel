package nat

import (
	"encoding/binary"
	"net"
	"testing"
	"time"
)

func TestSTUNMessageBuilding(t *testing.T) {
	client := NewSTUNClient("stun.l.google.com:19302", 3*time.Second)
	
	transactionID := make([]byte, 12)
	for i := range transactionID {
		transactionID[i] = byte(i)
	}
	
	// Test building request without CHANGE-REQUEST
	request := client.buildBindingRequest(transactionID, false, false)
	
	// Verify header
	if len(request) < stunHeaderSize {
		t.Fatalf("Request too short: %d bytes", len(request))
	}
	
	// Check message type
	messageType := binary.BigEndian.Uint16(request[0:2])
	if messageType != stunBindingRequest {
		t.Errorf("Expected message type 0x%04x, got 0x%04x", stunBindingRequest, messageType)
	}
	
	// Check magic cookie
	magicCookie := binary.BigEndian.Uint32(request[4:8])
	if magicCookie != stunMagicCookie {
		t.Errorf("Expected magic cookie 0x%08x, got 0x%08x", stunMagicCookie, magicCookie)
	}
	
	// Check transaction ID
	for i := 0; i < 12; i++ {
		if request[8+i] != transactionID[i] {
			t.Errorf("Transaction ID mismatch at byte %d: expected %d, got %d", i, transactionID[i], request[8+i])
		}
	}
}

func TestSTUNMessageWithChangeRequest(t *testing.T) {
	client := NewSTUNClient("stun.l.google.com:19302", 3*time.Second)
	
	transactionID := make([]byte, 12)
	
	// Test with change IP and port
	request := client.buildBindingRequest(transactionID, true, true)
	
	if len(request) < stunHeaderSize+8 {
		t.Fatalf("Request with CHANGE-REQUEST should be at least %d bytes, got %d", stunHeaderSize+8, len(request))
	}
	
	// Check message length field
	messageLength := binary.BigEndian.Uint16(request[2:4])
	if messageLength != 8 {
		t.Errorf("Expected message length 8, got %d", messageLength)
	}
	
	// Check CHANGE-REQUEST attribute
	attrType := binary.BigEndian.Uint16(request[stunHeaderSize : stunHeaderSize+2])
	if attrType != stunAttrChangeRequest {
		t.Errorf("Expected CHANGE-REQUEST attribute type 0x%04x, got 0x%04x", stunAttrChangeRequest, attrType)
	}
}

func TestSTUNAddressParsing(t *testing.T) {
	client := NewSTUNClient("stun.l.google.com:19302", 3*time.Second)
	
	// Test parsing MAPPED-ADDRESS
	// Format: 0x00 (reserved), 0x01 (IPv4), port (2 bytes), IP (4 bytes)
	addressData := []byte{
		0x00, 0x01, // Reserved, Family (IPv4)
		0x1F, 0x90, // Port 8080
		0xC0, 0xA8, 0x01, 0x64, // IP 192.168.1.100
	}
	
	addr := client.parseAddress(addressData)
	if addr == nil {
		t.Fatal("Failed to parse address")
	}
	
	if addr.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", addr.Port)
	}
	
	expectedIP := net.IPv4(192, 168, 1, 100)
	if !addr.IP.Equal(expectedIP) {
		t.Errorf("Expected IP %s, got %s", expectedIP, addr.IP)
	}
}

func TestSTUNXorAddressParsing(t *testing.T) {
	client := NewSTUNClient("stun.l.google.com:19302", 3*time.Second)
	
	transactionID := make([]byte, 12)
	
	// Create XOR-MAPPED-ADDRESS
	// Real port: 8080 (0x1F90)
	// Real IP: 192.168.1.100 (0xC0A80164)
	realPort := uint16(8080)
	realIP := uint32(0xC0A80164)
	
	// XOR with magic cookie
	xorPort := realPort ^ uint16(stunMagicCookie>>16)
	xorIP := realIP ^ stunMagicCookie
	
	addressData := []byte{
		0x00, 0x01, // Reserved, Family (IPv4)
		0x00, 0x00, // XOR'd port (placeholder)
		0x00, 0x00, 0x00, 0x00, // XOR'd IP (placeholder)
	}
	binary.BigEndian.PutUint16(addressData[2:4], xorPort)
	binary.BigEndian.PutUint32(addressData[4:8], xorIP)
	
	addr := client.parseXorAddress(addressData, transactionID)
	if addr == nil {
		t.Fatal("Failed to parse XOR address")
	}
	
	if addr.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", addr.Port)
	}
	
	expectedIP := net.IPv4(192, 168, 1, 100)
	if !addr.IP.Equal(expectedIP) {
		t.Errorf("Expected IP %s, got %s", expectedIP, addr.IP)
	}
}

func TestSTUNClientCreation(t *testing.T) {
	// Test with default timeout
	client1 := NewSTUNClient("stun.l.google.com:19302", 0)
	if client1.timeout != stunTimeout {
		t.Errorf("Expected default timeout %v, got %v", stunTimeout, client1.timeout)
	}
	
	// Test with custom timeout
	customTimeout := 5 * time.Second
	client2 := NewSTUNClient("stun.l.google.com:19302", customTimeout)
	if client2.timeout != customTimeout {
		t.Errorf("Expected custom timeout %v, got %v", customTimeout, client2.timeout)
	}
}

func TestSTUNResponseParsing(t *testing.T) {
	client := NewSTUNClient("stun.l.google.com:19302", 3*time.Second)
	
	transactionID := make([]byte, 12)
	for i := range transactionID {
		transactionID[i] = byte(i + 1)
	}
	
	// Build a minimal valid STUN response
	response := make([]byte, stunHeaderSize+12)
	
	// Message type: Binding Response
	binary.BigEndian.PutUint16(response[0:2], stunBindingResponse)
	
	// Message length: 12 bytes (one MAPPED-ADDRESS attribute)
	binary.BigEndian.PutUint16(response[2:4], 12)
	
	// Magic cookie
	binary.BigEndian.PutUint32(response[4:8], stunMagicCookie)
	
	// Transaction ID
	copy(response[8:20], transactionID)
	
	// MAPPED-ADDRESS attribute
	binary.BigEndian.PutUint16(response[20:22], stunAttrMappedAddress)
	binary.BigEndian.PutUint16(response[22:24], 8) // Length
	response[24] = 0x00 // Reserved
	response[25] = 0x01 // IPv4
	binary.BigEndian.PutUint16(response[26:28], 9000) // Port
	response[28] = 8   // IP: 8.8.8.8
	response[29] = 8
	response[30] = 8
	response[31] = 8
	
	// Parse the response
	result, err := client.parseBindingResponse(response, transactionID)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	
	if result.MappedAddr == nil {
		t.Fatal("No mapped address in result")
	}
	
	if result.MappedAddr.Port != 9000 {
		t.Errorf("Expected port 9000, got %d", result.MappedAddr.Port)
	}
	
	expectedIP := net.IPv4(8, 8, 8, 8)
	if !result.MappedAddr.IP.Equal(expectedIP) {
		t.Errorf("Expected IP %s, got %s", expectedIP, result.MappedAddr.IP)
	}
}

func TestSTUNInvalidResponse(t *testing.T) {
	client := NewSTUNClient("stun.l.google.com:19302", 3*time.Second)
	
	transactionID := make([]byte, 12)
	
	tests := []struct {
		name     string
		data     []byte
		expected error
	}{
		{
			name:     "Too short",
			data:     make([]byte, 10),
			expected: ErrSTUNInvalidResponse,
		},
		{
			name: "Wrong message type",
			data: func() []byte {
				msg := make([]byte, stunHeaderSize)
				binary.BigEndian.PutUint16(msg[0:2], 0xFFFF)
				binary.BigEndian.PutUint32(msg[4:8], stunMagicCookie)
				copy(msg[8:20], transactionID)
				return msg
			}(),
			expected: nil, // Returns error but not specific type
		},
		{
			name: "Wrong magic cookie",
			data: func() []byte {
				msg := make([]byte, stunHeaderSize)
				binary.BigEndian.PutUint16(msg[0:2], stunBindingResponse)
				binary.BigEndian.PutUint32(msg[4:8], 0xDEADBEEF)
				copy(msg[8:20], transactionID)
				return msg
			}(),
			expected: ErrSTUNInvalidResponse,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.parseBindingResponse(tt.data, transactionID)
			if err == nil {
				t.Error("Expected error, got nil")
			}
		})
	}
}

// TestSTUNIntegration tests real STUN server communication
// This test requires network access and may be skipped in CI environments
func TestSTUNIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	
	client := NewSTUNClient("stun.l.google.com:19302", 5*time.Second)
	
	result, err := client.Query(nil, false, false)
	if err != nil {
		// Network unavailable is acceptable in test environments
		if err == ErrSTUNTimeout {
			t.Skip("STUN server timeout - network may be unavailable")
		}
		t.Logf("STUN query failed (may be expected in test environment): %v", err)
		return
	}
	
	if result.MappedAddr == nil {
		t.Fatal("No mapped address returned from STUN server")
	}
	
	t.Logf("Mapped address: %s", result.MappedAddr)
	
	// Verify the address is valid
	if result.MappedAddr.Port == 0 {
		t.Error("Mapped port is 0")
	}
	
	if result.MappedAddr.IP == nil || result.MappedAddr.IP.IsUnspecified() {
		t.Error("Mapped IP is invalid")
	}
}
