package p2p

import (
	"testing"
	"time"
)

func TestSetKeepaliveIntervalFallsBackToDefaultForNonPositiveValues(t *testing.T) {
	t.Parallel()

	m := NewManager(0)
	m.SetKeepaliveInterval(0)
	if m.keepaliveInterval != KeepaliveInterval {
		t.Fatalf("expected zero interval to fall back to default %v, got %v", KeepaliveInterval, m.keepaliveInterval)
	}

	m.SetKeepaliveInterval(-1 * time.Second)
	if m.keepaliveInterval != KeepaliveInterval {
		t.Fatalf("expected negative interval to fall back to default %v, got %v", KeepaliveInterval, m.keepaliveInterval)
	}
}

func TestSetKeepaliveIntervalAcceptsPositiveValues(t *testing.T) {
	t.Parallel()

	m := NewManager(0)
	want := 25 * time.Second
	m.SetKeepaliveInterval(want)
	if m.keepaliveInterval != want {
		t.Fatalf("expected keepalive interval %v, got %v", want, m.keepaliveInterval)
	}
}
