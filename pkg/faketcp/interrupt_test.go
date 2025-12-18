package faketcp

import (
	"testing"
	"time"
)

// TestListenerInterrupt verifies that a listener can be interrupted by closing
func TestListenerInterrupt(t *testing.T) {
	// Start listener
	listener, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	
	listenerAddr := listener.Addr().String()
	t.Logf("Listener started on %s", listenerAddr)
	
	// Start accepting in a goroutine
	acceptDone := make(chan error, 1)
	go func() {
		_, err := listener.Accept()
		acceptDone <- err
	}()
	
	// Give Accept() time to start blocking
	time.Sleep(500 * time.Millisecond)
	
	// Close the listener
	t.Log("Closing listener...")
	err = listener.Close()
	if err != nil {
		t.Fatalf("Failed to close listener: %v", err)
	}
	
	// Accept should return with an error within a reasonable time
	select {
	case err := <-acceptDone:
		if err == nil {
			t.Error("Expected error from Accept after Close, got nil")
		}
		t.Logf("Accept returned with error (expected): %v", err)
	case <-time.After(3 * time.Second):
		t.Error("Accept did not return within timeout after Close - this indicates a hang")
	}
}

// TestConnReadInterrupt verifies that a connected socket read can be interrupted
func TestConnReadInterrupt(t *testing.T) {
	// Create a connected socket (won't actually connect but enough for testing)
	conn, err := Dial("127.0.0.1:19999", 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	
	// Start reading in a goroutine
	readDone := make(chan error, 1)
	go func() {
		_, err := conn.ReadPacket()
		readDone <- err
	}()
	
	// Give ReadPacket time to start blocking
	time.Sleep(500 * time.Millisecond)
	
	// Close the connection
	t.Log("Closing connection...")
	err = conn.Close()
	if err != nil {
		t.Fatalf("Failed to close connection: %v", err)
	}
	
	// ReadPacket should return within a reasonable time
	select {
	case err := <-readDone:
		if err == nil {
			t.Error("Expected error from ReadPacket after Close, got nil")
		}
		t.Logf("ReadPacket returned with error (expected): %v", err)
	case <-time.After(3 * time.Second):
		t.Error("ReadPacket did not return within timeout after Close - this indicates a hang")
	}
}
