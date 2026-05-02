package vclock

import (
	"testing"
	"time"
)

// localValue is a test-only type that satisfies ResolvableValue
type localValue struct {
	data      string
	timestamp int64
	version   map[string]int
}

func (v *localValue) GetTimestamp() int64          { return v.timestamp }
func (v *localValue) GetVersion() map[string]int   { return v.version }

func makeTestVal(data string, ts int64, vc VClock) *localValue {
	return &localValue{data: data, timestamp: ts, version: map[string]int(vc)}
}

func TestReconcileVC_IncomingWins(t *testing.T) {
	local := makeTestVal("old", time.Now().Add(-5*time.Second).UnixNano(),
		VClock{"node1": 1})
	incoming := makeTestVal("new", time.Now().UnixNano(),
		VClock{"node1": 1, "node2": 1}) // incoming is strictly After local

	winner, conflict := ReconcileVC(local, incoming, LWWGeneric[*localValue]{})
	if conflict {
		t.Error("expected no conflict")
	}
	if winner.data != "new" {
		t.Errorf("expected 'new', got '%s'", winner.data)
	}
}

func TestReconcileVC_LocalWins(t *testing.T) {
	local := makeTestVal("current", time.Now().UnixNano(),
		VClock{"node1": 2, "node2": 1})
	incoming := makeTestVal("stale", time.Now().Add(-10*time.Second).UnixNano(),
		VClock{"node1": 1}) // incoming is strictly Before local

	winner, conflict := ReconcileVC(local, incoming, LWWGeneric[*localValue]{})
	if conflict {
		t.Error("expected no conflict")
	}
	if winner.data != "current" {
		t.Errorf("expected 'current', got '%s'", winner.data)
	}
}

func TestReconcileVC_ConcurrentLWW(t *testing.T) {
	earlier := time.Now().Add(-2 * time.Second).UnixNano()
	later := time.Now().UnixNano()

	local := makeTestVal("local-write", earlier, VClock{"node1": 1})
	incoming := makeTestVal("remote-write", later, VClock{"node2": 1}) // Concurrent!

	winner, conflict := ReconcileVC(local, incoming, LWWGeneric[*localValue]{})
	if !conflict {
		t.Error("expected conflict to be detected")
	}
	// LWW: incoming has later timestamp → should win
	if winner.data != "remote-write" {
		t.Errorf("expected LWW winner 'remote-write', got '%s'", winner.data)
	}
}
