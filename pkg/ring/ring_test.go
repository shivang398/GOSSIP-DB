package ring

import (
	"testing"
)

func TestConsistentHash_GetReturnsNode(t *testing.T) {
	ch := New(50)
	ch.Add("node1", "node2", "node3")

	key := "user:alice"
	node := ch.Get(key)
	if node == "" {
		t.Fatal("expected a node, got empty string")
	}
	t.Logf("Key %q → %s", key, node)
}

func TestConsistentHash_Deterministic(t *testing.T) {
	ch := New(50)
	ch.Add("node1", "node2", "node3")

	key := "my-deterministic-key"
	first := ch.Get(key)
	for i := 0; i < 100; i++ {
		got := ch.Get(key)
		if got != first {
			t.Fatalf("non-deterministic result: got %q but expected %q", got, first)
		}
	}
}

func TestConsistentHash_RemoveRedistributes(t *testing.T) {
	ch := New(50)
	ch.Add("node1", "node2", "node3")

	before := ch.Get("test-key")

	ch.Remove("node1")
	ch.Remove("node2")

	after := ch.Get("test-key")
	if after == "" {
		t.Fatal("expected a node after removal, got empty string")
	}
	t.Logf("Before: %s → After removal of node1+node2: %s", before, after)
}

func TestConsistentHash_EmptyRing(t *testing.T) {
	ch := New(50)
	node := ch.Get("some-key")
	if node != "" {
		t.Fatalf("expected empty string for empty ring, got %q", node)
	}
}

func TestConsistentHash_Distribution(t *testing.T) {
	ch := New(150)
	nodes := []string{"node1", "node2", "node3"}
	ch.Add(nodes...)

	counts := make(map[string]int)
	total := 10000
	for i := 0; i < total; i++ {
		key := "key-" + string(rune('A'+i%26)) + "-" + string(rune('0'+i%10))
		n := ch.Get(key)
		counts[n]++
	}

	t.Logf("Key distribution across %d keys:", total)
	for node, count := range counts {
		pct := float64(count) / float64(total) * 100
		t.Logf("  %s → %d keys (%.1f%%)", node, count, pct)
	}
}
