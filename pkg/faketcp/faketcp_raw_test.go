package faketcp

import (
	"bytes"
	"net"
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

func TestListenerRawChooseReplyIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		replyIP net.IP
		localIP net.IP
		dstIP   net.IP
		want    net.IP
	}{
		{
			name:    "prefers explicit reply source ip",
			replyIP: net.ParseIP("10.2.0.12").To4(),
			localIP: net.ParseIP("49.232.146.200").To4(),
			dstIP:   net.ParseIP("10.2.0.12").To4(),
			want:    net.ParseIP("10.2.0.12").To4(),
		},
		{
			name:    "falls back to explicit local ip",
			replyIP: nil,
			localIP: net.ParseIP("49.232.146.200").To4(),
			dstIP:   net.ParseIP("10.2.0.12").To4(),
			want:    net.ParseIP("49.232.146.200").To4(),
		},
		{
			name:    "falls back to packet destination when listening on any",
			replyIP: nil,
			localIP: net.IPv4zero,
			dstIP:   net.ParseIP("10.2.0.12").To4(),
			want:    net.ParseIP("10.2.0.12").To4(),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			listener := &ListenerRaw{replyIP: tc.replyIP, localIP: tc.localIP}
			got := listener.chooseReplyIP(tc.dstIP)
			if !got.Equal(tc.want) {
				t.Fatalf("chooseReplyIP() = %v, want %v", got, tc.want)
			}
		})
	}
}
