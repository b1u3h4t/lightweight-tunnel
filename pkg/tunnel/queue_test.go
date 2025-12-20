package tunnel

import (
	"bytes"
	"testing"
	"time"
)

func TestEnqueueWithTimeoutSucceedsWhenQueueDrains(t *testing.T) {
	queue := make(chan []byte, 1)
	queue <- []byte{0x1}
	stopCh := make(chan struct{})

	// Drain the queue shortly after enqueue attempt
	go func() {
		time.Sleep(20 * time.Millisecond)
		<-queue
	}()

	packet := []byte{0x2}
	if !enqueueWithTimeout(queue, packet, stopCh) {
		t.Fatalf("expected packet to enqueue after queue drained")
	}

	select {
	case got := <-queue:
		if !bytes.Equal(got, packet) {
			t.Fatalf("unexpected packet content: %v", got)
		}
	case <-time.After(2 * QueueSendTimeout):
		t.Fatalf("packet was not enqueued in time")
	}
}

func TestEnqueueWithTimeoutTimesOut(t *testing.T) {
	queue := make(chan []byte, 1)
	queue <- []byte{0x1}
	stopCh := make(chan struct{})

	start := time.Now()
	if enqueueWithTimeout(queue, []byte{0x2}, stopCh) {
		t.Fatalf("expected enqueue to fail when queue remains full")
	}
	if time.Since(start) < QueueSendTimeout {
		t.Fatalf("enqueueWithTimeout returned before timeout expired")
	}
}
