package store

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestPutReplicated_TriggersWatch verifies that a replicated write propagates to
// active Watch subscribers exactly once.
func TestPutReplicated_TriggersWatch(t *testing.T) {
	dir, _ := os.MkdirTemp("", "watch-replicated-*")
	defer os.RemoveAll(dir)

	s, err := NewBadgerStoreWithID(dir, "node1", 100)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ctx := context.Background()
	key := "replicated-key"

	// Subscribe to watch before the write arrives
	ch, unsubscribe, err := s.Watch(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()

	// Simulate an incoming replicated value from node2
	incoming := &Value{
		Data:      []byte("from-node2"),
		Timestamp: time.Now().UnixNano(),
		Version:   map[string]int{"node2": 1},
	}

	if err := s.PutReplicated(ctx, key, incoming); err != nil {
		t.Fatalf("PutReplicated failed: %v", err)
	}

	// The watcher should receive the event
	select {
	case event := <-ch:
		if string(event.Data) != "from-node2" {
			t.Errorf("expected 'from-node2', got '%s'", event.Data)
		}
		t.Logf("✅ Watch received replicated event: data=%s version=%v", event.Data, event.Version)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: Watch did not receive replicated event")
	}
}

// TestPutReplicated_StaleDropped verifies that a replicated write that is
// older than the local value is silently dropped — no notification is sent.
func TestPutReplicated_StaleDropped(t *testing.T) {
	dir, _ := os.MkdirTemp("", "stale-replicated-*")
	defer os.RemoveAll(dir)

	s, err := NewBadgerStoreWithID(dir, "node1", 100)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ctx := context.Background()
	key := "stale-key"

	// Write a fresh value locally first (node1 counter = 1)
	fresh := &Value{
		Data:      []byte("local-fresh"),
		Timestamp: time.Now().UnixNano(),
		Version:   make(map[string]int),
	}
	if err := s.Put(ctx, key, fresh); err != nil {
		t.Fatal(err)
	}

	// Now subscribe — we don't want any more events for this key
	ch, unsubscribe, err := s.Watch(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()

	// Simulate a stale incoming value (node2 clock is behind node1's clock)
	stale := &Value{
		Data:      []byte("stale-from-node2"),
		Timestamp: time.Now().Add(-10 * time.Second).UnixNano(), // older timestamp
		Version:   map[string]int{"node2": 0},                  // empty — no causal history
	}

	if err := s.PutReplicated(ctx, key, stale); err != nil {
		t.Fatalf("PutReplicated returned unexpected error: %v", err)
	}

	// The watcher should NOT receive anything — stale update is dropped
	select {
	case event := <-ch:
		t.Errorf("❌ expected no watch event for stale update, but got: data=%s version=%v", event.Data, event.Version)
	case <-time.After(500 * time.Millisecond):
		t.Log("✅ Stale replicated update correctly dropped — no notification sent")
	}

	// Verify the stored value is still the fresh one
	got, _ := s.Get(ctx, key)
	if string(got.Data) != "local-fresh" {
		t.Errorf("expected 'local-fresh' to survive, got '%s'", got.Data)
	}
}

// TestPutReplicated_NoDuplicateNotifications verifies that calling PutReplicated
// twice with the same version only triggers one watch event.
func TestPutReplicated_NoDuplicateNotifications(t *testing.T) {
	dir, _ := os.MkdirTemp("", "dedup-replicated-*")
	defer os.RemoveAll(dir)

	s, err := NewBadgerStoreWithID(dir, "node1", 100)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ctx := context.Background()
	key := "dedup-key"

	ch, unsubscribe, err := s.Watch(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()

	incoming := &Value{
		Data:      []byte("v1"),
		Timestamp: time.Now().UnixNano(),
		Version:   map[string]int{"node2": 1},
	}

	// Apply twice (simulating duplicate gossip delivery)
	s.PutReplicated(ctx, key, incoming)
	s.PutReplicated(ctx, key, incoming)

	count := 0
	deadline := time.After(500 * time.Millisecond)
loop:
	for {
		select {
		case <-ch:
			count++
		case <-deadline:
			break loop
		}
	}

	if count != 1 {
		t.Errorf("❌ expected exactly 1 notification, got %d", count)
	} else {
		t.Logf("✅ Exactly 1 notification for 2 identical PutReplicated calls")
	}
}
