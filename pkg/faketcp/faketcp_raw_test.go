package faketcp

import (
	"bytes"
	"testing"
)

func TestListenerRawAcceptPreservesQueuedPayload(t *testing.T) {
	listener := &ListenerRaw{
		acceptQueue: make(chan *ConnRaw, 1),
		stopCh:      make(chan struct{}),
	}

	queued := []byte("first-auth-packet")
	conn := &ConnRaw{
		recvQueue: make(chan []byte, 1),
	}
	conn.recvQueue <- queued
	listener.acceptQueue <- conn

	accepted, err := listener.Accept()
	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}

	select {
	case got := <-accepted.recvQueue:
		if !bytes.Equal(got, queued) {
			t.Fatalf("queued payload mismatch: got %q want %q", got, queued)
		}
	default:
		t.Fatal("Accept drained the queued payload")
	}
}
